package limiter

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// RateLimitTransport 实现 http.RoundTripper，在 HTTP 出站前插入限流逻辑。
// 超限时不发起真实请求，直接返回 429 响应。
type RateLimitTransport struct {
	serviceName string            // 当前实例所属服务名，如 event、click
	base        http.RoundTripper // 限流通过后的真实 HTTP 传输层
	manager     *RuleManager      // 规则管理与 Redis 计数
}

// NewTransport 创建带限流能力的 RoundTripper（推荐入口）。
//
// serviceName 标识本服务，用于匹配 MySQL 中该服务的专属规则；
// db 用于加载规则，rdb 用于分布式计数与 Pub/Sub 同步。
// 每个 Transport 会新建一个 RuleManager；多服务共享缓存请用 NewTransportWithManager。
// 进程退出前请调用 (*RateLimitTransport).Close() 释放 Pub/Sub 等资源。
func NewTransport(serviceName string, db *gorm.DB, rdb *redis.Client, opts ...ManagerOption) (*RateLimitTransport, error) {
	rm, err := NewRuleManager(db, rdb, opts...)
	if err != nil {
		return nil, err
	}
	o := Options{}
	for _, fn := range opts {
		fn(&o)
	}
	o.applyDefaults()
	return &RateLimitTransport{
		serviceName: serviceName,
		base:        o.Base,
		manager:     rm,
	}, nil
}

// NewTransportWithManager 使用已有的 RuleManager 创建 Transport。
// 适用于同一进程内 event、data 等多个服务共用一套规则缓存和 Pub/Sub 订阅的场景。
// 关闭资源由共享的 RuleManager.Close() 负责，Transport 本身不持有独立订阅。
func NewTransportWithManager(serviceName string, rm *RuleManager, opts ...ManagerOption) *RateLimitTransport {
	o := Options{}
	for _, fn := range opts {
		fn(&o)
	}
	o.applyDefaults()
	return &RateLimitTransport{
		serviceName: serviceName,
		base:        o.Base,
		manager:     rm,
	}
}

// Manager 返回内部的规则管理器，用于发布规则更新、关闭订阅等生命周期操作。
func (t *RateLimitTransport) Manager() *RuleManager {
	return t.manager
}

// Close 停止内部 RuleManager（Pub/Sub 等）；可安全重复调用。
// 使用 NewTransportWithManager 且多 Transport 共享同一 Manager 时，只需对 Manager 调用一次 Close。
func (t *RateLimitTransport) Close() error {
	return t.manager.Close()
}

// RoundTrip 实现 http.RoundTripper。
var _ http.RoundTripper = (*RateLimitTransport)(nil)

// RoundTrip 实现 http.RoundTripper：
//
//	请求路径：40110 接口级封禁检查 → 滑动窗口/固定窗口限流 → 出站；
//	响应路径：解析 40110 + X-RateLimit-RetryIn，写入接口级封禁供后续请求拦截。
func (t *RateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	normalized := NormalizePath(req.URL.Path)

	// 【请求路径】40110 开发者频控封禁（接口级，不区分 service）
	if !t.manager.opts.DisablePlatformPenalty {
		penaltyKey := PenaltyKey(normalized)
		if blocked, retryAfter, err := CheckPenalty(req.Context(), t.manager.rdb, penaltyKey); err != nil {
			return nil, fmt.Errorf("penalty check: %w", err)
		} else if blocked {
			return rateLimitedResponse(req, retryAfter, RejectReasonPenalty), nil
		}
	}

	allowed, err := t.allowLocal(req.Context(), normalized)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return rateLimitedResponse(req, 0, RejectReasonLocal), nil
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}

	// 【响应路径】40110 开发者频控：写入接口级封禁，后续 CheckPenalty 拦截
	if !t.manager.opts.DisablePlatformPenalty {
		code, applied, retryAfter, perr := ApplyPlatformPenaltyFromResponse(
			req.Context(),
			t.manager.rdb,
			normalized,
			resp,
			t.manager.opts.PlatformPenaltyTTL,
		)
		if perr != nil {
			log.Printf("[oe-limiter] apply 40110 penalty failed: %v", perr)
		}
		if applied && t.manager.opts.OnPlatformRateLimit != nil {
			t.manager.opts.OnPlatformRateLimit(t.serviceName, normalized, code, retryAfter)
		}
	}
	return resp, nil
}

func (t *RateLimitTransport) allowLocal(ctx context.Context, normalized string) (bool, error) {
	serviceRule := t.manager.GetRule(t.serviceName, normalized)
	globalRule, hasGlobal := t.manager.matchRule("ALL", normalized)
	globalLimit := t.manager.opts.DefaultGlobalQPS
	if hasGlobal {
		globalLimit = globalRule.QPSLimit
	}
	return t.manager.limiter.AllowDoubleCheck(ctx, AllowRequest{
		ServiceKey:   RedisKey(t.serviceName, normalized),
		GlobalKey:    RedisKey("ALL", normalized),
		ServiceLimit: serviceRule.QPSLimit,
		GlobalLimit:  globalLimit,
		WindowSec:    t.manager.WindowSec(),
		WindowMS:     t.manager.WindowMS(),
		Mode:         t.manager.LimiterMode(),
	})
}

// Allow 在不经过 HTTP 的情况下单独做一次限流判断，供 gRPC、消息消费等场景复用。
//
// apiPath 为原始路径，内部会自动 NormalizePath；返回 true 表示本次允许通过。
func (t *RateLimitTransport) Allow(ctx context.Context, apiPath string) (bool, error) {
	normalized := NormalizePath(apiPath)
	serviceRule := t.manager.GetRule(t.serviceName, normalized)
	globalRule, hasGlobal := t.manager.matchRule("ALL", normalized)
	globalLimit := t.manager.opts.DefaultGlobalQPS
	if hasGlobal {
		globalLimit = globalRule.QPSLimit
	}
	return t.manager.limiter.AllowDoubleCheck(ctx, AllowRequest{
		ServiceKey:   RedisKey(t.serviceName, normalized),
		GlobalKey:    RedisKey("ALL", normalized),
		ServiceLimit: serviceRule.QPSLimit,
		GlobalLimit:  globalLimit,
		WindowSec:    t.manager.WindowSec(),
		WindowMS:     t.manager.WindowMS(),
		Mode:         t.manager.LimiterMode(),
	})
}

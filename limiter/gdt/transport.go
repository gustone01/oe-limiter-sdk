package gdt

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gustone01/oe-limiter-sdk/limiter/core"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Transport 实现 http.RoundTripper，在 HTTP 出站前插入腾讯广告限流逻辑。
type Transport struct {
	base    http.RoundTripper
	manager *RuleManager
}

// NewTransport 创建带腾讯广告限流能力的 RoundTripper。
func NewTransport(db *gorm.DB, rdb *redis.Client, opts ...Option) (*Transport, error) {
	rm, err := NewRuleManager(db, rdb, opts...)
	if err != nil {
		return nil, err
	}
	o := Options{}
	for _, fn := range opts {
		fn(&o)
	}
	o.applyDefaults()
	return &Transport{
		base:    o.Base,
		manager: rm,
	}, nil
}

// Manager 返回内部的规则管理器，可用于审核待审核接口。
func (t *Transport) Manager() *RuleManager {
	return t.manager
}

// Close 停止内部 RuleManager 的 Pub/Sub 订阅，释放资源。
func (t *Transport) Close() error {
	return t.manager.Close()
}

var _ http.RoundTripper = (*Transport)(nil)

// RoundTrip 实现 http.RoundTripper：QPM+QPD 限流 → 出站。
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	normalized := NormalizePath(req.URL.Path)

	allowed, err := t.allowLocal(req.Context(), normalized)
	if err != nil {
		return nil, fmt.Errorf("gdt rate limit check: %w", err)
	}
	if !allowed {
		return core.RateLimitedResponse(req, 0), nil
	}

	return t.base.RoundTrip(req)
}

func (t *Transport) allowLocal(ctx context.Context, normalized string) (bool, error) {
	rule := t.manager.GetRule(normalized)

	var checks []core.WindowCheck
	if rule.QPMLimit > 0 {
		checks = append(checks, core.WindowCheck{
			Key:      core.HashTagKey("gdt:limit", normalized, "qpm"),
			Limit:    rule.QPMLimit,
			WindowMS: 60000,
			Type:     core.WindowTypeSliding,
		})
	}
	if rule.QPDLimit > 0 {
		checks = append(checks, core.WindowCheck{
			Key:      core.HashTagKey("gdt:limit", normalized, "qpd"),
			Limit:    rule.QPDLimit,
			WindowMS: 86400000,
			Type:     core.WindowTypeCounter,
		})
	}

	if len(checks) == 0 {
		return true, nil
	}
	return t.manager.limiter.AllowMultiWindow(ctx, checks)
}

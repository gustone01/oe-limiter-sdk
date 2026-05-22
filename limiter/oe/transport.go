package oe

import (
	"context"
	"fmt"
	"net/http"

	"192.168.10.236/gustone/oe-limiter-sdk/limiter/core"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Transport 实现 http.RoundTripper，在 HTTP 出站前插入巨量引擎限流逻辑。
type Transport struct {
	base    http.RoundTripper
	manager *RuleManager
}

// NewTransport 创建带巨量引擎限流能力的 RoundTripper。
// 进程退出前请调用 Close() 释放 Pub/Sub 资源。
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

// Manager 返回内部的规则管理器。
func (t *Transport) Manager() *RuleManager {
	return t.manager
}

// Close 停止内部 RuleManager。
func (t *Transport) Close() error {
	return t.manager.Close()
}

var _ http.RoundTripper = (*Transport)(nil)

// RoundTrip 实现 http.RoundTripper：滑动窗口 QPS+QPM 限流 → 出站。
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	normalized := NormalizePath(req.URL.Path)

	allowed, err := t.allowLocal(req.Context(), normalized)
	if err != nil {
		return nil, fmt.Errorf("oe rate limit check: %w", err)
	}
	if !allowed {
		return core.RateLimitedResponse(req, 0), nil
	}

	return t.base.RoundTrip(req)
}

func (t *Transport) allowLocal(ctx context.Context, normalized string) (bool, error) {
	rule := t.manager.GetRule(normalized)
	if rule.QPSLimit <= 0 {
		return true, nil
	}

	checks := []core.WindowCheck{
		{
			Key:      core.HashTagKey("oe:limit", normalized, "qps"),
			Limit:    rule.QPSLimit,
			WindowMS: 1000,
			Type:     core.WindowTypeSliding,
		},
		{
			Key:      core.HashTagKey("oe:limit", normalized, "qpm"),
			Limit:    rule.QPSLimit * 100,
			WindowMS: 60000,
			Type:     core.WindowTypeSliding,
		},
	}
	return t.manager.limiter.AllowMultiWindow(ctx, checks)
}

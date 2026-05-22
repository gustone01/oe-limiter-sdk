package oe

import (
	"net/http"
)

const (
	defaultPubSubChannel = "oe:limit:rules:updated"
	defaultFallbackQPS   = 5
)

// Options 巨量引擎限流 SDK 的可选参数。
type Options struct {
	// PubSubChannel 规则变更广播 Redis 频道名，默认 "oe:limit:rules:updated"。
	PubSubChannel string
	// FallbackQPS 未匹配到规则时的兜底 QPS（同时用于待审核记录的建议值），默认 5。
	FallbackQPS int
	// OnDiscover 首次访问未配置接口时触发的回调（异步执行）。
	OnDiscover func(apiPath string)
	// DisablePendingSave 为 true 时不写入待审核表。
	DisablePendingSave bool
	// SkipAutoMigrate 为 true 时跳过自动建表（适用于 DBA 统一管理 DDL 的场景）。
	SkipAutoMigrate bool
	// Base 限流通过后实际发起 HTTP 请求的底层 RoundTripper，默认 http.DefaultTransport。
	Base http.RoundTripper
}

// applyDefaults 填充未设置的选项为默认值。
func (o *Options) applyDefaults() {
	if o.PubSubChannel == "" {
		o.PubSubChannel = defaultPubSubChannel
	}
	if o.FallbackQPS <= 0 {
		o.FallbackQPS = defaultFallbackQPS
	}
	if o.Base == nil {
		o.Base = http.DefaultTransport
	}
}

// Option 用于 NewTransport 的函数式配置项。
type Option func(*Options)

// WithPubSubChannel 设置规则变更的 Redis Pub/Sub 频道名。
func WithPubSubChannel(ch string) Option {
	return func(o *Options) { o.PubSubChannel = ch }
}

// WithFallbackQPS 设置未匹配规则时的兜底 QPS。
func WithFallbackQPS(qps int) Option {
	return func(o *Options) { o.FallbackQPS = qps }
}

// WithOnDiscover 注册「发现未配置接口」时的回调。
func WithOnDiscover(fn func(apiPath string)) Option {
	return func(o *Options) { o.OnDiscover = fn }
}

// WithDisablePendingSave 关闭自动发现写入 MySQL 待审核表。
func WithDisablePendingSave() Option {
	return func(o *Options) { o.DisablePendingSave = true }
}

// WithSkipAutoMigrate 关闭 SDK 启动时的自动建表。
func WithSkipAutoMigrate() Option {
	return func(o *Options) { o.SkipAutoMigrate = true }
}

// WithBaseTransport 设置通过限流检查后使用的底层 HTTP 传输层。
func WithBaseTransport(base http.RoundTripper) Option {
	return func(o *Options) { o.Base = base }
}

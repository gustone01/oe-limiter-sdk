package gdt

import (
	"net/http"
)

const (
	defaultPubSubChannel = "gdt:limit:rules:updated"
	defaultFallbackQPM   = 2000
	defaultFallbackQPD   = 0 // 0 表示不限制
)

// Options 腾讯广告限流 SDK 的可选参数。
type Options struct {
	// PubSubChannel 规则变更广播 Redis 频道名，默认 "gdt:limit:rules:updated"。
	PubSubChannel string
	// FallbackQPM 未匹配到规则时的兜底 QPM，默认 2000。
	FallbackQPM int
	// FallbackQPD 未匹配到规则时的兜底 QPD，0 表示不限制。
	FallbackQPD int
	// OnDiscover 首次访问未配置接口时触发的回调（异步执行）。
	OnDiscover func(apiPath string)
	// UnmatchedUnlimited 为 true 时，未匹配规则的接口仍自动发现并写入待审核表，但不应用兜底限流。
	UnmatchedUnlimited bool
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
	if o.FallbackQPM <= 0 {
		o.FallbackQPM = defaultFallbackQPM
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

// WithFallbackQPM 设置未匹配规则时的兜底 QPM。
func WithFallbackQPM(qpm int) Option {
	return func(o *Options) { o.FallbackQPM = qpm }
}

// WithFallbackQPD 设置未匹配规则时的兜底 QPD（0=不限）。
func WithFallbackQPD(qpd int) Option {
	return func(o *Options) { o.FallbackQPD = qpd }
}

// WithOnDiscover 注册「发现未配置接口」时的回调。
func WithOnDiscover(fn func(apiPath string)) Option {
	return func(o *Options) { o.OnDiscover = fn }
}

// WithUnmatchedUnlimited 开启「未匹配接口仅记录、不限流」模式。
// 适用于采集期或内网环境；生产环境请确认可接受审核前无限流风险。
// 自动发现、待审核表写入与 OnDiscover 回调行为不变，FallbackQPM 仍作为建议值写入 pending。
// 注意：不要与 WithDisablePendingSave 同时使用，否则新接口将既无限流也无记录。
func WithUnmatchedUnlimited() Option {
	return func(o *Options) { o.UnmatchedUnlimited = true }
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

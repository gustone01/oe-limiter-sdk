package limiter

import (
	"net/http"
	"time"
)

const (
	defaultPubSubChannel    = "oe:limit:rules:updated" // 默认规则变更广播频道
	defaultWindowSec        = 1                        // 默认计数窗口 1 秒
	defaultWindowMS         = 1000                     // 滑动窗口默认 1 秒（毫秒）
	defaultGlobalQPS        = 1 << 30                  // 未配置 ALL 全局规则时，视为几乎不限制
	defaultPendingSuggested = 5                        // 待审核记录建议 QPS
	defaultLimiterMode      = LimiterModeSliding       // 与开放平台统计方式一致
)

// Options 限流 SDK 的可选参数，可通过 ManagerOption 传入。
type Options struct {
	// PubSubChannel 管理后台修改规则后，向该 Redis 频道发布消息，触发各实例刷新缓存。
	PubSubChannel string
	// WindowSec 固定窗口模式下的窗口长度（秒）；滑动模式下降级为 WindowMS 的参考。
	WindowSec int
	// WindowMS 滑动窗口长度（毫秒），默认 1000，对应平台「当前时刻倒推 1 秒」。
	WindowMS int
	// LimiterMode 计数算法：LimiterModeSliding（默认）或 LimiterModeFixed。
	LimiterMode string
	// DisablePlatformPenalty 为 true 时不根据平台响应写入/检查封禁 Key。
	DisablePlatformPenalty bool
	// PlatformPenaltyTTL 平台 40110 未返回 X-RateLimit-RetryIn 时的默认封禁时长（默认 5 分钟）。
	PlatformPenaltyTTL time.Duration
	// OnPlatformRateLimit 平台 40110 开发者频控触发封禁时的回调。
	OnPlatformRateLimit func(serviceName, apiPath string, code int, retryAfter time.Duration)
	// DefaultGlobalQPS 当数据库中没有 service_name=ALL 的全局规则时，使用的默认全局上限。
	DefaultGlobalQPS int
	// OnDiscover 首次访问到未在 MySQL 中配置的接口时触发，可用于钉钉/邮件告警。
	OnDiscover func(serviceName, apiPath string)
	// PendingSuggestedQPS 写入待审核表时的建议 QPS，同时作为运行时兜底配额。
	PendingSuggestedQPS int
	// DisablePendingSave 为 true 时不写入待审核表（仅日志 + Redis 标记）。
	DisablePendingSave bool
	// SkipAutoMigrate 为 true 时跳过自动建表（生产环境由 DBA 执行 schema.sql 时使用）。
	SkipAutoMigrate bool
	// Base 限流通过后实际发起 HTTP 请求的底层 RoundTripper，默认 http.DefaultTransport。
	Base http.RoundTripper
}

// applyDefaults 为未设置的选项填充默认值。
func (o *Options) applyDefaults() {
	if o.PubSubChannel == "" {
		o.PubSubChannel = defaultPubSubChannel
	}
	if o.WindowSec <= 0 {
		o.WindowSec = defaultWindowSec
	}
	if o.WindowMS <= 0 {
		o.WindowMS = defaultWindowMS
	}
	if o.LimiterMode == "" {
		o.LimiterMode = defaultLimiterMode
	}
	if o.PlatformPenaltyTTL <= 0 {
		o.PlatformPenaltyTTL = defaultPenaltyTTL
	}
	if o.DefaultGlobalQPS <= 0 {
		o.DefaultGlobalQPS = defaultGlobalQPS
	}
	if o.PendingSuggestedQPS <= 0 {
		o.PendingSuggestedQPS = defaultPendingSuggested
	}
	if o.Base == nil {
		o.Base = http.DefaultTransport
	}
}

// ManagerOption 用于 NewRuleManager / NewTransport 的函数式配置项。
type ManagerOption func(*Options)

// WithPubSubChannel 设置规则变更的 Redis Pub/Sub 频道名。
func WithPubSubChannel(ch string) ManagerOption {
	return func(o *Options) { o.PubSubChannel = ch }
}

// WithWindow 设置限流计数的时间窗口长度（内部会转为秒，最小 1 秒）。
func WithWindow(d time.Duration) ManagerOption {
	return func(o *Options) {
		sec := int(d.Seconds())
		if sec < 1 {
			sec = 1
		}
		o.WindowSec = sec
	}
}

// WithDefaultGlobalQPS 设置没有 ALL 全局规则时的默认全局限流上限。
func WithDefaultGlobalQPS(qps int) ManagerOption {
	return func(o *Options) { o.DefaultGlobalQPS = qps }
}

// WithOnDiscover 注册「发现未配置接口」时的回调，仅每个接口首次触发一次。
func WithOnDiscover(fn func(serviceName, apiPath string)) ManagerOption {
	return func(o *Options) { o.OnDiscover = fn }
}

// WithPendingSuggestedQPS 设置自动发现写入待审核表的建议 QPS（默认 5）。
func WithPendingSuggestedQPS(qps int) ManagerOption {
	return func(o *Options) { o.PendingSuggestedQPS = qps }
}

// WithDisablePendingSave 关闭自动发现写入 MySQL 待审核表。
func WithDisablePendingSave() ManagerOption {
	return func(o *Options) { o.DisablePendingSave = true }
}

// WithSkipAutoMigrate 关闭 SDK 启动时的自动建表（表已由运维创建时使用）。
func WithSkipAutoMigrate() ManagerOption {
	return func(o *Options) { o.SkipAutoMigrate = true }
}

// WithBaseTransport 设置通过限流检查后使用的底层 HTTP 传输层。
func WithBaseTransport(base http.RoundTripper) ManagerOption {
	return func(o *Options) { o.Base = base }
}

// WithLimiterMode 设置限流算法：LimiterModeSliding（默认）或 LimiterModeFixed。
func WithLimiterMode(mode string) ManagerOption {
	return func(o *Options) { o.LimiterMode = mode }
}

// WithWindowMS 设置滑动窗口长度（毫秒），默认 1000。
func WithWindowMS(ms int) ManagerOption {
	return func(o *Options) { o.WindowMS = ms }
}

// WithDisablePlatformPenalty 关闭平台响应路径封禁（仅保留本地滑动窗口限流）。
func WithDisablePlatformPenalty() ManagerOption {
	return func(o *Options) { o.DisablePlatformPenalty = true }
}

// WithPlatformPenaltyTTL 设置平台惩罚默认封禁时长（未返回 X-RateLimit-RetryIn 时）。
func WithPlatformPenaltyTTL(d time.Duration) ManagerOption {
	return func(o *Options) { o.PlatformPenaltyTTL = d }
}

// WithOnPlatformRateLimit 注册平台频控封禁回调（在响应路径触发）。
func WithOnPlatformRateLimit(fn func(serviceName, apiPath string, code int, retryAfter time.Duration)) ManagerOption {
	return func(o *Options) { o.OnPlatformRateLimit = fn }
}

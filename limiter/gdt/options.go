package gdt

import (
	"net/http"
)

const (
	defaultPubSubChannel = "gdt:limit:rules:updated"
	defaultFallbackQPM   = 1000
	defaultFallbackQPD   = 0 // 0 表示不限制
)

// Options 腾讯广告限流 SDK 的可选参数。
type Options struct {
	PubSubChannel      string
	FallbackQPM        int
	FallbackQPD        int
	OnDiscover         func(apiPath string)
	DisablePendingSave bool
	SkipAutoMigrate    bool
	Base               http.RoundTripper
}

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

func WithPubSubChannel(ch string) Option {
	return func(o *Options) { o.PubSubChannel = ch }
}

func WithFallbackQPM(qpm int) Option {
	return func(o *Options) { o.FallbackQPM = qpm }
}

func WithFallbackQPD(qpd int) Option {
	return func(o *Options) { o.FallbackQPD = qpd }
}

func WithOnDiscover(fn func(apiPath string)) Option {
	return func(o *Options) { o.OnDiscover = fn }
}

func WithDisablePendingSave() Option {
	return func(o *Options) { o.DisablePendingSave = true }
}

func WithSkipAutoMigrate() Option {
	return func(o *Options) { o.SkipAutoMigrate = true }
}

func WithBaseTransport(base http.RoundTripper) Option {
	return func(o *Options) { o.Base = base }
}

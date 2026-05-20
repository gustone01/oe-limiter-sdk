package limiter

import (
	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"gorm.io/gorm"
)

// AutoMigrate 创建或更新限流表（oe_rate_limit_rules、oe_rate_limit_pending）。
//
// 默认由 NewRuleManager / NewTransport 自动调用，业务侧一般无需手动执行。
// 生产若由 DBA 管表，请使用 limiter.WithSkipAutoMigrate()。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.RateLimitRule{},
		&model.RateLimitPending{},
	)
}

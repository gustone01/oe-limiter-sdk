package oe

import (
	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"gorm.io/gorm"
)

// AutoMigrate 创建或更新巨量引擎限流表（oe_rate_limit_rules、oe_rate_limit_pending）。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.OeRateLimitRule{},
		&model.OeRateLimitPending{},
	)
}

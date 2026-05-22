package gdt

import (
	"github.com/gustone01/oe-limiter-sdk/model"

	"gorm.io/gorm"
)

// AutoMigrate 创建或更新腾讯广告限流表（gdt_rate_limit_rules、gdt_rate_limit_pending）。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.GdtRateLimitRule{},
		&model.GdtRateLimitPending{},
	)
}

package model

import "gorm.io/gorm"

// GdtRateLimitRule 腾讯广告限流规则，对应表 gdt_rate_limit_rules。
type GdtRateLimitRule struct {
	gorm.Model
	APIPathPrefix string `gorm:"column:api_path_prefix;size:128;not null;uniqueIndex;comment:API路径前缀（去版本号归一化后）" json:"api_path_prefix"`
	QPMLimit      int    `gorm:"column:qpm_limit;not null;default:0;comment:QPM配额（滑动窗口分钟级，0=不限）" json:"qpm_limit"`
	QPDLimit      int    `gorm:"column:qpd_limit;not null;default:0;comment:QPD配额（计数器日级，0=不限）" json:"qpd_limit"`
	Enabled       int    `gorm:"column:enabled;not null;default:1;comment:1启用 0禁用" json:"enabled"`
}

func (GdtRateLimitRule) TableName() string { return "gdt_rate_limit_rules" }

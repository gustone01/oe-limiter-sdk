package model

import "gorm.io/gorm"

// OeRateLimitRule 巨量引擎限流规则，对应表 oe_rate_limit_rules。
type OeRateLimitRule struct {
	gorm.Model
	APIPathPrefix string `gorm:"column:api_path_prefix;size:128;not null;uniqueIndex;comment:API路径前缀，最长前缀匹配（归一化后）" json:"api_path_prefix"`
	QPSLimit      int    `gorm:"column:qps_limit;not null;default:10;comment:QPS配额（滑动窗口秒级，QPM=QPS*100自动派生）" json:"qps_limit"`
	Enabled       int    `gorm:"column:enabled;not null;default:1;comment:1启用 0禁用" json:"enabled"`
}

func (OeRateLimitRule) TableName() string { return "oe_rate_limit_rules" }

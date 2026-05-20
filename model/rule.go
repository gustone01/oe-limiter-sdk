package model

import "gorm.io/gorm"

// RateLimitRule 对应表 oe_rate_limit_rules，存储各服务、各 API 路径前缀的 QPS 配额。
type RateLimitRule struct {
	gorm.Model
	// ServiceName 服务标识，如 click、data、event、gy；全局共享规则使用 ALL。
	ServiceName string `gorm:"column:service_name;size:32;not null;uniqueIndex:uk_service_api,priority:1" json:"service_name"`
	// APIPathPrefix API 路径前缀，请求路径以此开头即命中该规则（最长前缀优先）。
	APIPathPrefix string `gorm:"column:api_path_prefix;size:128;not null;uniqueIndex:uk_service_api,priority:2;index:idx_path" json:"api_path_prefix"`
	// QPSLimit 每秒允许的最大请求数（固定时间窗口内计数）。
	QPSLimit int `gorm:"column:qps_limit;not null;default:10" json:"qps_limit"`
	// IsShared 是否计入全局共享池：1=共享（如 ALL 服务规则），0=独占。
	IsShared int `gorm:"column:is_shared;not null;default:0" json:"is_shared"`
	// Enabled 是否启用：1=启用，0=禁用（禁用后不会加载到本地缓存）。
	Enabled int `gorm:"column:enabled;not null;default:1" json:"enabled"`
}

// TableName 指定 GORM 使用的表名。
func (RateLimitRule) TableName() string {
	return "oe_rate_limit_rules"
}

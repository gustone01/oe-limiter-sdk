package model

import (
	"time"

	"gorm.io/gorm"
)

// 待审核记录状态
const (
	PendingStatusPending  = 0 // 待审核
	PendingStatusApproved = 1 // 已通过（已写入正式规则表）
	PendingStatusRejected = 2 // 已拒绝
)

// RateLimitPending 自动发现的新接口，对应表 oe_rate_limit_pending。
// 审核通过后写入 oe_rate_limit_rules，并 PublishRuleUpdate。
type RateLimitPending struct {
	gorm.Model
	// ServiceName 服务标识，如 click、data、event、gy。
	ServiceName string `gorm:"column:service_name;size:32;not null;uniqueIndex:uk_pending_service_api,priority:1" json:"service_name"`
	// APIPathPrefix 归一化后的 API 路径前缀。
	APIPathPrefix string `gorm:"column:api_path_prefix;size:128;not null;uniqueIndex:uk_pending_service_api,priority:2" json:"api_path_prefix"`
	// SuggestedQPS 建议 QPS，审核通过时可覆盖。
	SuggestedQPS int `gorm:"column:suggested_qps;not null;default:5" json:"suggested_qps"`
	// Status 审核状态：0 待审核 / 1 已通过 / 2 已拒绝。
	Status int `gorm:"column:status;not null;default:0;index:idx_status" json:"status"`
	// Remark 审核备注。
	Remark string `gorm:"column:remark;size:255" json:"remark"`
	// DiscoveredAt 首次自动发现时间。
	DiscoveredAt time.Time `gorm:"column:discovered_at;not null" json:"discovered_at"`
	// ReviewedAt 审核完成时间。
	ReviewedAt *time.Time `gorm:"column:reviewed_at" json:"reviewed_at"`
}

// TableName 指定 GORM 使用的表名。
func (RateLimitPending) TableName() string {
	return "oe_rate_limit_pending"
}

package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	PendingStatusPending  = 0 // 待审核
	PendingStatusApproved = 1 // 已通过
	PendingStatusRejected = 2 // 已拒绝
)

// OeRateLimitPending 巨量引擎自动发现的新接口待审核记录，对应表 oe_rate_limit_pending。
type OeRateLimitPending struct {
	gorm.Model
	APIPathPrefix string     `gorm:"column:api_path_prefix;size:128;not null;uniqueIndex;comment:归一化路径" json:"api_path_prefix"`
	SuggestedQPS  int        `gorm:"column:suggested_qps;not null;default:5;comment:建议QPS" json:"suggested_qps"`
	Status        int        `gorm:"column:status;not null;default:0;index:idx_status;comment:0待审核 1已通过 2已拒绝" json:"status"`
	Remark        string     `gorm:"column:remark;size:255;comment:备注" json:"remark"`
	DiscoveredAt  time.Time  `gorm:"column:discovered_at;not null;comment:首次发现时间" json:"discovered_at"`
	ReviewedAt    *time.Time `gorm:"column:reviewed_at;comment:审核时间" json:"reviewed_at"`
}

func (OeRateLimitPending) TableName() string { return "oe_rate_limit_pending" }

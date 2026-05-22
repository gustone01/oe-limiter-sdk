package model

import (
	"time"

	"gorm.io/gorm"
)

// GdtRateLimitPending 腾讯广告自动发现的新接口待审核记录，对应表 gdt_rate_limit_pending。
type GdtRateLimitPending struct {
	gorm.Model
	APIPathPrefix string     `gorm:"column:api_path_prefix;size:128;not null;uniqueIndex;comment:归一化路径（去版本号）" json:"api_path_prefix"`
	SuggestedQPM  int        `gorm:"column:suggested_qpm;not null;default:100;comment:建议QPM" json:"suggested_qpm"`
	Status        int        `gorm:"column:status;not null;default:0;index:idx_status;comment:0待审核 1已通过 2已拒绝" json:"status"`
	Remark        string     `gorm:"column:remark;size:255;comment:备注" json:"remark"`
	DiscoveredAt  time.Time  `gorm:"column:discovered_at;not null;comment:首次发现时间" json:"discovered_at"`
	ReviewedAt    *time.Time `gorm:"column:reviewed_at;comment:审核时间" json:"reviewed_at"`
}

func (GdtRateLimitPending) TableName() string { return "gdt_rate_limit_pending" }

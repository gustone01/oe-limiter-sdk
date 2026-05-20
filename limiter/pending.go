package limiter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// savePending 将自动发现的接口写入待审核表（同一 service+path 仅保留一条，重复发现忽略）。
func (rm *RuleManager) savePending(ctx context.Context, serviceName, apiPath string) error {
	if rm.opts.DisablePendingSave {
		return nil
	}
	qps := rm.opts.PendingSuggestedQPS
	if qps <= 0 {
		qps = DefaultRule().QPSLimit
	}
	row := model.RateLimitPending{
		ServiceName:   serviceName,
		APIPathPrefix: apiPath,
		SuggestedQPS:  qps,
		Status:        model.PendingStatusPending,
		DiscoveredAt:  time.Now(),
	}
	// 已存在相同 service+path 时不重复插入（含待审核/已通过/已拒绝）
	return rm.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "service_name"}, {Name: "api_path_prefix"}},
		DoNothing: true,
	}).Create(&row).Error
}

// ListPending 查询所有待审核记录，供管理后台展示。
func (rm *RuleManager) ListPending(ctx context.Context) ([]model.RateLimitPending, error) {
	var rows []model.RateLimitPending
	err := rm.db.WithContext(ctx).
		Where("status = ?", model.PendingStatusPending).
		Order("discovered_at DESC").
		Find(&rows).Error
	return rows, err
}

// ApprovePending 审核通过：写入正式规则表 oe_rate_limit_rules，并标记待审核记录为已通过。
//
// qps 若 <=0 则使用待审核记录中的 suggested_qps；成功后需由调用方或本方法触发 PublishRuleUpdate。
func (rm *RuleManager) ApprovePending(ctx context.Context, pendingID uint, qps int) error {
	var pending model.RateLimitPending
	if err := rm.db.WithContext(ctx).First(&pending, pendingID).Error; err != nil {
		return err
	}
	if pending.Status != model.PendingStatusPending {
		return fmt.Errorf("pending id=%d status=%d, not pending", pendingID, pending.Status)
	}
	if qps <= 0 {
		qps = pending.SuggestedQPS
	}

	return rm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rule := model.RateLimitRule{
			ServiceName:   pending.ServiceName,
			APIPathPrefix: pending.APIPathPrefix,
			QPSLimit:      qps,
			IsShared:      0,
			Enabled:       1,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "service_name"}, {Name: "api_path_prefix"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"qps_limit", "enabled", "updated_at",
			}),
		}).Create(&rule).Error; err != nil {
			return err
		}

		now := time.Now()
		res := tx.Model(&model.RateLimitPending{}).
			Where("id = ? AND status = ?", pendingID, model.PendingStatusPending).
			Updates(map[string]interface{}{
				"status":      model.PendingStatusApproved,
				"reviewed_at": now,
				"remark":      "approved",
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("pending record already reviewed")
		}
		return nil
	})
}

// RejectPending 审核拒绝，仅更新待审核表状态，不写入正式规则。
func (rm *RuleManager) RejectPending(ctx context.Context, pendingID uint, remark string) error {
	now := time.Now()
	res := rm.db.WithContext(ctx).Model(&model.RateLimitPending{}).
		Where("id = ? AND status = ?", pendingID, model.PendingStatusPending).
		Updates(map[string]interface{}{
			"status":      model.PendingStatusRejected,
			"reviewed_at": now,
			"remark":      remark,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ApprovePendingAndReload 审核通过并刷新本实例规则缓存，同时广播给其他实例。
func (rm *RuleManager) ApprovePendingAndReload(ctx context.Context, pendingID uint, qps int) error {
	if err := rm.ApprovePending(ctx, pendingID, qps); err != nil {
		return err
	}
	if err := rm.reload(ctx); err != nil {
		return err
	}
	return rm.PublishRuleUpdate(ctx)
}

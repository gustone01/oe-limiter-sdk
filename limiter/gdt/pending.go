package gdt

import (
	"context"
	"time"

	"github.com/gustone01/oe-limiter-sdk/limiter/core"
	"github.com/gustone01/oe-limiter-sdk/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// savePending 将自动发现的接口写入待审核表（冲突则忽略）。
func (rm *RuleManager) savePending(ctx context.Context, apiPath string) error {
	if rm.opts.DisablePendingSave {
		return nil
	}
	qpm := rm.opts.FallbackQPM
	if qpm <= 0 {
		qpm = defaultFallbackQPM
	}
	row := model.GdtRateLimitPending{
		APIPathPrefix: apiPath,
		SuggestedQPM:  qpm,
		Status:        model.PendingStatusPending,
		DiscoveredAt:  time.Now(),
	}
	return rm.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "api_path_prefix"}},
		DoNothing: true,
	}).Create(&row).Error
}

// ListPending 查询所有待审核记录。
func (rm *RuleManager) ListPending(ctx context.Context) ([]model.GdtRateLimitPending, error) {
	var rows []model.GdtRateLimitPending
	err := rm.db.WithContext(ctx).
		Where("status = ?", model.PendingStatusPending).
		Order("discovered_at DESC").
		Find(&rows).Error
	return rows, err
}

// ApprovePending 审核通过：写入正式规则表并标记已通过。
func (rm *RuleManager) ApprovePending(ctx context.Context, pendingID uint, qpm, qpd int) error {
	var pending model.GdtRateLimitPending
	if err := rm.db.WithContext(ctx).First(&pending, pendingID).Error; err != nil {
		return err
	}
	if pending.Status != model.PendingStatusPending {
		return core.ErrPendingNotPending
	}
	if qpm <= 0 {
		qpm = pending.SuggestedQPM
	}

	return rm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rule := model.GdtRateLimitRule{
			APIPathPrefix: pending.APIPathPrefix,
			QPMLimit:      qpm,
			QPDLimit:      qpd,
			Enabled:       1,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "api_path_prefix"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"qpm_limit", "qpd_limit", "enabled", "updated_at",
			}),
		}).Create(&rule).Error; err != nil {
			return err
		}

		now := time.Now()
		res := tx.Model(&model.GdtRateLimitPending{}).
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
			return core.ErrPendingAlreadyReviewed
		}
		return nil
	})
}

// RejectPending 审核拒绝。
func (rm *RuleManager) RejectPending(ctx context.Context, pendingID uint, remark string) error {
	now := time.Now()
	res := rm.db.WithContext(ctx).Model(&model.GdtRateLimitPending{}).
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

// ApprovePendingAndReload 审核通过并刷新缓存 + 广播。
func (rm *RuleManager) ApprovePendingAndReload(ctx context.Context, pendingID uint, qpm, qpd int) error {
	if err := rm.ApprovePending(ctx, pendingID, qpm, qpd); err != nil {
		return err
	}
	if err := rm.reload(ctx); err != nil {
		return err
	}
	return rm.PublishRuleUpdate(ctx)
}

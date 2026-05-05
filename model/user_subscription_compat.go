package model

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostConsumeUserSubscriptionDelta is a legacy compatibility helper for old
// async task records that only persisted subscription_id and not the newer
// bucket/group allocation snapshot.
//
// Semantics follow the legacy path:
// - delta > 0: consume more subscription quota
// - delta < 0: refund subscription quota
//
// It intentionally does not attempt to reconstruct per-group daily usage
// because historical task rows do not carry enough information for that.
func PostConsumeUserSubscriptionDelta(userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()

		var sub UserSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", userSubscriptionId).
			First(&sub).Error; err != nil {
			return err
		}

		if sub.TotalQuota == 0 {
			return errors.New("legacy subscription delta 不支持无限额度订阅")
		}

		nextRemaining := sub.RemainingQuota - int(delta)
		if delta > 0 && nextRemaining < 0 {
			return fmt.Errorf("subscription remaining insufficient, remaining=%d delta=%d", sub.RemainingQuota, delta)
		}
		if nextRemaining < 0 {
			nextRemaining = 0
		}
		if nextRemaining > sub.TotalQuota {
			nextRemaining = sub.TotalQuota
		}
		if nextRemaining == sub.RemainingQuota {
			return nil
		}

		updates := map[string]interface{}{
			"remaining_quota": nextRemaining,
		}
		nextInvalidAt := sub.InvalidAt
		if nextRemaining > 0 {
			nextInvalidAt = 0
		} else if nextInvalidAt <= 0 {
			nextInvalidAt = now
		}
		if nextInvalidAt != sub.InvalidAt {
			updates["invalid_at"] = nextInvalidAt
		}

		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
			return err
		}

		if sub.Credited {
			if err := applyUserSubscriptionBalanceDeltaTx(tx, sub.UserId, sub.BillingUnit, nextRemaining-sub.RemainingQuota); err != nil {
				return err
			}
		}

		return refreshUserSubscriptionSnapshot(tx, sub.UserId, now)
	})
}

package model

import (
	"fmt"
	"one-api/common"

	"gorm.io/gorm"
)

// dropUserSubscriptionsCreditedOnlyIndexIfNeeded removes the legacy single-column index on
// user_subscriptions.credited in MySQL.
//
// Why:
// - activateDueSubscriptions / deactivateNotStartedSubscriptions use SELECT ... FOR UPDATE with
//   (user_id=?, credited=?, invalid_at=0, ...).
// - If MySQL picks the low-cardinality `credited` index, InnoDB may scan & next-key lock rows across
//   different users, which can create cross-user deadlocks (MySQL error 1213 / SQLSTATE 40001).
//
// The fix is to rely on user-scoped indexes (e.g. user_id or composite indexes) and avoid the
// credited-only index being chosen.
func dropUserSubscriptionsCreditedOnlyIndexIfNeeded(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("nil db")
	}
	if !common.UsingMySQL {
		return nil
	}

	const (
		tableName = "user_subscriptions"
		indexName = "idx_user_subscriptions_credited"
	)

	var cnt int64
	if err := tx.Raw(
		"SELECT COUNT(1) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		tableName,
		indexName,
	).Scan(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return nil
	}

	return tx.Exec("DROP INDEX `" + indexName + "` ON `" + tableName + "`").Error
}


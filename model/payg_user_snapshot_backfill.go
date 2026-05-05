package model

import (
	"errors"
	"fmt"
	"one-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func syncLockedUserPaygSnapshotFromBalancesTx(tx *gorm.DB, user *User) (bool, error) {
	if tx == nil {
		return false, errors.New("tx 为空")
	}
	if user == nil || user.Id <= 0 {
		return false, errors.New("user 无效")
	}
	if !tx.Migrator().HasTable(&PaygUserBalance{}) {
		return false, nil
	}

	var balances []PaygUserBalance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("product_id", "sort_order", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota", "history_quota").
		Where("user_id = ?", user.Id).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return false, err
	}

	totalRemaining := 0
	totalHistory := 0
	positiveBalances := make([]PaygUserBalance, 0, len(balances))
	for _, balance := range balances {
		if balance.RemainingQuota < 0 {
			return false, fmt.Errorf("用户 %d 的 payg_user_balance remaining_quota 数据错误", user.Id)
		}
		if balance.HistoryQuota < 0 {
			return false, fmt.Errorf("用户 %d 的 payg_user_balance history_quota 数据错误", user.Id)
		}
		totalRemaining += balance.RemainingQuota
		historyQuota := balance.HistoryQuota
		if historyQuota < balance.RemainingQuota {
			historyQuota = balance.RemainingQuota
		}
		totalHistory += historyQuota
		if balance.RemainingQuota > 0 {
			positiveBalances = append(positiveBalances, balance)
		}
	}

	unionGroupsJSON, err := UnionPaygAllowedGroupsFromBalances(positiveBalances)
	if err != nil {
		return false, err
	}

	updates := map[string]interface{}{}
	if user.PayAsYouGoQuota != totalRemaining {
		updates["payg_quota"] = totalRemaining
		user.PayAsYouGoQuota = totalRemaining
	}
	if user.PayAsYouGoHistoryQuota != totalHistory {
		updates["payg_history_quota"] = totalHistory
		user.PayAsYouGoHistoryQuota = totalHistory
	}
	if string(user.PayAsYouGoAllowedGroups) != string(unionGroupsJSON) {
		updates["payg_allowed_groups"] = unionGroupsJSON
		user.PayAsYouGoAllowedGroups = unionGroupsJSON
	}
	// `quota` is a legacy aggregate cache and may historically omit subscription quota,
	// but it must never be lower than the current PAYG remaining balance.
	if user.Quota < totalRemaining {
		updates["quota"] = totalRemaining
		user.Quota = totalRemaining
	}

	if len(updates) == 0 {
		return false, nil
	}
	if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(updates).Error; err != nil {
		return false, err
	}
	return true, nil
}

func SyncUserPaygSnapshotFromBalancesTx(tx *gorm.DB, userId int) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return false, errors.New("userId 无效")
	}
	if !tx.Migrator().HasTable(&PaygUserBalance{}) {
		return false, nil
	}

	var user User
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "quota", "payg_quota", "payg_history_quota", "payg_allowed_groups").
		Where("id = ?", userId).
		Limit(1).
		Find(&user)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, gorm.ErrRecordNotFound
	}
	return syncLockedUserPaygSnapshotFromBalancesTx(tx, &user)
}

// BackfillUsersPaygSnapshotFromBalances normalizes user-level PAYG aggregate snapshot fields
// from payg_user_balances, and guarantees users.quota >= users.payg_quota.
func BackfillUsersPaygSnapshotFromBalances(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&PaygUserBalance{}) {
		return nil
	}

	var orphanUserIDs []int
	if err := tx.Table("payg_user_balances b").
		Distinct("b.user_id").
		Joins("LEFT JOIN users u ON u.id = b.user_id AND u.deleted_at IS NULL").
		Where("b.user_id > 0").
		Where("u.id IS NULL").
		Order("b.user_id ASC").
		Pluck("b.user_id", &orphanUserIDs).Error; err != nil {
		return err
	}
	if len(orphanUserIDs) > 0 {
		common.SysLog(fmt.Sprintf("skip backfilling %d orphaned PAYG user snapshots because users are missing or deleted: user_ids=%v", len(orphanUserIDs), orphanUserIDs))
	}

	var userIDs []int
	if err := tx.Table("payg_user_balances b").
		Distinct("b.user_id").
		Joins("JOIN users u ON u.id = b.user_id AND u.deleted_at IS NULL").
		Where("b.user_id > 0").
		Order("b.user_id ASC").
		Pluck("b.user_id", &userIDs).Error; err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}

	changedCount := 0
	for _, userID := range userIDs {
		changed, err := SyncUserPaygSnapshotFromBalancesTx(tx, userID)
		if err != nil {
			return err
		}
		if changed {
			changedCount++
		}
	}

	if changedCount > 0 {
		common.SysLog(fmt.Sprintf("backfilled %d user PAYG snapshots from payg_user_balances", changedCount))
	}
	return nil
}

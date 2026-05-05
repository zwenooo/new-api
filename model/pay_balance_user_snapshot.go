package model

import (
	"errors"
	"fmt"
	"one-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func syncLockedUserPayRequestSnapshotFromBalancesTx(tx *gorm.DB, user *User) (bool, error) {
	if tx == nil {
		return false, errors.New("tx 为空")
	}
	if user == nil || user.Id <= 0 {
		return false, errors.New("user 无效")
	}
	if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
		return false, nil
	}

	var balances []PayRequestUserBalance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("product_id", "allowed_group_ids", "allowed_groups", "remaining_requests", "history_requests", "sort_order").
		Where("user_id = ?", user.Id).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return false, err
	}

	totalRemaining := 0
	totalHistory := 0
	positiveBalances := make([]PayRequestUserBalance, 0, len(balances))
	for _, balance := range balances {
		if balance.RemainingRequests < 0 {
			return false, fmt.Errorf("用户 %d 的 pay_request_user_balance remaining_requests 数据错误", user.Id)
		}
		if balance.HistoryRequests < 0 {
			return false, fmt.Errorf("用户 %d 的 pay_request_user_balance history_requests 数据错误", user.Id)
		}
		totalRemaining += balance.RemainingRequests
		historyRequests := balance.HistoryRequests
		if historyRequests < balance.RemainingRequests {
			historyRequests = balance.RemainingRequests
		}
		totalHistory += historyRequests
		if balance.RemainingRequests > 0 {
			positiveBalances = append(positiveBalances, balance)
		}
	}

	unionGroupsJSON, err := UnionPayRequestAllowedGroupsFromBalances(positiveBalances)
	if err != nil {
		return false, err
	}

	updates := map[string]interface{}{}
	if user.PayRequestQuota != totalRemaining {
		updates["pay_request_quota"] = totalRemaining
		user.PayRequestQuota = totalRemaining
	}
	if user.PayRequestHistoryQuota != totalHistory {
		updates["pay_request_history_quota"] = totalHistory
		user.PayRequestHistoryQuota = totalHistory
	}
	if string(user.PayRequestAllowedGroups) != string(unionGroupsJSON) {
		updates["pay_request_allowed_groups"] = unionGroupsJSON
		user.PayRequestAllowedGroups = unionGroupsJSON
	}

	if len(updates) == 0 {
		return false, nil
	}
	if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(updates).Error; err != nil {
		return false, err
	}
	return true, nil
}

func SyncUserPayRequestSnapshotFromBalancesTx(tx *gorm.DB, userId int) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return false, errors.New("userId 无效")
	}
	if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
		return false, nil
	}

	var user User
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "pay_request_quota", "pay_request_history_quota", "pay_request_allowed_groups").
		Where("id = ?", userId).
		Limit(1).
		Find(&user)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, gorm.ErrRecordNotFound
	}
	return syncLockedUserPayRequestSnapshotFromBalancesTx(tx, &user)
}

func BackfillUsersPayRequestSnapshotFromBalances(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
		return nil
	}

	var orphanUserIDs []int
	if err := tx.Table("pay_request_user_balances b").
		Distinct("b.user_id").
		Joins("LEFT JOIN users u ON u.id = b.user_id AND u.deleted_at IS NULL").
		Where("b.user_id > 0").
		Where("u.id IS NULL").
		Order("b.user_id ASC").
		Pluck("b.user_id", &orphanUserIDs).Error; err != nil {
		return err
	}
	if len(orphanUserIDs) > 0 {
		common.SysLog(fmt.Sprintf("skip backfilling %d orphaned pay-request user snapshots because users are missing or deleted: user_ids=%v", len(orphanUserIDs), orphanUserIDs))
	}

	var userIDs []int
	if err := tx.Table("pay_request_user_balances b").
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
		changed, err := SyncUserPayRequestSnapshotFromBalancesTx(tx, userID)
		if err != nil {
			return err
		}
		if changed {
			changedCount++
		}
	}
	if changedCount > 0 {
		common.SysLog(fmt.Sprintf("backfilled %d user pay-request snapshots from pay_request_user_balances", changedCount))
	}
	return nil
}

func syncLockedUserPayTokenSnapshotFromBalancesTx(tx *gorm.DB, user *User) (bool, error) {
	if tx == nil {
		return false, errors.New("tx 为空")
	}
	if user == nil || user.Id <= 0 {
		return false, errors.New("user 无效")
	}
	if !tx.Migrator().HasTable(&PayTokenUserBalance{}) {
		return false, nil
	}

	var balances []PayTokenUserBalance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("product_id", "allowed_group_ids", "allowed_groups", "remaining_tokens", "history_tokens", "sort_order").
		Where("user_id = ?", user.Id).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return false, err
	}

	totalRemaining := 0
	totalHistory := 0
	positiveBalances := make([]PayTokenUserBalance, 0, len(balances))
	for _, balance := range balances {
		if balance.RemainingTokens < 0 {
			return false, fmt.Errorf("用户 %d 的 pay_token_user_balance remaining_tokens 数据错误", user.Id)
		}
		if balance.HistoryTokens < 0 {
			return false, fmt.Errorf("用户 %d 的 pay_token_user_balance history_tokens 数据错误", user.Id)
		}
		totalRemaining += balance.RemainingTokens
		historyTokens := balance.HistoryTokens
		if historyTokens < balance.RemainingTokens {
			historyTokens = balance.RemainingTokens
		}
		totalHistory += historyTokens
		if balance.RemainingTokens > 0 {
			positiveBalances = append(positiveBalances, balance)
		}
	}

	unionGroupsJSON, err := UnionPayTokenAllowedGroupsFromBalances(positiveBalances)
	if err != nil {
		return false, err
	}

	updates := map[string]interface{}{}
	if user.PayTokenQuota != totalRemaining {
		updates["pay_token_quota"] = totalRemaining
		user.PayTokenQuota = totalRemaining
	}
	if user.PayTokenHistoryQuota != totalHistory {
		updates["pay_token_history_quota"] = totalHistory
		user.PayTokenHistoryQuota = totalHistory
	}
	if string(user.PayTokenAllowedGroups) != string(unionGroupsJSON) {
		updates["pay_token_allowed_groups"] = unionGroupsJSON
		user.PayTokenAllowedGroups = unionGroupsJSON
	}

	if len(updates) == 0 {
		return false, nil
	}
	if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(updates).Error; err != nil {
		return false, err
	}
	return true, nil
}

func SyncUserPayTokenSnapshotFromBalancesTx(tx *gorm.DB, userId int) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return false, errors.New("userId 无效")
	}
	if !tx.Migrator().HasTable(&PayTokenUserBalance{}) {
		return false, nil
	}

	var user User
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "pay_token_quota", "pay_token_history_quota", "pay_token_allowed_groups").
		Where("id = ?", userId).
		Limit(1).
		Find(&user)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, gorm.ErrRecordNotFound
	}
	return syncLockedUserPayTokenSnapshotFromBalancesTx(tx, &user)
}

func BackfillUsersPayTokenSnapshotFromBalances(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&PayTokenUserBalance{}) {
		return nil
	}

	var orphanUserIDs []int
	if err := tx.Table("pay_token_user_balances b").
		Distinct("b.user_id").
		Joins("LEFT JOIN users u ON u.id = b.user_id AND u.deleted_at IS NULL").
		Where("b.user_id > 0").
		Where("u.id IS NULL").
		Order("b.user_id ASC").
		Pluck("b.user_id", &orphanUserIDs).Error; err != nil {
		return err
	}
	if len(orphanUserIDs) > 0 {
		common.SysLog(fmt.Sprintf("skip backfilling %d orphaned pay-token user snapshots because users are missing or deleted: user_ids=%v", len(orphanUserIDs), orphanUserIDs))
	}

	var userIDs []int
	if err := tx.Table("pay_token_user_balances b").
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
		changed, err := SyncUserPayTokenSnapshotFromBalancesTx(tx, userID)
		if err != nil {
			return err
		}
		if changed {
			changedCount++
		}
	}
	if changedCount > 0 {
		common.SysLog(fmt.Sprintf("backfilled %d user pay-token snapshots from pay_token_user_balances", changedCount))
	}
	return nil
}

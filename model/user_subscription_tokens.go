package model

import (
	"errors"
	"time"

	"one-api/common"
	relaycommon "one-api/relay/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetUserTokenSubscriptionGroupCandidates returns possible using-groups that can be billed from token-based subscriptions.
func GetUserTokenSubscriptionGroupCandidates(userId int) ([]int, bool, error) {
	if userId <= 0 {
		return nil, false, errors.New("userId 无效")
	}
	candidates := make([]int, 0)
	hasSubscription := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}
		now := time.Now().Unix()
		var subs []UserSubscription
		if err := tx.Select("id", "source_preset_id").
			Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
			Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) == 0 {
			return nil
		}
		hasSubscription = true
		allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
		if err != nil {
			return err
		}
		seen := make(map[int]struct{})
		for _, sub := range subs {
			for _, gid := range allowedBySubID[sub.Id] {
				if gid <= 0 {
					continue
				}
				if _, ok := seen[gid]; ok {
					continue
				}
				seen[gid] = struct{}{}
				candidates = append(candidates, gid)
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return normalizeUniqueSortedIDs(candidates), hasSubscription, nil
}

func GetUserTokenSubscriptionCapacityForGroup(userId int, groupID int) (totalRemaining int, dailyCapacity int, totalUnlimited bool, dailyUnlimited bool, err error) {
	if userId <= 0 {
		return 0, 0, false, false, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return 0, 0, false, false, errors.New("group_id 无效")
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}
		now := time.Now().Unix()
		var subs []UserSubscription
		if err := tx.Select("id", "billing_unit", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "source_preset_id", "source_redemption_id").
			Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
			Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) == 0 {
			return nil
		}
		allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
		if err != nil {
			return err
		}

		subIDsForUsage := make([]int, 0, len(subs))
		effectiveGroupDailyLimitsBySubID, effectiveSourceBySubID, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx, subs)
		if err != nil {
			return err
		}
		for _, sub := range subs {
			if len(effectiveGroupDailyLimitsBySubID[sub.Id]) > 0 {
				subIDsForUsage = append(subIDsForUsage, sub.Id)
			}
		}
		statDate := statDateToday()
		usageBySubID, err := getUserSubscriptionGroupDailyUsageMapTx(tx, subIDsForUsage, groupID, statDate)
		if err != nil {
			return err
		}

		today := common.GetTodayDateInt()
		for _, sub := range subs {
			allowed := false
			for _, gid := range allowedBySubID[sub.Id] {
				if gid == groupID {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}

			unlimitedTotal := sub.TotalQuota == 0
			if unlimitedTotal {
				totalUnlimited = true
			} else {
				if sub.RemainingQuota > 0 {
					totalRemaining += sub.RemainingQuota
				}
			}

			groupDailyLimits := effectiveGroupDailyLimitsBySubID[sub.Id]
			if len(groupDailyLimits) > 0 {
				limit, ok := groupDailyLimits[groupID]
				if !ok {
					return effectiveSourceBySubID[sub.Id].missingGroupDailyLimitError(groupID)
				}

				used := usageBySubID[sub.Id]
				if used < 0 {
					return errors.New("订阅额度日用量数据错误")
				}

				if limit <= 0 {
					if unlimitedTotal {
						dailyUnlimited = true
						continue
					}
					usable := sub.RemainingQuota
					if usable < 0 {
						usable = 0
					}
					dailyCapacity += usable
					continue
				}

				remainToday := limit - used
				if remainToday < 0 {
					remainToday = 0
				}
				if unlimitedTotal {
					dailyCapacity += remainToday
				} else {
					usable := sub.RemainingQuota
					if usable < 0 {
						usable = 0
					}
					if usable > remainToday {
						usable = remainToday
					}
					dailyCapacity += usable
				}
				continue
			}

			// Legacy daily limit mode (daily_quota_limit).
			if sub.DailyQuotaLimit <= 0 {
				if unlimitedTotal {
					dailyUnlimited = true
					continue
				}
				usable := sub.RemainingQuota
				if usable < 0 {
					usable = 0
				}
				dailyCapacity += usable
				continue
			}

			used := sub.DailyQuotaUsed
			if sub.DailyQuotaResetDate != today {
				used = 0
			}
			remainToday := sub.DailyQuotaLimit - used
			if remainToday < 0 {
				remainToday = 0
			}
			if unlimitedTotal {
				dailyCapacity += remainToday
			} else {
				usable := sub.RemainingQuota
				if usable < 0 {
					usable = 0
				}
				if usable > remainToday {
					usable = remainToday
				}
				dailyCapacity += usable
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, false, false, err
	}
	return totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, nil
}

func CanUserTokenSubscriptionConsumeGroup(userId int, groupID int, requiredTokens int) (bool, error) {
	if requiredTokens <= 0 {
		return true, nil
	}
	totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, err := GetUserTokenSubscriptionCapacityForGroup(userId, groupID)
	if err != nil {
		return false, err
	}
	if !totalUnlimited && totalRemaining < requiredTokens {
		return false, nil
	}
	if !dailyUnlimited && dailyCapacity < requiredTokens {
		return false, nil
	}
	return true, nil
}

func sumTokenSubscriptionRemainingByGroup(tx *gorm.DB, userId int, groupID int) (int, bool, error) {
	if tx == nil {
		tx = DB
	}
	if groupID <= 0 {
		return 0, false, errors.New("group_id 无效")
	}
	var subs []UserSubscription
	now := time.Now().Unix()
	if err := tx.Select("id", "total_quota", "remaining_quota", "source_preset_id", "source_redemption_id").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
		Find(&subs).Error; err != nil {
		return 0, false, err
	}
	allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
	if err != nil {
		return 0, false, err
	}
	total := 0
	hasUnlimited := false
	for _, sub := range subs {
		allowed := false
		for _, gid := range allowedBySubID[sub.Id] {
			if gid == groupID {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		if sub.TotalQuota == 0 {
			hasUnlimited = true
			continue
		}
		if sub.RemainingQuota > 0 {
			total += sub.RemainingQuota
		}
	}
	return total, hasUnlimited, nil
}

// consumeTokensFromSubscriptionsByGroup consumes token-based subscription quota restricted by allowed_group_ids.
//
// Returns:
// - covered: total tokens covered by subscriptions (finite + unlimited-total)
// - deducted: tokens deducted from finite subscriptions (affects users.tokens_quota)
func consumeTokensFromSubscriptionsByGroup(tx *gorm.DB, userId int, required int, groupID int) (covered int, deducted int, err error) {
	covered, deducted, _, err = consumeTokensFromSubscriptionsByGroupWithAllocations(tx, userId, required, groupID)
	return covered, deducted, err
}

func consumeTokensFromSubscriptionsByGroupWithAllocations(tx *gorm.DB, userId int, required int, groupID int) (covered int, deducted int, allocations []relaycommon.SubscriptionUnitAllocation, err error) {
	if required <= 0 {
		return 0, 0, nil, nil
	}
	if groupID <= 0 {
		return 0, 0, nil, errors.New("group_id 无效")
	}
	now := time.Now().Unix()
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "billing_unit", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "source_preset_id", "source_redemption_id").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
		Order("CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC").
		Find(&subs).Error; err != nil {
		return 0, 0, nil, err
	}
	if len(subs) == 0 {
		return 0, 0, nil, nil
	}
	allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
	if err != nil {
		return 0, 0, nil, err
	}

	subIDsForUsage := make([]int, 0, len(subs))
	effectiveGroupDailyLimitsBySubID, effectiveSourceBySubID, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx, subs)
	if err != nil {
		return 0, 0, nil, err
	}
	for _, sub := range subs {
		if len(effectiveGroupDailyLimitsBySubID[sub.Id]) > 0 {
			subIDsForUsage = append(subIDsForUsage, sub.Id)
		}
	}
	statDate := statDateToday()
	usageBySubID, err := getUserSubscriptionGroupDailyUsageMapTx(tx, subIDsForUsage, groupID, statDate)
	if err != nil {
		return 0, 0, nil, err
	}

	today := common.GetTodayDateInt()
	covered = 0
	deducted = 0
	left := required
	for _, sub := range subs {
		if left <= 0 {
			break
		}
		allowed := false
		for _, gid := range allowedBySubID[sub.Id] {
			if gid == groupID {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}

		unlimitedTotal := sub.TotalQuota == 0
		usable := sub.RemainingQuota
		if unlimitedTotal {
			usable = left
		} else if usable <= 0 {
			continue
		}

		groupDailyLimits := effectiveGroupDailyLimitsBySubID[sub.Id]
		useGroupDailyLimit := len(groupDailyLimits) > 0

		if useGroupDailyLimit {
			limit, ok := groupDailyLimits[groupID]
			if !ok {
				return covered, deducted, nil, effectiveSourceBySubID[sub.Id].missingGroupDailyLimitError(groupID)
			}

			used := usageBySubID[sub.Id]
			if used < 0 {
				return covered, deducted, nil, errors.New("订阅额度日用量数据错误")
			}
			if limit > 0 {
				remainToday := limit - used
				if remainToday <= 0 {
					continue
				}
				if usable > remainToday {
					usable = remainToday
				}
			}
		} else if sub.DailyQuotaLimit > 0 {
			used := sub.DailyQuotaUsed
			reset := sub.DailyQuotaResetDate
			if reset != today {
				used = 0
				reset = today
			}
			remainToday := sub.DailyQuotaLimit - used
			if remainToday <= 0 {
				if reset != sub.DailyQuotaResetDate {
					if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(map[string]interface{}{
						"daily_quota_used":       used,
						"daily_quota_reset_date": reset,
					}).Error; err != nil {
						return covered, deducted, nil, err
					}
				}
				continue
			}
			if usable > remainToday {
				usable = remainToday
			}
		}

		if usable > left {
			usable = left
		}
		if usable <= 0 {
			continue
		}

		updates := map[string]interface{}{}
		if !unlimitedTotal {
			afterRemaining := sub.RemainingQuota - usable
			updates["remaining_quota"] = gorm.Expr("remaining_quota - ?", usable)
			updates["invalid_at"] = 0
			if afterRemaining <= 0 {
				updates["invalid_at"] = now
			}
		}

		if useGroupDailyLimit {
			if err := incrUserSubscriptionGroupDailyUsageTx(tx, sub.Id, groupID, statDate, usable); err != nil {
				return covered, deducted, nil, err
			}
			if sub.DailyQuotaLimit > 0 {
				used := sub.DailyQuotaUsed
				reset := sub.DailyQuotaResetDate
				if reset != today {
					used = 0
					reset = today
				}
				updates["daily_quota_used"] = used + usable
				updates["daily_quota_reset_date"] = reset
			}
		} else if sub.DailyQuotaLimit > 0 || unlimitedTotal {
			used := sub.DailyQuotaUsed
			reset := sub.DailyQuotaResetDate
			if reset != today {
				used = 0
				reset = today
			}
			updates["daily_quota_used"] = used + usable
			updates["daily_quota_reset_date"] = reset
		}

		if len(updates) > 0 {
			if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
				return covered, deducted, nil, err
			}
		}
		allocations = append(allocations, relaycommon.SubscriptionUnitAllocation{
			SubscriptionId:      sub.Id,
			GroupId:             groupID,
			StatDate:            statDate,
			Amount:              usable,
			UsesGroupDailyLimit: useGroupDailyLimit,
		})
		covered += usable
		if !unlimitedTotal {
			deducted += usable
		}
		left -= usable
	}

	return covered, deducted, allocations, nil
}

// restoreTokensToSubscriptionsByGroup restores token-based subscription quota back into subscriptions that allow the given group_id.
//
// Returns:
// - restored: total restored tokens (finite + unlimited-total usage rollback)
// - restoredDeducted: restored tokens that should be credited back to users.tokens_quota (finite subscriptions)
func restoreTokensToSubscriptionsByGroup(tx *gorm.DB, userId int, quota int, groupID int) (restored int, restoredDeducted int, err error) {
	if quota <= 0 {
		return 0, 0, nil
	}
	if groupID <= 0 {
		return 0, 0, errors.New("group_id 无效")
	}
	now := time.Now().Unix()
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "billing_unit", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "source_preset_id", "source_redemption_id", "start_at", "expire_at").
		Where("user_id = ? AND credited = ? AND ((total_quota > 0 AND remaining_quota < total_quota) OR total_quota = 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
		Order("CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC").
		Find(&subs).Error; err != nil {
		return 0, 0, err
	}
	if len(subs) == 0 {
		return 0, 0, nil
	}
	allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
	if err != nil {
		return 0, 0, err
	}

	subIDsForUsage := make([]int, 0, len(subs))
	effectiveGroupDailyLimitsBySubID, effectiveSourceBySubID, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx, subs)
	if err != nil {
		return 0, 0, err
	}
	for _, sub := range subs {
		if len(effectiveGroupDailyLimitsBySubID[sub.Id]) > 0 {
			subIDsForUsage = append(subIDsForUsage, sub.Id)
		}
	}
	statDate := statDateToday()
	usageBySubID, err := getUserSubscriptionGroupDailyUsageMapTx(tx, subIDsForUsage, groupID, statDate)
	if err != nil {
		return 0, 0, err
	}

	left := quota
	today := common.GetTodayDateInt()
	restored = 0
	restoredDeducted = 0
	for _, sub := range subs {
		if left <= 0 {
			break
		}
		allowed := false
		for _, gid := range allowedBySubID[sub.Id] {
			if gid == groupID {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}

		unlimitedTotal := sub.TotalQuota == 0

		groupDailyLimits := effectiveGroupDailyLimitsBySubID[sub.Id]
		useGroupDailyLimit := len(groupDailyLimits) > 0

		// Determine how much can be restored for this subscription.
		add := 0
		space := sub.TotalQuota - sub.RemainingQuota
		if space < 0 {
			space = 0
		}

		if useGroupDailyLimit {
			if _, ok := groupDailyLimits[groupID]; !ok {
				return restored, restoredDeducted, effectiveSourceBySubID[sub.Id].missingGroupDailyLimitError(groupID)
			}
			used := usageBySubID[sub.Id]
			if used < 0 {
				return restored, restoredDeducted, errors.New("订阅额度日用量数据错误")
			}
			add = used
			if !unlimitedTotal && add > space {
				add = space
			}
		} else if unlimitedTotal {
			used := sub.DailyQuotaUsed
			reset := sub.DailyQuotaResetDate
			if reset != today {
				used = 0
			}
			add = used
		} else {
			add = space
		}

		if add > left {
			add = left
		}
		if add <= 0 {
			continue
		}

		updates := map[string]interface{}{}
		if !unlimitedTotal {
			updates["remaining_quota"] = gorm.Expr("remaining_quota + ?", add)
			updates["invalid_at"] = 0
		}

		if useGroupDailyLimit {
			if err := decrUserSubscriptionGroupDailyUsageTx(tx, sub.Id, groupID, statDate, add); err != nil {
				return restored, restoredDeducted, err
			}
			// Keep legacy aggregate fields in sync (see consumeQuotaFromSubscriptionsByGroup).
			if sub.DailyQuotaLimit > 0 {
				used := sub.DailyQuotaUsed
				reset := sub.DailyQuotaResetDate
				if reset != today {
					used = 0
					reset = today
				}
				used -= add
				if used < 0 {
					used = 0
				}
				updates["daily_quota_used"] = used
				updates["daily_quota_reset_date"] = reset
			}
		} else if sub.DailyQuotaLimit > 0 || unlimitedTotal {
			used := sub.DailyQuotaUsed
			reset := sub.DailyQuotaResetDate
			if reset != today {
				used = 0
				reset = today
			}
			used -= add
			if used < 0 {
				used = 0
			}
			updates["daily_quota_used"] = used
			updates["daily_quota_reset_date"] = reset
		}

		if len(updates) > 0 {
			if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
				return restored, restoredDeducted, err
			}
		}
		restored += add
		if !unlimitedTotal {
			restoredDeducted += add
		}
		left -= add
	}

	return restored, restoredDeducted, nil
}

func restoreTokensToSubscriptionsWithAllocations(tx *gorm.DB, userId int, allocations []relaycommon.SubscriptionUnitAllocation) (restored int, restoredDeducted int, err error) {
	if len(allocations) == 0 {
		return 0, 0, nil
	}
	subIDs := make([]int, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.SubscriptionId <= 0 || allocation.Amount <= 0 {
			continue
		}
		subIDs = append(subIDs, allocation.SubscriptionId)
	}
	subIDs = normalizeUniqueSortedIDs(subIDs)
	if len(subIDs) == 0 {
		return 0, 0, nil
	}
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "billing_unit", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "credited", "start_at", "expire_at").
		Where("user_id = ? AND id IN ? AND billing_unit = ?", userId, subIDs, UserSubscriptionBillingUnitTokens).
		Find(&subs).Error; err != nil {
		return 0, 0, err
	}
	subByID := make(map[int]*UserSubscription, len(subs))
	for i := range subs {
		subByID[subs[i].Id] = &subs[i]
	}
	today := common.GetTodayDateInt()
	for _, allocation := range allocations {
		if allocation.SubscriptionId <= 0 || allocation.Amount <= 0 {
			continue
		}
		sub := subByID[allocation.SubscriptionId]
		if sub == nil {
			return restored, restoredDeducted, errors.New("订阅tokens归还目标不存在")
		}

		add := allocation.Amount
		unlimitedTotal := sub.TotalQuota == 0
		updates := map[string]interface{}{}
		if !unlimitedTotal {
			space := sub.TotalQuota - sub.RemainingQuota
			if space < add {
				return restored, restoredDeducted, errors.New("订阅tokens可归还额度不足")
			}
			updates["remaining_quota"] = gorm.Expr("remaining_quota + ?", add)
			updates["invalid_at"] = 0
			sub.RemainingQuota += add
		}

		if allocation.UsesGroupDailyLimit {
			if allocation.GroupId <= 0 {
				return restored, restoredDeducted, errors.New("group_id 无效")
			}
			statDate := allocation.StatDate
			if statDate <= 0 {
				statDate = statDateToday()
			}
			if err := decrUserSubscriptionGroupDailyUsageTx(tx, sub.Id, allocation.GroupId, statDate, add); err != nil {
				return restored, restoredDeducted, err
			}
			if sub.DailyQuotaLimit > 0 {
				used := sub.DailyQuotaUsed
				reset := sub.DailyQuotaResetDate
				if reset != today {
					used = 0
					reset = today
				}
				used -= add
				if used < 0 {
					used = 0
				}
				updates["daily_quota_used"] = used
				updates["daily_quota_reset_date"] = reset
				sub.DailyQuotaUsed = used
				sub.DailyQuotaResetDate = reset
			}
		} else if sub.DailyQuotaLimit > 0 || unlimitedTotal {
			used := sub.DailyQuotaUsed
			reset := sub.DailyQuotaResetDate
			if reset != today {
				used = 0
				reset = today
			}
			used -= add
			if used < 0 {
				used = 0
			}
			updates["daily_quota_used"] = used
			updates["daily_quota_reset_date"] = reset
			sub.DailyQuotaUsed = used
			sub.DailyQuotaResetDate = reset
		}

		if len(updates) > 0 {
			if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
				return restored, restoredDeducted, err
			}
		}
		restored += add
		if !unlimitedTotal {
			restoredDeducted += add
		}
	}
	return restored, restoredDeducted, nil
}

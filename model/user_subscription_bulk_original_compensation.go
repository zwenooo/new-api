package model

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"one-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BulkExtendOriginalSubscriptionsParams struct {
	FaultStartAt      int64
	FaultEndAt        int64
	SourcePresetIDs   []int
	ExcludedUserIDs   []int
	ExcludedUsernames []string
	ExtendDays        int
	DryRun            bool
}

type BulkExtendOriginalSubscriptionsResult struct {
	FaultStartAt      int64    `json:"fault_start_at"`
	FaultEndAt        int64    `json:"fault_end_at"`
	SourcePresetIDs   []int    `json:"source_preset_ids"`
	ExcludedUserIDs   []int    `json:"excluded_user_ids,omitempty"`
	ExcludedUsernames []string `json:"excluded_usernames,omitempty"`
	ExtendDays        int      `json:"extend_days"`
	DryRun            bool     `json:"dry_run"`

	ResolvedExcludedUserCount          int   `json:"resolved_excluded_user_count"`
	ResolvedExcludedUserIDsPreview     []int `json:"resolved_excluded_user_ids_preview,omitempty"`
	ResolvedExcludedUserIDsPreviewMore bool  `json:"resolved_excluded_user_ids_preview_more,omitempty"`
	ExcludedMatchedUserCount           int   `json:"excluded_matched_user_count"`
	MatchedUserCount                   int   `json:"matched_user_count"`
	MatchedUserIDsPreview              []int `json:"matched_user_ids_preview,omitempty"`
	MatchedUserIDsPreviewMore          bool  `json:"matched_user_ids_preview_more,omitempty"`

	MatchedQuotaSubscriptionCount   int64 `json:"matched_quota_subscription_count"`
	MatchedTokenSubscriptionCount   int64 `json:"matched_token_subscription_count"`
	MatchedRequestSubscriptionCount int64 `json:"matched_request_subscription_count"`

	MatchedQuotaCompensationAmount   int64   `json:"matched_quota_compensation_amount"`
	MatchedTokenCompensationAmount   float64 `json:"matched_token_compensation_amount"`
	MatchedRequestCompensationAmount float64 `json:"matched_request_compensation_amount"`

	ExtendedQuotaSubscriptionCount   int64 `json:"extended_quota_subscription_count"`
	ExtendedTokenSubscriptionCount   int64 `json:"extended_token_subscription_count"`
	ExtendedRequestSubscriptionCount int64 `json:"extended_request_subscription_count"`
}

type bulkExtendOriginalUserSubscriptionTarget struct {
	Id              int    `gorm:"column:id"`
	UserId          int    `gorm:"column:user_id"`
	BillingUnit     string `gorm:"column:billing_unit"`
	DailyQuotaLimit int    `gorm:"column:daily_quota_limit"`
}

type bulkExtendOriginalUserRequestSubscriptionTarget struct {
	Id                int `gorm:"column:id"`
	UserId            int `gorm:"column:user_id"`
	DailyRequestLimit int `gorm:"column:daily_request_limit"`
}

func validateBulkExtendOriginalSubscriptionsParams(params *BulkExtendOriginalSubscriptionsParams) error {
	if params == nil {
		return errors.New("params 为空")
	}
	if params.FaultStartAt <= 0 {
		return errors.New("fault_start_at 必须大于 0")
	}
	if params.FaultEndAt <= 0 {
		return errors.New("fault_end_at 必须大于 0")
	}
	if params.FaultStartAt > common.MaxSupportedUnixTimestamp {
		return errors.New("fault_start_at 过大，最大支持到 " + common.MaxSupportedUnixTimestampLabel)
	}
	if params.FaultEndAt > common.MaxSupportedUnixTimestamp {
		return errors.New("fault_end_at 过大，最大支持到 " + common.MaxSupportedUnixTimestampLabel)
	}
	if params.FaultStartAt > params.FaultEndAt {
		return errors.New("fault_start_at 不能晚于 fault_end_at")
	}

	sourcePresetIDs := normalizeUniqueSortedIDs(params.SourcePresetIDs)
	if len(sourcePresetIDs) == 0 {
		return errors.New("source_preset_ids 不能为空")
	}
	params.SourcePresetIDs = sourcePresetIDs
	params.ExcludedUserIDs = normalizeUniqueSortedIDs(params.ExcludedUserIDs)
	params.ExcludedUsernames = normalizeUniqueSortedStrings(params.ExcludedUsernames)

	if params.ExtendDays <= 0 {
		return errors.New("extend_days 必须大于 0")
	}
	if params.ExtendDays > 3650 {
		return errors.New("extend_days 过大")
	}

	return nil
}

func safeMultiplyInt64(a int64, b int64, errMsg string) (int64, error) {
	if a < 0 || b < 0 {
		return 0, errors.New(errMsg)
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > common.MaxSupportedUnixTimestamp/b {
		return 0, errors.New(errMsg)
	}
	return a * b, nil
}

func safeMultiplyPositiveInt(a int, b int, errMsg string) (int, error) {
	if a <= 0 || b <= 0 {
		return 0, errors.New(errMsg)
	}
	maxIntValue := int(^uint(0) >> 1)
	if a > maxIntValue/b {
		return 0, errors.New(errMsg)
	}
	return a * b, nil
}

func safeAddPositiveInt(a int, b int, errMsg string) (int, error) {
	if a < 0 || b < 0 {
		return 0, errors.New(errMsg)
	}
	maxIntValue := int(^uint(0) >> 1)
	if a > maxIntValue-b {
		return 0, errors.New(errMsg)
	}
	return a + b, nil
}

func safeAddInt64(a int64, b int64, errMsg string) (int64, error) {
	if b < 0 {
		return 0, errors.New(errMsg)
	}
	if a > common.MaxSupportedUnixTimestamp-b {
		return 0, errors.New(errMsg)
	}
	return a + b, nil
}

func collectBulkExtendOriginalExcludedMatchedUsersTx(tx *gorm.DB, params BulkExtendOriginalSubscriptionsParams, excludedUserIDs []int) (int, error) {
	if tx == nil {
		tx = DB
	}
	if len(excludedUserIDs) == 0 {
		return 0, nil
	}

	userSet := make(map[int]struct{}, len(excludedUserIDs))

	var quotaRows []struct {
		UserId int `gorm:"column:user_id"`
	}
	if err := tx.Model(&UserSubscription{}).
		Select("DISTINCT user_id").
		Where("user_id IN ?", excludedUserIDs).
		Where("source_preset_id IN ?", params.SourcePresetIDs).
		Where("(start_at = 0 OR start_at <= ?)", params.FaultEndAt).
		Where("expire_at > 0 AND expire_at >= ?", params.FaultStartAt).
		Where("(invalid_at = 0 OR invalid_at >= ?)", params.FaultStartAt).
		Where("daily_quota_limit > 0").
		Where("total_quota > 0").
		Find(&quotaRows).Error; err != nil {
		return 0, err
	}
	for _, row := range quotaRows {
		if row.UserId > 0 {
			userSet[row.UserId] = struct{}{}
		}
	}

	var requestRows []struct {
		UserId int `gorm:"column:user_id"`
	}
	if err := tx.Model(&UserRequestSubscription{}).
		Select("DISTINCT user_id").
		Where("user_id IN ?", excludedUserIDs).
		Where("source_preset_id IN ?", params.SourcePresetIDs).
		Where("(start_at = 0 OR start_at <= ?)", params.FaultEndAt).
		Where("expire_at > 0 AND expire_at >= ?", params.FaultStartAt).
		Where("(invalid_at = 0 OR invalid_at >= ?)", params.FaultStartAt).
		Where("daily_request_limit > 0").
		Where("total_request_limit > 0").
		Find(&requestRows).Error; err != nil {
		return 0, err
	}
	for _, row := range requestRows {
		if row.UserId > 0 {
			userSet[row.UserId] = struct{}{}
		}
	}

	return len(userSet), nil
}

func collectBulkExtendOriginalSubscriptionTargetsTx(tx *gorm.DB, params BulkExtendOriginalSubscriptionsParams, excludedUserIDs []int, result *BulkExtendOriginalSubscriptionsResult) ([]bulkExtendOriginalUserSubscriptionTarget, []bulkExtendOriginalUserSubscriptionTarget, []bulkExtendOriginalUserRequestSubscriptionTarget, []int, error) {
	if tx == nil {
		tx = DB
	}
	if result == nil {
		return nil, nil, nil, nil, errors.New("result 为空")
	}

	var quotaAndTokenRows []bulkExtendOriginalUserSubscriptionTarget
	if err := buildUserExclusionQuery(
		tx.Model(&UserSubscription{}).
			Select("id", "user_id", "billing_unit", "daily_quota_limit").
			Where("source_preset_id IN ?", params.SourcePresetIDs).
			Where("(start_at = 0 OR start_at <= ?)", params.FaultEndAt).
			Where("expire_at > 0 AND expire_at >= ?", params.FaultStartAt).
			Where("(invalid_at = 0 OR invalid_at >= ?)", params.FaultStartAt).
			Where("daily_quota_limit > 0").
			Where("total_quota > 0").
			Order("user_id ASC, id ASC"),
		excludedUserIDs,
	).Find(&quotaAndTokenRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}

	quotaTargets := make([]bulkExtendOriginalUserSubscriptionTarget, 0, len(quotaAndTokenRows))
	tokenTargets := make([]bulkExtendOriginalUserSubscriptionTarget, 0, len(quotaAndTokenRows))
	userIDSet := make(map[int]struct{}, len(quotaAndTokenRows))
	for _, row := range quotaAndTokenRows {
		unit, err := normalizeUserSubscriptionBillingUnit(row.BillingUnit)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		row.BillingUnit = unit
		switch unit {
		case UserSubscriptionBillingUnitQuota:
			quotaTargets = append(quotaTargets, row)
			result.MatchedQuotaSubscriptionCount++
			result.MatchedQuotaCompensationAmount += int64(row.DailyQuotaLimit) * int64(params.ExtendDays)
		case UserSubscriptionBillingUnitTokens:
			tokenTargets = append(tokenTargets, row)
			result.MatchedTokenSubscriptionCount++
			result.MatchedTokenCompensationAmount += discreteUnitsToDisplay(row.DailyQuotaLimit) * float64(params.ExtendDays)
		default:
			return nil, nil, nil, nil, errors.New("billing_unit 无效")
		}
		if row.UserId > 0 {
			userIDSet[row.UserId] = struct{}{}
		}
	}

	var requestTargets []bulkExtendOriginalUserRequestSubscriptionTarget
	if err := buildUserExclusionQuery(
		tx.Model(&UserRequestSubscription{}).
			Select("id", "user_id", "daily_request_limit").
			Where("source_preset_id IN ?", params.SourcePresetIDs).
			Where("(start_at = 0 OR start_at <= ?)", params.FaultEndAt).
			Where("expire_at > 0 AND expire_at >= ?", params.FaultStartAt).
			Where("(invalid_at = 0 OR invalid_at >= ?)", params.FaultStartAt).
			Where("daily_request_limit > 0").
			Where("total_request_limit > 0").
			Order("user_id ASC, id ASC"),
		excludedUserIDs,
	).Find(&requestTargets).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	for _, row := range requestTargets {
		result.MatchedRequestSubscriptionCount++
		result.MatchedRequestCompensationAmount += discreteUnitsToDisplay(row.DailyRequestLimit) * float64(params.ExtendDays)
		if row.UserId > 0 {
			userIDSet[row.UserId] = struct{}{}
		}
	}

	matchedUserIDs := make([]int, 0, len(userIDSet))
	for userID := range userIDSet {
		matchedUserIDs = append(matchedUserIDs, userID)
	}
	sort.Ints(matchedUserIDs)
	result.MatchedUserCount = len(matchedUserIDs)
	if len(matchedUserIDs) > bulkSubscriptionCompensationPreviewLimit {
		result.MatchedUserIDsPreview = append([]int(nil), matchedUserIDs[:bulkSubscriptionCompensationPreviewLimit]...)
		result.MatchedUserIDsPreviewMore = true
	} else {
		result.MatchedUserIDsPreview = append([]int(nil), matchedUserIDs...)
	}

	return quotaTargets, tokenTargets, requestTargets, matchedUserIDs, nil
}

func extendOriginalUserSubscriptionTx(tx *gorm.DB, userID int, subID int, sourcePresetIDs []int, extendDays int) (string, int, error) {
	if tx == nil {
		return "", 0, errors.New("tx 为空")
	}
	if userID <= 0 {
		return "", 0, errors.New("userId 无效")
	}
	if subID <= 0 {
		return "", 0, errors.New("subId 无效")
	}
	if extendDays <= 0 {
		return "", 0, errors.New("extend_days 无效")
	}

	deltaSeconds, err := safeMultiplyInt64(int64(extendDays), common.SecondsPerDay, "extend_days 过大")
	if err != nil {
		return "", 0, err
	}

	var sub UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", subID, userID).
		First(&sub).Error; err != nil {
		return "", 0, err
	}

	if len(sourcePresetIDs) > 0 {
		found := false
		for _, presetID := range sourcePresetIDs {
			if sub.SourcePresetId == presetID {
				found = true
				break
			}
		}
		if !found {
			return "", 0, errors.New("订阅来源商品已变化")
		}
	}

	billingUnit, err := normalizeUserSubscriptionBillingUnit(sub.BillingUnit)
	if err != nil {
		return "", 0, err
	}
	if sub.DailyQuotaLimit <= 0 {
		return "", 0, errors.New("订阅不是日限类型")
	}
	if sub.TotalQuota <= 0 {
		return "", 0, errors.New("订阅总额度不是有限值")
	}
	if sub.ExpireAt <= 0 {
		return "", 0, errors.New("订阅有效期不是有限值")
	}

	addQuota, err := safeMultiplyPositiveInt(sub.DailyQuotaLimit, extendDays, "补偿额度过大")
	if err != nil {
		return "", 0, err
	}
	newTotalQuota, err := safeAddPositiveInt(sub.TotalQuota, addQuota, "补偿后总额度过大")
	if err != nil {
		return "", 0, err
	}
	newRemainingQuota, err := safeAddPositiveInt(sub.RemainingQuota, addQuota, "补偿后剩余额度过大")
	if err != nil {
		return "", 0, err
	}
	newExpireAt, err := safeAddInt64(sub.ExpireAt, deltaSeconds, "补偿后过期时间过大")
	if err != nil {
		return "", 0, err
	}

	now := time.Now().Unix()
	shouldBeCredited := (sub.StartAt == 0 || sub.StartAt <= now) && newExpireAt >= now
	oldCreditedAmount := 0
	if sub.Credited && sub.RemainingQuota > 0 {
		oldCreditedAmount = sub.RemainingQuota
	}
	newCreditedAmount := 0
	if shouldBeCredited && newRemainingQuota > 0 {
		newCreditedAmount = newRemainingQuota
	}

	updates := map[string]any{
		"total_quota":     newTotalQuota,
		"remaining_quota": newRemainingQuota,
		"expire_at":       newExpireAt,
		"invalid_at":      0,
	}
	if shouldBeCredited != sub.Credited {
		updates["credited"] = shouldBeCredited
	}

	if err := tx.Model(&UserSubscription{}).
		Where("id = ? AND user_id = ?", subID, userID).
		Updates(updates).Error; err != nil {
		return "", 0, err
	}

	deltaQuota := newCreditedAmount - oldCreditedAmount
	if deltaQuota != 0 {
		if err := applyUserSubscriptionBalanceDeltaTx(tx, userID, billingUnit, deltaQuota); err != nil {
			return "", 0, err
		}
	}

	return billingUnit, addQuota, nil
}

func extendOriginalUserRequestSubscriptionTx(tx *gorm.DB, userID int, subID int, sourcePresetIDs []int, extendDays int) (int, error) {
	if tx == nil {
		return 0, errors.New("tx 为空")
	}
	if userID <= 0 {
		return 0, errors.New("userId 无效")
	}
	if subID <= 0 {
		return 0, errors.New("subId 无效")
	}
	if extendDays <= 0 {
		return 0, errors.New("extend_days 无效")
	}

	deltaSeconds, err := safeMultiplyInt64(int64(extendDays), common.SecondsPerDay, "extend_days 过大")
	if err != nil {
		return 0, err
	}

	var sub UserRequestSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", subID, userID).
		First(&sub).Error; err != nil {
		return 0, err
	}

	if len(sourcePresetIDs) > 0 {
		found := false
		for _, presetID := range sourcePresetIDs {
			if sub.SourcePresetId == presetID {
				found = true
				break
			}
		}
		if !found {
			return 0, errors.New("订阅来源商品已变化")
		}
	}

	if sub.DailyRequestLimit <= 0 {
		return 0, errors.New("订阅不是日限类型")
	}
	if sub.TotalRequestLimit <= 0 {
		return 0, errors.New("订阅总次数不是有限值")
	}
	if sub.ExpireAt <= 0 {
		return 0, errors.New("订阅有效期不是有限值")
	}

	addRequests, err := safeMultiplyPositiveInt(sub.DailyRequestLimit, extendDays, "补偿后总次数过大")
	if err != nil {
		return 0, err
	}
	newTotalRequestLimit, err := safeAddPositiveInt(sub.TotalRequestLimit, addRequests, "补偿后总次数过大")
	if err != nil {
		return 0, err
	}
	newExpireAt, err := safeAddInt64(sub.ExpireAt, deltaSeconds, "补偿后过期时间过大")
	if err != nil {
		return 0, err
	}

	updates := map[string]any{
		"total_request_limit": newTotalRequestLimit,
		"expire_at":           newExpireAt,
	}
	if err := tx.Model(&UserRequestSubscription{}).
		Where("id = ? AND user_id = ?", subID, userID).
		Updates(updates).Error; err != nil {
		return 0, err
	}

	return addRequests, nil
}

func BulkExtendOriginalSubscriptions(params BulkExtendOriginalSubscriptionsParams) (*BulkExtendOriginalSubscriptionsResult, error) {
	if err := validateBulkExtendOriginalSubscriptionsParams(&params); err != nil {
		return nil, err
	}
	if err := ensureBulkCompensationSourcePresetsTx(nil, params.SourcePresetIDs); err != nil {
		return nil, err
	}

	result := &BulkExtendOriginalSubscriptionsResult{
		FaultStartAt:      params.FaultStartAt,
		FaultEndAt:        params.FaultEndAt,
		SourcePresetIDs:   append([]int(nil), params.SourcePresetIDs...),
		ExcludedUserIDs:   append([]int(nil), params.ExcludedUserIDs...),
		ExcludedUsernames: append([]string(nil), params.ExcludedUsernames...),
		ExtendDays:        params.ExtendDays,
		DryRun:            params.DryRun,
	}

	var matchedUserIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		excludedUserIDs, err := resolveBulkCompensationExcludedUserIDsTx(tx, BulkCompensateSubscriptionsByPresetParams{
			ExcludedUserIDs:   params.ExcludedUserIDs,
			ExcludedUsernames: params.ExcludedUsernames,
		})
		if err != nil {
			return err
		}
		result.ResolvedExcludedUserCount = len(excludedUserIDs)
		if len(excludedUserIDs) > bulkSubscriptionCompensationPreviewLimit {
			result.ResolvedExcludedUserIDsPreview = append([]int(nil), excludedUserIDs[:bulkSubscriptionCompensationPreviewLimit]...)
			result.ResolvedExcludedUserIDsPreviewMore = true
		} else {
			result.ResolvedExcludedUserIDsPreview = append([]int(nil), excludedUserIDs...)
		}

		excludedMatchedUserCount, err := collectBulkExtendOriginalExcludedMatchedUsersTx(tx, params, excludedUserIDs)
		if err != nil {
			return err
		}
		result.ExcludedMatchedUserCount = excludedMatchedUserCount

		quotaTargets, tokenTargets, requestTargets, userIDs, err := collectBulkExtendOriginalSubscriptionTargetsTx(tx, params, excludedUserIDs, result)
		if err != nil {
			return err
		}
		matchedUserIDs = userIDs
		if params.DryRun || (len(quotaTargets) == 0 && len(tokenTargets) == 0 && len(requestTargets) == 0) {
			return nil
		}

		quotaTouchedUsers := make(map[int]struct{}, len(matchedUserIDs))

		for _, target := range quotaTargets {
			if _, _, err := extendOriginalUserSubscriptionTx(tx, target.UserId, target.Id, params.SourcePresetIDs, params.ExtendDays); err != nil {
				return fmt.Errorf("额度订阅 #%d 补偿失败: %w", target.Id, err)
			}
			result.ExtendedQuotaSubscriptionCount++
			quotaTouchedUsers[target.UserId] = struct{}{}
		}
		for _, target := range tokenTargets {
			if _, _, err := extendOriginalUserSubscriptionTx(tx, target.UserId, target.Id, params.SourcePresetIDs, params.ExtendDays); err != nil {
				return fmt.Errorf("Token订阅 #%d 补偿失败: %w", target.Id, err)
			}
			result.ExtendedTokenSubscriptionCount++
			quotaTouchedUsers[target.UserId] = struct{}{}
		}
		for _, userID := range matchedUserIDs {
			if _, ok := quotaTouchedUsers[userID]; !ok {
				continue
			}
			if err := refreshUserSubscriptionSnapshot(tx, userID, time.Now().Unix()); err != nil {
				return err
			}
		}
		for _, target := range requestTargets {
			if _, err := extendOriginalUserRequestSubscriptionTx(tx, target.UserId, target.Id, params.SourcePresetIDs, params.ExtendDays); err != nil {
				return fmt.Errorf("次数订阅 #%d 补偿失败: %w", target.Id, err)
			}
			result.ExtendedRequestSubscriptionCount++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if !params.DryRun && len(matchedUserIDs) > 0 {
		for _, userID := range matchedUserIDs {
			if err := invalidateUserCache(userID); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

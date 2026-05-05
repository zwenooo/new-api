package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"one-api/common"

	"gorm.io/gorm"
)

const bulkSubscriptionCompensationPreviewLimit = 100

type BulkCompensateSubscriptionsByPresetParams struct {
	FaultStartAt         int64
	FaultEndAt           int64
	SourcePresetIDs      []int
	ExcludedUserIDs      []int
	ExcludedUsernames    []string
	CompensationPresetID int
	ApplyMode            string
	Quantity             int
	DryRun               bool
}

type BulkCompensateSubscriptionsByPresetResult struct {
	FaultStartAt           int64    `json:"fault_start_at"`
	FaultEndAt             int64    `json:"fault_end_at"`
	SourcePresetIDs        []int    `json:"source_preset_ids"`
	ExcludedUserIDs        []int    `json:"excluded_user_ids,omitempty"`
	ExcludedUsernames      []string `json:"excluded_usernames,omitempty"`
	CompensationPresetID   int      `json:"compensation_preset_id"`
	CompensationPresetName string   `json:"compensation_preset_name"`
	CompensationPresetMode string   `json:"compensation_preset_mode"`
	ApplyMode              string   `json:"apply_mode"`
	Quantity               int      `json:"quantity"`
	DryRun                 bool     `json:"dry_run"`

	ResolvedExcludedUserCount          int   `json:"resolved_excluded_user_count"`
	ResolvedExcludedUserIDsPreview     []int `json:"resolved_excluded_user_ids_preview,omitempty"`
	ResolvedExcludedUserIDsPreviewMore bool  `json:"resolved_excluded_user_ids_preview_more,omitempty"`
	ExcludedMatchedUserCount           int   `json:"excluded_matched_user_count"`
	MatchedUserCount                   int   `json:"matched_user_count"`
	MatchedUserIDsPreview              []int `json:"matched_user_ids_preview,omitempty"`
	MatchedUserIDsPreviewMore          bool  `json:"matched_user_ids_preview_more,omitempty"`
	SubscriptionMatchedCount           int64 `json:"subscription_matched_count"`
	RequestSubscriptionMatchedCount    int64 `json:"request_subscription_matched_count"`

	CompensatedUserCount                int   `json:"compensated_user_count"`
	CreatedUserSubscriptionCount        int64 `json:"created_user_subscription_count"`
	CreatedUserRequestSubscriptionCount int64 `json:"created_user_request_subscription_count"`
}

func validateBulkCompensateSubscriptionsByPresetParams(params *BulkCompensateSubscriptionsByPresetParams) error {
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

	if params.CompensationPresetID <= 0 {
		return errors.New("compensation_preset_id 无效")
	}

	applyMode := strings.TrimSpace(params.ApplyMode)
	if applyMode == "" {
		applyMode = SubscriptionApplyModeStack
	}
	if applyMode != SubscriptionApplyModeStack && applyMode != SubscriptionApplyModeDefer {
		return errors.New("apply_mode 无效")
	}
	params.ApplyMode = applyMode

	if params.Quantity <= 0 {
		params.Quantity = 1
	}
	if params.Quantity > 100 {
		return errors.New("quantity 过大")
	}

	return nil
}

func normalizeUniqueSortedStrings(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func ensureBulkCompensationSourcePresetsTx(tx *gorm.DB, presetIDs []int) error {
	if tx == nil {
		tx = DB
	}
	ids := normalizeUniqueSortedIDs(presetIDs)
	if len(ids) == 0 {
		return errors.New("source_preset_ids 不能为空")
	}

	type row struct {
		Id   int    `gorm:"column:id"`
		Mode string `gorm:"column:mode"`
	}
	var rows []row
	if err := tx.Model(&RedemptionPreset{}).
		Select("id", "mode").
		Where("id IN ?", ids).
		Find(&rows).Error; err != nil {
		return err
	}

	if len(rows) != len(ids) {
		found := make(map[int]struct{}, len(rows))
		for _, row := range rows {
			if row.Id > 0 {
				found[row.Id] = struct{}{}
			}
		}
		for _, id := range ids {
			if _, ok := found[id]; !ok {
				return fmt.Errorf("筛选商品 #%d 不存在", id)
			}
		}
	}

	for _, row := range rows {
		switch strings.TrimSpace(row.Mode) {
		case "subscription", "tokens", "request":
		default:
			return fmt.Errorf("筛选商品 #%d 类型不支持批量补偿", row.Id)
		}
	}

	return nil
}

func buildBulkCompensationSourceString(compensationPresetID int, faultStartAt int64, faultEndAt int64) string {
	return fmt.Sprintf("admin_bulk_compensation:preset=%d,window=%d-%d", compensationPresetID, faultStartAt, faultEndAt)
}

func buildUserExclusionQuery(base *gorm.DB, excludedUserIDs []int) *gorm.DB {
	if base == nil || len(excludedUserIDs) == 0 {
		return base
	}
	return base.Where("user_id NOT IN ?", excludedUserIDs)
}

func fillBulkCompensationExcludedPreview(result *BulkCompensateSubscriptionsByPresetResult, excludedUserIDs []int) {
	if result == nil {
		return
	}
	result.ResolvedExcludedUserCount = len(excludedUserIDs)
	if len(excludedUserIDs) > bulkSubscriptionCompensationPreviewLimit {
		result.ResolvedExcludedUserIDsPreview = append([]int(nil), excludedUserIDs[:bulkSubscriptionCompensationPreviewLimit]...)
		result.ResolvedExcludedUserIDsPreviewMore = true
		return
	}
	result.ResolvedExcludedUserIDsPreview = append([]int(nil), excludedUserIDs...)
}

func resolveBulkCompensationExcludedUserIDsTx(tx *gorm.DB, params BulkCompensateSubscriptionsByPresetParams) ([]int, error) {
	if tx == nil {
		tx = DB
	}

	type userRow struct {
		Id       int    `gorm:"column:id"`
		Username string `gorm:"column:username"`
	}

	excludedByID := normalizeUniqueSortedIDs(params.ExcludedUserIDs)
	if len(excludedByID) > 0 {
		var rows []userRow
		if err := tx.Unscoped().
			Model(&User{}).
			Select("id").
			Where("id IN ?", excludedByID).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		found := make(map[int]struct{}, len(rows))
		for _, row := range rows {
			if row.Id > 0 {
				found[row.Id] = struct{}{}
			}
		}
		for _, userID := range excludedByID {
			if _, ok := found[userID]; !ok {
				return nil, fmt.Errorf("排除用户 ID #%d 不存在", userID)
			}
		}
	}

	excludedByUsername := normalizeUniqueSortedStrings(params.ExcludedUsernames)
	resolvedUserIDs := make([]int, 0, len(excludedByID)+len(excludedByUsername))
	resolvedUserIDs = append(resolvedUserIDs, excludedByID...)
	if len(excludedByUsername) > 0 {
		var rows []userRow
		if err := tx.Unscoped().
			Model(&User{}).
			Select("id", "username").
			Where("username IN ?", excludedByUsername).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		found := make(map[string]int, len(rows))
		for _, row := range rows {
			name := strings.TrimSpace(row.Username)
			if row.Id > 0 && name != "" {
				found[name] = row.Id
				resolvedUserIDs = append(resolvedUserIDs, row.Id)
			}
		}
		for _, username := range excludedByUsername {
			if _, ok := found[username]; !ok {
				return nil, fmt.Errorf("排除用户名 %q 不存在", username)
			}
		}
	}

	return normalizeUniqueSortedIDs(resolvedUserIDs), nil
}

func loadCompensationPresetRevisionTx(tx *gorm.DB, presetID int, applyMode string, quantity int) (*RedemptionPreset, *RedemptionPresetRevision, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, nil, errors.New("compensation_preset_id 无效")
	}

	var preset RedemptionPreset
	if err := tx.Select("id", "name").Where("id = ?", presetID).First(&preset).Error; err != nil {
		return nil, nil, err
	}
	revision, err := EnsureCurrentRedemptionPresetRevisionTx(tx, preset.Id)
	if err != nil {
		return nil, nil, err
	}
	mode := strings.TrimSpace(revision.Mode)
	switch mode {
	case "subscription", "tokens", "request":
	default:
		return nil, nil, errors.New("补偿商品类型错误")
	}
	if quantity > 1 && revision.MultiQuantityDeferOnly && applyMode != SubscriptionApplyModeDefer {
		return nil, nil, errors.New("生效方式错误")
	}
	if quantity > 1 && !revision.MultiQuantityEnabled {
		return nil, nil, errors.New("该商品不支持多数量")
	}
	return &preset, revision, nil
}

func collectMatchedUserIDsForBulkCompensationTx(tx *gorm.DB, params BulkCompensateSubscriptionsByPresetParams, excludedUserIDs []int, result *BulkCompensateSubscriptionsByPresetResult) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if result == nil {
		return nil, errors.New("result 为空")
	}

	buildSubscriptionQuery := func() *gorm.DB {
		return tx.Model(&UserSubscription{}).
			Where("source_preset_id IN ?", params.SourcePresetIDs).
			Where("(start_at = 0 OR start_at <= ?)", params.FaultEndAt).
			Where("(expire_at = 0 OR expire_at >= ?)", params.FaultStartAt).
			Where("(invalid_at = 0 OR invalid_at >= ?)", params.FaultStartAt)
	}

	if err := buildUserExclusionQuery(buildSubscriptionQuery(), excludedUserIDs).Count(&result.SubscriptionMatchedCount).Error; err != nil {
		return nil, err
	}

	var subscriptionUserIDs []int
	if err := buildSubscriptionQuery().Distinct("user_id").Pluck("user_id", &subscriptionUserIDs).Error; err != nil {
		return nil, err
	}

	buildRequestQuery := func() *gorm.DB {
		return tx.Model(&UserRequestSubscription{}).
			Where("source_preset_id IN ?", params.SourcePresetIDs).
			Where("(start_at = 0 OR start_at <= ?)", params.FaultEndAt).
			Where("(expire_at = 0 OR expire_at >= ?)", params.FaultStartAt).
			Where("(invalid_at = 0 OR invalid_at >= ?)", params.FaultStartAt)
	}

	if err := buildUserExclusionQuery(buildRequestQuery(), excludedUserIDs).Count(&result.RequestSubscriptionMatchedCount).Error; err != nil {
		return nil, err
	}

	var requestUserIDs []int
	if err := buildRequestQuery().Distinct("user_id").Pluck("user_id", &requestUserIDs).Error; err != nil {
		return nil, err
	}

	userIDSet := make(map[int]struct{}, len(subscriptionUserIDs)+len(requestUserIDs))
	for _, userID := range subscriptionUserIDs {
		if userID > 0 {
			userIDSet[userID] = struct{}{}
		}
	}
	for _, userID := range requestUserIDs {
		if userID > 0 {
			userIDSet[userID] = struct{}{}
		}
	}

	matchedUserIDs := make([]int, 0, len(userIDSet))
	excludedSet := make(map[int]struct{}, len(excludedUserIDs))
	for _, userID := range excludedUserIDs {
		if userID > 0 {
			excludedSet[userID] = struct{}{}
		}
	}
	for userID := range userIDSet {
		if _, ok := excludedSet[userID]; ok {
			result.ExcludedMatchedUserCount++
			continue
		}
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

	return matchedUserIDs, nil
}

func applyPreparedRedemptionPresetToUserTx(tx *gorm.DB, userID int, preset *RedemptionPreset, revision *RedemptionPresetRevision, applyMode string, quantity int, source string) (mode string, createdCount int64, err error) {
	if tx == nil {
		return "", 0, errors.New("tx 为空")
	}
	if userID <= 0 {
		return "", 0, errors.New("userId 无效")
	}
	if preset == nil || preset.Id <= 0 {
		return "", 0, errors.New("preset_id 无效")
	}
	if revision == nil {
		return "", 0, errors.New("revision 为空")
	}
	if applyMode == "" {
		applyMode = SubscriptionApplyModeStack
	}
	if applyMode != SubscriptionApplyModeStack && applyMode != SubscriptionApplyModeDefer {
		return "", 0, errors.New("apply_mode 无效")
	}
	if quantity <= 0 {
		return "", 0, errors.New("quantity 无效")
	}
	if quantity > 100 {
		return "", 0, errors.New("quantity 过大")
	}

	now := time.Now().Unix()
	if now <= 0 || now > common.MaxSupportedUnixTimestamp {
		return "", 0, errors.New("系统时间异常")
	}

	mode = strings.TrimSpace(revision.Mode)
	switch mode {
	case "subscription", "tokens":
		if quantity > 1 && revision.MultiQuantityDeferOnly && applyMode != SubscriptionApplyModeDefer {
			return "", 0, errors.New("生效方式错误")
		}
		if quantity > 1 && !revision.MultiQuantityEnabled {
			return "", 0, errors.New("该商品不支持多数量")
		}
		if revision.Quota < 0 {
			return "", 0, errors.New("商品额度无效")
		}

		var groupIDs []int
		if len(revision.AllowedGroupIds) > 0 {
			if err := common.Unmarshal([]byte(revision.AllowedGroupIds), &groupIDs); err != nil {
				return "", 0, err
			}
		}
		groupIDs = normalizeUniqueSortedIDs(groupIDs)
		if len(groupIDs) == 0 {
			return "", 0, errors.New("商品可用分组为空")
		}
		if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
			return "", 0, err
		}

		billingUnit := UserSubscriptionBillingUnitQuota
		if mode == "tokens" {
			billingUnit = UserSubscriptionBillingUnitTokens
		}

		startAt := now
		if applyMode == SubscriptionApplyModeDefer {
			maxExpireAt, err := GetUserSubscriptionMaxExpireAtWithBillingUnit(tx, userID, now, billingUnit)
			if err != nil {
				return "", 0, err
			}
			if maxExpireAt >= startAt {
				startAt = maxExpireAt + 1
			}
		}
		if startAt > common.MaxSupportedUnixTimestamp {
			return "", 0, errors.New("订阅开始时间过大")
		}
		if revision.QuotaValidDays < 0 {
			return "", 0, errors.New("商品有效期无效")
		}

		perUnitExtendSeconds := int64(0)
		if revision.QuotaValidDays > 0 {
			days := int64(revision.QuotaValidDays)
			if days > common.MaxSupportedUnixTimestamp/common.SecondsPerDay {
				return "", 0, errors.New("订阅有效期过大")
			}
			perUnitExtendSeconds = days * common.SecondsPerDay
		}

		nextDeferStartAt := startAt
		for i := 0; i < quantity; i++ {
			unitStartAt := startAt
			if applyMode == SubscriptionApplyModeDefer {
				unitStartAt = nextDeferStartAt
			}

			unitExpireAt := int64(0)
			if perUnitExtendSeconds > 0 {
				if unitStartAt > common.MaxSupportedUnixTimestamp {
					return "", 0, errors.New("订阅开始时间过大")
				}
				if perUnitExtendSeconds > common.MaxSupportedUnixTimestamp-unitStartAt {
					return "", 0, errors.New("订阅有效期过大")
				}
				unitExpireAt = unitStartAt + perUnitExtendSeconds
			}

			if _, err := CreateUserSubscriptionTxWithBillingUnit(
				tx,
				userID,
				unitStartAt,
				revision.Quota,
				revision.Quota,
				revision.DailyQuotaLimit,
				unitExpireAt,
				groupIDs,
				billingUnit,
				source,
				UserSubscriptionSourceRef{PresetId: preset.Id, PresetRevisionId: revision.Id},
			); err != nil {
				return "", 0, err
			}
			createdCount++

			if applyMode == SubscriptionApplyModeDefer && perUnitExtendSeconds > 0 {
				nextDeferStartAt = unitExpireAt + 1
			}
		}
		return mode, createdCount, nil
	case "request":
		if quantity > 1 && revision.MultiQuantityDeferOnly && applyMode != SubscriptionApplyModeDefer {
			return "", 0, errors.New("生效方式错误")
		}
		if quantity > 1 && !revision.MultiQuantityEnabled {
			return "", 0, errors.New("该商品不支持多数量")
		}
		if revision.DailyRequestLimit < 0 {
			return "", 0, errors.New("商品每日次数无效")
		}
		if revision.Quota < 0 {
			return "", 0, errors.New("商品总次数无效")
		}

		var groupIDs []int
		if len(revision.AllowedGroupIds) > 0 {
			if err := common.Unmarshal([]byte(revision.AllowedGroupIds), &groupIDs); err != nil {
				return "", 0, err
			}
		}
		groupIDs = normalizeUniqueSortedIDs(groupIDs)
		if len(groupIDs) == 0 {
			return "", 0, errors.New("商品可用分组为空")
		}
		if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
			return "", 0, err
		}

		startAt := now
		if applyMode == SubscriptionApplyModeDefer {
			maxExpireAt, err := GetUserRequestSubscriptionMaxExpireAt(tx, userID, now)
			if err != nil {
				return "", 0, err
			}
			if maxExpireAt >= startAt {
				startAt = maxExpireAt + 1
			}
		}
		if startAt > common.MaxSupportedUnixTimestamp {
			return "", 0, errors.New("订阅开始时间过大")
		}
		if revision.QuotaValidDays < 0 {
			return "", 0, errors.New("商品有效期无效")
		}

		perUnitExtendSeconds := int64(0)
		if revision.QuotaValidDays > 0 {
			days := int64(revision.QuotaValidDays)
			if days > common.MaxSupportedUnixTimestamp/common.SecondsPerDay {
				return "", 0, errors.New("订阅有效期过大")
			}
			perUnitExtendSeconds = days * common.SecondsPerDay
		}

		nextDeferStartAt := startAt
		for i := 0; i < quantity; i++ {
			unitStartAt := startAt
			if applyMode == SubscriptionApplyModeDefer {
				unitStartAt = nextDeferStartAt
			}

			unitExpireAt := int64(0)
			if perUnitExtendSeconds > 0 {
				if unitStartAt > common.MaxSupportedUnixTimestamp {
					return "", 0, errors.New("订阅开始时间过大")
				}
				if perUnitExtendSeconds > common.MaxSupportedUnixTimestamp-unitStartAt {
					return "", 0, errors.New("订阅有效期过大")
				}
				unitExpireAt = unitStartAt + perUnitExtendSeconds
			}

			if _, err := CreateUserRequestSubscriptionTx(
				tx,
				userID,
				unitStartAt,
				float64(revision.DailyRequestLimit),
				float64(revision.Quota),
				unitExpireAt,
				groupIDs,
				source,
				UserRequestSubscriptionSourceRef{PresetId: preset.Id, PresetRevisionId: revision.Id},
			); err != nil {
				return "", 0, err
			}
			createdCount++

			if applyMode == SubscriptionApplyModeDefer && perUnitExtendSeconds > 0 {
				nextDeferStartAt = unitExpireAt + 1
			}
		}
		return mode, createdCount, nil
	default:
		return "", 0, errors.New("补偿商品类型错误")
	}
}

func applyRedemptionPresetToUserTx(tx *gorm.DB, userID int, presetID int, applyMode string, quantity int, source string) (mode string, createdCount int64, err error) {
	preset, revision, err := loadCompensationPresetRevisionTx(tx, presetID, applyMode, quantity)
	if err != nil {
		return "", 0, err
	}
	return applyPreparedRedemptionPresetToUserTx(tx, userID, preset, revision, applyMode, quantity, source)
}

func BulkCompensateSubscriptionsByPreset(params BulkCompensateSubscriptionsByPresetParams) (*BulkCompensateSubscriptionsByPresetResult, error) {
	if err := validateBulkCompensateSubscriptionsByPresetParams(&params); err != nil {
		return nil, err
	}

	result := &BulkCompensateSubscriptionsByPresetResult{
		FaultStartAt:         params.FaultStartAt,
		FaultEndAt:           params.FaultEndAt,
		SourcePresetIDs:      append([]int(nil), params.SourcePresetIDs...),
		ExcludedUserIDs:      append([]int(nil), params.ExcludedUserIDs...),
		ExcludedUsernames:    append([]string(nil), params.ExcludedUsernames...),
		CompensationPresetID: params.CompensationPresetID,
		ApplyMode:            params.ApplyMode,
		Quantity:             params.Quantity,
		DryRun:               params.DryRun,
	}

	var matchedUserIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureBulkCompensationSourcePresetsTx(tx, params.SourcePresetIDs); err != nil {
			return err
		}

		compensationPreset, revision, err := loadCompensationPresetRevisionTx(tx, params.CompensationPresetID, params.ApplyMode, params.Quantity)
		if err != nil {
			return err
		}
		result.CompensationPresetName = strings.TrimSpace(compensationPreset.Name)
		result.CompensationPresetMode = strings.TrimSpace(revision.Mode)

		excludedUserIDs, err := resolveBulkCompensationExcludedUserIDsTx(tx, params)
		if err != nil {
			return err
		}
		fillBulkCompensationExcludedPreview(result, excludedUserIDs)

		matchedIDs, err := collectMatchedUserIDsForBulkCompensationTx(tx, params, excludedUserIDs, result)
		if err != nil {
			return err
		}
		matchedUserIDs = matchedIDs
		if params.DryRun || len(matchedUserIDs) == 0 {
			return nil
		}

		source := buildBulkCompensationSourceString(params.CompensationPresetID, params.FaultStartAt, params.FaultEndAt)
		for _, userID := range matchedUserIDs {
			mode, createdCount, err := applyPreparedRedemptionPresetToUserTx(tx, userID, compensationPreset, revision, params.ApplyMode, params.Quantity, source)
			if err != nil {
				return fmt.Errorf("用户 #%d 补偿失败: %w", userID, err)
			}
			switch mode {
			case "subscription", "tokens":
				result.CreatedUserSubscriptionCount += createdCount
			case "request":
				result.CreatedUserRequestSubscriptionCount += createdCount
			default:
				return errors.New("补偿商品类型错误")
			}
		}
		result.CompensatedUserCount = len(matchedUserIDs)
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

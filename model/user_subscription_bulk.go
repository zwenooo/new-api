package model

import (
	"errors"
	"sort"
	"time"

	"one-api/common"

	"gorm.io/gorm"
)

type SubscriptionType string

const (
	SubscriptionTypeQuota   SubscriptionType = "quota"
	SubscriptionTypeRequest SubscriptionType = "request"
)

type BulkSubscriptionDurationOperation string

const (
	BulkSubscriptionDurationOperationAddDays     BulkSubscriptionDurationOperation = "add_days"
	BulkSubscriptionDurationOperationSetExpireAt BulkSubscriptionDurationOperation = "set_expire_at"
)

type BulkUpdateSubscriptionDurationParams struct {
	UserIDs           []int
	SubscriptionTypes []SubscriptionType
	MinRemainingDays  int
	MaxRemainingDays  *int

	AddDays     *int
	SetExpireAt *int64

	DryRun bool
}

type BulkUpdateSubscriptionDurationResult struct {
	Now         int64    `json:"now"`
	MinExpireAt int64    `json:"min_expire_at"`
	MaxExpireAt *int64   `json:"max_expire_at,omitempty"`
	DryRun      bool     `json:"dry_run"`
	Operation   string   `json:"operation"`
	AddDays     *int     `json:"add_days,omitempty"`
	SetExpireAt *int64   `json:"set_expire_at,omitempty"`
	Types       []string `json:"subscription_types"`

	MatchedUserCount          int   `json:"matched_user_count"`
	MatchedUserIDsPreview     []int `json:"matched_user_ids_preview,omitempty"`
	MatchedUserIDsPreviewMore bool  `json:"matched_user_ids_preview_more,omitempty"`

	QuotaSubscriptionMatchedCount int64 `json:"quota_subscription_matched_count"`
	QuotaSubscriptionUpdatedCount int64 `json:"quota_subscription_updated_count"`
	QuotaMatchedUserCount         int   `json:"quota_matched_user_count"`

	RequestSubscriptionMatchedCount int64 `json:"request_subscription_matched_count"`
	RequestSubscriptionUpdatedCount int64 `json:"request_subscription_updated_count"`
	RequestMatchedUserCount         int   `json:"request_matched_user_count"`
}

func BulkUpdateSubscriptionDuration(params BulkUpdateSubscriptionDurationParams) (*BulkUpdateSubscriptionDurationResult, error) {
	if params.MinRemainingDays < 0 {
		return nil, errors.New("min_remaining_days 不能为负数")
	}
	if params.MaxRemainingDays != nil && *params.MaxRemainingDays < params.MinRemainingDays {
		return nil, errors.New("max_remaining_days 必须大于等于 min_remaining_days")
	}

	operation := BulkSubscriptionDurationOperation("")
	if (params.AddDays == nil) == (params.SetExpireAt == nil) {
		return nil, errors.New("add_days 与 set_expire_at 必须且只能提供一个")
	}

	addDays := 0
	setExpireAt := int64(0)
	if params.AddDays != nil {
		if *params.AddDays <= 0 {
			return nil, errors.New("add_days 必须大于 0")
		}
		addDays = *params.AddDays
		operation = BulkSubscriptionDurationOperationAddDays
	} else {
		if *params.SetExpireAt <= 0 {
			return nil, errors.New("set_expire_at 必须大于 0")
		}
		setExpireAt = *params.SetExpireAt
		operation = BulkSubscriptionDurationOperationSetExpireAt
	}

	if len(params.SubscriptionTypes) == 0 {
		return nil, errors.New("subscription_types is required")
	}
	typeSet := make(map[SubscriptionType]struct{}, len(params.SubscriptionTypes))
	types := make([]SubscriptionType, 0, len(params.SubscriptionTypes))
	typeNames := make([]string, 0, len(params.SubscriptionTypes))
	for _, t := range params.SubscriptionTypes {
		switch t {
		case SubscriptionTypeQuota, SubscriptionTypeRequest:
		default:
			return nil, errors.New("subscription_types 存在无效值")
		}
		if _, ok := typeSet[t]; ok {
			continue
		}
		typeSet[t] = struct{}{}
		types = append(types, t)
		typeNames = append(typeNames, string(t))
	}
	params.SubscriptionTypes = types

	if len(params.UserIDs) > 0 {
		seen := make(map[int]struct{}, len(params.UserIDs))
		userIDs := make([]int, 0, len(params.UserIDs))
		for _, userId := range params.UserIDs {
			if userId <= 0 {
				return nil, errors.New("user_ids 中存在无效的用户 ID")
			}
			if _, ok := seen[userId]; ok {
				continue
			}
			seen[userId] = struct{}{}
			userIDs = append(userIDs, userId)
		}
		params.UserIDs = userIDs
	}

	now := time.Now().Unix()
	if operation == BulkSubscriptionDurationOperationSetExpireAt && setExpireAt <= now {
		return nil, errors.New("set_expire_at 必须晚于当前时间")
	}
	if operation == BulkSubscriptionDurationOperationSetExpireAt && setExpireAt > common.MaxSupportedUnixTimestamp {
		return nil, errors.New("set_expire_at 过大，最大支持到 " + common.MaxSupportedUnixTimestampLabel)
	}

	minExpireAt := now + int64(params.MinRemainingDays)*24*60*60
	var maxExpireAt *int64
	if params.MaxRemainingDays != nil {
		v := now + int64(*params.MaxRemainingDays)*24*60*60
		maxExpireAt = &v
	}

	var deltaSeconds int64
	if operation == BulkSubscriptionDurationOperationAddDays {
		deltaSeconds = int64(addDays) * 24 * 60 * 60
		if deltaSeconds <= 0 {
			return nil, errors.New("add_days 数值过大")
		}
		if deltaSeconds >= common.MaxSupportedUnixTimestamp {
			return nil, errors.New("add_days 数值过大，最大支持到 " + common.MaxSupportedUnixTimestampLabel)
		}
	}

	quotaUserIDs := make([]int, 0)
	requestUserIDs := make([]int, 0)

	maxQuotaExpireAtBeforeAdd := int64(0)
	maxRequestExpireAtBeforeAdd := int64(0)
	if operation == BulkSubscriptionDurationOperationAddDays {
		maxQuotaExpireAtBeforeAdd = common.MaxSupportedUnixTimestamp - deltaSeconds
		maxRequestExpireAtBeforeAdd = common.MaxSupportedUnixTimestamp - deltaSeconds
	}

	result := &BulkUpdateSubscriptionDurationResult{
		Now:         now,
		MinExpireAt: minExpireAt,
		MaxExpireAt: maxExpireAt,
		DryRun:      params.DryRun,
		Operation:   string(operation),
		AddDays:     params.AddDays,
		SetExpireAt: params.SetExpireAt,
		Types:       typeNames,
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		activeUsersQuery := tx.Model(&User{}).Select("id")

		if _, ok := typeSet[SubscriptionTypeQuota]; ok {
			buildQuery := func() *gorm.DB {
				q := tx.Model(&UserSubscription{}).
					Where("user_id IN (?)", activeUsersQuery).
					Where("remaining_quota > 0").
					Where("expire_at > 0").
					Where("expire_at > ?", now).
					Where("expire_at > ?", minExpireAt)
				if maxQuotaExpireAtBeforeAdd > 0 {
					q = q.Where("expire_at <= ?", maxQuotaExpireAtBeforeAdd)
				}
				if maxExpireAt != nil {
					q = q.Where("expire_at <= ?", *maxExpireAt)
				}
				if len(params.UserIDs) > 0 {
					q = q.Where("user_id IN ?", params.UserIDs)
				}
				return q
			}

			var subsCount int64
			if err := buildQuery().Count(&subsCount).Error; err != nil {
				return err
			}
			result.QuotaSubscriptionMatchedCount = subsCount

			var userIDs []int
			if err := buildQuery().Distinct("user_id").Pluck("user_id", &userIDs).Error; err != nil {
				return err
			}
			sort.Ints(userIDs)
			quotaUserIDs = userIDs
			result.QuotaMatchedUserCount = len(userIDs)

			if !params.DryRun && subsCount > 0 {
				var updateVal any
				if operation == BulkSubscriptionDurationOperationAddDays {
					updateVal = gorm.Expr("expire_at + ?", deltaSeconds)
				} else {
					updateVal = setExpireAt
				}

				res := buildQuery().Updates(map[string]any{
					"expire_at": updateVal,
				})
				if res.Error != nil {
					return res.Error
				}
				result.QuotaSubscriptionUpdatedCount = res.RowsAffected

				for _, userId := range quotaUserIDs {
					if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
						return err
					}
				}
			}
		}

		if _, ok := typeSet[SubscriptionTypeRequest]; ok {
			buildQuery := func() *gorm.DB {
				q := tx.Model(&UserRequestSubscription{}).
					Where("user_id IN (?)", activeUsersQuery).
					Where("expire_at > 0").
					Where("expire_at > ?", now).
					Where("expire_at > ?", minExpireAt).
					Where("(total_request_limit = 0 OR total_request_used < total_request_limit)")
				if maxRequestExpireAtBeforeAdd > 0 {
					q = q.Where("expire_at <= ?", maxRequestExpireAtBeforeAdd)
				}
				if maxExpireAt != nil {
					q = q.Where("expire_at <= ?", *maxExpireAt)
				}
				if len(params.UserIDs) > 0 {
					q = q.Where("user_id IN ?", params.UserIDs)
				}
				return q
			}

			var subsCount int64
			if err := buildQuery().Count(&subsCount).Error; err != nil {
				return err
			}
			result.RequestSubscriptionMatchedCount = subsCount

			var userIDs []int
			if err := buildQuery().Distinct("user_id").Pluck("user_id", &userIDs).Error; err != nil {
				return err
			}
			sort.Ints(userIDs)
			requestUserIDs = userIDs
			result.RequestMatchedUserCount = len(userIDs)

			if !params.DryRun && subsCount > 0 {
				var updateVal any
				if operation == BulkSubscriptionDurationOperationAddDays {
					updateVal = gorm.Expr("expire_at + ?", deltaSeconds)
				} else {
					updateVal = setExpireAt
				}

				res := buildQuery().Updates(map[string]any{
					"expire_at": updateVal,
				})
				if res.Error != nil {
					return res.Error
				}
				result.RequestSubscriptionUpdatedCount = res.RowsAffected
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	userIDSet := make(map[int]struct{}, len(quotaUserIDs)+len(requestUserIDs))
	for _, userId := range quotaUserIDs {
		userIDSet[userId] = struct{}{}
	}
	for _, userId := range requestUserIDs {
		userIDSet[userId] = struct{}{}
	}
	mergedUserIDs := make([]int, 0, len(userIDSet))
	for userId := range userIDSet {
		mergedUserIDs = append(mergedUserIDs, userId)
	}
	sort.Ints(mergedUserIDs)
	result.MatchedUserCount = len(mergedUserIDs)

	const previewLimit = 100
	if len(mergedUserIDs) > previewLimit {
		result.MatchedUserIDsPreview = append([]int(nil), mergedUserIDs[:previewLimit]...)
		result.MatchedUserIDsPreviewMore = true
	} else {
		result.MatchedUserIDsPreview = append([]int(nil), mergedUserIDs...)
	}

	if !params.DryRun && (result.QuotaSubscriptionUpdatedCount > 0 || result.RequestSubscriptionUpdatedCount > 0) {
		for _, userId := range mergedUserIDs {
			if err := invalidateUserCache(userId); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

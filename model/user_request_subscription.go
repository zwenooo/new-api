package model

import (
	"database/sql"
	"errors"
	"one-api/common"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserRequestSubscription 记录用户的订阅型 request 权益（按日限次，按有效期到期）。
// 每次成功请求（日志类型为“消费”）应扣减 1 次；失败请求需要返还已预扣的次数。
type UserRequestSubscription struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId int `json:"user_id" gorm:"index"`

	DailyRequestLimit     int `json:"daily_request_limit" gorm:"type:bigint;default:0;column:daily_request_limit"`
	DailyRequestUsed      int `json:"daily_request_used" gorm:"type:bigint;default:0;column:daily_request_used"`
	DailyRequestResetDate int `json:"daily_request_reset_date"`

	// TotalRequestLimit limits total requests for this subscription.
	// 0 means unlimited.
	TotalRequestLimit int `json:"total_request_limit" gorm:"type:bigint;default:0;column:total_request_limit"`
	TotalRequestUsed  int `json:"total_request_used" gorm:"type:bigint;default:0;column:total_request_used"`
	SortOrder         int `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`

	// StartAt is the subscription's effective start timestamp (unix seconds).
	// It is used for "defer" mode subscriptions to start after existing ones.
	StartAt   int64 `json:"start_at" gorm:"bigint;default:0;index;column:start_at"`
	ExpireAt  int64 `json:"expire_at" gorm:"index"`
	InvalidAt int64 `json:"invalid_at" gorm:"bigint;default:0;index;column:invalid_at"`

	// AllowedGroups limits which channel groups (tiers) this subscription can be consumed from.
	AllowedGroups JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`

	// Source* fields are denormalized references used for management operations.
	SourceOrderId      int       `json:"source_order_id" gorm:"type:int;default:0;index;column:source_order_id"`
	SourcePresetId     int       `json:"source_preset_id" gorm:"type:int;default:0;index;column:source_preset_id"`
	SourceRedemptionId int       `json:"source_redemption_id" gorm:"type:int;default:0;index;column:source_redemption_id"`
	Source             string    `json:"source" gorm:"size:255"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type UserRequestSubscriptionSourceRef struct {
	OrderId          int
	PresetId         int
	PresetRevisionId int
	RedemptionId     int
}

const (
	userRequestSubscriptionActiveOrderExpr  = "sort_order DESC, CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC"
	userRequestSubscriptionPendingOrderExpr = "sort_order DESC, start_at ASC, CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC"
	userRequestSubscriptionExpiredOrderExpr = "sort_order DESC, expire_at DESC, id DESC"
)

type userRequestSubscriptionConsumeFailureReason int

const (
	userRequestSubscriptionConsumeFailureNone userRequestSubscriptionConsumeFailureReason = iota
	userRequestSubscriptionConsumeFailureGroupMismatch
	userRequestSubscriptionConsumeFailureDailyExhausted
	userRequestSubscriptionConsumeFailureTotalExhausted
)

type userRequestSubscriptionConsumeCheckResult struct {
	Consumable bool
	Reason     userRequestSubscriptionConsumeFailureReason

	DailyUsed  int
	DailyReset int
	TotalUsed  int
}

func normalizeUserRequestSubscriptionDailyUsage(sub UserRequestSubscription, today int) (used int, resetDate int) {
	used = sub.DailyRequestUsed
	resetDate = sub.DailyRequestResetDate
	if resetDate != today {
		used = 0
		resetDate = today
	}
	return used, resetDate
}

func canConsumeUserRequestSubscription(sub UserRequestSubscription, today int, count int) bool {
	if count <= 0 {
		return false
	}
	used, _ := normalizeUserRequestSubscriptionDailyUsage(sub, today)
	if sub.DailyRequestLimit > 0 && used+count > sub.DailyRequestLimit {
		return false
	}
	if sub.TotalRequestLimit > 0 && sub.TotalRequestUsed+count > sub.TotalRequestLimit {
		return false
	}
	return true
}

func evaluateUserRequestSubscriptionConsumption(sub UserRequestSubscription, allowedSet map[int]struct{}, groupID int, today int, count int) userRequestSubscriptionConsumeCheckResult {
	result := userRequestSubscriptionConsumeCheckResult{
		Reason: userRequestSubscriptionConsumeFailureNone,
	}
	if count <= 0 {
		return result
	}
	if len(allowedSet) == 0 || groupID <= 0 {
		result.Reason = userRequestSubscriptionConsumeFailureGroupMismatch
		return result
	}
	if _, ok := allowedSet[groupID]; !ok {
		result.Reason = userRequestSubscriptionConsumeFailureGroupMismatch
		return result
	}

	result.DailyUsed, result.DailyReset = normalizeUserRequestSubscriptionDailyUsage(sub, today)
	result.TotalUsed = sub.TotalRequestUsed

	if sub.DailyRequestLimit > 0 && result.DailyUsed+count > sub.DailyRequestLimit {
		result.Reason = userRequestSubscriptionConsumeFailureDailyExhausted
		return result
	}
	if sub.TotalRequestLimit > 0 && result.TotalUsed+count > sub.TotalRequestLimit {
		result.Reason = userRequestSubscriptionConsumeFailureTotalExhausted
		return result
	}

	result.Consumable = true
	return result
}

func buildUserRequestSubscriptionInsufficientError(hasGroupMatch bool, dailyExhausted bool, totalExhausted bool) error {
	if hasGroupMatch {
		switch {
		case dailyExhausted && totalExhausted:
			return errors.New("次数订阅当日或总次数已用尽")
		case dailyExhausted:
			return errors.New("次数订阅当日次数已用尽")
		case totalExhausted:
			return errors.New("次数订阅总次数已用尽")
		}
	}
	return errors.New("次数订阅不足")
}

func resolveUserRequestSubscriptionAllowedGroupsTx(tx *gorm.DB, subs []UserRequestSubscription) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	if len(subs) == 0 {
		return map[int][]int{}, nil
	}

	subIDs := make([]int, 0, len(subs))
	for _, sub := range subs {
		if sub.Id > 0 {
			subIDs = append(subIDs, sub.Id)
		}
	}

	revisionBindingBySubID, err := getUserRequestSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx, subIDs)
	if err != nil {
		return nil, err
	}
	subGroupIDs := make(map[int][]int, len(subIDs))
	if len(subIDs) > 0 && tx.Migrator().HasTable(&UserRequestSubscriptionGroup{}) {
		type row struct {
			SubscriptionId int `gorm:"column:subscription_id"`
			GroupId        int `gorm:"column:group_id"`
		}
		var rows []row
		if err := tx.Model(&UserRequestSubscriptionGroup{}).
			Select("subscription_id", "group_id").
			Where("subscription_id IN ?", subIDs).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.SubscriptionId <= 0 || r.GroupId <= 0 {
				continue
			}
			subGroupIDs[r.SubscriptionId] = append(subGroupIDs[r.SubscriptionId], r.GroupId)
		}
	}

	targets := make([]effectiveAllowedGroupTarget, 0, len(subs))
	for _, sub := range subs {
		if sub.Id <= 0 {
			continue
		}
		target := effectiveAllowedGroupTarget{
			OwnerID:          sub.Id,
			ProductID:        sub.SourcePresetId,
			SnapshotGroupIDs: subGroupIDs[sub.Id],
		}
		if binding, ok := revisionBindingBySubID[sub.Id]; ok {
			target.RevisionID = binding.RevisionId
		}
		targets = append(targets, target)
	}
	return resolveEffectiveAllowedGroupsTx(tx, targets, effectiveAllowedGroupResolverOptions{
		OwnerLabel:    "次数订阅",
		SnapshotLabel: "订阅快照",
	})
}

func CreateUserRequestSubscriptionTx(tx *gorm.DB, userId int, startAt int64, dailyLimit float64, totalLimit float64, expireAt int64, allowedGroupIDs []int, source string, srcRef UserRequestSubscriptionSourceRef) (*UserRequestSubscription, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if dailyLimit < 0 {
		return nil, errors.New("每日次数必须大于等于 0")
	}
	if totalLimit < 0 {
		return nil, errors.New("总次数必须大于等于 0")
	}
	if startAt < 0 {
		return nil, errors.New("start_at 无效")
	}
	if startAt == 0 {
		startAt = time.Now().Unix()
	}
	if expireAt < 0 {
		return nil, errors.New("expire_at 无效")
	}
	if expireAt > 0 && startAt > expireAt {
		return nil, errors.New("expire_at 必须晚于 start_at")
	}
	if tx == nil {
		tx = DB
	}
	dailyLimitStored, dailyLimitExact := discreteUnitsFromDisplayFloatExact(dailyLimit)
	if dailyLimit > 0 && !dailyLimitExact {
		return nil, errors.New("每日次数最多支持 3 位小数")
	}
	totalLimitStored, totalLimitExact := discreteUnitsFromDisplayFloatExact(totalLimit)
	if totalLimit > 0 && !totalLimitExact {
		return nil, errors.New("总次数最多支持 3 位小数")
	}

	groupIDs := normalizeUniqueSortedIDs(allowedGroupIDs)
	if len(groupIDs) == 0 {
		return nil, errors.New("可用分组不能为空")
	}
	if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
		return nil, err
	}

	sub := &UserRequestSubscription{
		UserId:                userId,
		DailyRequestLimit:     dailyLimitStored,
		DailyRequestUsed:      0,
		DailyRequestResetDate: common.GetTodayDateInt(),
		TotalRequestLimit:     totalLimitStored,
		TotalRequestUsed:      0,
		StartAt:               startAt,
		ExpireAt:              expireAt,
		InvalidAt:             0,
		AllowedGroups:         nil,
		SourceOrderId:         srcRef.OrderId,
		SourcePresetId:        srcRef.PresetId,
		SourceRedemptionId:    srcRef.RedemptionId,
		Source:                strings.TrimSpace(source),
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	if srcRef.PresetId > 0 && srcRef.PresetRevisionId > 0 {
		if err := upsertUserRequestSubscriptionPresetRevisionBindingTx(tx, sub.Id, srcRef.PresetId, srcRef.PresetRevisionId); err != nil {
			return nil, err
		}
	}
	if err := upsertUserRequestSubscriptionGroupsTx(tx, sub.Id, groupIDs); err != nil {
		return nil, err
	}
	return sub, nil
}

func GetUserRequestSubscriptionMaxExpireAt(tx *gorm.DB, userId int, now int64) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var maxExpire sql.NullInt64
	if err := tx.Model(&UserRequestSubscription{}).
		// 顺延仅参考“有限期订阅”的最大到期时间；忽略 expire_at=0（不限时）以及 year 9999 这种“伪不限时”的历史数据，
		// 以免把新购买的订阅顺延到永远之后。
		// 注意：顺延基准是“未到期的订阅周期”，不应依赖 invalid_at。
		Where("user_id = ? AND expire_at >= ? AND expire_at < ?", userId, now, common.MaxSupportedUnixTimestamp).
		Select("MAX(expire_at)").
		Scan(&maxExpire).Error; err != nil {
		return 0, err
	}
	if !maxExpire.Valid {
		return 0, nil
	}
	return maxExpire.Int64, nil
}

// GetUserRequestSubscriptionGroupCandidatesByCount returns possible group_ids that can
// be billed from request subscriptions for the given request-count cost.
func GetUserRequestSubscriptionGroupCandidatesByCount(userId int, count int) ([]int, bool, error) {
	if userId <= 0 {
		return nil, false, errors.New("userId 无效")
	}
	if count <= 0 {
		return nil, false, errors.New("count 无效")
	}
	now := time.Now().Unix()
	today := common.GetTodayDateInt()
	var subs []UserRequestSubscription
	if err := DB.Select("id", "source_preset_id", "daily_request_limit", "daily_request_used", "daily_request_reset_date", "total_request_limit", "total_request_used").
		Where("user_id = ? AND invalid_at = 0 AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?)", userId, now, now).
		Find(&subs).Error; err != nil {
		return nil, false, err
	}
	if len(subs) == 0 {
		return nil, false, nil
	}

	subIDs := make([]int, 0, len(subs))
	for _, sub := range subs {
		if sub.Id <= 0 {
			continue
		}
		if !canConsumeUserRequestSubscription(sub, today, count) {
			continue
		}
		subIDs = append(subIDs, sub.Id)
	}
	if len(subIDs) == 0 {
		return nil, false, nil
	}

	allowedGroupIDsBySubID, err := resolveUserRequestSubscriptionAllowedGroupsTx(DB, subs)
	if err != nil {
		return nil, false, err
	}
	groupIDs := make([]int, 0, len(subIDs)*2)
	for _, subID := range subIDs {
		groupIDs = append(groupIDs, allowedGroupIDsBySubID[subID]...)
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil, false, nil
	}
	return groupIDs, true, nil
}

// GetUserRequestSubscriptionGroupCandidates returns possible group_ids that can be billed from request subscriptions.
func GetUserRequestSubscriptionGroupCandidates(userId int) ([]int, bool, error) {
	return GetUserRequestSubscriptionGroupCandidatesByCount(userId, 1)
}

// PreConsumeUserRequestSubscription attempts to reserve "count" requests for (user, group_id).
// It returns the chosen subscription id for later refund on failure.
func PreConsumeUserRequestSubscription(userId int, groupID int, count int) (subId int, err error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return 0, errors.New("group_id 无效")
	}
	if count <= 0 {
		return 0, errors.New("count 无效")
	}

	now := time.Now().Unix()
	today := common.GetTodayDateInt()

	var selectedId int
	err = DB.Transaction(func(tx *gorm.DB) error {
		var subs []UserRequestSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "source_preset_id", "daily_request_limit", "daily_request_used", "daily_request_reset_date", "total_request_limit", "total_request_used", "expire_at").
			Where("user_id = ? AND invalid_at = 0 AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?)", userId, now, now).
			Order(userRequestSubscriptionActiveOrderExpr).
			Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) == 0 {
			return errors.New("次数订阅不足")
		}

		resolvedAllowedGroupIDsBySubID, err := resolveUserRequestSubscriptionAllowedGroupsTx(tx, subs)
		if err != nil {
			return err
		}
		allowedBySubID := make(map[int]map[int]struct{}, len(subs))
		for _, sub := range subs {
			groupIDs := resolvedAllowedGroupIDsBySubID[sub.Id]
			if len(groupIDs) == 0 {
				continue
			}
			set := make(map[int]struct{}, len(groupIDs))
			for _, groupID := range groupIDs {
				if groupID <= 0 {
					continue
				}
				set[groupID] = struct{}{}
			}
			if len(set) > 0 {
				allowedBySubID[sub.Id] = set
			}
		}

		hasGroupMatch := false
		dailyExhausted := false
		totalExhausted := false

		for _, sub := range subs {
			if sub.Id <= 0 {
				continue
			}
			allowedSet := allowedBySubID[sub.Id]
			check := evaluateUserRequestSubscriptionConsumption(sub, allowedSet, groupID, today, count)
			switch check.Reason {
			case userRequestSubscriptionConsumeFailureGroupMismatch:
				continue
			case userRequestSubscriptionConsumeFailureDailyExhausted:
				hasGroupMatch = true
				dailyExhausted = true
				continue
			case userRequestSubscriptionConsumeFailureTotalExhausted:
				hasGroupMatch = true
				totalExhausted = true
				continue
			case userRequestSubscriptionConsumeFailureNone:
				if !check.Consumable {
					continue
				}
			}
			hasGroupMatch = true

			updates := map[string]interface{}{
				"daily_request_used":       check.DailyUsed + count,
				"daily_request_reset_date": check.DailyReset,
			}
			if sub.TotalRequestLimit > 0 {
				updates["total_request_used"] = check.TotalUsed + count
			}
			if err := tx.Model(&UserRequestSubscription{}).
				Where("id = ? AND user_id = ?", sub.Id, userId).
				Updates(updates).Error; err != nil {
				return err
			}
			selectedId = sub.Id
			return nil
		}

		return buildUserRequestSubscriptionInsufficientError(hasGroupMatch, dailyExhausted, totalExhausted)
	})
	if err != nil {
		return 0, err
	}
	return selectedId, nil
}

func ReturnUserRequestSubscription(userId int, subId int, count int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if subId <= 0 {
		return errors.New("subId 无效")
	}
	if count <= 0 {
		return errors.New("count 无效")
	}

	today := common.GetTodayDateInt()
	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserRequestSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "daily_request_used", "daily_request_reset_date", "total_request_limit", "total_request_used").
			Where("id = ? AND user_id = ?", subId, userId).
			First(&sub).Error; err != nil {
			return err
		}

		used := sub.DailyRequestUsed
		reset := sub.DailyRequestResetDate
		if reset != today {
			used = 0
			reset = today
		}
		used -= count
		if used < 0 {
			used = 0
		}
		updates := map[string]interface{}{
			"daily_request_used":       used,
			"daily_request_reset_date": reset,
		}
		if sub.TotalRequestLimit > 0 {
			totalUsed := sub.TotalRequestUsed - count
			if totalUsed < 0 {
				totalUsed = 0
			}
			updates["total_request_used"] = totalUsed
		}
		return tx.Model(&UserRequestSubscription{}).Where("id = ? AND user_id = ?", subId, userId).Updates(updates).Error
	})
}

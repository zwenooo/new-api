package model

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"one-api/common"
	relaycommon "one-api/relay/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserSubscription 记录用户的订阅型权益，覆盖 credit/token 两种计量单位。
// request 单位的订阅使用 UserRequestSubscription 单独建模。
type UserSubscription struct {
	Id     int `json:"id" gorm:"primaryKey"`
	UserId int `json:"user_id" gorm:"index;index:idx_user_subscriptions_user_credited_invalid_at,priority:1"`
	// BillingUnit distinguishes the unit of this subscription:
	// - "quota": historical name, semantically means credit
	// - "tokens": raw tokens (charged by usage total_tokens)
	BillingUnit         string `json:"billing_unit" gorm:"type:varchar(16);default:'quota';index;column:billing_unit"`
	TotalQuota          int    `json:"total_quota" gorm:"type:bigint;default:0;column:total_quota"`
	RemainingQuota      int    `json:"remaining_quota" gorm:"type:bigint;default:0;column:remaining_quota"`
	DailyQuotaLimit     int    `json:"daily_quota_limit" gorm:"type:bigint;default:0;column:daily_quota_limit"`
	DailyQuotaUsed      int    `json:"daily_quota_used" gorm:"type:bigint;default:0;column:daily_quota_used"`
	DailyQuotaResetDate int    `json:"daily_quota_reset_date"`
	SortOrder           int    `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`
	// StartAt is the subscription's effective start timestamp (unix seconds).
	// It is used for "defer" mode subscriptions to start after existing ones.
	StartAt   int64 `json:"start_at" gorm:"bigint;default:0;index;column:start_at"`
	ExpireAt  int64 `json:"expire_at" gorm:"index"`
	InvalidAt int64 `json:"invalid_at" gorm:"bigint;default:0;index;index:idx_user_subscriptions_user_credited_invalid_at,priority:3;column:invalid_at"`
	// Credited indicates whether RemainingQuota has been credited into users.quota.
	// For deferred subscriptions (start_at in the future), this is false until start_at is reached.
	Credited bool `json:"credited" gorm:"type:boolean;default:true;index:idx_user_subscriptions_user_credited_invalid_at,priority:2;column:credited"`
	// AllowedGroups limits which channel groups (tiers) this subscription can be consumed from.
	//
	// IMPORTANT:
	// - For manually created subscriptions, explicit per-subscription bindings are authoritative.
	// - For preset-backed subscriptions, current product groups are authoritative at runtime.
	// - Preset revision / per-subscription bindings only act as fallback snapshots when the source product is missing.
	AllowedGroups JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	// Source* fields are denormalized references used for management operations (e.g., retroactive group grants).
	// They are optional and mainly populated when a subscription is created from an order/preset.
	SourceOrderId      int       `json:"source_order_id" gorm:"type:int;default:0;index;column:source_order_id"`
	SourcePresetId     int       `json:"source_preset_id" gorm:"type:int;default:0;index;column:source_preset_id"`
	SourceRedemptionId int       `json:"source_redemption_id" gorm:"type:int;default:0;index;column:source_redemption_id"`
	Source             string    `json:"source" gorm:"size:255"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

const (
	UserSubscriptionBillingUnitQuota  = "quota"
	UserSubscriptionBillingUnitTokens = "tokens"
)

const legacySubscriptionDefaultGroup = "codex"

const (
	userSubscriptionActiveOrderExpr  = "sort_order DESC, CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC"
	userSubscriptionPendingOrderExpr = "sort_order DESC, start_at ASC, CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC"
	userSubscriptionExpiredOrderExpr = "sort_order DESC, expire_at DESC, id DESC"
)

type UserSubscriptionSourceRef struct {
	OrderId          int
	PresetId         int
	PresetRevisionId int
	RedemptionId     int
}

func normalizeUserSubscriptionBillingUnit(unit string) (string, error) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return UserSubscriptionBillingUnitQuota, nil
	}
	switch unit {
	case UserSubscriptionBillingUnitQuota, UserSubscriptionBillingUnitTokens:
		return unit, nil
	default:
		return "", errors.New("billing_unit 无效")
	}
}

// resolveSubscriptionAllowedGroupsTx returns each subscription's effective allowed group ids.
//
// Source of truth:
//   - For preset-backed subscriptions, current product groups win.
//   - If the source product is missing, fall back to revision snapshot, then per-subscription snapshot.
//   - For non-product subscriptions, use explicit per-subscription bindings first.
func resolveSubscriptionAllowedGroupsTx(tx *gorm.DB, subs []UserSubscription) (map[int][]int, error) {
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

	revisionBindingBySubID, err := getUserSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx, subIDs)
	if err != nil {
		return nil, err
	}
	subGroupIDs := make(map[int][]int, len(subIDs))
	if len(subIDs) > 0 && tx.Migrator().HasTable(&UserSubscriptionGroup{}) {
		type row struct {
			SubscriptionId int `gorm:"column:subscription_id"`
			GroupId        int `gorm:"column:group_id"`
		}
		var rows []row
		if err := tx.Model(&UserSubscriptionGroup{}).
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
		OwnerLabel:    "订阅",
		SnapshotLabel: "订阅快照",
	})
}

type SubscriptionSummary struct {
	ID                   int                               `json:"id"`
	Source               string                            `json:"source"`
	SourceOrderId        int                               `json:"source_order_id,omitempty"`
	SourceOrderTradeNo   string                            `json:"source_order_trade_no,omitempty"`
	SourceOrderQuantity  int                               `json:"source_order_quantity,omitempty"`
	SourcePresetId       int                               `json:"source_preset_id"`
	SourcePresetName     string                            `json:"source_preset_name,omitempty"`
	SourceRedemptionId   int                               `json:"source_redemption_id"`
	SourceRedemptionName string                            `json:"source_redemption_name,omitempty"`
	AllowedGroupIds      []int                             `json:"allowed_group_ids"`
	SortOrder            int                               `json:"sort_order"`
	GroupDailyLimits     []GroupDailyQuotaLimit            `json:"group_daily_limits,omitempty"`
	GroupQuotaBreakdown  []SubscriptionGroupQuotaBreakdown `json:"group_quota_breakdown,omitempty"`
	TotalQuota           int                               `json:"total_quota"`
	RemainingQuota       int                               `json:"remaining_quota"`
	ConsumedQuota        int                               `json:"consumed_quota"`
	DailyQuotaLimit      int                               `json:"daily_quota_limit"`
	DailyQuotaUsed       int                               `json:"daily_quota_used"`
	StartAt              int64                             `json:"start_at"`
	ExpireAt             int64                             `json:"expire_at"`
	InvalidAt            int64                             `json:"invalid_at"`
}

type SubscriptionGroupCapacity struct {
	GroupId        int `json:"group_id"`
	TotalRemaining int `json:"total_remaining"`
	DailyCapacity  int `json:"daily_capacity"`
}

type SubscriptionGroupQuotaBreakdown struct {
	GroupId             int `json:"group_id"`
	DailyQuotaUsed      int `json:"daily_quota_used"`
	DailyQuotaAvailable int `json:"daily_quota_available"`
	DailyQuotaLimit     int `json:"daily_quota_limit"`
}

type QuotaBreakdown struct {
	PaygTotal     int `json:"payg_total"`
	PaygConsumed  int `json:"payg_consumed"`
	PaygRemaining int `json:"payg_remaining"`

	SubscriptionTotal               int                   `json:"subscription_total"`
	SubscriptionConsumed            int                   `json:"subscription_consumed"`
	SubscriptionRemaining           int                   `json:"subscription_remaining"`
	SubscriptionDailyLimit          int                   `json:"subscription_daily_limit"`
	SubscriptionDailyUsed           int                   `json:"subscription_daily_used"`
	SubscriptionDailyLimitUnlimited bool                  `json:"subscription_daily_limit_unlimited"`
	SubscriptionWindowStart         int64                 `json:"subscription_window_start"`
	SubscriptionWindowEnd           int64                 `json:"subscription_window_end"`
	HasSubscription                 bool                  `json:"has_subscription"`
	Subscriptions                   []SubscriptionSummary `json:"subscriptions"`
	SubscriptionsAll                []SubscriptionSummary `json:"subscriptions_all"`

	// TodayTotalUsed 统计“今日总消耗额度”，基于 quota_data 表按自然日汇总。
	TodayTotalUsed int `json:"today_total_used"`

	SubscriptionGroupCapacities []SubscriptionGroupCapacity `json:"subscription_group_capacities"`

	// SubscriptionConfigErrors records non-fatal validation errors when building subscription breakdown.
	// It is primarily used by UIs to surface misconfigurations (e.g. group-daily-limit coverage drift)
	// without hiding the entire subscription list.
	SubscriptionConfigErrors []string `json:"subscription_config_errors,omitempty"`
}

func deactivateNotStartedSubscriptions(tx *gorm.DB, userId int, now int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "billing_unit", "total_quota", "remaining_quota").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND start_at > ? AND (expire_at = 0 OR expire_at >= ?) AND (total_quota = 0 OR remaining_quota > 0)", userId, true, now, now).
		Find(&subs).Error; err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	ids := make([]int, 0, len(subs))
	quotaTotal := 0
	tokensTotal := 0
	for _, sub := range subs {
		if sub.Id <= 0 {
			continue
		}
		ids = append(ids, sub.Id)
		if sub.RemainingQuota <= 0 {
			continue
		}
		unit := strings.TrimSpace(sub.BillingUnit)
		if unit == "" {
			unit = UserSubscriptionBillingUnitQuota
		}
		switch unit {
		case UserSubscriptionBillingUnitTokens:
			tokensTotal += sub.RemainingQuota
		case UserSubscriptionBillingUnitQuota:
			quotaTotal += sub.RemainingQuota
		default:
			return errors.New("billing_unit 无效")
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Model(&UserSubscription{}).Where("id IN ?", ids).Update("credited", false).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if quotaTotal > 0 {
		updates["quota"] = gorm.Expr("CASE WHEN quota >= ? THEN quota - ? ELSE 0 END", quotaTotal, quotaTotal)
		updates["redeem_quota"] = gorm.Expr("CASE WHEN redeem_quota >= ? THEN redeem_quota - ? ELSE 0 END", quotaTotal, quotaTotal)
	}
	if tokensTotal > 0 {
		updates["tokens_quota"] = gorm.Expr("CASE WHEN tokens_quota >= ? THEN tokens_quota - ? ELSE 0 END", tokensTotal, tokensTotal)
	}
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&User{}).Where("id = ?", userId).Updates(updates).Error
}

func activateDueSubscriptions(tx *gorm.DB, userId int, now int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "billing_unit", "total_quota", "remaining_quota").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (total_quota = 0 OR remaining_quota > 0)", userId, false, now, now).
		Find(&subs).Error; err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	ids := make([]int, 0, len(subs))
	quotaTotal := 0
	tokensTotal := 0
	for _, sub := range subs {
		if sub.Id <= 0 {
			continue
		}
		ids = append(ids, sub.Id)
		if sub.RemainingQuota <= 0 {
			continue
		}
		unit := strings.TrimSpace(sub.BillingUnit)
		if unit == "" {
			unit = UserSubscriptionBillingUnitQuota
		}
		switch unit {
		case UserSubscriptionBillingUnitTokens:
			tokensTotal += sub.RemainingQuota
		case UserSubscriptionBillingUnitQuota:
			quotaTotal += sub.RemainingQuota
		default:
			return errors.New("billing_unit 无效")
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Model(&UserSubscription{}).Where("id IN ?", ids).Update("credited", true).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if quotaTotal > 0 {
		updates["quota"] = gorm.Expr("quota + ?", quotaTotal)
		updates["redeem_quota"] = gorm.Expr("redeem_quota + ?", quotaTotal)
	}
	if tokensTotal > 0 {
		updates["tokens_quota"] = gorm.Expr("tokens_quota + ?", tokensTotal)
		updates["tokens_history_quota"] = gorm.Expr("tokens_history_quota + ?", tokensTotal)
	}
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&User{}).Where("id = ?", userId).Updates(updates).Error
}

// ensureUserSubscriptionsFresh 会懒惰清理已过期的订阅，并刷新用户的订阅汇总字段。
func ensureUserSubscriptionsFresh(tx *gorm.DB, userId int) error {
	now := time.Now().Unix()
	if err := ensureUserSubscriptionsActiveAt(tx, userId, now); err != nil {
		return err
	}
	if err := ensureLegacyQuotaSubscriptionsMigratedAt(tx, userId, now); err != nil {
		return err
	}
	return refreshUserSubscriptionSnapshot(tx, userId, now)
}

func ensureUserSubscriptionsActiveAt(tx *gorm.DB, userId int, now int64) error {
	if err := purgeExpiredSubscriptions(tx, userId, now); err != nil {
		return err
	}
	if err := deactivateNotStartedSubscriptions(tx, userId, now); err != nil {
		return err
	}
	return activateDueSubscriptions(tx, userId, now)
}

// ensureUserQuotaSubscriptionsReadyForConsume keeps subscription state consistent (purge/activate/deactivate and
// legacy migration) but intentionally skips refreshUserSubscriptionSnapshot. Hot paths should update user snapshot
// fields (e.g. redeem_quota) with deltas instead of scanning every time.
func ensureUserQuotaSubscriptionsReadyForConsume(tx *gorm.DB, userId int, now int64) error {
	if err := ensureUserSubscriptionsActiveAt(tx, userId, now); err != nil {
		return err
	}
	return ensureLegacyQuotaSubscriptionsMigratedAt(tx, userId, now)
}

func ensureLegacyQuotaSubscriptionsMigratedAt(tx *gorm.DB, userId int, now int64) error {
	var legacy struct {
		Id                  int
		RedeemQuota         int
		RedeemQuotaExpireAt int64
		DailyQuotaLimit     int
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Model(&User{}).
		Where("id = ?", userId).
		Select("id", "redeem_quota", "redeem_quota_expire_at", "daily_quota_limit").
		First(&legacy).Error; err != nil {
		return err
	}

	var subCount int64
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, UserSubscriptionBillingUnitQuota).
		Count(&subCount).Error; err != nil {
		return err
	}
	if subCount != 0 || legacy.RedeemQuota <= 0 {
		return nil
	}
	if legacy.RedeemQuotaExpireAt > 0 && legacy.RedeemQuotaExpireAt < now {
		if err := tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
			"quota":                  gorm.Expr("CASE WHEN quota >= ? THEN quota - ? ELSE 0 END", legacy.RedeemQuota, legacy.RedeemQuota),
			"redeem_quota":           0,
			"redeem_quota_expire_at": 0,
		}).Error; err != nil {
			return err
		}
	} else {
		sub := &UserSubscription{
			UserId:              userId,
			BillingUnit:         UserSubscriptionBillingUnitQuota,
			TotalQuota:          legacy.RedeemQuota,
			RemainingQuota:      legacy.RedeemQuota,
			DailyQuotaLimit:     legacy.DailyQuotaLimit,
			DailyQuotaUsed:      0,
			DailyQuotaResetDate: common.GetTodayDateInt(),
			StartAt:             now,
			ExpireAt:            legacy.RedeemQuotaExpireAt,
			Credited:            true,
			Source:              "legacy",
			}
			if err := tx.Create(sub).Error; err != nil {
				return err
			}
			legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
			if err != nil {
				return err
			}
			if err := upsertUserSubscriptionGroupsTx(tx, sub.Id, []int{legacyDefaultModelGroupID}); err != nil {
				return err
			}
		}
		return nil
	}

// purgeExpiredSubscriptions 清理已到期但仍有余额的订阅额度。
func purgeExpiredSubscriptions(tx *gorm.DB, userId int, now int64) error {
	var expiredSubs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND expire_at > 0 AND expire_at < ? AND remaining_quota > 0", userId, now).
		Find(&expiredSubs).Error; err != nil {
		return err
	}
	if len(expiredSubs) == 0 {
		return nil
	}
	expiredQuotaDeduct := 0
	expiredTokensDeduct := 0
	for _, sub := range expiredSubs {
		if sub.Credited {
			unit := strings.TrimSpace(sub.BillingUnit)
			if unit == "" {
				unit = UserSubscriptionBillingUnitQuota
			}
			switch unit {
			case UserSubscriptionBillingUnitTokens:
				expiredTokensDeduct += sub.RemainingQuota
			case UserSubscriptionBillingUnitQuota:
				expiredQuotaDeduct += sub.RemainingQuota
			default:
				return errors.New("billing_unit 无效")
			}
		}
		updates := map[string]interface{}{
			"remaining_quota":        0,
			"daily_quota_used":       0,
			"daily_quota_reset_date": 0,
		}
		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
			return err
		}
	}
	updates := map[string]interface{}{}
	if expiredQuotaDeduct > 0 {
		updates["quota"] = gorm.Expr("CASE WHEN quota >= ? THEN quota - ? ELSE 0 END", expiredQuotaDeduct, expiredQuotaDeduct)
		updates["redeem_quota"] = gorm.Expr("CASE WHEN redeem_quota >= ? THEN redeem_quota - ? ELSE 0 END", expiredQuotaDeduct, expiredQuotaDeduct)
	}
	if expiredTokensDeduct > 0 {
		updates["tokens_quota"] = gorm.Expr("CASE WHEN tokens_quota >= ? THEN tokens_quota - ? ELSE 0 END", expiredTokensDeduct, expiredTokensDeduct)
	}
	if len(updates) > 0 {
		if err := tx.Model(&User{}).Where("id = ?", userId).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// refreshUserSubscriptionSnapshot 重新汇总订阅额度，并写回用户快照字段。
func refreshUserSubscriptionSnapshot(tx *gorm.DB, userId int, now int64) error {
	var snapshot struct {
		Total int
		Min   *int64
	}
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND credited = ? AND remaining_quota > 0 AND invalid_at = 0 AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Select("COALESCE(SUM(remaining_quota),0) AS total, MIN(CASE WHEN expire_at > 0 THEN expire_at END) AS min").
		Scan(&snapshot).Error; err != nil {
		return err
	}
	targetRedeemQuota := snapshot.Total
	targetExpireAt := int64(0)
	if snapshot.Min != nil && *snapshot.Min > now {
		targetExpireAt = *snapshot.Min
	}

	// Optimization: avoid redundant UPDATEs on every authenticated GET.
	// This keeps behavior identical while greatly reducing SQLite write-lock contention.
	var current struct {
		RedeemQuota         int   `gorm:"column:redeem_quota"`
		RedeemQuotaExpireAt int64 `gorm:"column:redeem_quota_expire_at"`
	}
	if err := tx.Model(&User{}).
		Where("id = ?", userId).
		Select("redeem_quota", "redeem_quota_expire_at").
		First(&current).Error; err != nil {
		return err
	}
	if current.RedeemQuota == targetRedeemQuota && current.RedeemQuotaExpireAt == targetExpireAt {
		return nil
	}
	return tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
		"redeem_quota":           targetRedeemQuota,
		"redeem_quota_expire_at": targetExpireAt,
	}).Error
}

// BackfillLegacyUserSubscriptionAllowedGroups sets allowed_groups for legacy subscription records that have
// empty/NULL allowed_groups, to avoid being treated as unrestricted.
//
// Current policy: legacy subscriptions are restricted to the effective legacy fallback group.
func BackfillLegacyUserSubscriptionAllowedGroups(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	legacyDefaultModelGroupCode, err := ResolveLegacyDefaultModelGroupCode(tx)
	if err != nil {
		return err
	}
	allowedGroups, err := MarshalGroupNamesJSON([]string{legacyDefaultModelGroupCode})
	if err != nil {
		return err
	}
	query := tx.Model(&UserSubscription{})
	query = query.Where(jsonColumnIsEmptyCondition("allowed_groups"))
	return query.Update("allowed_groups", allowedGroups).Error
}

// createUserSubscription 创建一条新的订阅记录，并刷新用户快照，返回最新订阅对象。
// createUserSubscription 创建一条新的订阅记录，并刷新用户快照，返回最新订阅对象。
// remaining 表示该订阅的“初始剩余额度”，会被规范在 [0, quota] 区间内。
func createUserSubscription(tx *gorm.DB, userId int, startAt int64, quota int, remaining int, dailyLimit int, expireAt int64, allowedGroupIDs []int, billingUnit string, source string, srcRef UserSubscriptionSourceRef) (*UserSubscription, error) {
	if quota < 0 {
		return nil, errors.New("subscription quota 不能小于0")
	}
	billingUnit, err := normalizeUserSubscriptionBillingUnit(billingUnit)
	if err != nil {
		return nil, err
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
	// 规范初始剩余额度：不为负，且不超过总额度
	if remaining < 0 {
		remaining = 0
	}
	if remaining > quota {
		remaining = quota
	}
	if billingUnit == UserSubscriptionBillingUnitTokens {
		quota = discreteUnitsFromDisplayInt(quota)
		remaining = discreteUnitsFromDisplayInt(remaining)
		dailyLimit = discreteUnitsFromDisplayInt(dailyLimit)
	}
	now := time.Now().Unix()
	credited := startAt <= now && (expireAt == 0 || expireAt >= now)

	ids := normalizeUniqueSortedIDs(allowedGroupIDs)
	if srcRef.PresetId > 0 {
		// Preset-backed subscriptions should derive allowed groups from the bound preset revision
		// (or legacy preset fallback), not from this deprecated JSON field.
		ids = nil
	}
	if srcRef.PresetId == 0 {
		if len(ids) == 0 {
			// Legacy-only compatibility: historical empty allowed_groups means legacy fallback group only.
			if strings.TrimSpace(source) == "legacy" {
				legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
				if err != nil {
					return nil, err
				}
				ids = []int{legacyDefaultModelGroupID}
			} else {
				return nil, errors.New("请选择可用分组")
			}
		}
		if err := ValidateGroupIDsExist(tx, ids); err != nil {
			return nil, err
		}
	}

	sub := &UserSubscription{
		UserId:              userId,
		BillingUnit:         billingUnit,
		TotalQuota:          quota,
		RemainingQuota:      remaining,
		DailyQuotaLimit:     dailyLimit,
		DailyQuotaUsed:      0,
		DailyQuotaResetDate: common.GetTodayDateInt(),
		StartAt:             startAt,
		ExpireAt:            expireAt,
		AllowedGroups:       nil,
		SourceOrderId:       srcRef.OrderId,
		SourcePresetId:      srcRef.PresetId,
		SourceRedemptionId:  srcRef.RedemptionId,
		Source:              source,
		Credited:            credited,
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	if srcRef.PresetId > 0 && srcRef.PresetRevisionId > 0 {
		if err := upsertUserSubscriptionPresetRevisionBindingTx(tx, sub.Id, srcRef.PresetId, srcRef.PresetRevisionId); err != nil {
			return nil, err
		}
	}
	// The `credited` flag is computed by business logic above. Persist it explicitly even when it's false.
	// Some gorm versions treat zero-values as "blank" when a `default:true` tag is present, which would
	// incorrectly store credited=true for deferred subscriptions and later break quota snapshots.
	if sub.Credited != credited {
		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Update("credited", credited).Error; err != nil {
			return nil, err
		}
		sub.Credited = credited
	}
	if len(ids) > 0 {
		if err := upsertUserSubscriptionGroupsTx(tx, sub.Id, ids); err != nil {
			return nil, err
		}
	}

	if credited && remaining > 0 {
		switch billingUnit {
		case UserSubscriptionBillingUnitTokens:
			if err := tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
				"tokens_quota":         gorm.Expr("tokens_quota + ?", remaining),
				"tokens_history_quota": gorm.Expr("tokens_history_quota + ?", remaining),
			}).Error; err != nil {
				return nil, err
			}
		case UserSubscriptionBillingUnitQuota:
			// 注意：仅将“初始剩余额度”计入用户总额度，避免把“已消耗部分”挪到自由额度
			if err := tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
				"quota": gorm.Expr("quota + ?", remaining),
			}).Error; err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("billing_unit 无效")
		}
	}

	if err := refreshUserSubscriptionSnapshot(tx, userId, time.Now().Unix()); err != nil {
		return nil, err
	}
	return sub, nil
}

// CreateUserSubscription 在一个新的事务中为用户新增订阅额度，并返回最新订阅详情。
// CreateUserSubscription 在一个新的事务中为用户新增订阅额度，并返回最新订阅详情。
func CreateUserSubscription(userId int, startAt int64, quota int, remaining int, dailyLimit int, expireAt int64, allowedGroupIDs []int, groupDailyLimits []GroupDailyQuotaLimit, source string) (*UserSubscription, error) {
	var result *UserSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}

		ids := normalizeUniqueSortedIDs(allowedGroupIDs)
		if len(ids) == 0 {
			return errors.New("请选择可用分组")
		}
		if err := ValidateGroupIDsExist(tx, ids); err != nil {
			return err
		}

		groupDailyLimitsProvided := groupDailyLimits != nil
		groupLimitByID := map[int]int(nil)
		if groupDailyLimitsProvided {
			normalized, err := normalizeGroupDailyQuotaLimits(groupDailyLimits)
			if err != nil {
				return err
			}

			if len(normalized) > 0 {
				derivedDailyLimit := 0
				hasUnlimited := false
				allowedSet := make(map[int]struct{}, len(ids))
				for _, gid := range ids {
					allowedSet[gid] = struct{}{}
				}
				limitSet := make(map[int]struct{}, len(normalized))
				for _, item := range normalized {
					if _, ok := allowedSet[item.GroupId]; !ok {
						return errors.New("分组日限额包含未授权分组")
					}
					limitSet[item.GroupId] = struct{}{}
					if item.DailyQuotaLimit == 0 {
						hasUnlimited = true
					} else if !hasUnlimited {
						derivedDailyLimit += item.DailyQuotaLimit
					}
				}
				if len(limitSet) != len(allowedSet) {
					return errors.New("分组日限额必须覆盖所有可用分组")
				}
				if hasUnlimited {
					derivedDailyLimit = 0
				}

				groupLimitByID = make(map[int]int, len(normalized))
				dailyLimit = derivedDailyLimit
				for _, item := range normalized {
					groupLimitByID[item.GroupId] = item.DailyQuotaLimit
				}
			} else {
				// Explicitly clear (disable group-daily-limit mode).
				groupLimitByID = map[int]int{}
			}
		}

		sub, err := createUserSubscription(tx, userId, startAt, quota, remaining, dailyLimit, expireAt, ids, UserSubscriptionBillingUnitQuota, source, UserSubscriptionSourceRef{})
		if err != nil {
			return err
		}
		if groupDailyLimitsProvided {
			if err := upsertUserSubscriptionGroupDailyLimitsTx(tx, sub.Id, groupLimitByID); err != nil {
				return err
			}
		}
		result = sub
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CreateUserSubscriptionTx 在给定事务中为用户新增订阅额度，并返回最新订阅详情。
// 注意：调用方负责提交/回滚事务。
func CreateUserSubscriptionTx(tx *gorm.DB, userId int, startAt int64, quota int, remaining int, dailyLimit int, expireAt int64, allowedGroupIDs []int, source string, srcRef UserSubscriptionSourceRef) (*UserSubscription, error) {
	return CreateUserSubscriptionTxWithBillingUnit(tx, userId, startAt, quota, remaining, dailyLimit, expireAt, allowedGroupIDs, UserSubscriptionBillingUnitQuota, source, srcRef)
}

func CreateUserSubscriptionTxWithBillingUnit(tx *gorm.DB, userId int, startAt int64, quota int, remaining int, dailyLimit int, expireAt int64, allowedGroupIDs []int, billingUnit string, source string, srcRef UserSubscriptionSourceRef) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx 为空")
	}
	if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
		return nil, err
	}
	return createUserSubscription(tx, userId, startAt, quota, remaining, dailyLimit, expireAt, allowedGroupIDs, billingUnit, source, srcRef)
}

func GetUserSubscriptionMaxExpireAt(tx *gorm.DB, userId int, now int64) (int64, error) {
	return GetUserSubscriptionMaxExpireAtWithBillingUnit(tx, userId, now, UserSubscriptionBillingUnitQuota)
}

func GetUserSubscriptionMaxExpireAtWithBillingUnit(tx *gorm.DB, userId int, now int64, billingUnit string) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	billingUnit, err := normalizeUserSubscriptionBillingUnit(billingUnit)
	if err != nil {
		return 0, err
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
		return 0, err
	}
	var maxExpire sql.NullInt64
	query := tx.Model(&UserSubscription{})
	if billingUnit == UserSubscriptionBillingUnitTokens {
		query = query.Where("billing_unit = ?", UserSubscriptionBillingUnitTokens)
	} else {
		query = query.Where("(billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", UserSubscriptionBillingUnitQuota)
	}
	if err := query.
		// 顺延仅参考“有限期订阅”的最大到期时间；忽略 expire_at=0（不限时）以及 year 9999 这种“伪不限时”的历史数据，
		// 以免把新购买的订阅顺延到永远之后。
		// 注意：这里的顺延基准是“未到期的订阅周期”，不应依赖 remaining_quota/invalid_at（额度可能已用尽但订阅仍未到期）。
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

// UpdateUserSubscriptionParams 描述允许修改的订阅字段。
type UpdateUserSubscriptionParams struct {
	TotalQuota       *int
	RemainingQuota   *int
	DailyQuotaLimit  *int
	StartAt          *int64
	ExpireAt         *int64
	AllowedGroupIds  *[]int
	GroupDailyLimits *[]GroupDailyQuotaLimit
}

func shiftPendingSubscriptionWindowToNow(startAt int64, expireAt int64, now int64) (int64, int64, error) {
	if now <= 0 || now > common.MaxSupportedUnixTimestamp {
		return 0, 0, errors.New("系统时间异常")
	}
	if startAt <= 0 || startAt <= now {
		return 0, 0, errors.New("订阅已生效")
	}
	if expireAt <= 0 {
		return now, 0, nil
	}
	if expireAt <= startAt {
		return 0, 0, errors.New("订阅有效期无效")
	}
	shift := startAt - now
	nextExpireAt := expireAt - shift
	if nextExpireAt <= now {
		return 0, 0, errors.New("订阅有效期无效")
	}
	return now, nextExpireAt, nil
}

func applyUserSubscriptionBalanceDeltaTx(tx *gorm.DB, userId int, billingUnit string, delta int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if delta == 0 {
		return nil
	}
	unit := strings.TrimSpace(billingUnit)
	if unit == "" {
		unit = UserSubscriptionBillingUnitQuota
	}
	updates := map[string]interface{}{}
	switch unit {
	case UserSubscriptionBillingUnitQuota:
		updates["quota"] = gorm.Expr("CASE WHEN quota + ? >= 0 THEN quota + ? ELSE 0 END", delta, delta)
	case UserSubscriptionBillingUnitTokens:
		updates["tokens_quota"] = gorm.Expr("CASE WHEN tokens_quota + ? >= 0 THEN tokens_quota + ? ELSE 0 END", delta, delta)
		if delta > 0 {
			updates["tokens_history_quota"] = gorm.Expr("tokens_history_quota + ?", delta)
		}
	default:
		return errors.New("billing_unit 无效")
	}
	return tx.Model(&User{}).Where("id = ?", userId).Updates(updates).Error
}

func updateUserSubscription(tx *gorm.DB, userId int, subId int, params UpdateUserSubscriptionParams) (*UserSubscription, error) {
	var sub UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", subId, userId).First(&sub).Error; err != nil {
		return nil, err
	}
	if params.AllowedGroupIds != nil && sub.SourcePresetId > 0 {
		return nil, errors.New("该订阅来源于商品，可用分组由商品控制，请修改商品配置")
	}
	if sub.BillingUnit == UserSubscriptionBillingUnitTokens {
		if params.TotalQuota != nil {
			scaled := discreteUnitsFromDisplayInt(*params.TotalQuota)
			params.TotalQuota = &scaled
		}
		if params.RemainingQuota != nil {
			scaled := discreteUnitsFromDisplayInt(*params.RemainingQuota)
			params.RemainingQuota = &scaled
		}
		if params.DailyQuotaLimit != nil {
			scaled := discreteUnitsFromDisplayInt(*params.DailyQuotaLimit)
			params.DailyQuotaLimit = &scaled
		}
		if params.GroupDailyLimits != nil {
			scaled := scaleGroupDailyLimitsToStored(*params.GroupDailyLimits)
			params.GroupDailyLimits = &scaled
		}
	}

	groupDailyLimitsProvided := params.GroupDailyLimits != nil
	groupLimitByID := map[int]int(nil)
	dailyLimitOverride := (*int)(nil)
	if groupDailyLimitsProvided {
		if sub.SourcePresetId > 0 {
			presetControls, err := currentPresetControlsUserSubscriptionSnapshotTx(tx, sub)
			if err != nil {
				return nil, err
			}
			if presetControls {
				return nil, errors.New("该订阅来源于商品，分组日限额由商品控制，请修改商品配置")
			}
		}
		if sub.SourceRedemptionId > 0 {
			return nil, errors.New("该订阅来源于兑换码，分组日限额由兑换码控制")
		}
		normalized, err := normalizeGroupDailyQuotaLimits(*params.GroupDailyLimits)
		if err != nil {
			return nil, err
		}
		if len(normalized) == 0 {
			groupLimitByID = map[int]int{}
		} else {
			var allowedIDs []int
			if params.AllowedGroupIds != nil {
				allowedIDs = normalizeUniqueSortedIDs(*params.AllowedGroupIds)
			} else {
				ids, err := getUserSubscriptionGroupIDsTx(tx, sub.Id)
				if err != nil {
					return nil, err
				}
				allowedIDs = ids
			}
			if len(allowedIDs) == 0 {
				return nil, errors.New("请选择可用分组")
			}

			hasUnlimited := false
			sum := 0
			allowedSet := make(map[int]struct{}, len(allowedIDs))
			for _, gid := range allowedIDs {
				allowedSet[gid] = struct{}{}
			}
			limitSet := make(map[int]struct{}, len(normalized))
			for _, item := range normalized {
				if _, ok := allowedSet[item.GroupId]; !ok {
					return nil, errors.New("分组日限额包含未授权分组")
				}
				limitSet[item.GroupId] = struct{}{}
				if item.DailyQuotaLimit == 0 {
					hasUnlimited = true
				} else if !hasUnlimited {
					sum += item.DailyQuotaLimit
				}
			}
			if len(limitSet) != len(allowedSet) {
				return nil, errors.New("分组日限额必须覆盖所有可用分组")
			}
			if hasUnlimited {
				sum = 0
			}

			groupLimitByID = make(map[int]int, len(normalized))
			for _, item := range normalized {
				groupLimitByID[item.GroupId] = item.DailyQuotaLimit
			}
			dailyLimitOverride = &sum
		}
	} else if params.AllowedGroupIds != nil {
		has, err := hasUserSubscriptionGroupDailyLimitsTx(tx, sub.Id)
		if err != nil {
			return nil, err
		}
		if has {
			return nil, errors.New("该订阅已启用“分组日限额”，修改可用分组时必须同时提交 group_daily_limits")
		}
	}

	dailyLimitParam := params.DailyQuotaLimit
	if dailyLimitOverride != nil {
		dailyLimitParam = dailyLimitOverride
	}

	updates := map[string]interface{}{}
	nextAllowedGroupIDs := []int(nil)
	newTotal := sub.TotalQuota
	if params.TotalQuota != nil {
		if *params.TotalQuota <= 0 {
			return nil, errors.New("subscription quota 必须大于0")
		}
		newTotal = *params.TotalQuota
		updates["total_quota"] = newTotal
	}

	newRemaining := sub.RemainingQuota
	if params.RemainingQuota != nil {
		if *params.RemainingQuota < 0 {
			return nil, errors.New("remaining quota 不能为负数")
		}
		newRemaining = *params.RemainingQuota
	}
	if newRemaining > newTotal {
		newRemaining = newTotal
	}

	newDailyLimit := sub.DailyQuotaLimit
	if dailyLimitParam != nil {
		if *dailyLimitParam < 0 {
			return nil, errors.New("daily quota limit 不能为负数")
		}
		newDailyLimit = *dailyLimitParam
		updates["daily_quota_limit"] = newDailyLimit
	}

	newDailyUsed := sub.DailyQuotaUsed
	newDailyReset := sub.DailyQuotaResetDate
	if dailyLimitParam != nil {
		if newDailyLimit <= 0 {
			newDailyUsed = 0
			newDailyReset = 0
		} else {
			today := common.GetTodayDateInt()
			if newDailyReset != today {
				newDailyUsed = 0
				newDailyReset = today
			}
			if newDailyUsed > newDailyLimit {
				newDailyUsed = newDailyLimit
			}
		}
	}

	now := time.Now().Unix()
	if now <= 0 || now > common.MaxSupportedUnixTimestamp {
		return nil, errors.New("系统时间异常")
	}

	newStartAt := sub.StartAt
	if params.StartAt != nil {
		if *params.StartAt <= 0 {
			return nil, errors.New("start_at 无效")
		}
		if *params.StartAt > common.MaxSupportedUnixTimestamp {
			return nil, errors.New("start_at 过大，最大支持到 " + common.MaxSupportedUnixTimestampLabel)
		}
		newStartAt = *params.StartAt
		updates["start_at"] = newStartAt
	}

	newExpireAt := sub.ExpireAt
	if params.ExpireAt != nil {
		if *params.ExpireAt < 0 {
			return nil, errors.New("expire_at 无效")
		}
		if *params.ExpireAt > common.MaxSupportedUnixTimestamp {
			return nil, errors.New("expire_at 过大，最大支持到 " + common.MaxSupportedUnixTimestampLabel)
		}
		newExpireAt = *params.ExpireAt
		updates["expire_at"] = newExpireAt
	}

	startForValidation := newStartAt
	if startForValidation <= 0 {
		startForValidation = sub.CreatedAt.Unix()
	}
	if newExpireAt > 0 && startForValidation > newExpireAt {
		return nil, errors.New("expire_at 必须晚于 start_at")
	}

	// If subscription is set to already expired, purge remaining quota immediately.
	if newExpireAt > 0 && newExpireAt < now {
		newRemaining = 0
		newDailyUsed = 0
		newDailyReset = 0
	}

	shouldBeCredited := (newStartAt == 0 || newStartAt <= now) && (newExpireAt == 0 || newExpireAt >= now)
	nextCredited := sub.Credited
	if params.StartAt != nil || params.ExpireAt != nil {
		nextCredited = shouldBeCredited
	}
	if nextCredited != sub.Credited {
		updates["credited"] = nextCredited
	}
	if params.AllowedGroupIds != nil {
		ids := normalizeUniqueSortedIDs(*params.AllowedGroupIds)
		if len(ids) == 0 {
			return nil, errors.New("请选择可用分组")
		}
		if err := ValidateGroupIDsExist(tx, ids); err != nil {
			return nil, err
		}
		// Guardrail: when group-daily-limit mode comes from a preset/revision/redemption snapshot,
		// the allowed groups must stay within the configured group set unless the caller also submits
		// a full replacement group_daily_limits payload.
		if !groupDailyLimitsProvided {
			effectiveLimitsBySubID, effectiveSourceBySubID, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx, []UserSubscription{sub})
			if err != nil {
				return nil, err
			}
			limitByGroupID := effectiveLimitsBySubID[sub.Id]
			effectiveSource := effectiveSourceBySubID[sub.Id]
			if len(limitByGroupID) > 0 && effectiveSource.Kind != "subscription" {
				for _, gid := range ids {
					if _, ok := limitByGroupID[gid]; ok {
						continue
					}
					switch effectiveSource.Kind {
					case "preset_revision":
						return nil, fmt.Errorf("该订阅来源于商品 revision 且已启用“分组日限额”，可用分组只能在版本已配置分组范围内调整")
					case "preset":
						return nil, fmt.Errorf("该订阅来源于商品且已启用“分组日限额”，可用分组只能在商品已配置分组范围内调整")
					case "redemption":
						return nil, fmt.Errorf("该订阅来源于兑换码且已启用“分组日限额”，可用分组只能在兑换码已配置分组范围内调整")
					}
				}
			}
		}
		nextAllowedGroupIDs = ids
	}

	if newRemaining != sub.RemainingQuota || params.RemainingQuota != nil || params.TotalQuota != nil {
		updates["remaining_quota"] = newRemaining
	}
	if newDailyUsed != sub.DailyQuotaUsed {
		updates["daily_quota_used"] = newDailyUsed
	}
	if newDailyReset != sub.DailyQuotaResetDate {
		updates["daily_quota_reset_date"] = newDailyReset
	}

	nextInvalidAt := sub.InvalidAt
	if newRemaining > 0 {
		nextInvalidAt = 0
	} else if nextInvalidAt <= 0 {
		nextInvalidAt = now
	}
	if nextInvalidAt != sub.InvalidAt {
		updates["invalid_at"] = nextInvalidAt
	}

	shouldUpsertGroups := nextAllowedGroupIDs != nil
	if len(updates) == 0 && !groupDailyLimitsProvided && !shouldUpsertGroups {
		return &sub, nil
	}

	updated := sub
	if len(updates) > 0 {
		oldCreditedAmount := 0
		if sub.Credited && sub.RemainingQuota > 0 {
			oldCreditedAmount = sub.RemainingQuota
		}
		newCreditedAmount := 0
		if nextCredited && newRemaining > 0 {
			newCreditedAmount = newRemaining
		}
		deltaQuota := newCreditedAmount - oldCreditedAmount
		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
			return nil, err
		}

		if deltaQuota != 0 {
			if err := applyUserSubscriptionBalanceDeltaTx(tx, userId, sub.BillingUnit, deltaQuota); err != nil {
				return nil, err
			}
		}

		if err := refreshUserSubscriptionSnapshot(tx, userId, now); err != nil {
			return nil, err
		}

		if err := tx.Where("id = ?", sub.Id).First(&updated).Error; err != nil {
			return nil, err
		}
	}

	if shouldUpsertGroups {
		if err := upsertUserSubscriptionGroupsTx(tx, sub.Id, nextAllowedGroupIDs); err != nil {
			return nil, err
		}
	}

	if groupDailyLimitsProvided {
		if err := upsertUserSubscriptionGroupDailyLimitsTx(tx, sub.Id, groupLimitByID); err != nil {
			return nil, err
		}
	}

	return &updated, nil
}

// UpdateUserSubscription 允许管理员调整指定订阅的额度属性。
func UpdateUserSubscription(userId int, subId int, params UpdateUserSubscriptionParams) (*UserSubscription, error) {
	var result *UserSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}
		sub, err := updateUserSubscription(tx, userId, subId, params)
		if err != nil {
			return err
		}
		result = sub
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ActivatePendingUserSubscription(userId int, subId int) (*UserSubscription, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if subId <= 0 {
		return nil, errors.New("subId 无效")
	}

	var result *UserSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}

		now := time.Now().Unix()
		var sub UserSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", subId, userId).
			First(&sub).Error; err != nil {
			return err
		}
		if sub.InvalidAt > 0 || (sub.ExpireAt > 0 && sub.ExpireAt < now) {
			return errors.New("订阅已失效")
		}
		if sub.StartAt <= 0 || sub.StartAt <= now || sub.Credited {
			return errors.New("订阅已生效")
		}

		nextStartAt, nextExpireAt, err := shiftPendingSubscriptionWindowToNow(sub.StartAt, sub.ExpireAt, now)
		if err != nil {
			return err
		}
		updated, err := updateUserSubscription(tx, userId, subId, UpdateUserSubscriptionParams{
			StartAt:  &nextStartAt,
			ExpireAt: &nextExpireAt,
		})
		if err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func deleteUserSubscription(tx *gorm.DB, userId int, subId int) error {
	var sub UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", subId, userId).First(&sub).Error; err != nil {
		return err
	}
	if sub.Credited && sub.RemainingQuota > 0 {
		if err := applyUserSubscriptionBalanceDeltaTx(tx, userId, sub.BillingUnit, -sub.RemainingQuota); err != nil {
			return err
		}
	}
	// Cleanup per-subscription per-group records to avoid orphaned rows.
	if tx.Migrator().HasTable(&UserSubscriptionGroup{}) {
		if err := tx.Where("subscription_id = ?", sub.Id).Delete(&UserSubscriptionGroup{}).Error; err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&UserSubscriptionGroupDailyLimit{}) {
		if err := tx.Where("subscription_id = ?", sub.Id).Delete(&UserSubscriptionGroupDailyLimit{}).Error; err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&UserSubscriptionGroupDailyUsage{}) {
		if err := tx.Where("subscription_id = ?", sub.Id).Delete(&UserSubscriptionGroupDailyUsage{}).Error; err != nil {
			return err
		}
	}
	if err := tx.Delete(&sub).Error; err != nil {
		return err
	}
	return refreshUserSubscriptionSnapshot(tx, userId, time.Now().Unix())
}

// DeleteUserSubscription 删除指定订阅并回收对应剩余额度。
func DeleteUserSubscription(userId int, subId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}
		return deleteUserSubscription(tx, userId, subId)
	})
}

func ReorderUserSubscriptions(userId int, orderedSubIDs []int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	ids := normalizeOrderedUniquePositiveIDs(orderedSubIDs)
	if len(ids) == 0 {
		return errors.New("subscription_ids 不能为空")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}

		var rows []struct {
			Id int `gorm:"column:id"`
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&UserSubscription{}).
			Select("id").
			Where("user_id = ? AND id IN ?", userId, ids).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return errors.New("订阅记录不存在")
		}

		caseSQL := "CASE id"
		args := make([]interface{}, 0, len(ids)*2)
		for idx, id := range ids {
			caseSQL += " WHEN ? THEN ?"
			args = append(args, id, len(ids)-idx)
		}
		caseSQL += " ELSE sort_order END"

		return tx.Model(&UserSubscription{}).
			Where("user_id = ? AND id IN ?", userId, ids).
			Update("sort_order", gorm.Expr(caseSQL, args...)).Error
	})
}

// consumeQuotaFromSubscriptions consumes subscription quota ordered by expiry.
//
// Returns:
// - covered: total quota covered by subscriptions (finite + unlimited-total)
// - deducted: quota deducted from finite subscriptions (affects users.quota)
func consumeQuotaFromSubscriptions(tx *gorm.DB, userId int, required int) (covered int, deducted int, err error) {
	if required <= 0 {
		return 0, 0, nil
	}
	now := time.Now().Unix()
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Order(userSubscriptionActiveOrderExpr).
		Find(&subs).Error; err != nil {
		return 0, 0, err
	}
	if len(subs) == 0 {
		return 0, 0, nil
	}
	today := common.GetTodayDateInt()
	covered = 0
	deducted = 0
	left := required
	for _, sub := range subs {
		if left <= 0 {
			break
		}
		unlimitedTotal := sub.TotalQuota == 0
		usable := sub.RemainingQuota
		if unlimitedTotal {
			usable = left
		} else if usable <= 0 {
			continue
		}

		// Daily limit handling (for unlimited-total subscriptions we still track daily usage for refunds/reporting).
		used := sub.DailyQuotaUsed
		reset := sub.DailyQuotaResetDate
		if reset != today {
			used = 0
			reset = today
		}

		if sub.DailyQuotaLimit > 0 {
			remainToday := sub.DailyQuotaLimit - used
			if remainToday <= 0 {
				if sub.DailyQuotaResetDate != reset {
					if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(map[string]interface{}{
						"daily_quota_used":       used,
						"daily_quota_reset_date": reset,
					}).Error; err != nil {
						return covered, deducted, err
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

		updates := map[string]interface{}{
			"invalid_at": 0,
		}
		if unlimitedTotal {
			updates["daily_quota_used"] = used + usable
			updates["daily_quota_reset_date"] = reset
		} else {
			afterRemaining := sub.RemainingQuota - usable
			updates["remaining_quota"] = gorm.Expr("remaining_quota - ?", usable)
			if afterRemaining <= 0 {
				updates["invalid_at"] = now
			}
			if sub.DailyQuotaLimit > 0 {
				updates["daily_quota_used"] = used + usable
				updates["daily_quota_reset_date"] = reset
			}
		}
		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
			return covered, deducted, err
		}
		covered += usable
		if !unlimitedTotal {
			deducted += usable
		}
		left -= usable
	}
	if deducted > 0 {
		// Caller must update user snapshot fields (redeem_quota/redeem_quota_expire_at) to avoid a full scan here.
	}
	return covered, deducted, nil
}

// consumeQuotaFromSubscriptionsByGroup consumes subscription quota restricted by allowed_group_ids.
//
// Returns:
// - covered: total quota covered by subscriptions (finite + unlimited-total)
// - deducted: quota deducted from finite subscriptions (affects users.quota)
func consumeQuotaFromSubscriptionsByGroup(tx *gorm.DB, userId int, required int, groupID int) (covered int, deducted int, err error) {
	covered, deducted, _, err = consumeQuotaFromSubscriptionsByGroupWithAllocations(tx, userId, required, groupID)
	return covered, deducted, err
}

func consumeQuotaFromSubscriptionsByGroupWithAllocations(tx *gorm.DB, userId int, required int, groupID int) (covered int, deducted int, allocations []relaycommon.SubscriptionUnitAllocation, err error) {
	if required <= 0 {
		return 0, 0, nil, nil
	}
	if groupID <= 0 {
		return 0, 0, nil, errors.New("group_id 无效")
	}
	now := time.Now().Unix()
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "source_preset_id", "source_redemption_id").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Order(userSubscriptionActiveOrderExpr).
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
			// Keep legacy aggregate fields in sync, so switching back to non-group daily limits
			// within the same day still behaves correctly.
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
			// Unlimited-total subscriptions still track daily usage for refunds/reporting even when daily_quota_limit==0.
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

	if deducted > 0 {
		if err := refreshUserSubscriptionSnapshot(tx, userId, time.Now().Unix()); err != nil {
			return covered, deducted, nil, err
		}
	}
	return covered, deducted, allocations, nil
}

// restoreQuotaToSubscriptions restores previously consumed quota back into the user's subscriptions.
//
// Returns:
// - restored: total restored quota (finite + unlimited-total usage rollback)
// - restoredDeducted: restored quota that should be credited back to users.quota (finite subscriptions)
func restoreQuotaToSubscriptions(tx *gorm.DB, userId int, quota int) (restored int, restoredDeducted int, err error) {
	if quota <= 0 {
		return 0, 0, nil
	}
	now := time.Now().Unix()
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "start_at", "expire_at").
		Where("user_id = ? AND credited = ? AND ((total_quota > 0 AND remaining_quota < total_quota) OR total_quota = 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Order(userSubscriptionActiveOrderExpr).
		Find(&subs).Error; err != nil {
		return 0, 0, err
	}
	if len(subs) == 0 {
		return 0, 0, nil
	}
	left := quota
	today := common.GetTodayDateInt()
	restored = 0
	restoredDeducted = 0
	for _, sub := range subs {
		if left <= 0 {
			break
		}
		unlimitedTotal := sub.TotalQuota == 0
		if unlimitedTotal {
			used := sub.DailyQuotaUsed
			reset := sub.DailyQuotaResetDate
			if reset != today {
				used = 0
				reset = today
			}
			if used <= 0 {
				continue
			}
			add := used
			if add > left {
				add = left
			}
			nextUsed := used - add
			if nextUsed < 0 {
				nextUsed = 0
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(map[string]interface{}{
				"daily_quota_used":       nextUsed,
				"daily_quota_reset_date": reset,
			}).Error; err != nil {
				return restored, restoredDeducted, err
			}
			restored += add
			left -= add
			continue
		}

		space := sub.TotalQuota - sub.RemainingQuota
		if space <= 0 {
			continue
		}
		add := space
		if add > left {
			add = left
		}
		updates := map[string]interface{}{
			"remaining_quota": gorm.Expr("remaining_quota + ?", add),
			"invalid_at":      0,
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
		}
		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
			return restored, restoredDeducted, err
		}
		restored += add
		restoredDeducted += add
		left -= add
	}
	if restoredDeducted > 0 {
		// Caller must update user snapshot fields (redeem_quota/redeem_quota_expire_at) to avoid a full scan here.
	}
	return restored, restoredDeducted, nil
}

func restoreQuotaToSubscriptionsWithAllocations(tx *gorm.DB, userId int, allocations []relaycommon.SubscriptionUnitAllocation) (restored int, restoredDeducted int, err error) {
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
		Select("id", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "credited", "start_at", "expire_at").
		Where("user_id = ? AND id IN ? AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, subIDs, UserSubscriptionBillingUnitQuota).
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
			return restored, restoredDeducted, fmt.Errorf("订阅 #%d 不可用于归还额度", allocation.SubscriptionId)
		}

		add := allocation.Amount
		unlimitedTotal := sub.TotalQuota == 0
		updates := map[string]interface{}{}
		if !unlimitedTotal {
			space := sub.TotalQuota - sub.RemainingQuota
			if space < add {
				return restored, restoredDeducted, fmt.Errorf("订阅 #%d 可归还额度不足", allocation.SubscriptionId)
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

// restoreQuotaToSubscriptionsByGroup restores quota into subscriptions that allow the given group_id.
//
// Returns:
// - restored: total restored quota (finite + unlimited-total usage rollback)
// - restoredDeducted: restored quota that should be credited back to users.quota (finite subscriptions)
func restoreQuotaToSubscriptionsByGroup(tx *gorm.DB, userId int, quota int, groupID int) (restored int, restoredDeducted int, err error) {
	if quota <= 0 {
		return 0, 0, nil
	}
	if groupID <= 0 {
		return 0, 0, errors.New("group_id 无效")
	}
	now := time.Now().Unix()
	var subs []UserSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "source_preset_id", "source_redemption_id", "start_at", "expire_at").
		Where("user_id = ? AND credited = ? AND ((total_quota > 0 AND remaining_quota < total_quota) OR total_quota = 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Order(userSubscriptionActiveOrderExpr).
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

	effectiveGroupDailyLimitsBySubID, effectiveSourceBySubID, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx, subs)
	if err != nil {
		return 0, 0, err
	}
	statDate := statDateToday()

	// Load today's per-subscription per-group usage for subscriptions that use group daily limits.
	subIDsForUsage := make([]int, 0, len(subs))
	for _, sub := range subs {
		if len(effectiveGroupDailyLimitsBySubID[sub.Id]) > 0 {
			subIDsForUsage = append(subIDsForUsage, sub.Id)
		}
	}
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
	if restoredDeducted > 0 {
		// Caller must update user snapshot fields (redeem_quota/redeem_quota_expire_at) to avoid a full scan here.
	}
	return restored, restoredDeducted, nil
}

// asyncEnsureUserSubscriptionsFresh 提供一个异步刷新能力给缓存场景使用。
func asyncEnsureUserSubscriptionsFresh(userId int) {
	gopool.Go(func() {
		_ = DB.Transaction(func(tx *gorm.DB) error {
			return ensureUserSubscriptionsFresh(tx, userId)
		})
	})
}

// GetUserSubscriptionGroupCandidates returns possible using-groups that can be billed from subscriptions.
func GetUserSubscriptionGroupCandidates(userId int) ([]int, bool, error) {
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
			Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
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

// GetUserSubscriptionCapacityForGroup returns:
// - totalRemaining: sum of remaining_quota across finite subscriptions that allow the group (ignores daily limits)
// - dailyCapacity: sum of today's usable quota across subscriptions that allow the group (applies daily limits)
// - totalUnlimited: whether any unlimited-total subscription allows the group
// - dailyUnlimited: whether today's usable quota is unlimited for the group
func GetUserSubscriptionCapacityForGroup(userId int, groupID int) (totalRemaining int, dailyCapacity int, totalUnlimited bool, dailyUnlimited bool, err error) {
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
		if err := tx.Select("id", "total_quota", "remaining_quota", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date", "source_preset_id", "source_redemption_id").
			Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
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

func GetUserQuotaBreakdown(userId int) (*QuotaBreakdown, error) {
	result := &QuotaBreakdown{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}

		configErrSet := make(map[string]struct{}, 8)
		addConfigErr := func(msg string) {
			msg = strings.TrimSpace(msg)
			if msg == "" {
				return
			}
			if _, ok := configErrSet[msg]; ok {
				return
			}
			configErrSet[msg] = struct{}{}
			result.SubscriptionConfigErrors = append(result.SubscriptionConfigErrors, msg)
		}

		var userSnapshot struct {
			Quota            int
			UsedQuota        int
			RedeemQuota      int
			PaygQuota        int `gorm:"column:payg_quota"`
			PaygHistoryQuota int `gorm:"column:payg_history_quota"`
		}
		if err := tx.Model(&User{}).
			Where("id = ?", userId).
			Select("quota", "used_quota", "redeem_quota", "payg_quota", "payg_history_quota").
			Scan(&userSnapshot).Error; err != nil {
			return err
		}

		now := time.Now().Unix()
		var activeSubs []UserSubscription
		if err := tx.Where("user_id = ? AND credited = ? AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
			Order(userSubscriptionActiveOrderExpr).
			Find(&activeSubs).Error; err != nil {
			return err
		}

		var pendingSubs []UserSubscription
		if err := tx.Where("user_id = ? AND credited = ? AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, false, now, UserSubscriptionBillingUnitQuota).
			Order(userSubscriptionPendingOrderExpr).
			Find(&pendingSubs).Error; err != nil {
			return err
		}

		// 订阅历史：包含已过期订阅（用于前端展示“已过期”）
		var expiredSubs []UserSubscription
		if err := tx.Where("user_id = ? AND expire_at > 0 AND expire_at < ? AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, now, UserSubscriptionBillingUnitQuota).
			Order(userSubscriptionExpiredOrderExpr).
			Limit(20).
			Find(&expiredSubs).Error; err != nil {
			return err
		}

		today := common.GetTodayDateInt()
		activeSummaries := make([]SubscriptionSummary, 0, len(activeSubs))
		allSummaries := make([]SubscriptionSummary, 0, len(activeSubs)+len(pendingSubs)+len(expiredSubs))
		var (
			totalQuota     int
			remainingQuota int
			consumedQuota  int
			dailyUsedSum   int
			dailyLimitSum  int
			dailyUnlimited bool
			windowStart    int64
			windowEnd      int64
		)

		combined := make([]UserSubscription, 0, len(activeSubs)+len(pendingSubs)+len(expiredSubs))
		combined = append(combined, activeSubs...)
		combined = append(combined, pendingSubs...)
		combined = append(combined, expiredSubs...)

		productIDs := make([]int, 0, len(combined))
		redemptionIDs := make([]int, 0, len(combined))
		orderIDs := make([]int, 0, len(combined))
		subIDs := make([]int, 0, len(combined))
		for _, sub := range combined {
			subIDs = append(subIDs, sub.Id)
			if sub.SourceOrderId > 0 {
				orderIDs = append(orderIDs, sub.SourceOrderId)
			}
			if sub.SourcePresetId > 0 {
				productIDs = append(productIDs, sub.SourcePresetId)
			}
			if sub.SourceRedemptionId > 0 {
				redemptionIDs = append(redemptionIDs, sub.SourceRedemptionId)
			}
		}

		presetNameByID := make(map[int]string, 0)
		if normalized := normalizeUniqueSortedIDs(productIDs); len(normalized) > 0 {
			type row struct {
				Id   int    `gorm:"column:id"`
				Name string `gorm:"column:name"`
			}
			var rows []row
			if err := tx.Model(&RedemptionPreset{}).
				Select("id", "name").
				Where("id IN ?", normalized).
				Find(&rows).Error; err != nil {
				return err
			}
			presetNameByID = make(map[int]string, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				name := strings.TrimSpace(r.Name)
				if name == "" {
					continue
				}
				presetNameByID[r.Id] = name
			}
		}

		redemptionNameByID := make(map[int]string, 0)
		if normalized := normalizeUniqueSortedIDs(redemptionIDs); len(normalized) > 0 {
			type row struct {
				Id   int    `gorm:"column:id"`
				Name string `gorm:"column:name"`
			}
			var rows []row
			if err := tx.Model(&Redemption{}).
				Select("id", "name").
				Where("id IN ?", normalized).
				Find(&rows).Error; err != nil {
				return err
			}
			redemptionNameByID = make(map[int]string, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				name := strings.TrimSpace(r.Name)
				if name == "" {
					continue
				}
				redemptionNameByID[r.Id] = name
			}
		}

		orderInfoByID := make(map[int]subscriptionBreakdownOrderInfo, 0)
		if normalized := normalizeUniqueSortedIDs(orderIDs); len(normalized) > 0 {
			type row struct {
				Id       int    `gorm:"column:id"`
				TradeNo  string `gorm:"column:trade_no"`
				Quantity int    `gorm:"column:quantity"`
			}
			var rows []row
			if err := tx.Model(&SubscriptionOrder{}).
				Select("id", "trade_no", "quantity").
				Where("id IN ?", normalized).
				Find(&rows).Error; err != nil {
				return err
			}
			orderInfoByID = make(map[int]subscriptionBreakdownOrderInfo, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				orderInfoByID[r.Id] = subscriptionBreakdownOrderInfo{
					TradeNo:  strings.TrimSpace(r.TradeNo),
					Quantity: r.Quantity,
				}
			}
		}

		allowedGroupIDsBySubID := make(map[int][]int, len(subIDs))
		if len(combined) > 0 {
			resolvedAllowedGroupIDsBySubID, resolveErr := resolveSubscriptionAllowedGroupsTx(tx, combined)
			if resolveErr != nil {
				return resolveErr
			}
			allowedGroupIDsBySubID = resolvedAllowedGroupIDsBySubID
		}
		for _, sub := range combined {
			if sub.Id <= 0 {
				continue
			}
			if len(allowedGroupIDsBySubID[sub.Id]) > 0 {
				continue
			}
			if sub.SourcePresetId > 0 {
				return fmt.Errorf("订阅 #%d 绑定商品 #%d 缺少可用分组", sub.Id, sub.SourcePresetId)
			}
			return fmt.Errorf("订阅 #%d 缺少可用分组", sub.Id)
		}

		effectiveGroupDailyLimitsBySubID, effectiveGroupDailyLimitSourceBySubID, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx, combined)
		if err != nil {
			return err
		}
		groupModeSubIDs := make([]int, 0, len(combined))
		for _, sub := range combined {
			if len(effectiveGroupDailyLimitsBySubID[sub.Id]) > 0 {
				groupModeSubIDs = append(groupModeSubIDs, sub.Id)
			}
		}
		groupModeUsedBySubID, err := sumUserSubscriptionDailyUsedAcrossGroupsTx(tx, groupModeSubIDs, statDateToday())
		if err != nil {
			return err
		}
		effectiveDailyLimitBySubID := buildUserSubscriptionEffectiveDailyLimitBySubID(effectiveGroupDailyLimitsBySubID)

		groupIDSet := make(map[int]struct{}, 16)
		for _, m := range effectiveGroupDailyLimitsBySubID {
			for gid := range m {
				groupIDSet[gid] = struct{}{}
			}
		}
		groupIDs := make([]int, 0, len(groupIDSet))
		for gid := range groupIDSet {
			groupIDs = append(groupIDs, gid)
		}
		groupIDs = normalizeUniqueSortedIDs(groupIDs)
		if len(groupIDs) > 0 {
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return err
			}
		}
		addGroupDailyLimitConfigErr := func(sub UserSubscription, groupID int) {
			addUserSubscriptionGroupDailyLimitConfigErr(addConfigErr, effectiveGroupDailyLimitSourceBySubID, sub, groupID)
		}

		// Build per-group subscription capacities (today usable + total remaining).
		// Clear and reuse groupIDSet
		for k := range groupIDSet {
			delete(groupIDSet, k)
		}
		for _, sub := range activeSubs {
			if sub.Id <= 0 || sub.RemainingQuota <= 0 {
				continue
			}
			for _, gid := range allowedGroupIDsBySubID[sub.Id] {
				if gid <= 0 {
					continue
				}
				groupIDSet[gid] = struct{}{}
			}
		}
		groupIDsForCapacity := make([]int, 0, len(groupIDSet))
		for gid := range groupIDSet {
			groupIDsForCapacity = append(groupIDsForCapacity, gid)
		}
		sort.Ints(groupIDsForCapacity)

		capacityByGroup := make(map[int]*SubscriptionGroupCapacity, len(groupIDsForCapacity))
		for _, gid := range groupIDsForCapacity {
			capacityByGroup[gid] = &SubscriptionGroupCapacity{
				GroupId:        gid,
				TotalRemaining: 0,
				DailyCapacity:  0,
			}
		}

		groupModeActiveSubIDs := make([]int, 0, len(activeSubs))
		for _, sub := range activeSubs {
			if sub.Id <= 0 || sub.RemainingQuota <= 0 {
				continue
			}
			if len(effectiveGroupDailyLimitsBySubID[sub.Id]) > 0 {
				groupModeActiveSubIDs = append(groupModeActiveSubIDs, sub.Id)
			}
		}

		groupModeUsageBySubID, err := getUserSubscriptionGroupDailyUsageBySubIDsTx(tx, groupModeActiveSubIDs, statDateToday())
		if err != nil {
			return err
		}
		buildSummary := func(sub UserSubscription) (SubscriptionSummary, error) {
			allowedGroupIDs := allowedGroupIDsBySubID[sub.Id]
			used, dailyLimit, groupDailyLimits := resolveUserSubscriptionSummaryDailyLimit(sub, today, effectiveGroupDailyLimitsBySubID, effectiveDailyLimitBySubID, groupModeUsedBySubID)
			return buildUserSubscriptionBreakdownSummary(
				sub,
				allowedGroupIDs,
				used,
				dailyLimit,
				groupDailyLimits,
				groupModeUsageBySubID[sub.Id],
				addGroupDailyLimitConfigErr,
				orderInfoByID[sub.SourceOrderId],
				presetNameByID[sub.SourcePresetId],
				redemptionNameByID[sub.SourceRedemptionId],
			)
		}

		for _, sub := range activeSubs {
			remaining := sub.RemainingQuota
			if sub.Id <= 0 || remaining <= 0 {
				continue
			}
			allowedGroupIDs := allowedGroupIDsBySubID[sub.Id]
			if len(allowedGroupIDs) == 0 {
				continue
			}

			effectiveGroupDailyLimits := effectiveGroupDailyLimitsBySubID[sub.Id]
			useGroupDailyLimit := len(effectiveGroupDailyLimits) > 0

			if useGroupDailyLimit {
				usedByGroupID := groupModeUsageBySubID[sub.Id]
				for _, gid := range allowedGroupIDs {
					cap := capacityByGroup[gid]
					if cap == nil {
						continue
					}
					cap.TotalRemaining += remaining

					limit, ok := effectiveGroupDailyLimits[gid]
					if !ok {
						addGroupDailyLimitConfigErr(sub, gid)
						continue
					}
					used := 0
					if usedByGroupID != nil {
						used = usedByGroupID[gid]
					}
					if used < 0 {
						return errors.New("订阅额度日用量数据错误")
					}
					usable := remaining
					if limit > 0 {
						remainToday := limit - used
						if remainToday < 0 {
							remainToday = 0
						}
						if usable > remainToday {
							usable = remainToday
						}
					}
					if usable < 0 {
						usable = 0
					}
					cap.DailyCapacity += usable
				}
				continue
			}

			// Legacy daily limit: applies regardless of group.
			used := sub.DailyQuotaUsed
			if sub.DailyQuotaLimit > 0 && sub.DailyQuotaResetDate != today {
				used = 0
			}
			for _, gid := range allowedGroupIDs {
				cap := capacityByGroup[gid]
				if cap == nil {
					continue
				}
				cap.TotalRemaining += remaining
				usable := remaining
				if sub.DailyQuotaLimit > 0 {
					remainToday := sub.DailyQuotaLimit - used
					if remainToday < 0 {
						remainToday = 0
					}
					if usable > remainToday {
						usable = remainToday
					}
				}
				if usable < 0 {
					usable = 0
				}
				cap.DailyCapacity += usable
			}
		}

		capacities := make([]SubscriptionGroupCapacity, 0, len(groupIDsForCapacity))
		for _, gid := range groupIDsForCapacity {
			cap := capacityByGroup[gid]
			if cap == nil {
				continue
			}
			capacities = append(capacities, *cap)
		}
		result.SubscriptionGroupCapacities = capacities

		for _, sub := range activeSubs {
			summary, err := buildSummary(sub)
			if err != nil {
				return err
			}
			activeSummaries = append(activeSummaries, summary)
			allSummaries = append(allSummaries, summary)

			totalQuota += summary.TotalQuota
			remainingQuota += summary.RemainingQuota
			consumedQuota += summary.ConsumedQuota
			dailyUsedSum += summary.DailyQuotaUsed
			if summary.DailyQuotaLimit <= 0 {
				dailyUnlimited = true
			} else {
				dailyLimitSum += summary.DailyQuotaLimit
			}

			if windowStart == 0 || summary.StartAt < windowStart {
				windowStart = summary.StartAt
			}
			if summary.ExpireAt > windowEnd {
				windowEnd = summary.ExpireAt
			}
		}

		for _, sub := range pendingSubs {
			summary, err := buildSummary(sub)
			if err != nil {
				return err
			}
			allSummaries = append(allSummaries, summary)
		}

		// 追加已过期订阅（不影响订阅汇总口径：汇总仅统计未过期且仍有效的订阅）
		for _, sub := range expiredSubs {
			summary, err := buildSummary(sub)
			if err != nil {
				return err
			}
			allSummaries = append(allSummaries, summary)
		}

		result.Subscriptions = activeSummaries
		result.SubscriptionsAll = allSummaries
		result.HasSubscription = len(activeSummaries) > 0
		result.SubscriptionTotal = totalQuota
		result.SubscriptionRemaining = remainingQuota
		result.SubscriptionConsumed = consumedQuota
		result.SubscriptionDailyUsed = dailyUsedSum
		result.SubscriptionDailyLimitUnlimited = dailyUnlimited && len(activeSummaries) > 0
		if dailyUnlimited {
			result.SubscriptionDailyLimit = 0
		} else {
			result.SubscriptionDailyLimit = dailyLimitSum
		}
		result.SubscriptionWindowStart = windowStart
		result.SubscriptionWindowEnd = windowEnd

		paygRemaining := userSnapshot.PaygQuota
		paygTotal := userSnapshot.PaygHistoryQuota
		if paygRemaining < 0 {
			return errors.New("payg_quota 数据错误")
		}
		if paygTotal < 0 {
			return errors.New("payg_history_quota 数据错误")
		}
		if paygTotal < paygRemaining {
			return errors.New("payg quota 数据不一致")
		}
		paygConsumed := paygTotal - paygRemaining
		result.PaygRemaining = paygRemaining
		result.PaygConsumed = paygConsumed
		result.PaygTotal = paygTotal

		redeemRemaining := userSnapshot.RedeemQuota
		if redeemRemaining < 0 {
			return errors.New("redeem_quota 数据错误")
		}
		if redeemRemaining != remainingQuota {
			addConfigErr(fmt.Sprintf("用户快照 redeem_quota=%d 与订阅汇总 remaining_quota=%d 不一致，已以用户快照为准", redeemRemaining, remainingQuota))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// 追加统计：今日总消耗额度（订阅 + 自由）
	todayStart := common.GetStartOfDayUnix(time.Now())
	todayEnd := time.Now().Unix()
	if todayEnd < todayStart {
		todayEnd = todayStart
	}
	if quotaData, qErr := GetQuotaDataByUserId(userId, todayStart, todayEnd); qErr == nil {
		totalToday := 0
		for _, q := range quotaData {
			if q != nil {
				totalToday += q.Quota
			}
		}
		result.TodayTotalUsed = totalToday
	}

	return result, nil
}

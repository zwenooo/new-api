package model

import (
	"errors"
	"one-api/common"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupDailyQuotaLimit describes per-group daily quota settings in API payloads.
// DailyQuotaLimit uses quota units (tokens). 0 means unlimited for that group.
type GroupDailyQuotaLimit struct {
	GroupId         int `json:"group_id"`
	DailyQuotaLimit int `json:"daily_quota_limit"`
}

// SubscriptionProductGroupDailyLimit stores per-product per-group daily quota limits.
// It is the source of truth when present: subscriptions linked to a preset (product) will
// derive per-group daily limits from this table.
type SubscriptionProductGroupDailyLimit struct {
	ProductId       int       `json:"product_id" gorm:"primaryKey;autoIncrement:false;column:product_id;index:idx_sub_product_group_daily_limits_product"`
	GroupId         int       `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_sub_product_group_daily_limits_group"`
	DailyLimitQuota int       `json:"daily_limit_quota" gorm:"type:bigint;not null;default:0;column:daily_limit_quota"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (SubscriptionProductGroupDailyLimit) TableName() string {
	return "subscription_product_group_daily_limits"
}

// RedemptionGroupDailyLimit stores per-redemption per-group daily quota limits.
// Redemption codes are designed to be stable snapshots; therefore their per-group daily limits are
// copied from the preset at generation time and stored here.
type RedemptionGroupDailyLimit struct {
	RedemptionId    int       `json:"redemption_id" gorm:"primaryKey;autoIncrement:false;column:redemption_id;index:idx_redemption_group_daily_limits_redemption"`
	GroupId         int       `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_redemption_group_daily_limits_group"`
	DailyLimitQuota int       `json:"daily_limit_quota" gorm:"type:bigint;not null;default:0;column:daily_limit_quota"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (RedemptionGroupDailyLimit) TableName() string {
	return "redemption_group_daily_limits"
}

// UserSubscriptionGroupDailyLimit stores per-subscription per-group daily quota limits.
// It is the source of truth for manually created subscriptions (source_preset_id/source_redemption_id == 0).
//
// DailyLimitQuota uses quota units (tokens). 0 means unlimited for that group.
type UserSubscriptionGroupDailyLimit struct {
	SubscriptionId  int       `json:"subscription_id" gorm:"primaryKey;autoIncrement:false;column:subscription_id;index:idx_user_sub_group_daily_limits_subscription"`
	GroupId         int       `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_user_sub_group_daily_limits_group"`
	DailyLimitQuota int       `json:"daily_limit_quota" gorm:"type:bigint;not null;default:0;column:daily_limit_quota"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (UserSubscriptionGroupDailyLimit) TableName() string {
	return "user_subscription_group_daily_limits"
}

// UserSubscriptionGroupDailyUsage stores per-subscription per-group daily usage.
// StatDate uses YYYYMMDD in server-local timezone (same as existing daily quota logic).
type UserSubscriptionGroupDailyUsage struct {
	SubscriptionId int       `json:"subscription_id" gorm:"primaryKey;autoIncrement:false;column:subscription_id;index:idx_user_sub_group_daily_usages_subscription"`
	GroupId        int       `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_user_sub_group_daily_usages_group"`
	StatDate       int       `json:"stat_date" gorm:"primaryKey;autoIncrement:false;column:stat_date;index:idx_user_sub_group_daily_usages_stat_date"`
	UsedQuota      int       `json:"used_quota" gorm:"type:bigint;not null;default:0;column:used_quota"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (UserSubscriptionGroupDailyUsage) TableName() string {
	return "user_subscription_group_daily_usages"
}

func normalizeGroupDailyQuotaLimits(raw []GroupDailyQuotaLimit) ([]GroupDailyQuotaLimit, error) {
	if raw == nil {
		return nil, nil
	}
	out := make([]GroupDailyQuotaLimit, 0, len(raw))
	seen := make(map[int]struct{}, len(raw))
	for _, item := range raw {
		groupID := item.GroupId
		if groupID <= 0 {
			return nil, errors.New("分组 id 无效")
		}
		if _, ok := seen[groupID]; ok {
			return nil, errors.New("分组日限额配置重复")
		}
		seen[groupID] = struct{}{}
		if item.DailyQuotaLimit < 0 {
			return nil, errors.New("每日额度必须大于等于 0")
		}
		out = append(out, GroupDailyQuotaLimit{
			GroupId:         groupID,
			DailyQuotaLimit: item.DailyQuotaLimit,
		})
	}
	if len(out) == 0 {
		return []GroupDailyQuotaLimit{}, nil
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GroupId < out[j].GroupId
	})
	return out, nil
}

func getSubscriptionProductGroupDailyLimitsByProductIDsTx(tx *gorm.DB, productIDs []int) (map[int]map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]map[int]int, len(productIDs))
	ids := normalizeUniqueSortedIDs(productIDs)
	if len(ids) == 0 {
		return out, nil
	}

	type row struct {
		ProductId       int `gorm:"column:product_id"`
		GroupId         int `gorm:"column:group_id"`
		DailyLimitQuota int `gorm:"column:daily_limit_quota"`
	}
	var rows []row
	if err := tx.Model(&SubscriptionProductGroupDailyLimit{}).
		Select("product_id", "group_id", "daily_limit_quota").
		Where("product_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ProductId <= 0 || r.GroupId <= 0 {
			continue
		}
		if r.DailyLimitQuota < 0 {
			return nil, errors.New("daily_limit_quota 数据错误")
		}
		m, ok := out[r.ProductId]
		if !ok {
			m = make(map[int]int, 8)
			out[r.ProductId] = m
		}
		m[r.GroupId] = r.DailyLimitQuota
	}
	return out, nil
}

func getRedemptionGroupDailyLimitsByRedemptionIDsTx(tx *gorm.DB, redemptionIDs []int) (map[int]map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]map[int]int, len(redemptionIDs))
	ids := normalizeUniqueSortedIDs(redemptionIDs)
	if len(ids) == 0 {
		return out, nil
	}

	type row struct {
		RedemptionId    int `gorm:"column:redemption_id"`
		GroupId         int `gorm:"column:group_id"`
		DailyLimitQuota int `gorm:"column:daily_limit_quota"`
	}
	var rows []row
	if err := tx.Model(&RedemptionGroupDailyLimit{}).
		Select("redemption_id", "group_id", "daily_limit_quota").
		Where("redemption_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.RedemptionId <= 0 || r.GroupId <= 0 {
			continue
		}
		if r.DailyLimitQuota < 0 {
			return nil, errors.New("daily_limit_quota 数据错误")
		}
		m, ok := out[r.RedemptionId]
		if !ok {
			m = make(map[int]int, 8)
			out[r.RedemptionId] = m
		}
		m[r.GroupId] = r.DailyLimitQuota
	}
	return out, nil
}

func hasSubscriptionProductGroupDailyLimitsTx(tx *gorm.DB, productID int) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return false, errors.New("product_id 无效")
	}
	var cnt int64
	if err := tx.Model(&SubscriptionProductGroupDailyLimit{}).Where("product_id = ?", productID).Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func getUserSubscriptionGroupDailyLimitsBySubscriptionIDsTx(tx *gorm.DB, subIDs []int) (map[int]map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]map[int]int, len(subIDs))
	ids := normalizeUniqueSortedIDs(subIDs)
	if len(ids) == 0 {
		return out, nil
	}

	type row struct {
		SubscriptionId  int `gorm:"column:subscription_id"`
		GroupId         int `gorm:"column:group_id"`
		DailyLimitQuota int `gorm:"column:daily_limit_quota"`
	}
	var rows []row
	if err := tx.Model(&UserSubscriptionGroupDailyLimit{}).
		Select("subscription_id", "group_id", "daily_limit_quota").
		Where("subscription_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.SubscriptionId <= 0 || r.GroupId <= 0 {
			continue
		}
		if r.DailyLimitQuota < 0 {
			return nil, errors.New("daily_limit_quota 数据错误")
		}
		m, ok := out[r.SubscriptionId]
		if !ok {
			m = make(map[int]int, 8)
			out[r.SubscriptionId] = m
		}
		m[r.GroupId] = r.DailyLimitQuota
	}
	return out, nil
}

func hasUserSubscriptionGroupDailyLimitsTx(tx *gorm.DB, subID int) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if subID <= 0 {
		return false, errors.New("subscription_id 无效")
	}
	var cnt int64
	if err := tx.Model(&UserSubscriptionGroupDailyLimit{}).
		Where("subscription_id = ?", subID).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func upsertUserSubscriptionGroupDailyLimitsTx(tx *gorm.DB, subID int, groupLimitByID map[int]int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if subID <= 0 {
		return errors.New("subscription_id 无效")
	}

	if err := tx.Where("subscription_id = ?", subID).Delete(&UserSubscriptionGroupDailyLimit{}).Error; err != nil {
		return err
	}
	if len(groupLimitByID) == 0 {
		return nil
	}

	groupIDs := make([]int, 0, len(groupLimitByID))
	for gid := range groupLimitByID {
		groupIDs = append(groupIDs, gid)
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
		return err
	}

	rows := make([]UserSubscriptionGroupDailyLimit, 0, len(groupIDs))
	for _, gid := range groupIDs {
		limit := groupLimitByID[gid]
		if limit < 0 {
			return errors.New("daily_limit_quota 必须大于等于 0")
		}
		rows = append(rows, UserSubscriptionGroupDailyLimit{
			SubscriptionId:  subID,
			GroupId:         gid,
			DailyLimitQuota: limit,
		})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func upsertSubscriptionProductGroupDailyLimitsTx(tx *gorm.DB, productID int, groupLimitByID map[int]int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if productID <= 0 {
		return errors.New("product_id 无效")
	}

	if err := tx.Where("product_id = ?", productID).Delete(&SubscriptionProductGroupDailyLimit{}).Error; err != nil {
		return err
	}
	if len(groupLimitByID) == 0 {
		return nil
	}

	groupIDs := make([]int, 0, len(groupLimitByID))
	for gid := range groupLimitByID {
		groupIDs = append(groupIDs, gid)
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
		return err
	}

	rows := make([]SubscriptionProductGroupDailyLimit, 0, len(groupIDs))
	for _, gid := range groupIDs {
		limit := groupLimitByID[gid]
		if limit < 0 {
			return errors.New("daily_limit_quota 必须大于等于 0")
		}
		rows = append(rows, SubscriptionProductGroupDailyLimit{
			ProductId:       productID,
			GroupId:         gid,
			DailyLimitQuota: limit,
		})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getUserSubscriptionGroupDailyUsageMapTx(tx *gorm.DB, subIDs []int, groupID int, statDate int) (map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]int, len(subIDs))
	ids := normalizeUniqueSortedIDs(subIDs)
	if len(ids) == 0 || groupID <= 0 || statDate <= 0 {
		return out, nil
	}

	type row struct {
		SubscriptionId int `gorm:"column:subscription_id"`
		UsedQuota      int `gorm:"column:used_quota"`
	}
	var rows []row
	if err := tx.Model(&UserSubscriptionGroupDailyUsage{}).
		Select("subscription_id", "used_quota").
		Where("subscription_id IN ? AND group_id = ? AND stat_date = ?", ids, groupID, statDate).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.SubscriptionId <= 0 {
			continue
		}
		if r.UsedQuota < 0 {
			return nil, errors.New("used_quota 数据错误")
		}
		out[r.SubscriptionId] = r.UsedQuota
	}
	return out, nil
}

func getUserSubscriptionGroupDailyUsageBySubIDsTx(tx *gorm.DB, subIDs []int, statDate int) (map[int]map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]map[int]int, len(subIDs))
	ids := normalizeUniqueSortedIDs(subIDs)
	if len(ids) == 0 || statDate <= 0 {
		return out, nil
	}

	type row struct {
		SubscriptionId int `gorm:"column:subscription_id"`
		GroupId        int `gorm:"column:group_id"`
		UsedQuota      int `gorm:"column:used_quota"`
	}
	var rows []row
	if err := tx.Model(&UserSubscriptionGroupDailyUsage{}).
		Select("subscription_id", "group_id", "used_quota").
		Where("subscription_id IN ? AND stat_date = ?", ids, statDate).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.SubscriptionId <= 0 || r.GroupId <= 0 {
			continue
		}
		if r.UsedQuota < 0 {
			return nil, errors.New("used_quota 数据错误")
		}
		m, ok := out[r.SubscriptionId]
		if !ok {
			m = make(map[int]int, 8)
			out[r.SubscriptionId] = m
		}
		m[r.GroupId] = r.UsedQuota
	}
	return out, nil
}

func incrUserSubscriptionGroupDailyUsageTx(tx *gorm.DB, subID int, groupID int, statDate int, delta int) error {
	if tx == nil {
		tx = DB
	}
	if subID <= 0 {
		return errors.New("subscription_id 无效")
	}
	if groupID <= 0 {
		return errors.New("group_id 无效")
	}
	if statDate <= 0 {
		return errors.New("stat_date 无效")
	}
	if delta <= 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "subscription_id"},
			{Name: "group_id"},
			{Name: "stat_date"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", delta),
			"updated_at": time.Now(),
		}),
	}).Create(&UserSubscriptionGroupDailyUsage{
		SubscriptionId: subID,
		GroupId:        groupID,
		StatDate:       statDate,
		UsedQuota:      delta,
	}).Error
}

func decrUserSubscriptionGroupDailyUsageTx(tx *gorm.DB, subID int, groupID int, statDate int, delta int) error {
	if tx == nil {
		tx = DB
	}
	if subID <= 0 {
		return errors.New("subscription_id 无效")
	}
	if groupID <= 0 {
		return errors.New("group_id 无效")
	}
	if statDate <= 0 {
		return errors.New("stat_date 无效")
	}
	if delta <= 0 {
		return nil
	}
	return tx.Model(&UserSubscriptionGroupDailyUsage{}).
		Where("subscription_id = ? AND group_id = ? AND stat_date = ?", subID, groupID, statDate).
		Updates(map[string]interface{}{
			"used_quota": gorm.Expr("CASE WHEN used_quota >= ? THEN used_quota - ? ELSE 0 END", delta, delta),
			"updated_at": time.Now(),
		}).Error
}

func sumUserSubscriptionDailyUsedAcrossGroupsTx(tx *gorm.DB, subIDs []int, statDate int) (map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]int, len(subIDs))
	ids := normalizeUniqueSortedIDs(subIDs)
	if len(ids) == 0 || statDate <= 0 {
		return out, nil
	}

	type row struct {
		SubscriptionId int `gorm:"column:subscription_id"`
		UsedQuota      int `gorm:"column:used_quota"`
	}
	var rows []row
	if err := tx.Model(&UserSubscriptionGroupDailyUsage{}).
		Select("subscription_id, COALESCE(SUM(used_quota),0) AS used_quota").
		Where("subscription_id IN ? AND stat_date = ?", ids, statDate).
		Group("subscription_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.SubscriptionId <= 0 {
			continue
		}
		if r.UsedQuota < 0 {
			return nil, errors.New("used_quota 数据错误")
		}
		out[r.SubscriptionId] = r.UsedQuota
	}
	return out, nil
}

func resolveGroupIDByCodeTx(tx *gorm.DB, groupCode string) (groupID int, ok bool, err error) {
	if tx == nil {
		tx = DB
	}
	groupCode = strings.TrimSpace(groupCode)
	if groupCode == "" {
		return 0, false, nil
	}
	m, err := GroupCodeIDMap(tx, []string{groupCode})
	if err != nil {
		return 0, false, err
	}
	groupID = m[groupCode]
	if groupID <= 0 {
		return 0, false, errors.New("分组不存在: " + groupCode)
	}
	return groupID, true, nil
}

func statDateToday() int {
	return common.GetTodayDateInt()
}

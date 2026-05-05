package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"one-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RedemptionPresetRevision struct {
	Id int `json:"id" gorm:"primaryKey"`

	PresetId     int   `json:"preset_id" gorm:"not null;index:idx_redemption_preset_revisions_preset_revision,priority:1"`
	RevisionNo   int   `json:"revision_no" gorm:"not null;index:idx_redemption_preset_revisions_preset_revision,priority:2"`
	IsCurrent    bool  `json:"is_current" gorm:"type:boolean;default:false;index:idx_redemption_preset_revisions_preset_current,priority:2"`
	SnapshotTime int64 `json:"snapshot_time" gorm:"bigint;not null;index;column:snapshot_time"`

	Name                   string                 `json:"name" gorm:"size:64;not null"`
	Description            string                 `json:"description" gorm:"type:text;column:description"`
	Mode                   string                 `json:"mode" gorm:"type:varchar(32);not null;column:mode"`
	SortOrder              int                    `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`
	PriceFen               int64                  `json:"price_fen" gorm:"type:bigint;default:0;column:price_fen"`
	Stock                  *int                   `json:"stock" gorm:"type:int;column:stock"`
	PurchaseLimit          int                    `json:"purchase_limit" gorm:"type:int;default:0;column:purchase_limit"`
	MultiQuantityEnabled   bool                   `json:"multi_quantity_enabled" gorm:"type:boolean;default:false;column:multi_quantity_enabled"`
	MultiQuantityDeferOnly bool                   `json:"multi_quantity_defer_only" gorm:"type:boolean;default:true;column:multi_quantity_defer_only"`
	Enabled                bool                   `json:"enabled" gorm:"type:boolean;default:true;column:enabled"`
	Archived               bool                   `json:"archived" gorm:"type:boolean;default:false;column:archived"`
	Quota                  int                    `json:"quota" gorm:"default:0"`
	DailyQuotaLimit        int                    `json:"daily_quota_limit" gorm:"default:0"`
	DailyRequestLimit      int                    `json:"daily_request_limit" gorm:"default:0;column:daily_request_limit"`
	QuotaValidDays         int                    `json:"quota_valid_days" gorm:"default:0"`
	ExpiredTime            int64                  `json:"expired_time" gorm:"bigint;default:0"`
	PlanValidDays          int                    `json:"plan_valid_days" gorm:"default:0;column:plan_valid_days"`
	ChannelIds             JSONValue              `json:"channel_ids" gorm:"type:json;column:channel_ids"`
	PresetCreatedTime      int64                  `json:"preset_created_time" gorm:"bigint;default:0;column:preset_created_time"`
	PresetUpdatedTime      int64                  `json:"preset_updated_time" gorm:"bigint;default:0;column:preset_updated_time"`
	AllowedGroupIds        JSONValue              `json:"allowed_group_ids" gorm:"-"`
	GroupDailyLimits       []GroupDailyQuotaLimit `json:"group_daily_limits" gorm:"-"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	Preset                 RedemptionPreset       `json:"-" gorm:"foreignKey:PresetId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (RedemptionPresetRevision) TableName() string {
	return "redemption_preset_revisions"
}

type RedemptionPresetRevisionGroup struct {
	RevisionId int                      `json:"revision_id" gorm:"primaryKey;autoIncrement:false;column:revision_id"`
	GroupId    int                      `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_redemption_preset_revision_groups_group"`
	Revision   RedemptionPresetRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Group      Group                    `json:"-" gorm:"foreignKey:GroupId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (RedemptionPresetRevisionGroup) TableName() string {
	return "redemption_preset_revision_groups"
}

type RedemptionPresetRevisionGroupDailyLimit struct {
	RevisionId      int                      `json:"revision_id" gorm:"primaryKey;autoIncrement:false;column:revision_id"`
	GroupId         int                      `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_redemption_preset_revision_daily_limits_group"`
	DailyLimitQuota int                      `json:"daily_limit_quota" gorm:"type:int;not null;default:0;column:daily_limit_quota"`
	Revision        RedemptionPresetRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Group           Group                    `json:"-" gorm:"foreignKey:GroupId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (RedemptionPresetRevisionGroupDailyLimit) TableName() string {
	return "redemption_preset_revision_group_daily_limits"
}

type UserSubscriptionPresetRevisionBinding struct {
	SubscriptionId int                      `json:"subscription_id" gorm:"primaryKey;autoIncrement:false;column:subscription_id"`
	PresetId       int                      `json:"preset_id" gorm:"not null;index:idx_user_sub_preset_revision_bindings_preset"`
	RevisionId     int                      `json:"revision_id" gorm:"not null;index:idx_user_sub_preset_revision_bindings_revision"`
	Subscription   UserSubscription         `json:"-" gorm:"foreignKey:SubscriptionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Preset         RedemptionPreset         `json:"-" gorm:"foreignKey:PresetId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Revision       RedemptionPresetRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (UserSubscriptionPresetRevisionBinding) TableName() string {
	return "user_subscription_preset_revision_bindings"
}

type UserRequestSubscriptionPresetRevisionBinding struct {
	SubscriptionId int                      `json:"subscription_id" gorm:"primaryKey;autoIncrement:false;column:subscription_id"`
	PresetId       int                      `json:"preset_id" gorm:"not null;index:idx_user_request_sub_preset_revision_bindings_preset"`
	RevisionId     int                      `json:"revision_id" gorm:"not null;index:idx_user_request_sub_preset_revision_bindings_revision"`
	Subscription   UserRequestSubscription  `json:"-" gorm:"foreignKey:SubscriptionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Preset         RedemptionPreset         `json:"-" gorm:"foreignKey:PresetId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Revision       RedemptionPresetRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (UserRequestSubscriptionPresetRevisionBinding) TableName() string {
	return "user_request_subscription_preset_revision_bindings"
}

type RedemptionPresetUpsertOptions struct {
	SyncSoldAssets bool `json:"sync_sold_assets"`
}

type UserSubscriptionPresetRevisionBindingRecord struct {
	SubscriptionId int `gorm:"column:subscription_id"`
	PresetId       int `gorm:"column:preset_id"`
	RevisionId     int `gorm:"column:revision_id"`
}

func normalizeRedemptionPresetRevisionGroupLimitMap(raw map[int]int) (map[int]int, error) {
	if raw == nil {
		return map[int]int{}, nil
	}
	out := make(map[int]int, len(raw))
	for gid, limit := range raw {
		if gid <= 0 {
			return nil, errors.New("分组 id 无效")
		}
		if limit < 0 {
			return nil, errors.New("每日额度必须大于等于 0")
		}
		out[gid] = limit
	}
	return out, nil
}

func extractAllowedGroupIDsFromRedemptionPreset(preset *RedemptionPreset) ([]int, error) {
	if preset == nil {
		return nil, errors.New("preset 为空")
	}
	if len(preset.AllowedGroupIds) == 0 {
		return nil, nil
	}
	var groupIDs []int
	if err := common.Unmarshal([]byte(preset.AllowedGroupIds), &groupIDs); err != nil {
		return nil, err
	}
	return normalizeUniqueSortedIDs(groupIDs), nil
}

func copyRedemptionPresetToRevision(preset *RedemptionPreset, snapshotTime int64) *RedemptionPresetRevision {
	if preset == nil {
		return nil
	}
	mode := strings.TrimSpace(preset.Mode)
	if resolvedMode := ResolveCompatibleRedemptionPresetMode(preset); resolvedMode != "" {
		mode = resolvedMode
	}
	return &RedemptionPresetRevision{
		PresetId:               preset.Id,
		SnapshotTime:           snapshotTime,
		Name:                   preset.Name,
		Description:            preset.Description,
		Mode:                   mode,
		SortOrder:              preset.SortOrder,
		PriceFen:               preset.PriceFen,
		Stock:                  preset.Stock,
		PurchaseLimit:          preset.PurchaseLimit,
		MultiQuantityEnabled:   preset.MultiQuantityEnabled,
		MultiQuantityDeferOnly: preset.MultiQuantityDeferOnly,
		Enabled:                preset.Enabled,
		Archived:               preset.Archived,
		Quota:                  preset.Quota,
		DailyQuotaLimit:        preset.DailyQuotaLimit,
		DailyRequestLimit:      preset.DailyRequestLimit,
		QuotaValidDays:         preset.QuotaValidDays,
		ExpiredTime:            preset.ExpiredTime,
		PlanValidDays:          preset.PlanValidDays,
		ChannelIds:             preset.ChannelIds,
		PresetCreatedTime:      preset.CreatedTime,
		PresetUpdatedTime:      preset.UpdatedTime,
	}
}

func getRedemptionPresetRevisionGroupIDsByRevisionIDsTx(tx *gorm.DB, revisionIDs []int) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int][]int, len(revisionIDs))
	ids := normalizeUniqueSortedIDs(revisionIDs)
	if len(ids) == 0 {
		return out, nil
	}

	type row struct {
		RevisionId int `gorm:"column:revision_id"`
		GroupId    int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&RedemptionPresetRevisionGroup{}).
		Select("revision_id", "group_id").
		Where("revision_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.RevisionId <= 0 || r.GroupId <= 0 {
			continue
		}
		out[r.RevisionId] = append(out[r.RevisionId], r.GroupId)
	}
	for revisionID, groupIDs := range out {
		filtered, err := filterExistingSortedIDsTx(tx, groupIDs)
		if err != nil {
			return nil, err
		}
		out[revisionID] = filtered
	}
	return out, nil
}

func getRedemptionPresetRevisionGroupDailyLimitsByRevisionIDsTx(tx *gorm.DB, revisionIDs []int) (map[int]map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]map[int]int, len(revisionIDs))
	ids := normalizeUniqueSortedIDs(revisionIDs)
	if len(ids) == 0 {
		return out, nil
	}

	type row struct {
		RevisionId      int `gorm:"column:revision_id"`
		GroupId         int `gorm:"column:group_id"`
		DailyLimitQuota int `gorm:"column:daily_limit_quota"`
	}
	var rows []row
	if err := tx.Model(&RedemptionPresetRevisionGroupDailyLimit{}).
		Select("revision_id", "group_id", "daily_limit_quota").
		Where("revision_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.RevisionId <= 0 || r.GroupId <= 0 {
			continue
		}
		if r.DailyLimitQuota < 0 {
			return nil, errors.New("daily_limit_quota 数据错误")
		}
		limitByGroupID, ok := out[r.RevisionId]
		if !ok {
			limitByGroupID = make(map[int]int, 8)
			out[r.RevisionId] = limitByGroupID
		}
		limitByGroupID[r.GroupId] = r.DailyLimitQuota
	}
	return out, nil
}

func hydrateRedemptionPresetRevisionsTx(tx *gorm.DB, revisions []*RedemptionPresetRevision) error {
	if tx == nil {
		tx = DB
	}
	if len(revisions) == 0 {
		return nil
	}
	revisionIDs := make([]int, 0, len(revisions))
	revisionByID := make(map[int]*RedemptionPresetRevision, len(revisions))
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		revisionIDs = append(revisionIDs, revision.Id)
		revisionByID[revision.Id] = revision
	}
	revisionIDs = normalizeUniqueSortedIDs(revisionIDs)
	if len(revisionIDs) == 0 {
		return nil
	}

	groupIDsByRevisionID, err := getRedemptionPresetRevisionGroupIDsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return err
	}
	groupDailyLimitsByRevisionID, err := getRedemptionPresetRevisionGroupDailyLimitsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return err
	}
	groupIDSet := make(map[int]struct{}, 16)
	for _, groupIDs := range groupIDsByRevisionID {
		for _, gid := range groupIDs {
			groupIDSet[gid] = struct{}{}
		}
	}
	for _, limitByGroupID := range groupDailyLimitsByRevisionID {
		for gid := range limitByGroupID {
			groupIDSet[gid] = struct{}{}
		}
	}
	allGroupIDs := make([]int, 0, len(groupIDSet))
	for gid := range groupIDSet {
		allGroupIDs = append(allGroupIDs, gid)
	}
	allGroupIDs = normalizeUniqueSortedIDs(allGroupIDs)
	if len(allGroupIDs) > 0 {
		validIDs, err := filterExistingSortedIDsTx(tx, allGroupIDs)
		if err != nil {
			return err
		}
		validSet := make(map[int]struct{}, len(validIDs))
		for _, gid := range validIDs {
			validSet[gid] = struct{}{}
		}
		for revisionID, groupIDs := range groupIDsByRevisionID {
			filtered := make([]int, 0, len(groupIDs))
			for _, gid := range groupIDs {
				if _, ok := validSet[gid]; !ok {
					continue
				}
				filtered = append(filtered, gid)
			}
			groupIDsByRevisionID[revisionID] = normalizeUniqueSortedIDs(filtered)
		}
		for revisionID, limitByGroupID := range groupDailyLimitsByRevisionID {
			if len(limitByGroupID) == 0 {
				continue
			}
			for gid := range limitByGroupID {
				if _, ok := validSet[gid]; ok {
					continue
				}
				delete(limitByGroupID, gid)
			}
			groupDailyLimitsByRevisionID[revisionID] = limitByGroupID
		}
	}

	for revisionID, revision := range revisionByID {
		if revision == nil {
			continue
		}
		groupIDs := normalizeUniqueSortedIDs(groupIDsByRevisionID[revisionID])
		if len(groupIDs) > 0 {
			if b, err := common.Marshal(groupIDs); err == nil {
				revision.AllowedGroupIds = JSONValue(b)
			} else {
				return err
			}
		} else {
			revision.AllowedGroupIds = JSONValue([]byte("[]"))
		}

		limitByGroupID := groupDailyLimitsByRevisionID[revisionID]
		if len(limitByGroupID) == 0 {
			revision.GroupDailyLimits = []GroupDailyQuotaLimit{}
			continue
		}
		items := make([]GroupDailyQuotaLimit, 0, len(limitByGroupID))
		for gid, limit := range limitByGroupID {
			items = append(items, GroupDailyQuotaLimit{
				GroupId:         gid,
				DailyQuotaLimit: limit,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].GroupId < items[j].GroupId
		})
		revision.GroupDailyLimits = items
		NormalizeCompatibleRedemptionPresetRevisionMode(revision)
	}
	for _, revision := range revisionByID {
		if revision == nil || len(revision.GroupDailyLimits) > 0 {
			continue
		}
		NormalizeCompatibleRedemptionPresetRevisionMode(revision)
	}
	return nil
}

func loadRedemptionPresetSnapshotConfigTx(tx *gorm.DB, preset *RedemptionPreset) ([]int, map[int]int, error) {
	if tx == nil {
		tx = DB
	}
	if preset == nil || preset.Id <= 0 {
		return nil, nil, errors.New("preset 无效")
	}
	groupIDs, err := getSubscriptionProductGroupIDsTx(tx, preset.Id)
	if err != nil {
		return nil, nil, err
	}
	if len(groupIDs) == 0 {
		groupIDs, err = extractAllowedGroupIDsFromRedemptionPreset(preset)
		if err != nil {
			return nil, nil, err
		}
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) > 0 {
		groupIDs, err = filterExistingSortedIDsTx(tx, groupIDs)
		if err != nil {
			return nil, nil, err
		}
	}

	limitsByProductID, err := getSubscriptionProductGroupDailyLimitsByProductIDsTx(tx, []int{preset.Id})
	if err != nil {
		return nil, nil, err
	}
	groupLimitByID, err := normalizeRedemptionPresetRevisionGroupLimitMap(limitsByProductID[preset.Id])
	if err != nil {
		return nil, nil, err
	}
	return groupIDs, groupLimitByID, nil
}

func createRedemptionPresetRevisionTx(tx *gorm.DB, preset *RedemptionPreset) (*RedemptionPresetRevision, error) {
	if tx == nil {
		tx = DB
	}
	if preset == nil || preset.Id <= 0 {
		return nil, errors.New("preset 无效")
	}
	groupIDs, groupLimitByID, err := loadRedemptionPresetSnapshotConfigTx(tx, preset)
	if err != nil {
		return nil, err
	}
	snapshotPreset := *preset
	if len(snapshotPreset.AllowedGroupIds) == 0 && len(groupIDs) > 0 {
		if b, err := common.Marshal(groupIDs); err == nil {
			snapshotPreset.AllowedGroupIds = JSONValue(b)
		} else {
			return nil, err
		}
	}
	var maxRevisionNo int
	if err := tx.Model(&RedemptionPresetRevision{}).
		Where("preset_id = ?", preset.Id).
		Select("COALESCE(MAX(revision_no),0)").
		Scan(&maxRevisionNo).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&RedemptionPresetRevision{}).
		Where("preset_id = ? AND is_current = ?", preset.Id, true).
		Update("is_current", false).Error; err != nil {
		return nil, err
	}

	revision := copyRedemptionPresetToRevision(&snapshotPreset, common.GetTimestamp())
	revision.RevisionNo = maxRevisionNo + 1
	revision.IsCurrent = true
	if err := tx.Create(revision).Error; err != nil {
		return nil, err
	}
	if err := upsertRedemptionPresetRevisionGroupsTx(tx, revision.Id, groupIDs); err != nil {
		return nil, err
	}
	if err := upsertRedemptionPresetRevisionGroupDailyLimitsTx(tx, revision.Id, groupLimitByID); err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetRevisionsTx(tx, []*RedemptionPresetRevision{revision}); err != nil {
		return nil, err
	}
	return revision, nil
}

func ensureCurrentRedemptionPresetRevisionTx(tx *gorm.DB, presetID int) (*RedemptionPresetRevision, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, errors.New("preset_id 无效")
	}
	var revision RedemptionPresetRevision
	err := tx.Where("preset_id = ? AND is_current = ?", presetID, true).
		Order("revision_no DESC, id DESC").
		First(&revision).Error
	if err == nil {
		if err := hydrateRedemptionPresetRevisionsTx(tx, []*RedemptionPresetRevision{&revision}); err != nil {
			return nil, err
		}
		return &revision, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var preset RedemptionPreset
	if err := tx.Where("id = ?", presetID).First(&preset).Error; err != nil {
		return nil, err
	}
	return createRedemptionPresetRevisionTx(tx, &preset)
}

func findCurrentRedemptionPresetRevisionTx(tx *gorm.DB, presetID int) (*RedemptionPresetRevision, bool, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, false, errors.New("preset_id 无效")
	}
	var revision RedemptionPresetRevision
	result := tx.Where("preset_id = ? AND is_current = ?", presetID, true).
		Order("revision_no DESC, id DESC").
		Limit(1).
		Find(&revision)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	if err := hydrateRedemptionPresetRevisionsTx(tx, []*RedemptionPresetRevision{&revision}); err != nil {
		return nil, false, err
	}
	return &revision, true, nil
}

func findLatestRedemptionPresetRevisionTx(tx *gorm.DB, presetID int) (*RedemptionPresetRevision, bool, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, false, errors.New("preset_id 无效")
	}
	var revision RedemptionPresetRevision
	result := tx.Where("preset_id = ?", presetID).
		Order("revision_no DESC, id DESC").
		Limit(1).
		Find(&revision)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	if err := hydrateRedemptionPresetRevisionsTx(tx, []*RedemptionPresetRevision{&revision}); err != nil {
		return nil, false, err
	}
	return &revision, true, nil
}

// Backfill must tolerate historical rows that still reference a deleted preset.
// In that case we can reuse the latest stored revision if it exists; otherwise the
// row is an orphaned historical reference and should be skipped by the caller.
func ensureRedemptionPresetRevisionForBackfillTx(tx *gorm.DB, presetID int) (*RedemptionPresetRevision, bool, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, false, errors.New("preset_id 无效")
	}
	if revision, ok, err := findCurrentRedemptionPresetRevisionTx(tx, presetID); err != nil {
		return nil, false, err
	} else if ok {
		return revision, true, nil
	}

	var preset RedemptionPreset
	result := tx.Where("id = ?", presetID).Limit(1).Find(&preset)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected > 0 {
		revision, err := createRedemptionPresetRevisionTx(tx, &preset)
		if err != nil {
			return nil, false, err
		}
		return revision, true, nil
	}

	if revision, ok, err := findLatestRedemptionPresetRevisionTx(tx, presetID); err != nil {
		return nil, false, err
	} else if ok {
		return revision, true, nil
	}
	return nil, false, nil
}

func EnsureCurrentRedemptionPresetRevisionTx(tx *gorm.DB, presetID int) (*RedemptionPresetRevision, error) {
	return ensureCurrentRedemptionPresetRevisionTx(tx, presetID)
}

func EnsureCurrentRedemptionPresetRevision(presetID int) (*RedemptionPresetRevision, error) {
	return ensureCurrentRedemptionPresetRevisionTx(DB, presetID)
}

func ListRedemptionPresetRevisionsTx(tx *gorm.DB, presetID int) ([]*RedemptionPresetRevision, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, errors.New("preset_id 无效")
	}
	var revisions []*RedemptionPresetRevision
	if err := tx.Where("preset_id = ?", presetID).
		Order("revision_no DESC, id DESC").
		Find(&revisions).Error; err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetRevisionsTx(tx, revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

func ListRedemptionPresetRevisions(presetID int) ([]*RedemptionPresetRevision, error) {
	return ListRedemptionPresetRevisionsTx(DB, presetID)
}

func GetRedemptionPresetRevisionTx(tx *gorm.DB, presetID int, revisionID int) (*RedemptionPresetRevision, error) {
	if tx == nil {
		tx = DB
	}
	if presetID <= 0 {
		return nil, errors.New("preset_id 无效")
	}
	if revisionID <= 0 {
		return nil, errors.New("revision_id 无效")
	}
	var revision RedemptionPresetRevision
	if err := tx.Where("id = ? AND preset_id = ?", revisionID, presetID).First(&revision).Error; err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetRevisionsTx(tx, []*RedemptionPresetRevision{&revision}); err != nil {
		return nil, err
	}
	return &revision, nil
}

func RestoreRedemptionPresetFromRevision(tx *gorm.DB, presetID int, revisionID int, options RedemptionPresetUpsertOptions) (*RedemptionPreset, error) {
	if tx == nil {
		var restored *RedemptionPreset
		err := DB.Transaction(func(tx *gorm.DB) error {
			next, err := RestoreRedemptionPresetFromRevision(tx, presetID, revisionID, options)
			if err != nil {
				return err
			}
			restored = next
			return nil
		})
		if err != nil {
			return nil, err
		}
		return restored, nil
	}
	revision, err := GetRedemptionPresetRevisionTx(tx, presetID, revisionID)
	if err != nil {
		return nil, err
	}
	preset := &RedemptionPreset{
		Id:                     presetID,
		Name:                   revision.Name,
		Description:            revision.Description,
		Mode:                   revision.Mode,
		SortOrder:              revision.SortOrder,
		PriceFen:               revision.PriceFen,
		Stock:                  revision.Stock,
		PurchaseLimit:          revision.PurchaseLimit,
		MultiQuantityEnabled:   revision.MultiQuantityEnabled,
		MultiQuantityDeferOnly: revision.MultiQuantityDeferOnly,
		Enabled:                revision.Enabled,
		Archived:               revision.Archived,
		Quota:                  revision.Quota,
		DailyQuotaLimit:        revision.DailyQuotaLimit,
		DailyRequestLimit:      revision.DailyRequestLimit,
		QuotaValidDays:         revision.QuotaValidDays,
		ExpiredTime:            revision.ExpiredTime,
		PlanValidDays:          revision.PlanValidDays,
		ChannelIds:             revision.ChannelIds,
		AllowedGroupIds:        revision.AllowedGroupIds,
		GroupDailyLimits:       append([]GroupDailyQuotaLimit(nil), revision.GroupDailyLimits...),
		CreatedTime:            revision.PresetCreatedTime,
		UpdatedTime:            revision.PresetUpdatedTime,
	}
	return UpsertRedemptionPreset(tx, preset, options)
}

func upsertRedemptionPresetRevisionGroupsTx(tx *gorm.DB, revisionID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if err := tx.Where("revision_id = ?", revisionID).Delete(&RedemptionPresetRevisionGroup{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	rows := make([]RedemptionPresetRevisionGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, RedemptionPresetRevisionGroup{
			RevisionId: revisionID,
			GroupId:    groupID,
		})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func upsertRedemptionPresetRevisionGroupDailyLimitsTx(tx *gorm.DB, revisionID int, groupLimitByID map[int]int) error {
	if tx == nil {
		tx = DB
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	normalized, err := normalizeRedemptionPresetRevisionGroupLimitMap(groupLimitByID)
	if err != nil {
		return err
	}
	if err := tx.Where("revision_id = ?", revisionID).Delete(&RedemptionPresetRevisionGroupDailyLimit{}).Error; err != nil {
		return err
	}
	if len(normalized) == 0 {
		return nil
	}
	groupIDs := make([]int, 0, len(normalized))
	for gid := range normalized {
		groupIDs = append(groupIDs, gid)
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
		return err
	}
	rows := make([]RedemptionPresetRevisionGroupDailyLimit, 0, len(groupIDs))
	for _, gid := range groupIDs {
		rows = append(rows, RedemptionPresetRevisionGroupDailyLimit{
			RevisionId:      revisionID,
			GroupId:         gid,
			DailyLimitQuota: normalized[gid],
		})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func upsertUserSubscriptionPresetRevisionBindingTx(tx *gorm.DB, subscriptionID int, presetID int, revisionID int) error {
	if tx == nil {
		tx = DB
	}
	if subscriptionID <= 0 {
		return errors.New("subscription_id 无效")
	}
	if presetID <= 0 {
		return errors.New("preset_id 无效")
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	binding := &UserSubscriptionPresetRevisionBinding{
		SubscriptionId: subscriptionID,
		PresetId:       presetID,
		RevisionId:     revisionID,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscription_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"preset_id", "revision_id"}),
	}).Create(binding).Error
}

func upsertUserRequestSubscriptionPresetRevisionBindingTx(tx *gorm.DB, subscriptionID int, presetID int, revisionID int) error {
	if tx == nil {
		tx = DB
	}
	if subscriptionID <= 0 {
		return errors.New("subscription_id 无效")
	}
	if presetID <= 0 {
		return errors.New("preset_id 无效")
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	binding := &UserRequestSubscriptionPresetRevisionBinding{
		SubscriptionId: subscriptionID,
		PresetId:       presetID,
		RevisionId:     revisionID,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscription_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"preset_id", "revision_id"}),
	}).Create(binding).Error
}

func getUserSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx *gorm.DB, subscriptionIDs []int) (map[int]UserSubscriptionPresetRevisionBindingRecord, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]UserSubscriptionPresetRevisionBindingRecord, len(subscriptionIDs))
	ids := normalizeUniqueSortedIDs(subscriptionIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []UserSubscriptionPresetRevisionBindingRecord
	if err := tx.Model(&UserSubscriptionPresetRevisionBinding{}).
		Select("subscription_id", "preset_id", "revision_id").
		Where("subscription_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.SubscriptionId <= 0 || row.PresetId <= 0 || row.RevisionId <= 0 {
			continue
		}
		out[row.SubscriptionId] = row
	}
	return out, nil
}

func getUserRequestSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx *gorm.DB, subscriptionIDs []int) (map[int]UserSubscriptionPresetRevisionBindingRecord, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]UserSubscriptionPresetRevisionBindingRecord, len(subscriptionIDs))
	ids := normalizeUniqueSortedIDs(subscriptionIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []UserSubscriptionPresetRevisionBindingRecord
	if err := tx.Model(&UserRequestSubscriptionPresetRevisionBinding{}).
		Select("subscription_id", "preset_id", "revision_id").
		Where("subscription_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.SubscriptionId <= 0 || row.PresetId <= 0 || row.RevisionId <= 0 {
			continue
		}
		out[row.SubscriptionId] = row
	}
	return out, nil
}

func currentPresetControlsUserSubscriptionSnapshotTx(tx *gorm.DB, sub UserSubscription) (bool, error) {
	_ = tx
	return sub.Id > 0 && sub.SourcePresetId > 0, nil
}

func syncQuotaUserSubscriptionToRevision(tx *gorm.DB, sub *UserSubscription, revision *RedemptionPresetRevision, groupIDs []int, now int64, quotaDeltaByUserID map[int]int, tokenDeltaByUserID map[int]int) error {
	if tx == nil {
		tx = DB
	}
	if sub == nil || sub.Id <= 0 {
		return errors.New("subscription 无效")
	}
	if revision == nil || revision.Id <= 0 || revision.PresetId <= 0 {
		return errors.New("revision 无效")
	}
	consumed := sub.TotalQuota - sub.RemainingQuota
	if consumed < 0 {
		consumed = 0
	}
	newTotal := revision.Quota
	if newTotal < 0 {
		return errors.New("revision quota 无效")
	}
	if strings.TrimSpace(sub.BillingUnit) == UserSubscriptionBillingUnitTokens {
		newTotal = discreteUnitsFromDisplayInt(newTotal)
	}
	newRemaining := newTotal - consumed
	if newRemaining < 0 {
		newRemaining = 0
	}
	newExpireAt, err := derivePresetRevisionExpireAt(sub.StartAt, revision.QuotaValidDays)
	if err != nil {
		return err
	}
	newDailyLimit := revision.DailyQuotaLimit
	if strings.TrimSpace(sub.BillingUnit) == UserSubscriptionBillingUnitTokens {
		newDailyLimit = discreteUnitsFromDisplayInt(newDailyLimit)
	}
	newDailyUsed := sub.DailyQuotaUsed
	newDailyReset := sub.DailyQuotaResetDate
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
	if newExpireAt > 0 && newExpireAt < now {
		newRemaining = 0
		newDailyUsed = 0
		newDailyReset = 0
	}

	oldCreditedAmount := 0
	if sub.Credited && sub.RemainingQuota > 0 {
		oldCreditedAmount = sub.RemainingQuota
	}
	newCredited := (sub.StartAt == 0 || sub.StartAt <= now) && (newExpireAt == 0 || newExpireAt >= now)
	newCreditedAmount := 0
	if newCredited && newRemaining > 0 {
		newCreditedAmount = newRemaining
	}

	invalidAt := sub.InvalidAt
	if newRemaining > 0 {
		invalidAt = 0
	} else if invalidAt <= 0 {
		invalidAt = now
	}
	updates := map[string]interface{}{
		"total_quota":            newTotal,
		"remaining_quota":        newRemaining,
		"daily_quota_limit":      newDailyLimit,
		"daily_quota_used":       newDailyUsed,
		"daily_quota_reset_date": newDailyReset,
		"expire_at":              newExpireAt,
		"invalid_at":             invalidAt,
		"credited":               newCredited,
	}
	if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
		return err
	}
	if err := upsertUserSubscriptionPresetRevisionBindingTx(tx, sub.Id, revision.PresetId, revision.Id); err != nil {
		return err
	}
	if err := tx.Where("subscription_id = ?", sub.Id).Delete(&UserSubscriptionGroup{}).Error; err != nil {
		return err
	}
	if err := tx.Where("subscription_id = ?", sub.Id).Delete(&UserSubscriptionGroupDailyLimit{}).Error; err != nil {
		return err
	}

	delta := newCreditedAmount - oldCreditedAmount
	switch strings.TrimSpace(sub.BillingUnit) {
	case "", UserSubscriptionBillingUnitQuota:
		if delta != 0 {
			quotaDeltaByUserID[sub.UserId] += delta
		}
	case UserSubscriptionBillingUnitTokens:
		if delta != 0 {
			tokenDeltaByUserID[sub.UserId] += delta
		}
	default:
		return errors.New("billing_unit 无效")
	}
	_ = groupIDs
	return nil
}

func syncRequestUserSubscriptionToRevision(tx *gorm.DB, sub *UserRequestSubscription, revision *RedemptionPresetRevision, groupIDs []int, now int64) error {
	if tx == nil {
		tx = DB
	}
	if sub == nil || sub.Id <= 0 {
		return errors.New("subscription 无效")
	}
	if revision == nil || revision.Id <= 0 || revision.PresetId <= 0 {
		return errors.New("revision 无效")
	}
	newExpireAt, err := derivePresetRevisionExpireAt(sub.StartAt, revision.QuotaValidDays)
	if err != nil {
		return err
	}
	newDailyLimit := discreteUnitsFromDisplayInt(revision.DailyRequestLimit)
	if newDailyLimit < 0 {
		return errors.New("revision daily_request_limit 无效")
	}
	newDailyUsed := sub.DailyRequestUsed
	newDailyReset := sub.DailyRequestResetDate
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
	consumed := sub.TotalRequestUsed
	if consumed < 0 {
		consumed = 0
	}
	newTotalLimit := discreteUnitsFromDisplayInt(revision.Quota)
	if newTotalLimit < 0 {
		return errors.New("revision quota 无效")
	}
	newTotalUsed := sub.TotalRequestUsed
	if newTotalUsed < 0 {
		newTotalUsed = 0
	}
	if newTotalLimit > 0 && newTotalUsed > newTotalLimit {
		newTotalUsed = newTotalLimit
	}
	if newTotalLimit > 0 && consumed > newTotalLimit {
		consumed = newTotalLimit
	}
	invalidAt := sub.InvalidAt
	if newTotalLimit == 0 || consumed < newTotalLimit {
		invalidAt = 0
	} else if invalidAt <= 0 {
		invalidAt = now
	}
	updates := map[string]interface{}{
		"daily_request_limit":      newDailyLimit,
		"daily_request_used":       newDailyUsed,
		"daily_request_reset_date": newDailyReset,
		"total_request_limit":      newTotalLimit,
		"total_request_used":       newTotalUsed,
		"expire_at":                newExpireAt,
		"invalid_at":               invalidAt,
	}
	if newExpireAt > 0 && newExpireAt < now {
		updates["daily_request_used"] = 0
		updates["daily_request_reset_date"] = 0
	}
	if err := tx.Model(&UserRequestSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
		return err
	}
	if err := upsertUserRequestSubscriptionPresetRevisionBindingTx(tx, sub.Id, revision.PresetId, revision.Id); err != nil {
		return err
	}
	if err := upsertUserRequestSubscriptionGroupsTx(tx, sub.Id, groupIDs); err != nil {
		return err
	}
	return nil
}

func derivePresetRevisionExpireAt(startAt int64, quotaValidDays int) (int64, error) {
	if quotaValidDays < 0 {
		return 0, errors.New("quota_valid_days 无效")
	}
	if quotaValidDays == 0 {
		return 0, nil
	}
	if startAt <= 0 {
		return 0, errors.New("start_at 无效")
	}
	extendSeconds := int64(quotaValidDays) * common.SecondsPerDay
	if extendSeconds > common.MaxSupportedUnixTimestamp-startAt {
		return 0, errors.New("订阅有效期过大")
	}
	return startAt + extendSeconds, nil
}

func syncPresetSoldAssetsToRevisionTx(tx *gorm.DB, preset *RedemptionPreset, revision *RedemptionPresetRevision) error {
	if tx == nil {
		tx = DB
	}
	if preset == nil || preset.Id <= 0 {
		return errors.New("preset 无效")
	}
	if revision == nil || revision.Id <= 0 || revision.PresetId != preset.Id {
		return errors.New("revision 无效")
	}
	groupIDsByRevisionID, err := getRedemptionPresetRevisionGroupIDsByRevisionIDsTx(tx, []int{revision.Id})
	if err != nil {
		return err
	}
	groupIDs := normalizeUniqueSortedIDs(groupIDsByRevisionID[revision.Id])
	if (preset.Mode == "subscription" || preset.Mode == "tokens" || preset.Mode == "request") && len(groupIDs) == 0 {
		return errors.New("商品 revision 缺少可用分组")
	}

	now := time.Now().Unix()
	quotaDeltaByUserID := make(map[int]int, 8)
	tokenDeltaByUserID := make(map[int]int, 8)

	if preset.Mode == "subscription" || preset.Mode == "tokens" {
		expectedBillingUnit := UserSubscriptionBillingUnitQuota
		if preset.Mode == "tokens" {
			expectedBillingUnit = UserSubscriptionBillingUnitTokens
		}
		var subs []UserSubscription
		query := tx.Where("source_preset_id = ?", preset.Id)
		if expectedBillingUnit == UserSubscriptionBillingUnitTokens {
			query = query.Where("billing_unit = ?", UserSubscriptionBillingUnitTokens)
		} else {
			query = query.Where("(billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", UserSubscriptionBillingUnitQuota)
		}
		if err := query.Find(&subs).Error; err != nil {
			return err
		}
		for i := range subs {
			if err := syncQuotaUserSubscriptionToRevision(tx, &subs[i], revision, groupIDs, now, quotaDeltaByUserID, tokenDeltaByUserID); err != nil {
				return err
			}
		}
	}

	if preset.Mode == "request" {
		var subs []UserRequestSubscription
		if err := tx.Where("source_preset_id = ?", preset.Id).Find(&subs).Error; err != nil {
			return err
		}
		for i := range subs {
			if err := syncRequestUserSubscriptionToRevision(tx, &subs[i], revision, groupIDs, now); err != nil {
				return err
			}
		}
	}

	userIDSet := make(map[int]struct{}, len(quotaDeltaByUserID)+len(tokenDeltaByUserID))
	for userID, delta := range quotaDeltaByUserID {
		if delta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Update("quota", gorm.Expr("CASE WHEN quota + ? >= 0 THEN quota + ? ELSE 0 END", delta, delta)).Error; err != nil {
				return err
			}
			userIDSet[userID] = struct{}{}
		}
	}
	for userID, delta := range tokenDeltaByUserID {
		if delta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Update("tokens_quota", gorm.Expr("CASE WHEN tokens_quota + ? >= 0 THEN tokens_quota + ? ELSE 0 END", delta, delta)).Error; err != nil {
				return err
			}
			userIDSet[userID] = struct{}{}
		}
	}
	for userID := range userIDSet {
		if err := refreshUserSubscriptionSnapshot(tx, userID, now); err != nil {
			return err
		}
	}
	return nil
}

func BackfillRedemptionPresetRevisions(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&RedemptionPreset{}) || !tx.Migrator().HasTable(&RedemptionPresetRevision{}) {
		return nil
	}
	var presets []RedemptionPreset
	if err := tx.Find(&presets).Error; err != nil {
		return err
	}
	for i := range presets {
		if _, err := ensureCurrentRedemptionPresetRevisionTx(tx, presets[i].Id); err != nil {
			return err
		}
	}
	return nil
}

func BackfillSubscriptionOrderPresetRevisions(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&SubscriptionOrder{}) || !tx.Migrator().HasColumn(&SubscriptionOrder{}, "preset_revision_id") {
		return nil
	}
	var orders []SubscriptionOrder
	if err := tx.Model(&SubscriptionOrder{}).
		Select("id", "preset_id", "preset_revision_id").
		Where("preset_id > 0 AND preset_revision_id = 0").
		Find(&orders).Error; err != nil {
		return err
	}
	for _, order := range orders {
		revision, ok, err := ensureRedemptionPresetRevisionForBackfillTx(tx, order.PresetId)
		if err != nil {
			return err
		}
		if !ok {
			common.SysLog(fmt.Sprintf("skip backfilling subscription order preset revision: order_id=%d preset_id=%d because preset is deleted and no revision snapshot exists", order.Id, order.PresetId))
			continue
		}
		if err := tx.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).
			Update("preset_revision_id", revision.Id).Error; err != nil {
			return err
		}
	}
	return nil
}

func BackfillUserPresetRevisionBindings(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&UserSubscription{}) || !tx.Migrator().HasTable(&UserRequestSubscription{}) {
		return nil
	}

	var quotaSubs []UserSubscription
	if err := tx.Model(&UserSubscription{}).
		Select("id", "source_preset_id").
		Where("source_preset_id > 0").
		Find(&quotaSubs).Error; err != nil {
		return err
	}
	if len(quotaSubs) > 0 {
		existing, err := getUserSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx, func() []int {
			ids := make([]int, 0, len(quotaSubs))
			for _, sub := range quotaSubs {
				ids = append(ids, sub.Id)
			}
			return ids
		}())
		if err != nil {
			return err
		}
		for _, sub := range quotaSubs {
			if sub.Id <= 0 || sub.SourcePresetId <= 0 {
				continue
			}
			if _, ok := existing[sub.Id]; ok {
				continue
			}
			revision, found, err := ensureRedemptionPresetRevisionForBackfillTx(tx, sub.SourcePresetId)
			if err != nil {
				return err
			}
			if !found {
				common.SysLog(fmt.Sprintf("skip backfilling quota subscription preset revision binding: subscription_id=%d preset_id=%d because preset is deleted and no revision snapshot exists", sub.Id, sub.SourcePresetId))
				continue
			}
			if err := upsertUserSubscriptionPresetRevisionBindingTx(tx, sub.Id, sub.SourcePresetId, revision.Id); err != nil {
				return err
			}
		}
	}

	var requestSubs []UserRequestSubscription
	if err := tx.Model(&UserRequestSubscription{}).
		Select("id", "source_preset_id").
		Where("source_preset_id > 0").
		Find(&requestSubs).Error; err != nil {
		return err
	}
	if len(requestSubs) == 0 {
		return nil
	}
	existing, err := getUserRequestSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx, func() []int {
		ids := make([]int, 0, len(requestSubs))
		for _, sub := range requestSubs {
			ids = append(ids, sub.Id)
		}
		return ids
	}())
	if err != nil {
		return err
	}
	for _, sub := range requestSubs {
		if sub.Id <= 0 || sub.SourcePresetId <= 0 {
			continue
		}
		if _, ok := existing[sub.Id]; ok {
			continue
		}
		revision, found, err := ensureRedemptionPresetRevisionForBackfillTx(tx, sub.SourcePresetId)
		if err != nil {
			return err
		}
		if !found {
			common.SysLog(fmt.Sprintf("skip backfilling request subscription preset revision binding: subscription_id=%d preset_id=%d because preset is deleted and no revision snapshot exists", sub.Id, sub.SourcePresetId))
			continue
		}
		if err := upsertUserRequestSubscriptionPresetRevisionBindingTx(tx, sub.Id, sub.SourcePresetId, revision.Id); err != nil {
			return err
		}
	}
	return nil
}

package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"sort"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

// RedemptionPreset stores a pre-configured product spec used to generate redemption codes.
// PriceFen is the settlement amount in RMB fen (分) used for affiliate commission calculation.
type RedemptionPreset struct {
	Id int `json:"id"`

	Name        string `json:"name" gorm:"size:64;uniqueIndex;not null"`
	Description string `json:"description" gorm:"type:text;column:description"`
	Mode        string `json:"mode" gorm:"type:varchar(32);default:'subscription';column:mode"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;default:0;index"`

	PriceFen int64 `json:"price_fen" gorm:"type:bigint;default:0;column:price_fen"`
	// Stock is the remaining inventory for purchases (mainly used in /console/subscription).
	// nil means unlimited; 0 means sold out.
	Stock *int `json:"stock" gorm:"type:int;column:stock"`
	// PurchaseLimit limits how many times a user can purchase this preset via /api/subscription/order.
	// 0 means unlimited.
	PurchaseLimit int `json:"purchase_limit" gorm:"type:int;default:0;column:purchase_limit"`
	// MultiQuantityEnabled controls whether this preset can be purchased with quantity > 1 in /console/subscription.
	// When enabled, users can choose a quantity in /console/subscription.
	MultiQuantityEnabled bool `json:"multi_quantity_enabled" gorm:"type:boolean;default:false;column:multi_quantity_enabled"`
	// MultiQuantityDeferOnly controls whether quantity > 1 is restricted to "defer" apply_mode.
	// Default true to preserve historical behavior: multi-quantity purchases are defer-only (no stacking).
	MultiQuantityDeferOnly bool `json:"multi_quantity_defer_only" gorm:"type:boolean;default:true;column:multi_quantity_defer_only"`
	// Enabled controls whether this preset is on sale (shown) in /console/subscription.
	Enabled bool `json:"enabled" gorm:"type:boolean;default:true;index;column:enabled"`
	// Archived controls whether this preset is retired from product management listings.
	// Archived products are forcibly treated as not on sale.
	Archived bool `json:"archived" gorm:"type:boolean;default:false;index;column:archived"`

	// Quota represents:
	// - subscription: total quota to credit, 0 means unlimited.
	// - tokens: total tokens to credit, 0 means unlimited.
	// - request: total requests to credit, 0 means unlimited.
	// - payg: fixed quota to credit, must be > 0.
	Quota           int `json:"quota" gorm:"default:0"`
	DailyQuotaLimit int `json:"daily_quota_limit" gorm:"default:0"`
	// DailyRequestLimit configures per-day request count for request-count subscription products.
	DailyRequestLimit int `json:"daily_request_limit" gorm:"default:0;column:daily_request_limit"`
	QuotaValidDays    int `json:"quota_valid_days" gorm:"default:0"`

	ExpiredTime int64 `json:"expired_time" gorm:"bigint;default:0"`

	PlanValidDays int       `json:"plan_valid_days" gorm:"default:0;column:plan_valid_days"`
	ChannelIds    JSONValue `json:"channel_ids" gorm:"type:json;column:channel_ids"`
	// AllowedGroups limits which channel groups (tiers) this product can be consumed from.
	// Stored as JSON array of strings, e.g. ["sub"] or ["payg"].
	AllowedGroups   JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	AllowedGroupIds JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`
	// GroupDailyLimits configures per-group daily quota limits for subscription products.
	//
	// Semantics:
	// - When this field is omitted (nil) in the upsert payload, existing DB rows (if any) are kept intact.
	//   This preserves backward compatibility with older consoles that don't send this field.
	// - When provided as [] (empty array), all per-group daily limits are cleared (disables group-daily-limit mode).
	// - When provided with entries, it enables group-daily-limit mode and must cover all allowed_groups.
	GroupDailyLimits []GroupDailyQuotaLimit `json:"group_daily_limits" gorm:"-"`

	CreatedTime int64 `json:"created_time" gorm:"bigint"`
	UpdatedTime int64 `json:"updated_time" gorm:"bigint"`
}

func (p *RedemptionPreset) NormalizeAndValidate() error {
	if p == nil {
		return errors.New("preset 为空")
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)

	if utf8.RuneCountInString(p.Name) == 0 || utf8.RuneCountInString(p.Name) > 64 {
		return errors.New("商品名称长度必须在 1-64 之间")
	}
	if utf8.RuneCountInString(p.Description) > 2048 {
		return errors.New("商品描述长度不能超过 2048")
	}
	if p.SortOrder < 0 {
		return errors.New("sort_order 不能小于 0")
	}
	if p.PriceFen < 0 {
		return errors.New("price_fen 不能小于 0")
	}
	if p.PurchaseLimit < 0 {
		return errors.New("purchase_limit 不能小于 0")
	}
	if p.Stock != nil && *p.Stock < 0 {
		return errors.New("stock 不能小于 0")
	}
	if p.DailyQuotaLimit < 0 {
		return errors.New("daily_quota_limit 必须大于等于 0")
	}
	if p.DailyRequestLimit < 0 {
		return errors.New("daily_request_limit 必须大于等于 0")
	}
	if p.QuotaValidDays < 0 {
		return errors.New("quota_valid_days 不能小于 0")
	}
	if p.PlanValidDays < 0 {
		return errors.New("plan_valid_days 不能小于 0")
	}
	if p.ExpiredTime < 0 {
		return errors.New("expired_time 不能小于 0")
	}
	if p.Archived {
		p.Enabled = false
	}

	normalizedChannelIds := make([]int, 0)
	if len(p.ChannelIds) > 0 {
		var rawIds []int
		if err := common.Unmarshal([]byte(p.ChannelIds), &rawIds); err != nil {
			return errors.New("渠道集合解析失败")
		}
		idSet := make(map[int]struct{}, len(rawIds))
		for _, id := range rawIds {
			if id <= 0 {
				continue
			}
			if _, ok := idSet[id]; ok {
				continue
			}
			idSet[id] = struct{}{}
			normalizedChannelIds = append(normalizedChannelIds, id)
		}
	}

	normalizedAllowedGroupIDs := make([]int, 0)

	if len(p.AllowedGroups) > 0 {
		return errors.New("allowed_groups 已废弃，请使用 allowed_group_ids")
	}
	p.AllowedGroups = nil

	if len(p.AllowedGroupIds) > 0 {
		var rawIDs []int
		if err := common.Unmarshal([]byte(p.AllowedGroupIds), &rawIDs); err != nil {
			return errors.New("可用分组解析失败")
		}
		normalizedAllowedGroupIDs = normalizeUniqueSortedIDs(rawIDs)
		if len(normalizedAllowedGroupIDs) > 0 {
			if err := ValidateGroupIDsExist(nil, normalizedAllowedGroupIDs); err != nil {
				return err
			}
			if b, err := common.Marshal(normalizedAllowedGroupIDs); err == nil {
				p.AllowedGroupIds = JSONValue(b)
			} else {
				return err
			}
		} else {
			p.AllowedGroupIds = nil
		}
	} else {
		p.AllowedGroupIds = nil
	}

	mode := strings.TrimSpace(p.Mode)
	if mode == "" {
		return errors.New("mode 必须显式指定")
	}
	if mode == "free" {
		return errors.New("自由额度商品已下线")
	}
	if mode == "xiaotuan" {
		return errors.New("小团订阅商品已下线")
	}
	p.Mode = mode

	switch mode {
	case "subscription":
		if p.Quota < 0 {
			return errors.New("额度必须大于等于 0")
		}
		if p.DailyQuotaLimit < 0 {
			return errors.New("每日额度必须大于等于 0")
		}
		if p.DailyRequestLimit != 0 {
			return errors.New("订阅额度商品参数错误")
		}
		if p.PlanValidDays != 0 || len(normalizedChannelIds) != 0 {
			return errors.New("订阅额度商品参数错误")
		}
		if len(normalizedAllowedGroupIDs) == 0 {
			return errors.New("请选择可用分组")
		}
	case "tokens":
		if p.Quota < 0 {
			return errors.New("额度必须大于等于 0")
		}
		if p.DailyQuotaLimit < 0 {
			return errors.New("每日额度必须大于等于 0")
		}
		if p.DailyRequestLimit != 0 {
			return errors.New("tokens 商品参数错误")
		}
		if p.PlanValidDays != 0 || len(normalizedChannelIds) != 0 {
			return errors.New("tokens 商品参数错误")
		}
		if len(normalizedAllowedGroupIDs) == 0 {
			return errors.New("请选择可用分组")
		}
	case "request":
		// Request-count subscription: request quota (total) + optional daily request limit + validity days + allowed groups.
		if p.DailyRequestLimit < 0 {
			return errors.New("每日次数必须大于等于 0")
		}
		if p.Quota < 0 {
			return errors.New("总次数必须大于等于 0")
		}
		if p.DailyQuotaLimit != 0 {
			return errors.New("次数订阅商品不支持日额度")
		}
		if p.PlanValidDays != 0 || len(normalizedChannelIds) != 0 {
			return errors.New("次数订阅商品参数错误")
		}
		if len(normalizedAllowedGroupIDs) == 0 {
			return errors.New("请选择可用分组")
		}
	case "payg":
		p.MultiQuantityEnabled = false
		p.MultiQuantityDeferOnly = true
		if p.Quota <= 0 {
			return errors.New("额度必须大于 0")
		}
		if p.DailyQuotaLimit != 0 || p.DailyRequestLimit != 0 || p.QuotaValidDays != 0 || p.PlanValidDays != 0 || len(normalizedChannelIds) != 0 {
			return errors.New("按量付费商品参数错误")
		}
		if len(normalizedAllowedGroupIDs) == 0 {
			return errors.New("请选择可用分组")
		}
	default:
		return errors.New("无效的商品类型")
	}
	return nil
}

func UpsertRedemptionPreset(tx *gorm.DB, preset *RedemptionPreset, options RedemptionPresetUpsertOptions) (*RedemptionPreset, error) {
	if preset == nil {
		return nil, errors.New("preset 为空")
	}
	groupDailyLimitsProvided := preset.GroupDailyLimits != nil
	if err := preset.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	preset.UpdatedTime = now

	upsertTx := func(tx *gorm.DB) (*RedemptionPreset, error) {
		var existing RedemptionPreset
		var existingAllowedGroupIDs []int
		newAllowedGroupIDs := []int{}
		if len(preset.AllowedGroupIds) > 0 {
			if err := common.Unmarshal([]byte(preset.AllowedGroupIds), &newAllowedGroupIDs); err != nil {
				return nil, err
			}
			newAllowedGroupIDs = normalizeUniqueSortedIDs(newAllowedGroupIDs)
		}
		loadExistingAllowedGroupIDs := func(presetID int, existing *RedemptionPreset) ([]int, error) {
			if presetID <= 0 {
				return nil, nil
			}
			ids, err := getSubscriptionProductGroupIDsTx(tx, presetID)
			if err != nil {
				return nil, err
			}
			if len(ids) > 0 {
				return normalizeUniqueSortedIDs(ids), nil
			}
			if existing != nil && len(existing.AllowedGroupIds) > 0 {
				var ids []int
				if err := common.Unmarshal([]byte(existing.AllowedGroupIds), &ids); err == nil {
					return normalizeUniqueSortedIDs(ids), nil
				}
			}
			return nil, nil
		}
		ensureGroupDailyLimitsNotStale := func(presetID int) error {
			if presetID <= 0 {
				return nil
			}
			mode := strings.TrimSpace(preset.Mode)
			if mode != "subscription" && mode != "tokens" {
				return nil
			}
			// Only guard when the product already has group-daily-limit rows but the payload doesn't carry them.
			if groupDailyLimitsProvided {
				return nil
			}
			has, err := hasSubscriptionProductGroupDailyLimitsTx(tx, presetID)
			if err != nil {
				return err
			}
			if !has {
				return nil
			}

			// Prevent silently changing allowed_groups without updating group_daily_limits.
			var newIDs []int
			if len(preset.AllowedGroupIds) > 0 {
				if err := common.Unmarshal([]byte(preset.AllowedGroupIds), &newIDs); err != nil {
					return err
				}
				newIDs = normalizeUniqueSortedIDs(newIDs)
			}
			oldIDs := existingAllowedGroupIDs
			if len(oldIDs) == 0 {
				var err error
				oldIDs, err = loadExistingAllowedGroupIDs(presetID, &existing)
				if err != nil {
					return err
				}
				existingAllowedGroupIDs = oldIDs
			}

			if len(oldIDs) != len(newIDs) {
				return errors.New("该商品已启用“分组日限额”，修改可用分组时必须同时提交 group_daily_limits")
			}
			for i := range oldIDs {
				if oldIDs[i] != newIDs[i] {
					return errors.New("该商品已启用“分组日限额”，修改可用分组时必须同时提交 group_daily_limits")
				}
			}
			return nil
		}
		syncSubscriptionProductGroupDailyLimits := func(presetID int, allowedGroupIDs []int) error {
			if presetID <= 0 {
				return nil
			}
			// Non-subscription products must not carry group-daily-limit configs.
			mode := strings.TrimSpace(preset.Mode)
			if mode != "subscription" && mode != "tokens" {
				return upsertSubscriptionProductGroupDailyLimitsTx(tx, presetID, map[int]int{})
			}
			if !groupDailyLimitsProvided {
				return nil
			}

			allowedIDs := normalizeUniqueSortedIDs(allowedGroupIDs)
			if len(allowedIDs) == 0 {
				return errors.New("请选择可用分组")
			}

			normalized, err := normalizeGroupDailyQuotaLimits(preset.GroupDailyLimits)
			if err != nil {
				return err
			}

			if len(normalized) == 0 {
				// Explicitly clear (disable group-daily-limit mode).
				return upsertSubscriptionProductGroupDailyLimitsTx(tx, presetID, map[int]int{})
			}

			derivedDailyLimit := 0
			for _, item := range normalized {
				if item.DailyQuotaLimit == 0 {
					derivedDailyLimit = 0
					break
				}
				derivedDailyLimit += item.DailyQuotaLimit
			}
			// Keep preset.daily_quota_limit consistent with the derived total, so legacy/subsequent
			// flows (e.g. subscription creation) have a sensible aggregate value.
			preset.DailyQuotaLimit = derivedDailyLimit
			if err := tx.Model(&RedemptionPreset{}).Where("id = ?", presetID).
				Update("daily_quota_limit", derivedDailyLimit).Error; err != nil {
				return err
			}

			allowedSet := make(map[int]struct{}, len(allowedIDs))
			for _, gid := range allowedIDs {
				allowedSet[gid] = struct{}{}
			}
			limitSet := make(map[int]struct{}, len(normalized))
			groupLimitByID := make(map[int]int, len(normalized))
			for _, item := range normalized {
				if _, ok := allowedSet[item.GroupId]; !ok {
					return errors.New("分组日限额包含未授权分组")
				}
				limitSet[item.GroupId] = struct{}{}
				groupLimitByID[item.GroupId] = item.DailyQuotaLimit
			}
			if len(limitSet) != len(allowedSet) {
				return errors.New("分组日限额必须覆盖所有可用分组")
			}
			return upsertSubscriptionProductGroupDailyLimitsTx(tx, presetID, groupLimitByID)
		}
		syncSubscriptionProductGroups := func(presetID int, allowedGroupIDs []int) error {
			if presetID <= 0 {
				return nil
			}
			mode := strings.TrimSpace(preset.Mode)
			if mode != "subscription" && mode != "tokens" && mode != "request" {
				return tx.Where("product_id = ?", presetID).Delete(&SubscriptionProductGroup{}).Error
			}
			groupIDs := normalizeUniqueSortedIDs(allowedGroupIDs)
			if len(groupIDs) == 0 {
				return tx.Where("product_id = ?", presetID).Delete(&SubscriptionProductGroup{}).Error
			}
			return upsertSubscriptionProductGroupsTx(tx, presetID, groupIDs)
		}
		ensureAllowedGroupsNotCleared := func(presetID int) error {
			if presetID <= 0 {
				return nil
			}
			ids := normalizeUniqueSortedIDs(newAllowedGroupIDs)
			if len(ids) > 0 {
				return nil
			}
			var cnt int64
			if err := tx.Model(&UserSubscription{}).Where("source_preset_id = ?", presetID).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				return errors.New("该商品已存在订阅购买记录，可用分组不能为空")
			}
			if err := tx.Model(&UserRequestSubscription{}).Where("source_preset_id = ?", presetID).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				return errors.New("该商品已存在订阅购买记录，可用分组不能为空")
			}
			return nil
		}
		finalizeRevision := func(currentPreset *RedemptionPreset, existingMode string) (*RedemptionPreset, error) {
			revision, err := createRedemptionPresetRevisionTx(tx, currentPreset)
			if err != nil {
				return nil, err
			}
			if !options.SyncSoldAssets {
				return currentPreset, nil
			}
			mode := strings.TrimSpace(currentPreset.Mode)
			if mode != "subscription" && mode != "tokens" && mode != "request" {
				return nil, errors.New("当前仅支持订阅类商品同步已售资产")
			}
			if existingMode != "" && existingMode != mode {
				return nil, errors.New("跨商品类型的已售资产同步暂不支持，请先仅更新未来销售商品")
			}
			if err := syncPresetSoldAssetsToRevisionTx(tx, currentPreset, revision); err != nil {
				return nil, err
			}
			return currentPreset, nil
		}

		if preset.Id > 0 {
			if err := tx.First(&existing, "id = ?", preset.Id).Error; err != nil {
				return nil, err
			}
			if err := ensureGroupDailyLimitsNotStale(preset.Id); err != nil {
				return nil, err
			}
			if preset.Name != "" && preset.Name != existing.Name {
				var cnt int64
				if err := tx.Model(&RedemptionPreset{}).
					Where("name = ? AND id <> ?", preset.Name, preset.Id).
					Count(&cnt).Error; err != nil {
					return nil, err
				}
				if cnt > 0 {
					return nil, errors.New("商品名称已存在")
				}
			}

			if existing.CreatedTime > 0 {
				preset.CreatedTime = existing.CreatedTime
			} else if preset.CreatedTime == 0 {
				preset.CreatedTime = now
			}
			if err := ensureAllowedGroupsNotCleared(preset.Id); err != nil {
				return nil, err
			}

			if err := tx.Save(preset).Error; err != nil {
				return nil, err
			}
			if err := syncSubscriptionProductGroups(preset.Id, newAllowedGroupIDs); err != nil {
				return nil, err
			}
			if err := syncSubscriptionProductGroupDailyLimits(preset.Id, newAllowedGroupIDs); err != nil {
				return nil, err
			}
			if _, err := ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
				return nil, err
			}
			return finalizeRevision(preset, strings.TrimSpace(existing.Mode))
		}

		err := tx.Where("name = ?", preset.Name).First(&existing).Error
		if err == nil {
			preset.Id = existing.Id
			if err := ensureGroupDailyLimitsNotStale(preset.Id); err != nil {
				return nil, err
			}
			if existing.CreatedTime > 0 {
				preset.CreatedTime = existing.CreatedTime
			} else if preset.CreatedTime == 0 {
				preset.CreatedTime = now
			}
			if err := ensureAllowedGroupsNotCleared(preset.Id); err != nil {
				return nil, err
			}

			if err := tx.Save(preset).Error; err != nil {
				return nil, err
			}
			if err := syncSubscriptionProductGroups(preset.Id, newAllowedGroupIDs); err != nil {
				return nil, err
			}
			if err := syncSubscriptionProductGroupDailyLimits(preset.Id, newAllowedGroupIDs); err != nil {
				return nil, err
			}
			if _, err := ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
				return nil, err
			}
			return finalizeRevision(preset, strings.TrimSpace(existing.Mode))
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if preset.CreatedTime == 0 {
			preset.CreatedTime = now
		}
		if err := tx.Create(preset).Error; err != nil {
			return nil, err
		}
		if err := syncSubscriptionProductGroups(preset.Id, newAllowedGroupIDs); err != nil {
			return nil, err
		}
		if err := syncSubscriptionProductGroupDailyLimits(preset.Id, newAllowedGroupIDs); err != nil {
			return nil, err
		}
		if _, err := ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
			return nil, err
		}
		return finalizeRevision(preset, "")
	}

	if tx == nil {
		var res *RedemptionPreset
		err := DB.Transaction(func(tx *gorm.DB) error {
			p, err := upsertTx(tx)
			if err != nil {
				return err
			}
			res = p
			return nil
		})
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	return upsertTx(tx)
}

func GetRedemptionPresetByName(name string) (*RedemptionPreset, error) {
	if name == "" {
		return nil, errors.New("name 为空")
	}
	var preset RedemptionPreset
	if err := DB.Where("name = ?", name).First(&preset).Error; err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetAllowedGroups(&preset); err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetGroupDailyLimits(&preset); err != nil {
		return nil, err
	}
	NormalizeCompatibleRedemptionPresetMode(&preset)
	return &preset, nil
}

func GetRedemptionPresetById(id int) (*RedemptionPreset, error) {
	if id <= 0 {
		return nil, errors.New("id 无效")
	}
	var preset RedemptionPreset
	if err := DB.Where("id = ?", id).First(&preset).Error; err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetAllowedGroups(&preset); err != nil {
		return nil, err
	}
	if err := hydrateRedemptionPresetGroupDailyLimits(&preset); err != nil {
		return nil, err
	}
	NormalizeCompatibleRedemptionPresetMode(&preset)
	return &preset, nil
}

func ListRedemptionPresets() ([]*RedemptionPreset, error) {
	var presets []*RedemptionPreset
	if err := DB.Order("sort_order DESC, updated_time DESC, id DESC").Find(&presets).Error; err != nil {
		return nil, err
	}
	for _, preset := range presets {
		if preset == nil {
			continue
		}
		if err := hydrateRedemptionPresetAllowedGroups(preset); err != nil {
			return nil, err
		}
	}
	if err := hydrateRedemptionPresetsGroupDailyLimits(presets); err != nil {
		return nil, err
	}
	for _, preset := range presets {
		NormalizeCompatibleRedemptionPresetMode(preset)
	}
	return presets, nil
}

func ListSubscriptionRedemptionPresets() ([]*RedemptionPreset, error) {
	var presets []*RedemptionPreset
	if err := DB.Where("price_fen > 0 AND enabled = ?", true).
		Order("sort_order DESC, updated_time DESC, id DESC").
		Find(&presets).Error; err != nil {
		return nil, err
	}
	for _, preset := range presets {
		if preset == nil {
			continue
		}
		if err := hydrateRedemptionPresetAllowedGroups(preset); err != nil {
			return nil, err
		}
	}
	if err := hydrateRedemptionPresetsGroupDailyLimits(presets); err != nil {
		return nil, err
	}
	filtered := make([]*RedemptionPreset, 0, len(presets))
	for _, preset := range presets {
		if preset == nil {
			continue
		}
		NormalizeCompatibleRedemptionPresetMode(preset)
		mode := strings.TrimSpace(preset.Mode)
		if mode != "subscription" && mode != "tokens" && mode != "request" {
			continue
		}
		if !jsonValueHasElements(preset.AllowedGroupIds) {
			continue
		}
		filtered = append(filtered, preset)
	}
	return filtered, nil
}

func hydrateRedemptionPresetAllowedGroups(preset *RedemptionPreset) error {
	if preset == nil {
		return nil
	}
	if preset.Id <= 0 {
		return nil
	}
	// allowed_groups is deprecated; use allowed_group_ids as the source of truth.
	preset.AllowedGroups = nil
	if len(preset.AllowedGroupIds) == 0 {
		groupIDs, err := getSubscriptionProductGroupIDsTx(DB, preset.Id)
		if err != nil {
			return err
		}
		if len(groupIDs) > 0 {
			if b, err := common.Marshal(groupIDs); err == nil {
				preset.AllowedGroupIds = JSONValue(b)
			}
		}
	}
	return nil
}

func hydrateRedemptionPresetGroupDailyLimits(preset *RedemptionPreset) error {
	if preset == nil || preset.Id <= 0 {
		return nil
	}
	return hydrateRedemptionPresetsGroupDailyLimits([]*RedemptionPreset{preset})
}

func hydrateRedemptionPresetsGroupDailyLimits(presets []*RedemptionPreset) error {
	if len(presets) == 0 {
		return nil
	}
	idToPreset := make(map[int]*RedemptionPreset, len(presets))
	productIDs := make([]int, 0, len(presets))
	for _, preset := range presets {
		if preset == nil || preset.Id <= 0 {
			continue
		}
		idToPreset[preset.Id] = preset
		productIDs = append(productIDs, preset.Id)
	}
	productIDs = normalizeUniqueSortedIDs(productIDs)
	if len(productIDs) == 0 {
		return nil
	}

	limitByProductID, err := getSubscriptionProductGroupDailyLimitsByProductIDsTx(DB, productIDs)
	if err != nil {
		return err
	}

	groupIDSet := make(map[int]struct{}, 16)
	for _, m := range limitByProductID {
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
		if err := ValidateGroupIDsExist(DB, groupIDs); err != nil {
			return err
		}
	}

	for pid, m := range limitByProductID {
		preset, ok := idToPreset[pid]
		if !ok || preset == nil {
			continue
		}
		items := make([]GroupDailyQuotaLimit, 0, len(m))
		for gid, quota := range m {
			if gid <= 0 {
				continue
			}
			if quota < 0 {
				return fmt.Errorf("订阅商品 #%d daily_limit_quota 数据错误", pid)
			}
			items = append(items, GroupDailyQuotaLimit{
				GroupId:         gid,
				DailyQuotaLimit: quota,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].GroupId < items[j].GroupId
		})
		preset.GroupDailyLimits = items
	}

	// Keep the field stable for frontend: return [] instead of null.
	for _, preset := range presets {
		if preset == nil || preset.Id <= 0 {
			continue
		}
		if preset.GroupDailyLimits == nil {
			preset.GroupDailyLimits = []GroupDailyQuotaLimit{}
		}
	}
	return nil
}

func DeleteRedemptionPresetById(id int) error {
	if id <= 0 {
		return errors.New("id 无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&RedemptionPreset{}).Error; err != nil {
			return err
		}

		var soldCount int64
		if err := tx.Model(&UserSubscription{}).Where("source_preset_id = ?", id).Count(&soldCount).Error; err != nil {
			return err
		}
		if soldCount > 0 {
			return errors.New("商品已有关联的已售订阅资产，禁止删除，请改为停用商品")
		}
		if err := tx.Model(&UserRequestSubscription{}).Where("source_preset_id = ?", id).Count(&soldCount).Error; err != nil {
			return err
		}
		if soldCount > 0 {
			return errors.New("商品已有关联的已售订阅资产，禁止删除，请改为停用商品")
		}
		if err := tx.Model(&SubscriptionOrder{}).Where("preset_id = ?", id).Count(&soldCount).Error; err != nil {
			return err
		}
		if soldCount > 0 {
			return errors.New("商品已有关联的订单历史，禁止删除，请改为停用商品")
		}

		// Clean up product bindings to avoid leaving orphan references.
		if tx.Migrator().HasTable(&SubscriptionProductGroup{}) {
			if err := tx.Where("product_id = ?", id).Delete(&SubscriptionProductGroup{}).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable(&SubscriptionProductGroupDailyLimit{}) {
			if err := tx.Where("product_id = ?", id).Delete(&SubscriptionProductGroupDailyLimit{}).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable(&RedemptionPresetRevisionGroup{}) {
			var revisionIDs []int
			if err := tx.Model(&RedemptionPresetRevision{}).
				Where("preset_id = ?", id).
				Order("id ASC").
				Pluck("id", &revisionIDs).Error; err != nil {
				return err
			}
			revisionIDs = normalizeUniqueSortedIDs(revisionIDs)
			if len(revisionIDs) > 0 {
				if err := tx.Where("revision_id IN ?", revisionIDs).Delete(&RedemptionPresetRevisionGroup{}).Error; err != nil {
					return err
				}
				if err := tx.Where("revision_id IN ?", revisionIDs).Delete(&RedemptionPresetRevisionGroupDailyLimit{}).Error; err != nil {
					return err
				}
			}
		}
		if tx.Migrator().HasTable(&RedemptionPresetRevision{}) {
			if err := tx.Where("preset_id = ?", id).Delete(&RedemptionPresetRevision{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Delete(&RedemptionPreset{}, "id = ?", id).Error; err != nil {
			return err
		}
		if _, err := ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
			return err
		}
		return nil
	})
}

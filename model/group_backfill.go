package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"one-api/setting"
	"one-api/setting/payg_setting"
	"one-api/setting/ratio_setting"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func readLegacyOptionValue(tx *gorm.DB, key string) (string, bool, error) {
	if tx == nil {
		tx = DB
	}
	var opt Option
	// Use Find instead of First so missing options don't produce gorm.ErrRecordNotFound logs
	// in debug mode. Missing legacy options are expected and should not look like errors.
	result := tx.Select("key", "value").Where(commonKeyCol+" = ?", key).Limit(1).Find(&opt)
	if result.Error != nil {
		return "", false, result.Error
	}
	if result.RowsAffected == 0 {
		return "", false, nil
	}
	return opt.Value, true, nil
}

func parseLegacyGroupRatioOption(raw string) (map[string]float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	out := make(map[string]float64)
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, fmt.Errorf("GroupRatio 配置格式错误: %w", err)
	}
	// Validate
	for name, ratio := range out {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("GroupRatio 存在空分组名")
		}
		if ratio < 0 {
			return nil, fmt.Errorf("GroupRatio 分组倍率必须大于等于 0: %s", name)
		}
	}
	return out, nil
}

func parseLegacyUserUsableGroupsOption(raw string) (map[string]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	out := make(map[string]string)
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, fmt.Errorf("UserUsableGroups 配置格式错误: %w", err)
	}
	for k := range out {
		if strings.TrimSpace(k) == "" {
			return nil, errors.New("UserUsableGroups 存在空分组名")
		}
	}
	return out, nil
}

func BackfillGroupsFromLegacyGroupsTable(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&Group{}) {
		return nil
	}
	if !tx.Migrator().HasTable("groups") {
		return nil
	}

	var count int64
	if err := tx.Model(&Group{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type legacyGroupRow struct {
		Code           string    `gorm:"column:code"`
		DisplayName    string    `gorm:"column:display_name"`
		Ratio          float64   `gorm:"column:ratio"`
		UserSelectable bool      `gorm:"column:user_selectable"`
		Enabled        bool      `gorm:"column:enabled"`
		CreatedAt      time.Time `gorm:"column:created_at"`
		UpdatedAt      time.Time `gorm:"column:updated_at"`
	}

	var rows []legacyGroupRow
	if err := tx.Table("groups").
		Select("code", "display_name", "ratio", "user_selectable", "enabled", "created_at", "updated_at").
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	groups := make([]Group, 0, len(rows))
	seenCode := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		code, err := normalizeGroupCode(row.Code)
		if err != nil {
			return fmt.Errorf("legacy groups 分组 code 无效(%q): %w", row.Code, err)
		}
		if _, ok := seenCode[code]; ok {
			continue
		}
		seenCode[code] = struct{}{}
		if err := validateGroupRatio(row.Ratio); err != nil {
			return fmt.Errorf("legacy groups 分组 %s 倍率无效: %w", code, err)
		}
		description, err := normalizeGroupDescription(row.DisplayName)
		if err != nil {
			return fmt.Errorf("legacy groups 分组 %s 说明无效: %w", code, err)
		}
		g := Group{
			Code:           code,
			Name:           code,
			DisplayName:    code,
			Description:    description,
			Ratio:          row.Ratio,
			UserSelectable: row.UserSelectable,
			Enabled:        row.Enabled,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}
		groups = append(groups, g)
	}
	if len(groups) == 0 {
		return nil
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Code < groups[j].Code })
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&groups).Error
}

// BackfillGroupsFromLegacyOptions initializes the new groups table from legacy options:
// - GroupRatio (name/code -> ratio)
// - UserUsableGroups (name/code -> description, and also marks user_selectable)
//
// It only runs when groups table is empty.
func BackfillGroupsFromLegacyOptions(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}

	var count int64
	if err := tx.Model(&Group{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// Legacy sources (optional).
	rawGroupRatio, hasGroupRatio, err := readLegacyOptionValue(tx, "GroupRatio")
	if err != nil {
		return err
	}
	rawUsableGroups, hasUsableGroups, err := readLegacyOptionValue(tx, "UserUsableGroups")
	if err != nil {
		return err
	}

	groupRatio := map[string]float64{}
	if hasGroupRatio {
		parsed, err := parseLegacyGroupRatioOption(rawGroupRatio)
		if err != nil {
			return err
		}
		for k, v := range parsed {
			code := strings.TrimSpace(k)
			if code == "" {
				continue
			}
			groupRatio[code] = v
		}
	}
	if groupRatio == nil {
		groupRatio = make(map[string]float64)
	}

	usableGroups := map[string]string{}
	if hasUsableGroups {
		parsed, err := parseLegacyUserUsableGroupsOption(rawUsableGroups)
		if err != nil {
			return err
		}
		for k, v := range parsed {
			code := strings.TrimSpace(k)
			if code == "" {
				continue
			}
			usableGroups[code] = v
		}
	}
	if usableGroups == nil {
		usableGroups = make(map[string]string)
	}
	if len(usableGroups) == 0 {
		usableGroups["default"] = "默认分组"
	}

	if len(groupRatio) == 0 {
		groupRatio["default"] = 1
	}

	if _, ok := groupRatio["default"]; !ok {
		groupRatio["default"] = 1
	}

	// Create groups for the union of:
	// - GroupRatio keys (ratio source of truth)
	// - UserUsableGroups keys (UI selectable groups might not have explicit ratio config historically)
	codeSet := make(map[string]struct{}, len(groupRatio)+len(usableGroups)+4)
	for code := range groupRatio {
		codeSet[code] = struct{}{}
	}
	for code := range usableGroups {
		codeSet[code] = struct{}{}
	}
	codeSet["default"] = struct{}{}

	codes := make([]string, 0, len(codeSet))
	for code := range codeSet {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	groups := make([]Group, 0, len(codes))
	for _, code := range codes {
		if strings.TrimSpace(code) == "auto" {
			// "auto" is a pseudo-group. Keep it out of DB.
			continue
		}
		normalizedCode, err := normalizeGroupCode(code)
		if err != nil {
			return fmt.Errorf("分组 code 无效(%q): %w", code, err)
		}
		ratio, ok := groupRatio[normalizedCode]
		if !ok {
			ratio = 1
		}
		if err := validateGroupRatio(ratio); err != nil {
			return fmt.Errorf("分组 %s 倍率无效: %w", normalizedCode, err)
		}

		desc, selectable := usableGroups[normalizedCode]
		description, err := normalizeGroupDescription(desc)
		if err != nil {
			return fmt.Errorf("分组 %s 说明无效: %w", normalizedCode, err)
		}

		g := Group{
			Code:           normalizedCode,
			Name:           normalizedCode,
			DisplayName:    normalizedCode,
			Description:    description,
			Ratio:          ratio,
			UserSelectable: selectable,
			Enabled:        true,
		}
		groups = append(groups, g)
	}
	if len(groups) == 0 {
		return errors.New("初始化分组失败：GroupRatio 为空")
	}

	// Avoid race on multi-node first boot.
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&groups).Error
}

// SyncGroupSettingsFromDB updates in-memory group ratio + user-usable group maps from groups table.
// It preserves the existing public APIs in ratio_setting/setting packages, while moving the source
// of truth to database table `groups`.
func SyncGroupSettingsFromDB(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}

	if !tx.Migrator().HasTable(&Group{}) {
		return errors.New("groups 表未就绪")
	}

	if updated, err := ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
		return err
	} else if updated > 0 {
		common.SysLog(fmt.Sprintf("reconciled group no-billing product refs for %d groups", updated))
	}

	groups, err := ListGroups(tx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		// Try initialize from legacy options once.
		if err := BackfillGroupsFromLegacyOptions(tx); err != nil {
			return err
		}
		groups, err = ListGroups(tx)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			return errors.New("分组表为空，无法加载分组配置")
		}
	}

	syncGroupLabelsFromGroups(groups)
	if err := syncGroupNoBillingFromGroups(groups); err != nil {
		return err
	}

	groupRatio := make(map[int]float64, len(groups))
	usableGroups := make(map[int]string, len(groups))
	enabledGroups := make(map[int]bool, len(groups))
	manualAllowedModels := make(map[int][]string)
	prefillGroupIDsByGroup := make(map[int][]int)
	prefillGroupIDSet := make(map[int]struct{})
	allowedUserAgents := make(map[int][]string)

	for _, g := range groups {
		if g.Id <= 0 {
			continue
		}
		groupRatio[g.Id] = g.Ratio
		if _, list, err := normalizeAllowedModelsJSON(g.AllowedModels); err != nil {
			return fmt.Errorf("分组 %s 可选模型配置无效: %w", strings.TrimSpace(g.Code), err)
		} else if len(list) > 0 {
			manualAllowedModels[g.Id] = list
		}
		if _, ids, err := normalizeAllowedModelPrefillGroupIDsJSON(g.AllowedModelPrefillGroupIds); err != nil {
			return fmt.Errorf("分组 %s 可选模型预填组配置无效: %w", strings.TrimSpace(g.Code), err)
		} else if len(ids) > 0 {
			prefillGroupIDsByGroup[g.Id] = ids
			for _, id := range ids {
				prefillGroupIDSet[id] = struct{}{}
			}
		}
		if _, list, err := normalizeAllowedUserAgentsJSON(g.AllowedUserAgents); err != nil {
			return fmt.Errorf("分组 %s 允许UA 配置无效: %w", strings.TrimSpace(g.Code), err)
		} else if len(list) > 0 {
			allowedUserAgents[g.Id] = list
		}
		if g.Enabled {
			enabledGroups[g.Id] = true
		}
			if g.Enabled && g.UserSelectable && !IsInternalDefaultModelGroupCode(g.Code) {
				desc := strings.TrimSpace(g.Description)
				if desc == "" {
					desc = strings.TrimSpace(g.Code)
			}
			usableGroups[g.Id] = desc
		}
	}

	allowedModels := make(map[int]map[string]struct{})
	if len(prefillGroupIDSet) > 0 {
		prefillGroupIDs := make([]int, 0, len(prefillGroupIDSet))
		for id := range prefillGroupIDSet {
			prefillGroupIDs = append(prefillGroupIDs, id)
		}
		sort.Ints(prefillGroupIDs)

		var prefillGroups []PrefillGroup
		if err := tx.Model(&PrefillGroup{}).
			Select("id", "items", "type").
			Where("id IN ? AND type = ?", prefillGroupIDs, "model").
			Find(&prefillGroups).Error; err != nil {
			return err
		}
		if len(prefillGroups) != len(prefillGroupIDs) {
			existSet := make(map[int]struct{}, len(prefillGroups))
			for _, pg := range prefillGroups {
				if pg.Id > 0 {
					existSet[pg.Id] = struct{}{}
				}
			}
			missing := make([]int, 0)
			for _, id := range prefillGroupIDs {
				if _, ok := existSet[id]; ok {
					continue
				}
				missing = append(missing, id)
			}
			if len(missing) > 0 {
				sort.Ints(missing)
				return fmt.Errorf("模型预填组不存在: %v", missing)
			}
		}

		prefillModelsByID := make(map[int]map[string]struct{}, len(prefillGroups))
		for _, pg := range prefillGroups {
			if pg.Id <= 0 {
				continue
			}
			var items []string
			if err := json.Unmarshal(pg.Items, &items); err != nil {
				return fmt.Errorf("模型预填组 #%d items 配置无效: %w", pg.Id, err)
			}
			normalized, err := normalizeAllowedModels(items)
			if err != nil {
				return fmt.Errorf("模型预填组 #%d items 配置无效: %w", pg.Id, err)
			}
			if len(normalized) == 0 {
				return fmt.Errorf("模型预填组 #%d items 不能为空", pg.Id)
			}
			set := make(map[string]struct{}, len(normalized))
			for _, m := range normalized {
				set[m] = struct{}{}
			}
			prefillModelsByID[pg.Id] = set
		}

		for _, g := range groups {
			if g.Id <= 0 {
				continue
			}
			models := make(map[string]struct{})
			if list := manualAllowedModels[g.Id]; len(list) > 0 {
				for _, m := range list {
					models[m] = struct{}{}
				}
			}
			if ids := prefillGroupIDsByGroup[g.Id]; len(ids) > 0 {
				for _, id := range ids {
					set := prefillModelsByID[id]
					if set == nil {
						return fmt.Errorf("模型预填组不存在: %d", id)
					}
					for m := range set {
						models[m] = struct{}{}
					}
				}
			}
			if len(models) > 0 {
				allowedModels[g.Id] = models
			}
		}
	} else {
		for gid, list := range manualAllowedModels {
			if gid <= 0 || len(list) == 0 {
				continue
			}
			set := make(map[string]struct{}, len(list))
			for _, m := range list {
				set[m] = struct{}{}
			}
			allowedModels[gid] = set
		}
	}

	// Convert to JSON and feed existing settings updaters.
	if b, err := common.Marshal(groupRatio); err == nil {
		if err := ratio_setting.UpdateGroupRatioByJSONString(string(b)); err != nil {
			return err
		}
	} else {
		return err
	}
	if b, err := common.Marshal(usableGroups); err == nil {
		if err := setting.UpdateUserUsableGroupsByJSONString(string(b)); err != nil {
			return err
		}
	} else {
		return err
	}
	if b, err := common.Marshal(enabledGroups); err == nil {
		if err := setting.UpdateEnabledGroupsByJSONString(string(b)); err != nil {
			return err
		}
	} else {
		return err
	}
	setting.ReplaceGroupAllowedModels(allowedModels)
	setting.ReplaceGroupAllowedUserAgents(allowedUserAgents)
	markGroupSettingsSynced(readGroupSettingsRevisionFromOptionMap())
	return nil
}

func SyncGroupSettingsFromLegacyOptions(tx *gorm.DB) error {
	return errors.New("legacy group settings options are deprecated; groups 表必须存在")
}

// EnsureGroupsCoverReferences ensures `groups` table contains entries for critical built-in group codes.
//
// IMPORTANT:
// - It intentionally does NOT scan legacy snapshot tables/options to infer missing group codes.
// - New groups should be created explicitly via admin APIs; legacy snapshots are not authoritative.
//
// It only INSERTs missing codes and never mutates existing rows.
func EnsureGroupsCoverReferences(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&Group{}) {
		return nil
	}

	// Load legacy maps for description/user_selectable/ratio hints.
	ratioHints := map[string]float64{}
	if raw, ok, err := readLegacyOptionValue(tx, "GroupRatio"); err != nil {
		return err
	} else if ok {
		parsed, err := parseLegacyGroupRatioOption(raw)
		if err != nil {
			return err
		}
		for k, v := range parsed {
			code := strings.TrimSpace(k)
			if code == "" {
				continue
			}
			ratioHints[code] = v
		}
	}
	usableHints := map[string]string{}
	if raw, ok, err := readLegacyOptionValue(tx, "UserUsableGroups"); err != nil {
		return err
	} else if ok {
		parsed, err := parseLegacyUserUsableGroupsOption(raw)
		if err != nil {
			return err
		}
		for k, v := range parsed {
			code := strings.TrimSpace(k)
			if code == "" {
				continue
			}
			usableHints[code] = v
		}
	}

	addCode := func(dst map[string]struct{}, code string) {
		c := strings.TrimSpace(code)
		if c == "" || c == "auto" {
			return
		}
		dst[c] = struct{}{}
	}

	candidates := make(map[string]struct{}, 64)
	// NOTE: legacy GroupRatio/UserUsableGroups options are compatibility-only once `groups` table exists.
	// Keep them as ratio/description hints, but do not treat their keys as "references" that re-create groups.
	addCode(candidates, "default")

	if len(candidates) == 0 {
		return nil
	}

	var existing []string
	if err := tx.Model(&Group{}).Pluck("code", &existing).Error; err != nil {
		return err
	}
	existSet := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		existSet[strings.TrimSpace(c)] = struct{}{}
	}

	toCreate := make([]Group, 0, len(candidates))
	for raw := range candidates {
		if _, ok := existSet[raw]; ok {
			continue
		}
		code, err := normalizeGroupCode(raw)
		if err != nil {
			// Only skip the reserved pseudo-group; other errors should be surfaced.
			if strings.TrimSpace(raw) == "auto" {
				continue
			}
			return err
		}

		ratio := 1.0
		if v, ok := ratioHints[code]; ok {
			ratio = v
		}
		if err := validateGroupRatio(ratio); err != nil {
			return err
		}

		desc, selectable := usableHints[code]
		description, err := normalizeGroupDescription(desc)
		if err != nil {
			return err
		}

		toCreate = append(toCreate, Group{
			Code:           code,
			Name:           code,
			DisplayName:    code,
			Description:    description,
			Ratio:          ratio,
			UserSelectable: selectable,
			Enabled:        true,
		})
	}

	if len(toCreate) == 0 {
		return nil
	}
	sort.Slice(toCreate, func(i, j int) bool { return toCreate[i].Code < toCreate[j].Code })
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&toCreate).Error
}

func parseLegacyGroupIDsOption(raw string) ([]int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return nil, err
	}
	return normalizeUniqueSortedIDs(ids), nil
}

func parseLegacyPaygProductsOption(raw string) ([]payg_setting.PaygProduct, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var products []payg_setting.PaygProduct
	if err := json.Unmarshal([]byte(trimmed), &products); err != nil {
		return nil, err
	}
	return products, nil
}

func parseLegacyPayRequestProductsOption(raw string) ([]payg_setting.PayRequestProduct, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var products []payg_setting.PayRequestProduct
	if err := json.Unmarshal([]byte(trimmed), &products); err != nil {
		return nil, err
	}
	return products, nil
}

func parseLegacyPayTokenProductsOption(raw string) ([]payg_setting.PayTokenProduct, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var products []payg_setting.PayTokenProduct
	if err := json.Unmarshal([]byte(trimmed), &products); err != nil {
		return nil, err
	}
	return products, nil
}

// legacyGroupIDsFromCodes resolves legacy group code references into group_ids.
func legacyGroupIDsFromCodes(tx *gorm.DB, codes []string) ([]int, error) {
	ids, missing, err := existingLegacyGroupIDsFromCodes(tx, codes)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("分组不存在: %s", strings.Join(missing, ", "))
	}
	return ids, nil
}

func existingLegacyGroupIDsFromCodes(tx *gorm.DB, codes []string) ([]int, []string, error) {
	normalized, err := NormalizeGroupNames(codes)
	if err != nil {
		return nil, nil, err
	}
	if len(normalized) == 0 {
		return nil, nil, nil
	}

	numericIDs := make([]int, 0, len(normalized))
	codeInputs := make([]string, 0, len(normalized))
	for _, code := range normalized {
		if id, ok := parsePositiveIntString(code); ok {
			numericIDs = append(numericIDs, id)
			continue
		}
		codeInputs = append(codeInputs, code)
	}

	codeIDMap := make(map[string]int, len(codeInputs))
	if len(codeInputs) > 0 {
		codeIDMap, _, err = groupCodeIDMapLoose(tx, codeInputs)
		if err != nil {
			return nil, nil, err
		}
	}

	validNumericSet := make(map[int]struct{}, len(numericIDs))
	if len(numericIDs) > 0 {
		validNumericIDs, err := filterExistingSortedIDsTx(tx, numericIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range validNumericIDs {
			validNumericSet[id] = struct{}{}
		}
	}

	ids := make([]int, 0, len(normalized))
	missing := make([]string, 0)
	for _, code := range normalized {
		if id, ok := parsePositiveIntString(code); ok {
			if _, exists := validNumericSet[id]; !exists {
				missing = append(missing, code)
				continue
			}
			ids = append(ids, id)
			continue
		}
		c := strings.TrimSpace(code)
		id := codeIDMap[c]
		if id <= 0 {
			missing = append(missing, c)
			continue
		}
		ids = append(ids, id)
	}
	return normalizeUniqueSortedIDs(ids), missing, nil
}

// LegacyGroupIDsFromCodes resolves legacy group code references into group_ids.
//
// IMPORTANT: This is for legacy data migration/backfill only.
// It is strict (no "guessing"): legacy rows must reference existing group codes.
func LegacyGroupIDsFromCodes(tx *gorm.DB, codes []string) ([]int, error) {
	return legacyGroupIDsFromCodes(tx, codes)
}

// BackfillGroupBindingsFromLegacyData performs one-shot migration from legacy string/json group references
// to group_id-based mapping tables.
func BackfillGroupBindingsFromLegacyData(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}

	// channels.group -> channel_groups
	if tx.Migrator().HasTable(&Channel{}) && tx.Migrator().HasTable(&ChannelGroup{}) {
		var existing []int
		if err := tx.Model(&ChannelGroup{}).Distinct("channel_id").Pluck("channel_id", &existing).Error; err != nil {
			return err
		}
		existSet := make(map[int]struct{}, len(existing))
		for _, id := range existing {
			if id > 0 {
				existSet[id] = struct{}{}
			}
		}

		var channels []Channel
		if err := tx.Model(&Channel{}).Select("id", commonGroupCol).Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			if channel.Id > 0 {
				if _, ok := existSet[channel.Id]; ok {
					continue
				}
			}
			codes, err := ParseGroupNamesCSV(channel.Group)
			if err != nil {
				return fmt.Errorf("channel#%d group 解析失败: %w", channel.Id, err)
			}
			if len(codes) == 0 {
				// Preserve legacy behavior: empty group implies "default".
				codes = []string{"default"}
			}
			ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
			if err != nil {
				return fmt.Errorf("channel#%d group 回填失败: %w", channel.Id, err)
			}
				if len(ids) == 0 {
					legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
					if err != nil {
						return fmt.Errorf("channel#%d group 回填失败: %w", channel.Id, err)
					}
					ids = []int{legacyDefaultModelGroupID}
				}
			if err := upsertChannelGroupsTx(tx, channel.Id, ids); err != nil {
				return err
			}
		}
	}

	// tokens.allowed_groups + tokens.group -> token_allowed_groups + tokens.default_group_id
	if tx.Migrator().HasTable(&Token{}) && tx.Migrator().HasTable(&TokenAllowedGroup{}) {
		var existing []int
		if err := tx.Model(&TokenAllowedGroup{}).Distinct("token_id").Pluck("token_id", &existing).Error; err != nil {
			return err
		}
		existSet := make(map[int]struct{}, len(existing))
		for _, id := range existing {
			if id > 0 {
				existSet[id] = struct{}{}
			}
		}

		var tokens []Token
		if err := tx.Model(&Token{}).Select("id", commonGroupCol, "allowed_groups", "default_group_id").Find(&tokens).Error; err != nil {
			return err
		}
		for _, token := range tokens {
			hasAllowedBindings := false
			if token.Id > 0 {
				if _, ok := existSet[token.Id]; ok {
					hasAllowedBindings = true
				}
			}
			if !hasAllowedBindings {
				codes, err := ParseGroupNamesJSON(token.AllowedGroups)
				if err != nil {
					return fmt.Errorf("token#%d allowed_groups 解析失败: %w", token.Id, err)
				}
				if len(codes) == 0 && strings.TrimSpace(token.Group) != "" {
					codes = []string{token.Group}
				}
				if len(codes) == 0 {
					return fmt.Errorf("token#%d 缺少可用分组（allowed_groups 为空且 group 为空）", token.Id)
				}
				ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
				if err != nil {
					return fmt.Errorf("token#%d 分组回填失败: %w", token.Id, err)
				}
					if len(ids) == 0 {
						legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
						if err != nil {
							return fmt.Errorf("token#%d 分组回填失败: %w", token.Id, err)
						}
						ids = []int{legacyDefaultModelGroupID}
					}
				if err := upsertTokenAllowedGroupsTx(tx, token.Id, ids); err != nil {
					return err
				}

				defaultGroupID := token.DefaultGroupId
				defaultCode := strings.TrimSpace(token.Group)
				if defaultGroupID == 0 && defaultCode != "" {
					mapByCode, _, err := groupCodeIDMapLoose(tx, []string{defaultCode})
					if err != nil {
						return err
					}
					defaultGroupID = mapByCode[defaultCode]
				}
				if defaultGroupID <= 0 {
					defaultGroupID = FirstGroupIDKeepOrder(ids)
				}
				if defaultGroupID != token.DefaultGroupId {
					if err := tx.Model(&Token{}).Where("id = ?", token.Id).Update("default_group_id", defaultGroupID).Error; err != nil {
						return err
					}
				}
			}
		}
	}

	// users.group -> users.group_id
	if tx.Migrator().HasTable(&User{}) && tx.Migrator().HasColumn(&User{}, "group_id") {
		type row struct {
			Id      int    `gorm:"column:id"`
			Group   string `gorm:"column:group"`
			GroupId int    `gorm:"column:group_id"`
		}
		var rows []row
		if err := tx.Model(&User{}).Select("id", commonGroupCol, "group_id").Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if r.Id <= 0 {
				continue
			}
			if r.GroupId > 0 {
				continue
			}
			code := strings.TrimSpace(r.Group)
			var ids []int
			var err error
			if code != "" {
				ids, _, err = existingLegacyGroupIDsFromCodes(tx, []string{code})
				if err != nil {
					return fmt.Errorf("user#%d group 回填失败: %w", r.Id, err)
				}
			}
				if len(ids) == 0 {
					legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
					if err != nil {
						return fmt.Errorf("user#%d group 回填失败: %w", r.Id, err)
					}
					ids = []int{legacyDefaultModelGroupID}
				}
			if len(ids) != 1 || ids[0] <= 0 {
				return fmt.Errorf("user#%d group 回填失败: 无效 group_id", r.Id)
			}
			if err := tx.Model(&User{}).Where("id = ?", r.Id).Update("group_id", ids[0]).Error; err != nil {
				return err
			}
		}
	}

	// user_subscriptions.allowed_groups -> user_subscription_groups
	// user_request_subscriptions.allowed_groups -> user_request_subscription_groups
	{
		subCodesByID := map[int][]string{}
		subPresetIDByID := map[int]int{}
		reqCodesByID := map[int][]string{}
		codeSet := map[string]struct{}{}

		if tx.Migrator().HasTable(&UserSubscription{}) && tx.Migrator().HasTable(&UserSubscriptionGroup{}) {
			var existing []int
			if err := tx.Model(&UserSubscriptionGroup{}).Distinct("subscription_id").Pluck("subscription_id", &existing).Error; err != nil {
				return err
			}
			existSet := make(map[int]struct{}, len(existing))
			for _, id := range existing {
				if id > 0 {
					existSet[id] = struct{}{}
				}
			}

			type row struct {
				Id             int       `gorm:"column:id"`
				AllowedGroups  JSONValue `gorm:"column:allowed_groups"`
				SourcePresetId int       `gorm:"column:source_preset_id"`
			}
			var rows []row
			if err := tx.Model(&UserSubscription{}).Select("id", "allowed_groups", "source_preset_id").Find(&rows).Error; err != nil {
				return err
			}
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				if _, ok := existSet[r.Id]; ok {
					continue
				}
				codes, err := ParseGroupNamesJSON(r.AllowedGroups)
				if err != nil {
					return fmt.Errorf("subscription#%d allowed_groups 解析失败: %w", r.Id, err)
				}
				// Legacy/manual subscriptions: empty allowed_groups means the effective legacy fallback group only.
				// Preset-linked subscriptions derive groups from subscription_product_groups; only backfill snapshots when present.
				if len(codes) == 0 {
					if r.SourcePresetId > 0 {
						continue
					}
						legacyDefaultModelGroupCode, fallbackErr := ResolveLegacyDefaultModelGroupCode(tx)
						if fallbackErr != nil {
							return fallbackErr
						}
						codes = []string{legacyDefaultModelGroupCode}
					}
				subCodesByID[r.Id] = codes
				subPresetIDByID[r.Id] = r.SourcePresetId
				for _, code := range codes {
					c := strings.TrimSpace(code)
					if c == "" {
						continue
					}
					codeSet[c] = struct{}{}
				}
			}
		}

		if tx.Migrator().HasTable(&UserRequestSubscription{}) && tx.Migrator().HasTable(&UserRequestSubscriptionGroup{}) {
			var existing []int
			if err := tx.Model(&UserRequestSubscriptionGroup{}).Distinct("subscription_id").Pluck("subscription_id", &existing).Error; err != nil {
				return err
			}
			existSet := make(map[int]struct{}, len(existing))
			for _, id := range existing {
				if id > 0 {
					existSet[id] = struct{}{}
				}
			}

			type row struct {
				Id            int       `gorm:"column:id"`
				AllowedGroups JSONValue `gorm:"column:allowed_groups"`
			}
			var rows []row
			if err := tx.Model(&UserRequestSubscription{}).Select("id", "allowed_groups").Find(&rows).Error; err != nil {
				return err
			}
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				if _, ok := existSet[r.Id]; ok {
					continue
				}
				codes, err := ParseGroupNamesJSON(r.AllowedGroups)
				if err != nil {
					return fmt.Errorf("request_subscription#%d allowed_groups 解析失败: %w", r.Id, err)
				}
				if len(codes) == 0 {
					continue
				}
				reqCodesByID[r.Id] = codes
				for _, code := range codes {
					c := strings.TrimSpace(code)
					if c == "" {
						continue
					}
					codeSet[c] = struct{}{}
				}
			}
		}

		if len(codeSet) > 0 {
			codes := make([]string, 0, len(codeSet))
			for code := range codeSet {
				c := strings.TrimSpace(code)
				if c == "" {
					continue
				}
				codes = append(codes, c)
			}
			sort.Strings(codes)
			codeIDMap, _, err := groupCodeIDMapLoose(tx, codes)
			if err != nil {
				return err
			}

			subRows := make([]UserSubscriptionGroup, 0, 64)
			for subID, groupCodes := range subCodesByID {
				if subID <= 0 {
					continue
				}
				gids := make([]int, 0, len(groupCodes))
				for _, code := range groupCodes {
					c := strings.TrimSpace(code)
					if c == "" {
						continue
					}
					id := codeIDMap[c]
					if id <= 0 {
						continue
					}
					gids = append(gids, id)
				}
				gids = normalizeUniqueSortedIDs(gids)
				if len(gids) == 0 {
					if subPresetIDByID[subID] > 0 {
						continue
					}
						legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
						if err != nil {
							return err
						}
						gids = []int{legacyDefaultModelGroupID}
					}
				for _, gid := range gids {
					subRows = append(subRows, UserSubscriptionGroup{SubscriptionId: subID, GroupId: gid})
				}
			}
			if len(subRows) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&subRows).Error; err != nil {
					return err
				}
			}

			reqRows := make([]UserRequestSubscriptionGroup, 0, 64)
			for subID, groupCodes := range reqCodesByID {
				if subID <= 0 {
					continue
				}
				gids := make([]int, 0, len(groupCodes))
				for _, code := range groupCodes {
					c := strings.TrimSpace(code)
					if c == "" {
						continue
					}
					id := codeIDMap[c]
					if id <= 0 {
						continue
					}
					gids = append(gids, id)
				}
				gids = normalizeUniqueSortedIDs(gids)
				if len(gids) == 0 {
					continue
				}
				for _, gid := range gids {
					reqRows = append(reqRows, UserRequestSubscriptionGroup{SubscriptionId: subID, GroupId: gid})
				}
			}
			if len(reqRows) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reqRows).Error; err != nil {
					return err
				}
			}
		}
	}

	// redemption_presets.allowed_groups/allowed_group_ids -> subscription_product_groups
	if tx.Migrator().HasTable(&RedemptionPreset{}) {
		var presets []RedemptionPreset
		if err := tx.Model(&RedemptionPreset{}).Select("id", "mode", "allowed_groups", "allowed_group_ids").Find(&presets).Error; err != nil {
			return err
		}
		for _, preset := range presets {
			if preset.Id <= 0 {
				continue
			}
			if preset.Mode != "subscription" && preset.Mode != "tokens" && preset.Mode != "request" {
				continue
			}
			groupIDs := []int{}
			if len(preset.AllowedGroupIds) > 0 {
				ids, err := parseLegacyGroupIDsOption(string(preset.AllowedGroupIds))
				if err != nil {
					return fmt.Errorf("preset#%d allowed_group_ids 解析失败: %w", preset.Id, err)
				}
				groupIDs, err = filterExistingActiveGroupIDsKeepOrderTx(tx, ids)
				if err != nil {
					return err
				}
				if len(groupIDs) > 0 && !equalOrderedPositiveIDs(ids, groupIDs) {
					b, err := MarshalGroupIDsJSONKeepOrder(groupIDs)
					if err != nil {
						return err
					}
					if err := tx.Model(&RedemptionPreset{}).Where("id = ?", preset.Id).Update("allowed_group_ids", b).Error; err != nil {
						return err
					}
				}
			}
			if len(groupIDs) == 0 {
				codes, err := ParseGroupNamesJSON(preset.AllowedGroups)
				if err != nil {
					return fmt.Errorf("preset#%d allowed_groups 解析失败: %w", preset.Id, err)
				}
				if len(codes) == 0 {
					continue
				}
				ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
				if err != nil {
					return fmt.Errorf("preset#%d 分组回填失败: %w", preset.Id, err)
				}
				groupIDs = ids
			}
				if len(groupIDs) == 0 {
					legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
					if err != nil {
						return err
					}
					groupIDs = []int{legacyDefaultModelGroupID}
				}
			if err := upsertSubscriptionProductGroupsTx(tx, preset.Id, groupIDs); err != nil {
				return err
			}
			// Populate redemption_presets.allowed_group_ids for legacy rows, so list/filtering can rely on ids.
			if len(preset.AllowedGroupIds) == 0 {
				ids := normalizeUniqueSortedIDs(groupIDs)
				if len(ids) > 0 {
					if b, err := common.Marshal(ids); err == nil {
						if err := tx.Model(&RedemptionPreset{}).Where("id = ?", preset.Id).Update("allowed_group_ids", JSONValue(b)).Error; err != nil {
							return err
						}
					} else {
						return err
					}
				}
			}
		}
	}

	// redemptions.allowed_groups -> redemptions.allowed_group_ids
	if tx.Migrator().HasTable(&Redemption{}) && tx.Migrator().HasColumn(&Redemption{}, "allowed_group_ids") {
		type row struct {
			Id            int       `gorm:"column:id"`
			AllowedGroups JSONValue `gorm:"column:allowed_groups"`
		}
		var rows []row
		query := tx.Model(&Redemption{}).Select("id", "allowed_groups")
		query = query.Where(fmt.Sprintf("%s AND %s", jsonColumnIsEmptyCondition("allowed_group_ids"), jsonColumnIsNotEmptyCondition("allowed_groups")))
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if r.Id <= 0 {
				continue
			}
			codes, err := ParseGroupNamesJSON(r.AllowedGroups)
			if err != nil {
				return fmt.Errorf("redemption#%d allowed_groups 解析失败: %w", r.Id, err)
			}
			if len(codes) == 0 {
				continue
			}
			groupIDs, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
			if err != nil {
				return fmt.Errorf("redemption#%d 分组回填失败: %w", r.Id, err)
			}
			if len(groupIDs) == 0 {
				continue
			}
			b, err := common.Marshal(groupIDs)
			if err != nil {
				return err
			}
			if err := tx.Model(&Redemption{}).Where("id = ?", r.Id).Update("allowed_group_ids", JSONValue(b)).Error; err != nil {
				return err
			}
		}
	}

	// payg.products option -> payg_products/payg_product_groups
	if raw, ok, err := readLegacyOptionValue(tx, "payg.products"); err != nil {
		return err
	} else if ok {
		products, err := parseLegacyPaygProductsOption(raw)
		if err != nil {
			return fmt.Errorf("payg.products 解析失败: %w", err)
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			// Prefer allowed_group_ids (new format). Fallback to legacy allowed_groups.
			groupIDs := normalizeUniqueSortedIDs(product.AllowedGroupIds)
			if len(groupIDs) > 0 {
				groupIDs, err = filterExistingActiveGroupIDsKeepOrderTx(tx, groupIDs)
				if err != nil {
					return err
				}
			}
			if len(groupIDs) == 0 {
				ids, _, err := existingLegacyGroupIDsFromCodes(tx, product.AllowedGroups)
				if err != nil {
					return fmt.Errorf("payg product#%d 分组回填失败: %w", product.Id, err)
				}
				groupIDs = ids
			}
			if len(groupIDs) == 0 {
				continue
			}
			if err := upsertPaygProductTx(tx, PaygProduct{
				Id:          product.Id,
				Name:        product.Name,
				Description: product.Description,
				Enabled:     product.Enabled,
				SortOrder:   product.SortOrder,
				Stock:       product.Stock,
			}, groupIDs); err != nil {
				return err
			}
			// Keep existing balances consistent with current product config.
			groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
			if err != nil {
				return err
			}
			if err := tx.Model(&PaygUserBalance{}).
				Where("product_id = ?", product.Id).
				Updates(map[string]interface{}{
					"product_name": strings.TrimSpace(product.Name),
					"sort_order":   product.SortOrder,
				}).Error; err != nil {
				return err
			}
			if err := tx.Model(&PaygUserBalance{}).
				Where("product_id = ? AND (override_allowed_group_ids IS NULL OR override_allowed_group_ids = 0)", product.Id).
				Update("allowed_group_ids", groupIDsJSON).Error; err != nil {
				return err
			}
		}
	}

	// payg.pay_request_products option -> pay_request_products/pay_request_product_groups
	if raw, ok, err := readLegacyOptionValue(tx, "payg.pay_request_products"); err != nil {
		return err
	} else if ok {
		products, err := parseLegacyPayRequestProductsOption(raw)
		if err != nil {
			return fmt.Errorf("payg.pay_request_products 解析失败: %w", err)
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			// Prefer allowed_group_ids (new format). Fallback to legacy allowed_groups.
			groupIDs := normalizeUniqueSortedIDs(product.AllowedGroupIds)
			if len(groupIDs) > 0 {
				groupIDs, err = filterExistingActiveGroupIDsKeepOrderTx(tx, groupIDs)
				if err != nil {
					return err
				}
			}
			if len(groupIDs) == 0 {
				ids, _, err := existingLegacyGroupIDsFromCodes(tx, product.AllowedGroups)
				if err != nil {
					return fmt.Errorf("pay_request product#%d 分组回填失败: %w", product.Id, err)
				}
				groupIDs = ids
			}
			if len(groupIDs) == 0 {
				continue
			}
			if err := upsertPayRequestProductTx(tx, PayRequestProduct{
				Id:          product.Id,
				Name:        product.Name,
				Description: product.Description,
				Enabled:     product.Enabled,
				SortOrder:   product.SortOrder,
				Stock:       product.Stock,
			}, groupIDs); err != nil {
				return err
			}
			// Keep existing balances consistent with current product config.
			groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
			if err != nil {
				return err
			}
			if err := tx.Model(&PayRequestUserBalance{}).
				Where("product_id = ?", product.Id).
				Updates(map[string]interface{}{
					"product_name":      strings.TrimSpace(product.Name),
					"sort_order":        product.SortOrder,
					"allowed_group_ids": groupIDsJSON,
				}).Error; err != nil {
				return err
			}
		}
	}

	// payg.pay_token_products option -> pay_token_products/pay_token_product_groups
	if raw, ok, err := readLegacyOptionValue(tx, "payg.pay_token_products"); err != nil {
		return err
	} else if ok {
		products, err := parseLegacyPayTokenProductsOption(raw)
		if err != nil {
			return fmt.Errorf("payg.pay_token_products 解析失败: %w", err)
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			// Prefer allowed_group_ids (new format). Fallback to legacy allowed_groups.
			groupIDs := normalizeUniqueSortedIDs(product.AllowedGroupIds)
			if len(groupIDs) > 0 {
				groupIDs, err = filterExistingActiveGroupIDsKeepOrderTx(tx, groupIDs)
				if err != nil {
					return err
				}
			}
			if len(groupIDs) == 0 {
				ids, _, err := existingLegacyGroupIDsFromCodes(tx, product.AllowedGroups)
				if err != nil {
					return fmt.Errorf("pay_token product#%d 分组回填失败: %w", product.Id, err)
				}
				groupIDs = ids
			}
			if len(groupIDs) == 0 {
				continue
			}
			if err := upsertPayTokenProductTx(tx, PayTokenProduct{
				Id:          product.Id,
				Name:        product.Name,
				Description: product.Description,
				Enabled:     product.Enabled,
				SortOrder:   product.SortOrder,
				Stock:       product.Stock,
			}, groupIDs); err != nil {
				return err
			}
			// Keep existing balances consistent with current product config.
			groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
			if err != nil {
				return err
			}
			if err := tx.Model(&PayTokenUserBalance{}).
				Where("product_id = ?", product.Id).
				Updates(map[string]interface{}{
					"product_name":      strings.TrimSpace(product.Name),
					"sort_order":        product.SortOrder,
					"allowed_group_ids": groupIDsJSON,
				}).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

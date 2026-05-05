package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

// Group defines model group metadata.
//
// Business bindings must use Id as foreign key. Group name (Code/Name) is editable.
type Group struct {
	Id                          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Code                        string    `json:"code" gorm:"type:varchar(64);not null;uniqueIndex:idx_model_groups_code"`
	Name                        string    `json:"name" gorm:"-"`
	DisplayName                 string    `json:"display_name" gorm:"column:name;type:varchar(64);not null;default:'';uniqueIndex:idx_model_groups_name"`
	Description                 string    `json:"description" gorm:"type:text;column:description"`
	AllowedModels               JSONValue `json:"allowed_models,omitempty" gorm:"type:json;column:allowed_models"`
	AllowedModelPrefillGroupIds JSONValue `json:"allowed_model_prefill_group_ids,omitempty" gorm:"type:json;column:allowed_model_prefill_group_ids"`
	AllowedUserAgents           JSONValue `json:"allowed_user_agents,omitempty" gorm:"type:json;column:allowed_user_agents"`
	Ratio                       float64   `json:"ratio" gorm:"type:double precision;not null;default:1"`
	NoBilling                   bool      `json:"no_billing" gorm:"type:boolean;not null;default:false;column:no_billing"`
	NoBillingProductKeys        JSONValue `json:"no_billing_product_keys,omitempty" gorm:"type:json;column:no_billing_product_keys"`
	UserSelectable              bool      `json:"user_selectable" gorm:"type:boolean;not null;default:true;column:user_selectable"`
	Enabled                     bool      `json:"enabled" gorm:"type:boolean;not null;default:true"`
	Archived                    bool      `json:"archived" gorm:"type:boolean;not null;default:false;index;column:archived"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

func (Group) TableName() string {
	return "model_groups"
}

func (group *Group) NormalizeForResponse() {
	if group == nil {
		return
	}
	code := strings.TrimSpace(group.Code)
	group.Code = code
	group.Name = code
	if strings.TrimSpace(group.DisplayName) == "" {
		group.DisplayName = code
	}
}

func NormalizeGroupsForResponse(groups []Group) {
	for i := range groups {
		groups[i].NormalizeForResponse()
	}
}

func normalizeGroupCode(code string) (string, error) {
	c := strings.TrimSpace(code)
	if c == "" {
		return "", errors.New("分组名不能为空")
	}
	if utf8.RuneCountInString(c) > 64 {
		return "", errors.New("分组名过长")
	}
	if c == "auto" {
		return "", errors.New("分组名不允许为 auto")
	}
	return c, nil
}

func normalizeGroupDescription(description string) (string, error) {
	desc := strings.TrimSpace(description)
	if utf8.RuneCountInString(desc) > 2048 {
		return "", errors.New("分组说明过长")
	}
	return desc, nil
}

func validateGroupRatio(ratio float64) error {
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return errors.New("分组倍率必须为有限数字")
	}
	if ratio < 0 {
		return errors.New("分组倍率必须大于等于 0")
	}
	return nil
}

func activeGroupScope(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return tx
	}
	if !tx.Migrator().HasColumn(&Group{}, "archived") {
		return tx
	}
	return tx.Where("archived = ?", false)
}

func ListGroups(tx *gorm.DB) ([]Group, error) {
	if tx == nil {
		tx = DB
	}
	var groups []Group
	if err := activeGroupScope(tx).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	NormalizeGroupsForResponse(groups)
	return groups, nil
}

func GetGroupByID(tx *gorm.DB, id int) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	if id <= 0 {
		return nil, errors.New("分组 id 无效")
	}
	var g Group
	if err := tx.Where("id = ?", id).First(&g).Error; err != nil {
		return nil, err
	}
	g.NormalizeForResponse()
	return &g, nil
}

func GetGroupByCode(tx *gorm.DB, code string) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	normalized, err := normalizeGroupCode(code)
	if err != nil {
		return nil, err
	}
	var g Group
	if err := activeGroupScope(tx).Where("code = ?", normalized).First(&g).Error; err != nil {
		return nil, err
	}
	g.NormalizeForResponse()
	return &g, nil
}

// ValidateGroupCodesExist validates group codes and ensures they exist in table `groups`.
// It rejects empty codes and the reserved pseudo-group "auto".
func ValidateGroupCodesExist(tx *gorm.DB, codes []string) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	// Compatibility window: before migrations, `groups` table might not exist yet.
	// In this case, we skip existence validation (legacy behavior allowed arbitrary groups).
	if !tx.Migrator().HasTable(&Group{}) {
		return nil
	}
	if len(codes) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, raw := range codes {
		c, err := normalizeGroupCode(raw)
		if err != nil {
			return err
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		normalized = append(normalized, c)
	}
	if len(normalized) == 0 {
		return nil
	}

	var existing []string
	if err := activeGroupScope(tx).Model(&Group{}).Where("code IN ?", normalized).Pluck("code", &existing).Error; err != nil {
		return err
	}
	existSet := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		existSet[c] = struct{}{}
	}
	missing := make([]string, 0)
	for _, c := range normalized {
		if _, ok := existSet[c]; ok {
			continue
		}
		missing = append(missing, c)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("分组不存在: %s", strings.Join(missing, ", "))
	}
	return nil
}

// NormalizeGroupCodesAndEnsureExist normalizes, deduplicates and validates group codes.
func NormalizeGroupCodesAndEnsureExist(tx *gorm.DB, codes []string) ([]string, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}
	if len(codes) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, raw := range codes {
		code, err := normalizeGroupCode(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if err := ValidateGroupCodesExist(tx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GroupCodeIDMap returns mapping of group code -> id.
func GroupCodeIDMap(tx *gorm.DB, codes []string) (map[string]int, error) {
	out, normalized, err := groupCodeIDMapLoose(tx, codes)
	if err != nil {
		return nil, err
	}
	if len(out) != len(normalized) {
		missing := make([]string, 0)
		for _, code := range normalized {
			if _, ok := out[code]; !ok {
				missing = append(missing, code)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("分组不存在: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func groupCodeIDMapLoose(tx *gorm.DB, codes []string) (map[string]int, []string, error) {
	if tx == nil {
		tx = DB
	}
	seen := make(map[string]struct{}, len(codes))
	normalized := make([]string, 0, len(codes))
	for _, raw := range codes {
		code, err := normalizeGroupCode(raw)
		if err != nil {
			if strings.TrimSpace(raw) == "auto" {
				continue
			}
			return nil, nil, err
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	if len(normalized) == 0 {
		return map[string]int{}, nil, nil
	}
	type row struct {
		Id   int    `gorm:"column:id"`
		Code string `gorm:"column:code"`
	}
	var rows []row
	if err := activeGroupScope(tx).Model(&Group{}).Select("id", "code").Where("code IN ?", normalized).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		code := strings.TrimSpace(r.Code)
		if code == "" || r.Id <= 0 {
			continue
		}
		out[code] = r.Id
	}
	return out, normalized, nil
}

// GroupIDCodeMap returns mapping of group id -> code.
func GroupIDCodeMap(tx *gorm.DB, ids []int) (map[int]string, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}
	if len(ids) == 0 {
		return map[int]string{}, nil
	}
	idSet := make(map[int]struct{}, len(ids))
	normalizedIDs := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := idSet[id]; ok {
			continue
		}
		idSet[id] = struct{}{}
		normalizedIDs = append(normalizedIDs, id)
	}
	if len(normalizedIDs) == 0 {
		return map[int]string{}, nil
	}
	type row struct {
		Id   int    `gorm:"column:id"`
		Code string `gorm:"column:code"`
	}
	var rows []row
	if err := activeGroupScope(tx).Model(&Group{}).Select("id", "code").Where("id IN ?", normalizedIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int]string, len(rows))
	for _, r := range rows {
		if r.Id <= 0 {
			continue
		}
		code := strings.TrimSpace(r.Code)
		if code == "" {
			continue
		}
		out[r.Id] = code
	}
	if len(out) != len(normalizedIDs) {
		missing := make([]int, 0)
		for _, id := range normalizedIDs {
			if _, ok := out[id]; !ok {
				missing = append(missing, id)
			}
		}
		sort.Ints(missing)
		missingStr := make([]string, 0, len(missing))
		for _, id := range missing {
			missingStr = append(missingStr, fmt.Sprintf("%d", id))
		}
		return nil, fmt.Errorf("分组不存在: %s", strings.Join(missingStr, ", "))
	}
	return out, nil
}

func GroupIDsFromCodes(tx *gorm.DB, codes []string) ([]int, error) {
	codeID, err := GroupCodeIDMap(tx, codes)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeGroupCodesAndEnsureExist(tx, codes)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(normalized))
	for _, code := range normalized {
		id, ok := codeID[code]
		if !ok || id <= 0 {
			return nil, fmt.Errorf("分组不存在: %s", code)
		}
		out = append(out, id)
	}
	return out, nil
}

func GroupCodesFromIDs(tx *gorm.DB, ids []int) ([]string, error) {
	idCode, err := GroupIDCodeMap(tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		code, ok := idCode[id]
		if !ok {
			return nil, fmt.Errorf("分组不存在: %d", id)
		}
		out = append(out, code)
	}
	return out, nil
}

func ValidateGroupIDsExist(tx *gorm.DB, ids []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	_, err := GroupIDCodeMap(tx, ids)
	return err
}

func filterExistingActiveGroupIDsKeepOrderTx(tx *gorm.DB, ids []int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}
	normalized := normalizeUniquePositiveIDsKeepOrder(ids)
	if len(normalized) == 0 {
		return nil, nil
	}
	var existing []int
	if err := activeGroupScope(tx).Model(&Group{}).Where("id IN ?", normalized).Order("id ASC").Pluck("id", &existing).Error; err != nil {
		return nil, err
	}
	existSet := make(map[int]struct{}, len(existing))
	for _, id := range existing {
		existSet[id] = struct{}{}
	}
	out := make([]int, 0, len(normalized))
	for _, id := range normalized {
		if _, ok := existSet[id]; !ok {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

func CreateGroup(tx *gorm.DB, group *Group) error {
	if group == nil {
		return errors.New("group 为空")
	}
	if tx == nil {
		tx = DB
	}
	code, err := normalizeGroupCode(group.Code)
	if err != nil {
		return err
	}
	description, err := normalizeGroupDescription(group.Description)
	if err != nil {
		return err
	}
	if err := validateGroupRatio(group.Ratio); err != nil {
		return err
	}
	if normalized, _, err := normalizeAllowedModelsJSON(group.AllowedModels); err != nil {
		return err
	} else {
		group.AllowedModels = normalized
	}
	if normalized, ids, err := normalizeAllowedModelPrefillGroupIDsJSON(group.AllowedModelPrefillGroupIds); err != nil {
		return err
	} else {
		group.AllowedModelPrefillGroupIds = normalized
		if err := ValidatePrefillGroupIDsExist(tx, "model", ids); err != nil {
			return err
		}
	}
	if normalized, _, err := normalizeAllowedUserAgentsJSON(group.AllowedUserAgents); err != nil {
		return err
	} else {
		group.AllowedUserAgents = normalized
	}
	if normalized, _, err := validateGroupNoBillingConfig(tx, group.NoBilling, group.NoBillingProductKeys); err != nil {
		return err
	} else {
		group.NoBillingProductKeys = normalized
	}
	group.Code = code
	group.Name = code
	group.DisplayName = code
	group.Description = description
	group.Id = 0
	return tx.Create(group).Error
}

type UpdateGroupParams struct {
	Code                        *string
	Description                 *string
	AllowedModels               *JSONValue
	AllowedModelPrefillGroupIds *JSONValue
	AllowedUserAgents           *JSONValue
	Ratio                       *float64
	NoBilling                   *bool
	NoBillingProductKeys        *JSONValue
	UserSelectable              *bool
	Enabled                     *bool
}

func replaceGroupName(groups []string, from string, to string) ([]string, bool, error) {
	if len(groups) == 0 || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || from == to {
		return groups, false, nil
	}
	changed := false
	for i := range groups {
		if groups[i] != from {
			continue
		}
		groups[i] = to
		changed = true
	}
	if !changed {
		return groups, false, nil
	}
	normalized, err := NormalizeGroupNames(groups)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

func updateOptionValueTx(tx *gorm.DB, key string, value string) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if err := tx.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", value).Error; err != nil {
		return err
	}
	return nil
}

func renameAutoGroupsOptionTx(tx *gorm.DB, from string, to string) (bool, error) {
	raw, ok, err := readLegacyOptionValue(tx, "AutoGroups")
	if err != nil || !ok {
		return false, err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return false, nil
	}
	var groups []string
	if err := json.Unmarshal([]byte(trimmed), &groups); err != nil {
		return false, err
	}
	next, changed, err := replaceGroupName(groups, from, to)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	b, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	if err := updateOptionValueTx(tx, "AutoGroups", string(b)); err != nil {
		return false, err
	}
	return true, nil
}

func renameGroupGroupRatioOptionTx(tx *gorm.DB, from string, to string) (bool, error) {
	raw, ok, err := readLegacyOptionValue(tx, "GroupGroupRatio")
	if err != nil || !ok {
		return false, err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return false, nil
	}
	parsed := map[string]map[string]float64{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return false, err
	}
	changed := false
	next := make(map[string]map[string]float64, len(parsed))
	for outer, inner := range parsed {
		newOuter := outer
		if outer == from {
			newOuter = to
			changed = true
		}
		mappedInner, exists := next[newOuter]
		if !exists {
			mappedInner = make(map[string]float64, len(inner))
		}
		for innerKey, ratio := range inner {
			newInnerKey := innerKey
			if innerKey == from {
				newInnerKey = to
				changed = true
			}
			mappedInner[newInnerKey] = ratio
		}
		next[newOuter] = mappedInner
	}
	if !changed {
		return false, nil
	}
	b, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	if err := updateOptionValueTx(tx, "GroupGroupRatio", string(b)); err != nil {
		return false, err
	}
	return true, nil
}

func renamePaygProductsOptionTx(tx *gorm.DB, from string, to string) (bool, error) {
	raw, ok, err := readLegacyOptionValue(tx, "payg.products")
	if err != nil || !ok {
		return false, err
	}
	products, err := parseLegacyPaygProductsOption(raw)
	if err != nil {
		return false, err
	}
	if len(products) == 0 {
		return false, nil
	}
	changed := false
	for i := range products {
		next, ok, err := replaceGroupName(products[i].AllowedGroups, from, to)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		products[i].AllowedGroups = next
		changed = true
	}
	if !changed {
		return false, nil
	}
	b, err := json.Marshal(products)
	if err != nil {
		return false, err
	}
	if err := updateOptionValueTx(tx, "payg.products", string(b)); err != nil {
		return false, err
	}
	return true, nil
}

func renamePaygAllowedGroupsOptionTx(tx *gorm.DB, from string, to string) (bool, error) {
	raw, ok, err := readLegacyOptionValue(tx, "payg.allowed_groups")
	if err != nil || !ok {
		return false, err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return false, nil
	}
	var groups []string
	if err := json.Unmarshal([]byte(trimmed), &groups); err != nil {
		return false, err
	}
	next, changed, err := replaceGroupName(groups, from, to)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	b, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	if err := updateOptionValueTx(tx, "payg.allowed_groups", string(b)); err != nil {
		return false, err
	}
	return true, nil
}

func UpdateGroupByID(tx *gorm.DB, id int, params UpdateGroupParams) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	if id <= 0 {
		return nil, errors.New("分组 id 无效")
	}
	before, err := GetGroupByID(tx, id)
	if err != nil {
		return nil, err
	}
	updates := make(map[string]interface{})
	nextCode := strings.TrimSpace(before.Code)
	if params.Code != nil {
		name, err := normalizeGroupCode(*params.Code)
		if err != nil {
			return nil, err
		}
		nextCode = name
		updates["code"] = name
		updates["name"] = name
	}
	if params.Description != nil {
		description, err := normalizeGroupDescription(*params.Description)
		if err != nil {
			return nil, err
		}
		updates["description"] = description
	}
	if params.AllowedModels != nil {
		normalized, _, err := normalizeAllowedModelsJSON(*params.AllowedModels)
		if err != nil {
			return nil, err
		}
		updates["allowed_models"] = normalized
	}
	if params.AllowedModelPrefillGroupIds != nil {
		normalized, ids, err := normalizeAllowedModelPrefillGroupIDsJSON(*params.AllowedModelPrefillGroupIds)
		if err != nil {
			return nil, err
		}
		if err := ValidatePrefillGroupIDsExist(tx, "model", ids); err != nil {
			return nil, err
		}
		updates["allowed_model_prefill_group_ids"] = normalized
	}
	if params.AllowedUserAgents != nil {
		normalized, _, err := normalizeAllowedUserAgentsJSON(*params.AllowedUserAgents)
		if err != nil {
			return nil, err
		}
		updates["allowed_user_agents"] = normalized
	}
	if params.Ratio != nil {
		if err := validateGroupRatio(*params.Ratio); err != nil {
			return nil, err
		}
		updates["ratio"] = *params.Ratio
	}
	if params.NoBilling != nil {
		updates["no_billing"] = *params.NoBilling
	}
	var nextNoBilling = before.NoBilling
	if params.NoBilling != nil {
		nextNoBilling = *params.NoBilling
	}
	nextNoBillingProductKeys := before.NoBillingProductKeys
	if params.NoBillingProductKeys != nil {
		nextNoBillingProductKeys = *params.NoBillingProductKeys
	}
	if params.NoBilling != nil || params.NoBillingProductKeys != nil {
		normalized, _, err := validateGroupNoBillingConfig(tx, nextNoBilling, nextNoBillingProductKeys)
		if err != nil {
			return nil, err
		}
		updates["no_billing_product_keys"] = normalized
	}
	if params.UserSelectable != nil {
		updates["user_selectable"] = *params.UserSelectable
	}
	if params.Enabled != nil {
		updates["enabled"] = *params.Enabled
	}
	if len(updates) == 0 {
		return GetGroupByID(tx, id)
	}
	if err := tx.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Group{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if before.Code != nextCode {
		loadOptionsFromDatabase()
	}
	return GetGroupByID(tx, id)
}

func UpdateGroupByCode(tx *gorm.DB, code string, params UpdateGroupParams) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	group, err := GetGroupByCode(tx, code)
	if err != nil {
		return nil, err
	}
	return UpdateGroupByID(tx, group.Id, params)
}

func DeleteGroupByID(tx *gorm.DB, id int) (*GroupCleanupSummary, error) {
	return SoftDeleteGroupByID(tx, id)
}

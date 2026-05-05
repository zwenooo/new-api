package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"one-api/common"
	"one-api/setting/payg_setting"

	"gorm.io/gorm"
)

const (
	GroupNoBillingProductKindSubscription = "subscription"
	GroupNoBillingProductKindTokens       = "tokens"
	GroupNoBillingProductKindRequest      = "request"
	GroupNoBillingProductKindPayg         = "payg"
	GroupNoBillingProductKindPayRequest   = "pay_request"
	GroupNoBillingProductKindPayToken     = "pay_token"
)

var supportedGroupNoBillingProductKinds = map[string]struct{}{
	GroupNoBillingProductKindSubscription: {},
	GroupNoBillingProductKindTokens:       {},
	GroupNoBillingProductKindRequest:      {},
	GroupNoBillingProductKindPayg:         {},
	GroupNoBillingProductKindPayRequest:   {},
	GroupNoBillingProductKindPayToken:     {},
}

type GroupNoBillingProductRef struct {
	Kind      string `json:"kind"`
	ProductId int    `json:"product_id"`
}

func (ref GroupNoBillingProductRef) Key() string {
	return BuildGroupNoBillingProductKey(ref.Kind, ref.ProductId)
}

type GroupNoBillingProductOption struct {
	Key       string `json:"key"`
	Kind      string `json:"kind"`
	ProductId int    `json:"product_id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
}

func BuildGroupNoBillingProductKey(kind string, productID int) string {
	kind = strings.TrimSpace(kind)
	if kind == "" || productID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", kind, productID)
}

func parseGroupNoBillingProductKey(raw string) (GroupNoBillingProductRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return GroupNoBillingProductRef{}, errors.New("限定商品不能为空")
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return GroupNoBillingProductRef{}, fmt.Errorf("限定商品格式无效: %s", raw)
	}
	kind := strings.TrimSpace(parts[0])
	if _, ok := supportedGroupNoBillingProductKinds[kind]; !ok {
		return GroupNoBillingProductRef{}, fmt.Errorf("限定商品类型无效: %s", kind)
	}
	productID := common.String2Int(strings.TrimSpace(parts[1]))
	if productID <= 0 {
		return GroupNoBillingProductRef{}, fmt.Errorf("限定商品 id 无效: %s", raw)
	}
	return GroupNoBillingProductRef{
		Kind:      kind,
		ProductId: productID,
	}, nil
}

func normalizeGroupNoBillingProductKeys(raw []string) ([]GroupNoBillingProductRef, []string, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	refs := make([]GroupNoBillingProductRef, 0, len(raw))
	keys := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		ref, err := parseGroupNoBillingProductKey(item)
		if err != nil {
			return nil, nil, err
		}
		key := ref.Key()
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
		keys = append(keys, key)
	}
	if len(refs) == 0 {
		return nil, nil, nil
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Key() < refs[j].Key()
	})
	sort.Strings(keys)
	return refs, keys, nil
}

func ParseGroupNoBillingProductKeysJSON(value JSONValue) ([]GroupNoBillingProductRef, error) {
	if len(value) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var raw []string
	if err := common.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, errors.New("不计费限定商品解析失败")
	}
	refs, _, err := normalizeGroupNoBillingProductKeys(raw)
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func normalizeGroupNoBillingProductKeysJSON(raw JSONValue) (JSONValue, []GroupNoBillingProductRef, error) {
	refs, err := ParseGroupNoBillingProductKeysJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		key := ref.Key()
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, nil, nil
	}
	b, err := common.Marshal(keys)
	if err != nil {
		return nil, nil, err
	}
	return JSONValue(b), refs, nil
}

func validateGroupNoBillingIDsByQuery(scope *gorm.DB, ids []int, label string) error {
	if len(ids) == 0 {
		return nil
	}
	var existing []int
	if err := scope.Pluck("id", &existing).Error; err != nil {
		return err
	}
	existingSet := make(map[int]struct{}, len(existing))
	for _, id := range existing {
		if id > 0 {
			existingSet[id] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, id := range ids {
		if _, ok := existingSet[id]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("%d", id))
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s不存在: %s", label, strings.Join(missing, ", "))
	}
	return nil
}

func validateGroupNoBillingIDsByFinder(ids []int, label string, exists func(int) bool) error {
	if len(ids) == 0 {
		return nil
	}
	missing := make([]string, 0)
	for _, id := range ids {
		if exists(id) {
			continue
		}
		missing = append(missing, fmt.Sprintf("%d", id))
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s不存在: %s", label, strings.Join(missing, ", "))
	}
	return nil
}

func validateGroupNoBillingProductRefsExist(tx *gorm.DB, refs []GroupNoBillingProductRef) error {
	if tx == nil {
		tx = DB
	}
	if len(refs) == 0 {
		return nil
	}

	grouped := make(map[string][]int, len(supportedGroupNoBillingProductKinds))
	for _, ref := range refs {
		if ref.ProductId <= 0 {
			continue
		}
		grouped[ref.Kind] = append(grouped[ref.Kind], ref.ProductId)
	}
	for kind, ids := range grouped {
		grouped[kind] = normalizeUniqueSortedIDs(ids)
	}

	if ids := grouped[GroupNoBillingProductKindSubscription]; len(ids) > 0 {
		if !tx.Migrator().HasTable(&RedemptionPreset{}) {
			return errors.New("订阅商品表未就绪")
		}
		if err := validateGroupNoBillingIDsByQuery(
			tx.Model(&RedemptionPreset{}).Where("id IN ? AND mode = ?", ids, GroupNoBillingProductKindSubscription),
			ids,
			"订阅额度商品",
		); err != nil {
			return err
		}
	}
	if ids := grouped[GroupNoBillingProductKindTokens]; len(ids) > 0 {
		if !tx.Migrator().HasTable(&RedemptionPreset{}) {
			return errors.New("tokens 商品表未就绪")
		}
		if err := validateGroupNoBillingIDsByQuery(
			tx.Model(&RedemptionPreset{}).Where("id IN ? AND mode = ?", ids, GroupNoBillingProductKindTokens),
			ids,
			"tokens 商品",
		); err != nil {
			return err
		}
	}
	if ids := grouped[GroupNoBillingProductKindRequest]; len(ids) > 0 {
		if !tx.Migrator().HasTable(&RedemptionPreset{}) {
			return errors.New("次数订阅商品表未就绪")
		}
		if err := validateGroupNoBillingIDsByQuery(
			tx.Model(&RedemptionPreset{}).Where("id IN ? AND mode = ?", ids, GroupNoBillingProductKindRequest),
			ids,
			"次数订阅商品",
		); err != nil {
			return err
		}
	}
	if ids := grouped[GroupNoBillingProductKindPayg]; len(ids) > 0 {
		if tx.Migrator().HasTable(&PaygProduct{}) {
			if err := validateGroupNoBillingIDsByQuery(
				tx.Model(&PaygProduct{}).Where("id IN ?", ids),
				ids,
				"按量商品",
			); err != nil {
				return err
			}
		} else {
			if err := validateGroupNoBillingIDsByFinder(ids, "按量商品", func(id int) bool {
				_, ok := payg_setting.FindPaygProductByID(id)
				return ok
			}); err != nil {
				return err
			}
		}
	}
	if ids := grouped[GroupNoBillingProductKindPayRequest]; len(ids) > 0 {
		if tx.Migrator().HasTable(&PayRequestProduct{}) {
			if err := validateGroupNoBillingIDsByQuery(
				tx.Model(&PayRequestProduct{}).Where("id IN ?", ids),
				ids,
				"按次商品",
			); err != nil {
				return err
			}
		} else {
			if err := validateGroupNoBillingIDsByFinder(ids, "按次商品", func(id int) bool {
				_, ok := payg_setting.FindPayRequestProductByID(id)
				return ok
			}); err != nil {
				return err
			}
		}
	}
	if ids := grouped[GroupNoBillingProductKindPayToken]; len(ids) > 0 {
		if tx.Migrator().HasTable(&PayTokenProduct{}) {
			if err := validateGroupNoBillingIDsByQuery(
				tx.Model(&PayTokenProduct{}).Where("id IN ?", ids),
				ids,
				"按token商品",
			); err != nil {
				return err
			}
		} else {
			if err := validateGroupNoBillingIDsByFinder(ids, "按token商品", func(id int) bool {
				_, ok := payg_setting.FindPayTokenProductByID(id)
				return ok
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateGroupNoBillingConfig(tx *gorm.DB, enabled bool, raw JSONValue) (JSONValue, []GroupNoBillingProductRef, error) {
	normalized, refs, err := normalizeGroupNoBillingProductKeysJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	if enabled && len(refs) == 0 {
		return nil, nil, errors.New("开启不计费时必须至少选择一个限定商品")
	}
	if err := validateGroupNoBillingProductRefsExist(tx, refs); err != nil {
		return nil, nil, err
	}
	return normalized, refs, nil
}

func listExistingGroupNoBillingPresetModesByIDTx(tx *gorm.DB, ids []int) (map[int]string, error) {
	out := make(map[int]string)
	ids = normalizeUniqueSortedIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&RedemptionPreset{}) {
		return out, nil
	}

	type presetRow struct {
		Id   int    `gorm:"column:id"`
		Mode string `gorm:"column:mode"`
	}
	var rows []presetRow
	if err := tx.Model(&RedemptionPreset{}).
		Select("id", "mode").
		Where("id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		mode := strings.TrimSpace(row.Mode)
		switch mode {
		case GroupNoBillingProductKindSubscription, GroupNoBillingProductKindTokens, GroupNoBillingProductKindRequest:
			out[row.Id] = mode
		}
	}
	return out, nil
}

func listExistingGroupNoBillingProductIDsByQueryTx(scope *gorm.DB, ids []int) (map[int]struct{}, error) {
	out := make(map[int]struct{})
	ids = normalizeUniqueSortedIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	var existing []int
	if err := scope.Where("id IN ?", ids).Pluck("id", &existing).Error; err != nil {
		return nil, err
	}
	for _, id := range existing {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func buildGroupNoBillingProductKeysJSON(keys []string) (JSONValue, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	_, normalizedKeys, err := normalizeGroupNoBillingProductKeys(keys)
	if err != nil {
		return nil, err
	}
	if len(normalizedKeys) == 0 {
		return nil, nil
	}
	b, err := common.Marshal(normalizedKeys)
	if err != nil {
		return nil, err
	}
	return JSONValue(b), nil
}

type groupNoBillingReconcileState struct {
	Group         Group
	NormalizedRaw JSONValue
	Refs          []GroupNoBillingProductRef
}

// ReconcileGroupNoBillingProductKeysTx keeps group no-billing product references consistent with the
// current product catalog. Missing products are removed, subscription product kind changes are remapped
// to the current mode, and groups left without any valid product refs are no longer marked no-billing.
func ReconcileGroupNoBillingProductKeysTx(tx *gorm.DB) (int, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&Group{}) {
		return 0, nil
	}

	var groups []Group
	if err := tx.Model(&Group{}).
		Select("id", "code", "no_billing", "no_billing_product_keys").
		Find(&groups).Error; err != nil {
		return 0, err
	}
	if len(groups) == 0 {
		return 0, nil
	}

	states := make([]groupNoBillingReconcileState, 0, len(groups))
	presetIDs := make([]int, 0, len(groups))
	paygIDs := make([]int, 0, len(groups))
	payRequestIDs := make([]int, 0, len(groups))
	payTokenIDs := make([]int, 0, len(groups))

	for _, group := range groups {
		if !group.NoBilling && !jsonValueHasElements(group.NoBillingProductKeys) {
			continue
		}
		normalized, refs, err := normalizeGroupNoBillingProductKeysJSON(group.NoBillingProductKeys)
		if err != nil {
			code := strings.TrimSpace(group.Code)
			if code == "" {
				code = fmt.Sprintf("#%d", group.Id)
			}
			return 0, fmt.Errorf("分组 %s 不计费限定商品配置无效: %w", code, err)
		}
		states = append(states, groupNoBillingReconcileState{
			Group:         group,
			NormalizedRaw: normalized,
			Refs:          refs,
		})
		for _, ref := range refs {
			switch ref.Kind {
			case GroupNoBillingProductKindSubscription, GroupNoBillingProductKindTokens, GroupNoBillingProductKindRequest:
				presetIDs = append(presetIDs, ref.ProductId)
			case GroupNoBillingProductKindPayg:
				paygIDs = append(paygIDs, ref.ProductId)
			case GroupNoBillingProductKindPayRequest:
				payRequestIDs = append(payRequestIDs, ref.ProductId)
			case GroupNoBillingProductKindPayToken:
				payTokenIDs = append(payTokenIDs, ref.ProductId)
			}
		}
	}

	presetModeByID, err := listExistingGroupNoBillingPresetModesByIDTx(tx, presetIDs)
	if err != nil {
		return 0, err
	}

	paygByID := make(map[int]struct{})
	paygIDs = normalizeUniqueSortedIDs(paygIDs)
	if len(paygIDs) > 0 {
		if tx.Migrator().HasTable(&PaygProduct{}) {
			paygByID, err = listExistingGroupNoBillingProductIDsByQueryTx(tx.Model(&PaygProduct{}), paygIDs)
			if err != nil {
				return 0, err
			}
		} else {
			for _, id := range paygIDs {
				if _, ok := payg_setting.FindPaygProductByID(id); ok {
					paygByID[id] = struct{}{}
				}
			}
		}
	}

	payRequestByID := make(map[int]struct{})
	payRequestIDs = normalizeUniqueSortedIDs(payRequestIDs)
	if len(payRequestIDs) > 0 {
		if tx.Migrator().HasTable(&PayRequestProduct{}) {
			payRequestByID, err = listExistingGroupNoBillingProductIDsByQueryTx(tx.Model(&PayRequestProduct{}), payRequestIDs)
			if err != nil {
				return 0, err
			}
		} else {
			for _, id := range payRequestIDs {
				if _, ok := payg_setting.FindPayRequestProductByID(id); ok {
					payRequestByID[id] = struct{}{}
				}
			}
		}
	}

	payTokenByID := make(map[int]struct{})
	payTokenIDs = normalizeUniqueSortedIDs(payTokenIDs)
	if len(payTokenIDs) > 0 {
		if tx.Migrator().HasTable(&PayTokenProduct{}) {
			payTokenByID, err = listExistingGroupNoBillingProductIDsByQueryTx(tx.Model(&PayTokenProduct{}), payTokenIDs)
			if err != nil {
				return 0, err
			}
		} else {
			for _, id := range payTokenIDs {
				if _, ok := payg_setting.FindPayTokenProductByID(id); ok {
					payTokenByID[id] = struct{}{}
				}
			}
		}
	}

	updated := 0
	for _, state := range states {
		nextKeys := make([]string, 0, len(state.Refs))
		for _, ref := range state.Refs {
			switch ref.Kind {
			case GroupNoBillingProductKindSubscription, GroupNoBillingProductKindTokens, GroupNoBillingProductKindRequest:
				mode, ok := presetModeByID[ref.ProductId]
				if !ok {
					continue
				}
				nextKeys = append(nextKeys, BuildGroupNoBillingProductKey(mode, ref.ProductId))
			case GroupNoBillingProductKindPayg:
				if _, ok := paygByID[ref.ProductId]; ok {
					nextKeys = append(nextKeys, ref.Key())
				}
			case GroupNoBillingProductKindPayRequest:
				if _, ok := payRequestByID[ref.ProductId]; ok {
					nextKeys = append(nextKeys, ref.Key())
				}
			case GroupNoBillingProductKindPayToken:
				if _, ok := payTokenByID[ref.ProductId]; ok {
					nextKeys = append(nextKeys, ref.Key())
				}
			}
		}

		nextRaw, err := buildGroupNoBillingProductKeysJSON(nextKeys)
		if err != nil {
			return 0, err
		}
		nextNoBilling := state.Group.NoBilling
		if nextNoBilling && !jsonValueHasElements(nextRaw) {
			nextNoBilling = false
		}

		if nextNoBilling == state.Group.NoBilling && string(nextRaw) == string(state.NormalizedRaw) {
			continue
		}

		if err := tx.Model(&Group{}).
			Where("id = ?", state.Group.Id).
			Updates(map[string]interface{}{
				"no_billing":              nextNoBilling,
				"no_billing_product_keys": nextRaw,
			}).Error; err != nil {
			return 0, err
		}
		updated++
	}
	return updated, nil
}

func ListGroupNoBillingProductOptions(tx *gorm.DB) ([]GroupNoBillingProductOption, error) {
	if tx == nil {
		tx = DB
	}
	options := make([]GroupNoBillingProductOption, 0, 64)

	if tx != nil && tx.Migrator().HasTable(&RedemptionPreset{}) {
		type presetRow struct {
			Id      int    `gorm:"column:id"`
			Name    string `gorm:"column:name"`
			Mode    string `gorm:"column:mode"`
			Enabled bool   `gorm:"column:enabled"`
		}
		var presets []presetRow
		if err := tx.Model(&RedemptionPreset{}).
			Select("id", "name", "mode", "enabled").
			Where("mode IN ?", []string{
				GroupNoBillingProductKindSubscription,
				GroupNoBillingProductKindTokens,
				GroupNoBillingProductKindRequest,
			}).
			Order("sort_order DESC, updated_time DESC, id DESC").
			Find(&presets).Error; err != nil {
			return nil, err
		}
		for _, preset := range presets {
			if preset.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(preset.Mode, preset.Id),
				Kind:      preset.Mode,
				ProductId: preset.Id,
				Name:      strings.TrimSpace(preset.Name),
				Enabled:   preset.Enabled,
			})
		}
	}

	if tx != nil && tx.Migrator().HasTable(&PaygProduct{}) {
		var products []PaygProduct
		if err := tx.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
			return nil, err
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayg, product.Id),
				Kind:      GroupNoBillingProductKindPayg,
				ProductId: product.Id,
				Name:      strings.TrimSpace(product.Name),
				Enabled:   product.Enabled,
			})
		}
	} else {
		for _, product := range payg_setting.GetPaygSettings().Products {
			if product.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayg, product.Id),
				Kind:      GroupNoBillingProductKindPayg,
				ProductId: product.Id,
				Name:      strings.TrimSpace(product.Name),
				Enabled:   product.Enabled,
			})
		}
	}

	if tx != nil && tx.Migrator().HasTable(&PayRequestProduct{}) {
		var products []PayRequestProduct
		if err := tx.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
			return nil, err
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayRequest, product.Id),
				Kind:      GroupNoBillingProductKindPayRequest,
				ProductId: product.Id,
				Name:      strings.TrimSpace(product.Name),
				Enabled:   product.Enabled,
			})
		}
	} else {
		for _, product := range payg_setting.GetPaygSettings().PayRequestProducts {
			if product.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayRequest, product.Id),
				Kind:      GroupNoBillingProductKindPayRequest,
				ProductId: product.Id,
				Name:      strings.TrimSpace(product.Name),
				Enabled:   product.Enabled,
			})
		}
	}

	if tx != nil && tx.Migrator().HasTable(&PayTokenProduct{}) {
		var products []PayTokenProduct
		if err := tx.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
			return nil, err
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayToken, product.Id),
				Kind:      GroupNoBillingProductKindPayToken,
				ProductId: product.Id,
				Name:      strings.TrimSpace(product.Name),
				Enabled:   product.Enabled,
			})
		}
	} else {
		for _, product := range payg_setting.GetPaygSettings().PayTokenProducts {
			if product.Id <= 0 {
				continue
			}
			options = append(options, GroupNoBillingProductOption{
				Key:       BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayToken, product.Id),
				Kind:      GroupNoBillingProductKindPayToken,
				ProductId: product.Id,
				Name:      strings.TrimSpace(product.Name),
				Enabled:   product.Enabled,
			})
		}
	}

	return options, nil
}

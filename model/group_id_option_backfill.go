package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// BackfillGroupIDOptionsFromLegacyOptions migrates legacy option values that still reference groups
// by string code (or numeric strings) into group_id-based JSON formats.
//
// This is strict and deterministic:
// - Numeric strings are parsed as group_id.
// - Non-numeric strings are treated as group code and must exist in `model_groups.code`.
func BackfillGroupIDOptionsFromLegacyOptions(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&Option{}) || !tx.Migrator().HasTable(&Group{}) {
		return nil
	}

	if err := backfillAutoGroupsOption(tx); err != nil {
		return err
	}
	if err := backfillTopupGroupRatioOption(tx); err != nil {
		return err
	}
	if err := backfillGroupGroupRatioOption(tx); err != nil {
		return err
	}
	if err := backfillModelRequestConcurrencyLimitGroupOption(tx); err != nil {
		return err
	}
	if err := backfillModelRequestRateLimitGroupOption(tx); err != nil {
		return err
	}
	if err := backfillCxPoolGroupIDsOption(tx); err != nil {
		return err
	}
	return nil
}

func backfillCxPoolGroupIDsOption(tx *gorm.DB) error {
	raw, ok, err := readLegacyOptionValue(tx, "cx_pool.cx_group_ids")
	if err != nil || !ok {
		return err
	}
	trimmed, usedDefault := normalizeJSONOrDefault(raw, "[]")

	var ids []int
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return fmt.Errorf("cx_pool.cx_group_ids 配置格式错误: %w", err)
	}
	ids = dedupIDsPreserveOrder(ids)
	if len(ids) == 0 {
		if usedDefault && strings.TrimSpace(raw) != trimmed {
			return updateOptionValueTx(tx, "cx_pool.cx_group_ids", trimmed)
		}
		return nil
	}

	idCodeMap, err := GroupIDCodeMap(tx, ids)
	if err != nil {
		existing := make([]int, 0, len(ids))
		for _, id := range ids {
			var count int64
			if qErr := activeGroupScope(tx).Model(&Group{}).Where("id = ?", id).Count(&count).Error; qErr != nil {
				return qErr
			}
			if count > 0 {
				existing = append(existing, id)
			}
		}
		ids = dedupIDsPreserveOrder(existing)
	} else {
		next := make([]int, 0, len(ids))
		for _, id := range ids {
			if _, ok := idCodeMap[id]; !ok {
				continue
			}
			next = append(next, id)
		}
		ids = dedupIDsPreserveOrder(next)
	}

	nextJSON := "[]"
	if len(ids) > 0 {
		b, err := MarshalGroupIDsJSONKeepOrder(ids)
		if err != nil {
			return err
		}
		nextJSON = string(b)
	}
	if strings.TrimSpace(raw) == nextJSON {
		return nil
	}
	return updateOptionValueTx(tx, "cx_pool.cx_group_ids", nextJSON)
}

func normalizeJSONOrDefault(raw string, defaultJSON string) (trimmed string, usedDefault bool) {
	trimmed = strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "<nil>" {
		return defaultJSON, true
	}
	return trimmed, false
}

func parsePositiveIntString(raw string) (int, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parseNonNegativeIntAny(v any) (int, error) {
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, errors.New("值必须为有限数字")
		}
		if t != math.Trunc(t) {
			return 0, errors.New("值必须为整数")
		}
		if t < 0 {
			return 0, errors.New("值必须大于等于 0")
		}
		return int(t), nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, errors.New("值不能为空")
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, errors.New("值必须为整数")
		}
		if n < 0 {
			return 0, errors.New("值必须大于等于 0")
		}
		return n, nil
	default:
		return 0, fmt.Errorf("值类型无效: %T", v)
	}
}

func parsePositiveIntAny(v any) (int, error) {
	n, err := parseNonNegativeIntAny(v)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, errors.New("值必须大于 0")
	}
	return n, nil
}

func parseNonNegativeFloatAny(v any) (float64, error) {
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, errors.New("倍率必须为有限数字")
		}
		if t < 0 {
			return 0, errors.New("倍率必须大于等于 0")
		}
		return t, nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, errors.New("倍率不能为空")
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, errors.New("倍率必须为有限数字")
		}
		if f < 0 {
			return 0, errors.New("倍率必须大于等于 0")
		}
		return f, nil
	default:
		return 0, fmt.Errorf("倍率类型无效: %T", v)
	}
}

func dedupIDsPreserveOrder(ids []int) []int {
	if len(ids) == 0 {
		return ids
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func filterExistingSortedIDsTx(tx *gorm.DB, ids []int) ([]int, error) {
	next, err := filterExistingActiveGroupIDsKeepOrderTx(tx, ids)
	if err != nil {
		return nil, err
	}
	return normalizeUniqueSortedIDs(next), nil
}

func backfillAutoGroupsOption(tx *gorm.DB) error {
	raw, ok, err := readLegacyOptionValue(tx, "AutoGroups")
	if err != nil || !ok {
		return err
	}
	trimmed, usedDefault := normalizeJSONOrDefault(raw, "[]")

	var ids []int
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		// Already the new format (JSON array of ints).
		for _, id := range ids {
			if id <= 0 {
				return errors.New("AutoGroups 存在非正整数 group_id")
			}
		}
		next, err := filterExistingActiveGroupIDsKeepOrderTx(tx, ids)
		if err != nil {
			return err
		}
		if usedDefault && strings.TrimSpace(raw) == trimmed && equalOrderedPositiveIDs(ids, next) {
			return nil
		}
		b, err := json.Marshal(next)
		if err != nil {
			return err
		}
		return updateOptionValueTx(tx, "AutoGroups", string(b))
	}

	var legacy []string
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return fmt.Errorf("AutoGroups 配置格式错误: %w", err)
	}
	normalizedCodes, err := NormalizeGroupNames(legacy)
	if err != nil {
		return err
	}
	if len(normalizedCodes) == 0 {
		if trimmed != "[]" {
			return updateOptionValueTx(tx, "AutoGroups", "[]")
		}
		return nil
	}

	codesToResolve := make([]string, 0, len(normalizedCodes))
	for _, code := range normalizedCodes {
		if _, ok := parsePositiveIntString(code); ok {
			continue
		}
		codesToResolve = append(codesToResolve, code)
	}
	codeID, _, err := groupCodeIDMapLoose(tx, codesToResolve)
	if err != nil {
		return err
	}

	next := make([]int, 0, len(normalizedCodes))
	for _, code := range normalizedCodes {
		if id, ok := parsePositiveIntString(code); ok {
			next = append(next, id)
			continue
		}
		id := codeID[code]
		if id <= 0 {
			continue
		}
		next = append(next, id)
	}
	next = dedupIDsPreserveOrder(next)
	next, err = filterExistingActiveGroupIDsKeepOrderTx(tx, next)
	if err != nil {
		return err
	}

	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return updateOptionValueTx(tx, "AutoGroups", string(b))
}

func backfillTopupGroupRatioOption(tx *gorm.DB) error {
	raw, ok, err := readLegacyOptionValue(tx, "TopupGroupRatio")
	if err != nil || !ok {
		return err
	}
	trimmed, usedDefault := normalizeJSONOrDefault(raw, "{}")

	parsed := make(map[int]float64)
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		ids := make([]int, 0, len(parsed))
		for gid, ratio := range parsed {
			if gid <= 0 {
				return errors.New("TopupGroupRatio 存在非正整数 group_id")
			}
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
				return fmt.Errorf("TopupGroupRatio 分组倍率无效: group_id=%d", gid)
			}
			ids = append(ids, gid)
		}
		if len(ids) > 0 {
			validIDs, err := filterExistingSortedIDsTx(tx, ids)
			if err != nil {
				return err
			}
			validSet := make(map[int]struct{}, len(validIDs))
			for _, id := range validIDs {
				validSet[id] = struct{}{}
			}
			for gid := range parsed {
				if _, ok := validSet[gid]; ok {
					continue
				}
				delete(parsed, gid)
			}
		}
		if usedDefault && strings.TrimSpace(raw) == trimmed && len(parsed) == len(ids) {
			return nil
		}
		b, err := json.Marshal(parsed)
		if err != nil {
			return err
		}
		return updateOptionValueTx(tx, "TopupGroupRatio", string(b))
	}

	var legacy map[string]any
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return fmt.Errorf("TopupGroupRatio 配置格式错误: %w", err)
	}
	if len(legacy) == 0 {
		if usedDefault && strings.TrimSpace(raw) != trimmed {
			return updateOptionValueTx(tx, "TopupGroupRatio", trimmed)
		}
		return nil
	}

	codes := make([]string, 0)
	keys := make([]string, 0, len(legacy))
	for k := range legacy {
		keys = append(keys, k)
		kk := strings.TrimSpace(k)
		if kk == "" {
			return errors.New("TopupGroupRatio 存在空分组 key")
		}
		if _, ok := parsePositiveIntString(kk); ok {
			continue
		}
		codes = append(codes, kk)
	}
	codeID, _, err := groupCodeIDMapLoose(tx, codes)
	if err != nil {
		return err
	}

	next := make(map[int]float64, len(legacy))
	ids := make([]int, 0, len(legacy))
	for _, rawKey := range keys {
		kk := strings.TrimSpace(rawKey)
		gid, ok := parsePositiveIntString(kk)
		if !ok {
			gid = codeID[kk]
		}
		if gid <= 0 {
			continue
		}
		ratio, err := parseNonNegativeFloatAny(legacy[rawKey])
		if err != nil {
			return fmt.Errorf("TopupGroupRatio 值无效: group_id=%d: %w", gid, err)
		}
		next[gid] = ratio
		ids = append(ids, gid)
	}
	if len(ids) > 0 {
		validIDs, err := filterExistingSortedIDsTx(tx, ids)
		if err != nil {
			return err
		}
		validSet := make(map[int]struct{}, len(validIDs))
		for _, id := range validIDs {
			validSet[id] = struct{}{}
		}
		for gid := range next {
			if _, ok := validSet[gid]; ok {
				continue
			}
			delete(next, gid)
		}
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return updateOptionValueTx(tx, "TopupGroupRatio", string(b))
}

func backfillGroupGroupRatioOption(tx *gorm.DB) error {
	raw, ok, err := readLegacyOptionValue(tx, "GroupGroupRatio")
	if err != nil || !ok {
		return err
	}
	trimmed, usedDefault := normalizeJSONOrDefault(raw, "{}")

	parsed := make(map[int]map[int]float64)
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		ids := make([]int, 0)
		for ugid, inner := range parsed {
			if ugid <= 0 {
				return errors.New("GroupGroupRatio 存在非正整数 user group_id")
			}
			ids = append(ids, ugid)
			for gid, ratio := range inner {
				if gid <= 0 {
					return errors.New("GroupGroupRatio 存在非正整数 using group_id")
				}
				if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
					return fmt.Errorf("GroupGroupRatio 倍率无效: %d->%d", ugid, gid)
				}
				ids = append(ids, gid)
			}
		}
		if len(ids) > 0 {
			validIDs, err := filterExistingSortedIDsTx(tx, ids)
			if err != nil {
				return err
			}
			validSet := make(map[int]struct{}, len(validIDs))
			for _, id := range validIDs {
				validSet[id] = struct{}{}
			}
			for ugid, inner := range parsed {
				if _, ok := validSet[ugid]; !ok {
					delete(parsed, ugid)
					continue
				}
				for gid := range inner {
					if _, ok := validSet[gid]; ok {
						continue
					}
					delete(inner, gid)
				}
				if len(inner) == 0 {
					delete(parsed, ugid)
				}
			}
		}
		if usedDefault && strings.TrimSpace(raw) == trimmed {
			return nil
		}
		b, err := json.Marshal(parsed)
		if err != nil {
			return err
		}
		return updateOptionValueTx(tx, "GroupGroupRatio", string(b))
	}

	var legacy map[string]any
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return fmt.Errorf("GroupGroupRatio 配置格式错误: %w", err)
	}
	if len(legacy) == 0 {
		if usedDefault && strings.TrimSpace(raw) != trimmed {
			return updateOptionValueTx(tx, "GroupGroupRatio", trimmed)
		}
		return nil
	}

	codes := make([]string, 0)
	for outer := range legacy {
		okey := strings.TrimSpace(outer)
		if okey == "" {
			return errors.New("GroupGroupRatio 存在空分组 key")
		}
		if _, ok := parsePositiveIntString(okey); !ok {
			codes = append(codes, okey)
		}
		innerAny, _ := legacy[outer]
		if innerAny == nil {
			continue
		}
		innerMap, ok := innerAny.(map[string]any)
		if !ok {
			return errors.New("GroupGroupRatio 内层必须为 JSON 对象")
		}
		for inner := range innerMap {
			ikey := strings.TrimSpace(inner)
			if ikey == "" {
				return errors.New("GroupGroupRatio 存在空内层分组 key")
			}
			if _, ok := parsePositiveIntString(ikey); !ok {
				codes = append(codes, ikey)
			}
		}
	}
	codeID, _, err := groupCodeIDMapLoose(tx, codes)
	if err != nil {
		return err
	}

	next := make(map[int]map[int]float64, len(legacy))
	allIDs := make([]int, 0)
	for outerRaw, innerAny := range legacy {
		outerKey := strings.TrimSpace(outerRaw)
		ugid, ok := parsePositiveIntString(outerKey)
		if !ok {
			ugid = codeID[outerKey]
		}
		if ugid <= 0 {
			continue
		}
		innerMap, ok := innerAny.(map[string]any)
		if !ok {
			return errors.New("GroupGroupRatio 内层必须为 JSON 对象")
		}
		convertedInner := make(map[int]float64, len(innerMap))
		for innerRaw, ratioAny := range innerMap {
			innerKey := strings.TrimSpace(innerRaw)
			gid, ok := parsePositiveIntString(innerKey)
			if !ok {
				gid = codeID[innerKey]
			}
			if gid <= 0 {
				continue
			}
			ratio, err := parseNonNegativeFloatAny(ratioAny)
			if err != nil {
				return fmt.Errorf("GroupGroupRatio 值无效: %d->%d: %w", ugid, gid, err)
			}
			convertedInner[gid] = ratio
			allIDs = append(allIDs, gid)
		}
		next[ugid] = convertedInner
		allIDs = append(allIDs, ugid)
	}
	if len(allIDs) > 0 {
		validIDs, err := filterExistingSortedIDsTx(tx, allIDs)
		if err != nil {
			return err
		}
		validSet := make(map[int]struct{}, len(validIDs))
		for _, id := range validIDs {
			validSet[id] = struct{}{}
		}
		for ugid, inner := range next {
			if _, ok := validSet[ugid]; !ok {
				delete(next, ugid)
				continue
			}
			for gid := range inner {
				if _, ok := validSet[gid]; ok {
					continue
				}
				delete(inner, gid)
			}
			if len(inner) == 0 {
				delete(next, ugid)
			}
		}
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return updateOptionValueTx(tx, "GroupGroupRatio", string(b))
}

func backfillModelRequestConcurrencyLimitGroupOption(tx *gorm.DB) error {
	raw, ok, err := readLegacyOptionValue(tx, "ModelRequestConcurrencyLimitGroup")
	if err != nil || !ok {
		return err
	}
	trimmed, usedDefault := normalizeJSONOrDefault(raw, "{}")

	parsed := make(map[int]int)
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		ids := make([]int, 0, len(parsed))
		for gid, limit := range parsed {
			if gid <= 0 {
				return errors.New("ModelRequestConcurrencyLimitGroup 存在非正整数 group_id")
			}
			if limit < 0 {
				return fmt.Errorf("ModelRequestConcurrencyLimitGroup 值无效: group_id=%d", gid)
			}
			ids = append(ids, gid)
		}
		if len(ids) > 0 {
			validIDs, err := filterExistingSortedIDsTx(tx, ids)
			if err != nil {
				return err
			}
			validSet := make(map[int]struct{}, len(validIDs))
			for _, id := range validIDs {
				validSet[id] = struct{}{}
			}
			for gid := range parsed {
				if _, ok := validSet[gid]; ok {
					continue
				}
				delete(parsed, gid)
			}
		}
		if usedDefault && strings.TrimSpace(raw) == trimmed {
			return nil
		}
		b, err := json.Marshal(parsed)
		if err != nil {
			return err
		}
		return updateOptionValueTx(tx, "ModelRequestConcurrencyLimitGroup", string(b))
	}

	var legacy map[string]any
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return fmt.Errorf("ModelRequestConcurrencyLimitGroup 配置格式错误: %w", err)
	}
	if len(legacy) == 0 {
		if usedDefault && strings.TrimSpace(raw) != trimmed {
			return updateOptionValueTx(tx, "ModelRequestConcurrencyLimitGroup", trimmed)
		}
		return nil
	}
	codes := make([]string, 0)
	for k := range legacy {
		kk := strings.TrimSpace(k)
		if kk == "" {
			return errors.New("ModelRequestConcurrencyLimitGroup 存在空分组 key")
		}
		if _, ok := parsePositiveIntString(kk); !ok {
			codes = append(codes, kk)
		}
	}
	codeID, _, err := groupCodeIDMapLoose(tx, codes)
	if err != nil {
		return err
	}

	next := make(map[int]int, len(legacy))
	ids := make([]int, 0, len(legacy))
	for rawKey, v := range legacy {
		kk := strings.TrimSpace(rawKey)
		gid, ok := parsePositiveIntString(kk)
		if !ok {
			gid = codeID[kk]
		}
		if gid <= 0 {
			continue
		}
		limit, err := parseNonNegativeIntAny(v)
		if err != nil {
			return fmt.Errorf("ModelRequestConcurrencyLimitGroup 值无效: group_id=%d: %w", gid, err)
		}
		next[gid] = limit
		ids = append(ids, gid)
	}
	if len(ids) > 0 {
		validIDs, err := filterExistingSortedIDsTx(tx, ids)
		if err != nil {
			return err
		}
		validSet := make(map[int]struct{}, len(validIDs))
		for _, id := range validIDs {
			validSet[id] = struct{}{}
		}
		for gid := range next {
			if _, ok := validSet[gid]; ok {
				continue
			}
			delete(next, gid)
		}
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return updateOptionValueTx(tx, "ModelRequestConcurrencyLimitGroup", string(b))
}

func backfillModelRequestRateLimitGroupOption(tx *gorm.DB) error {
	raw, ok, err := readLegacyOptionValue(tx, "ModelRequestRateLimitGroup")
	if err != nil || !ok {
		return err
	}
	trimmed, usedDefault := normalizeJSONOrDefault(raw, "{}")

	parsed := make(map[int][2]int)
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		ids := make([]int, 0, len(parsed))
		for gid, limits := range parsed {
			if gid <= 0 {
				return errors.New("ModelRequestRateLimitGroup 存在非正整数 group_id")
			}
			if limits[0] < 0 || limits[1] < 1 {
				return fmt.Errorf("ModelRequestRateLimitGroup 值无效: group_id=%d", gid)
			}
			ids = append(ids, gid)
		}
		if len(ids) > 0 {
			validIDs, err := filterExistingSortedIDsTx(tx, ids)
			if err != nil {
				return err
			}
			validSet := make(map[int]struct{}, len(validIDs))
			for _, id := range validIDs {
				validSet[id] = struct{}{}
			}
			for gid := range parsed {
				if _, ok := validSet[gid]; ok {
					continue
				}
				delete(parsed, gid)
			}
		}
		if usedDefault && strings.TrimSpace(raw) == trimmed {
			return nil
		}
		b, err := json.Marshal(parsed)
		if err != nil {
			return err
		}
		return updateOptionValueTx(tx, "ModelRequestRateLimitGroup", string(b))
	}

	var legacy map[string]any
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return fmt.Errorf("ModelRequestRateLimitGroup 配置格式错误: %w", err)
	}
	if len(legacy) == 0 {
		if usedDefault && strings.TrimSpace(raw) != trimmed {
			return updateOptionValueTx(tx, "ModelRequestRateLimitGroup", trimmed)
		}
		return nil
	}

	codes := make([]string, 0)
	for k := range legacy {
		kk := strings.TrimSpace(k)
		if kk == "" {
			return errors.New("ModelRequestRateLimitGroup 存在空分组 key")
		}
		if _, ok := parsePositiveIntString(kk); !ok {
			codes = append(codes, kk)
		}
	}
	codeID, _, err := groupCodeIDMapLoose(tx, codes)
	if err != nil {
		return err
	}

	next := make(map[int][2]int, len(legacy))
	ids := make([]int, 0, len(legacy))
	for rawKey, v := range legacy {
		kk := strings.TrimSpace(rawKey)
		gid, ok := parsePositiveIntString(kk)
		if !ok {
			gid = codeID[kk]
		}
		if gid <= 0 {
			continue
		}

		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("ModelRequestRateLimitGroup 值无效: group_id=%d", gid)
		}
		if len(arr) != 2 {
			return fmt.Errorf("ModelRequestRateLimitGroup 值必须为长度=2的数组: group_id=%d", gid)
		}
		total, err := parseNonNegativeIntAny(arr[0])
		if err != nil {
			return fmt.Errorf("ModelRequestRateLimitGroup total 无效: group_id=%d: %w", gid, err)
		}
		success, err := parsePositiveIntAny(arr[1])
		if err != nil {
			return fmt.Errorf("ModelRequestRateLimitGroup success 无效: group_id=%d: %w", gid, err)
		}

		next[gid] = [2]int{total, success}
		ids = append(ids, gid)
	}
	if len(ids) > 0 {
		validIDs, err := filterExistingSortedIDsTx(tx, ids)
		if err != nil {
			return err
		}
		validSet := make(map[int]struct{}, len(validIDs))
		for _, id := range validIDs {
			validSet[id] = struct{}{}
		}
		for gid := range next {
			if _, ok := validSet[gid]; ok {
				continue
			}
			delete(next, gid)
		}
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return updateOptionValueTx(tx, "ModelRequestRateLimitGroup", string(b))
}

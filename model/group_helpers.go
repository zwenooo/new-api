package model

import (
	"errors"
	"one-api/common"
	"strings"
	"unicode/utf8"
)

func NormalizeGroupNames(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, v := range raw {
		name := strings.TrimSpace(v)
		if name == "" {
			continue
		}
		if utf8.RuneCountInString(name) > 64 {
			return nil, errors.New("分组名称过长")
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// ParseGroupNamesCSV parses a comma-separated group string (e.g. "default,vip") into normalized group codes.
// Empty input returns nil, nil.
func ParseGroupNamesCSV(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, ",")
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, ",")
	return NormalizeGroupNames(parts)
}

func ParseGroupNamesJSON(value JSONValue) ([]string, error) {
	if len(value) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var raw []string
	if err := common.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, errors.New("可用分组解析失败")
	}
	return NormalizeGroupNames(raw)
}

func MarshalGroupNamesJSON(groups []string) (JSONValue, error) {
	normalized, err := NormalizeGroupNames(groups)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	b, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return JSONValue(b), nil
}

func MergeGroupNames(a []string, b []string) ([]string, error) {
	na, err := NormalizeGroupNames(a)
	if err != nil {
		return nil, err
	}
	nb, err := NormalizeGroupNames(b)
	if err != nil {
		return nil, err
	}
	if len(na) == 0 {
		return nb, nil
	}
	if len(nb) == 0 {
		return na, nil
	}
	seen := make(map[string]struct{}, len(na)+len(nb))
	out := make([]string, 0, len(na)+len(nb))
	for _, v := range na {
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range nb {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

func ParseGroupIDsJSON(value JSONValue) ([]int, error) {
	if len(value) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var raw []int
	if err := common.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, errors.New("可用分组解析失败")
	}
	return normalizeUniqueSortedIDs(raw), nil
}

func MarshalGroupIDsJSON(ids []int) (JSONValue, error) {
	normalized := normalizeUniqueSortedIDs(ids)
	if len(normalized) == 0 {
		return nil, nil
	}
	b, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return JSONValue(b), nil
}

func normalizeUniquePositiveIDsKeepOrder(ids []int) []int {
	if len(ids) == 0 {
		return nil
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
	if len(out) == 0 {
		return nil
	}
	return out
}

func FirstGroupIDKeepOrder(ids []int) int {
	normalized := normalizeUniquePositiveIDsKeepOrder(ids)
	if len(normalized) == 0 {
		return 0
	}
	return normalized[0]
}

func FirstGroupIDFromJSONKeepOrder(value JSONValue) (int, error) {
	ids, err := ParseGroupIDsJSONKeepOrder(value)
	if err != nil {
		return 0, err
	}
	return FirstGroupIDKeepOrder(ids), nil
}

// ParseGroupIDsJSONKeepOrder parses JSON array of group IDs and preserves the original order.
// It deduplicates and drops non-positive ids.
func ParseGroupIDsJSONKeepOrder(value JSONValue) ([]int, error) {
	if len(value) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var raw []int
	if err := common.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, errors.New("可用分组解析失败")
	}
	return normalizeUniquePositiveIDsKeepOrder(raw), nil
}

// MarshalGroupIDsJSONKeepOrder marshals group IDs into JSON array while keeping the provided order.
// It deduplicates and drops non-positive ids.
func MarshalGroupIDsJSONKeepOrder(ids []int) (JSONValue, error) {
	normalized := normalizeUniquePositiveIDsKeepOrder(ids)
	if len(normalized) == 0 {
		return nil, nil
	}
	b, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return JSONValue(b), nil
}

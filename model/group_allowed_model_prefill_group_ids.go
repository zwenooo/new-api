package model

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

func normalizeAllowedModelPrefillGroupIDs(ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]int, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, raw := range ids {
		if raw <= 0 {
			return nil, errors.New("预填组 id 必须为正整数")
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Ints(out)
	return out, nil
}

func normalizeAllowedModelPrefillGroupIDsJSON(raw JSONValue) (JSONValue, []int, error) {
	if raw == nil {
		return nil, nil, nil
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil, nil
	}

	var ids []int
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return nil, nil, err
	}
	normalized, err := normalizeAllowedModelPrefillGroupIDs(ids)
	if err != nil {
		return nil, nil, err
	}
	if len(normalized) == 0 {
		return nil, nil, nil
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, err
	}
	return JSONValue(b), normalized, nil
}


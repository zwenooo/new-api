package model

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

func normalizeAllowedModels(models []string) ([]string, error) {
	if len(models) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, raw := range models {
		model := strings.TrimSpace(raw)
		if model == "" {
			return nil, errors.New("模型名不能为空")
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

func normalizeAllowedModelsJSON(raw JSONValue) (JSONValue, []string, error) {
	if raw == nil {
		return nil, nil, nil
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil, nil
	}

	var models []string
	if err := json.Unmarshal([]byte(trimmed), &models); err != nil {
		return nil, nil, err
	}
	normalized, err := normalizeAllowedModels(models)
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


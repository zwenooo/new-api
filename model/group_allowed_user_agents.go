package model

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

func normalizeAllowedUserAgents(agents []string) ([]string, error) {
	if len(agents) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(agents))
	seen := make(map[string]struct{}, len(agents))
	for _, raw := range agents {
		kw := strings.ToLower(strings.TrimSpace(raw))
		if kw == "" {
			return nil, errors.New("UA 关键词不能为空")
		}
		if _, ok := seen[kw]; ok {
			continue
		}
		seen[kw] = struct{}{}
		out = append(out, kw)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

func normalizeAllowedUserAgentsJSON(raw JSONValue) (JSONValue, []string, error) {
	if raw == nil {
		return nil, nil, nil
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil, nil
	}

	var agents []string
	if err := json.Unmarshal([]byte(trimmed), &agents); err != nil {
		return nil, nil, err
	}
	normalized, err := normalizeAllowedUserAgents(agents)
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

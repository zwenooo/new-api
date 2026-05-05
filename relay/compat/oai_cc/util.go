package oai_cc

import "strings"

func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func getNested(m map[string]any, keys ...string) any {
	if m == nil || len(keys) == 0 {
		return nil
	}
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok || mm == nil {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func normalizeSpaceSingleLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}


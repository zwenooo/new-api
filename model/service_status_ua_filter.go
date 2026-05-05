package model

import (
	"crypto/sha256"
	"encoding/hex"
	"one-api/common"
	"one-api/setting/operation_setting"
	"sort"
	"strings"
	"sync"
)

type serviceStatusUAContainsCache struct {
	rawMode     string
	rawKeywords string

	mode     string
	keywords []string
	hash     string
	mu       sync.RWMutex
}

const (
	serviceStatusUAFilterModeInclude = "include"
	serviceStatusUAFilterModeExclude = "exclude"
)

var serviceStatusUAContains serviceStatusUAContainsCache

func normalizeServiceStatusUAFilterMode(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case serviceStatusUAFilterModeInclude, serviceStatusUAFilterModeExclude:
		return raw
	default:
		return serviceStatusUAFilterModeInclude
	}
}

func getServiceStatusUAFilter() (string, []string, string) {
	ms := operation_setting.GetMonitorSetting()
	rawMode := strings.TrimSpace(ms.ServiceStatusUAFilterMode)
	rawKeywords := strings.TrimSpace(ms.ServiceStatusUAContains)

	serviceStatusUAContains.mu.RLock()
	if rawMode == serviceStatusUAContains.rawMode && rawKeywords == serviceStatusUAContains.rawKeywords {
		mode := serviceStatusUAContains.mode
		keywords := serviceStatusUAContains.keywords
		hash := serviceStatusUAContains.hash
		serviceStatusUAContains.mu.RUnlock()
		return mode, keywords, hash
	}
	serviceStatusUAContains.mu.RUnlock()

	mode := normalizeServiceStatusUAFilterMode(rawMode)
	keywords := normalizeServiceStatusUAKeywords(rawKeywords)
	hash := hashServiceStatusUAFilter(mode, keywords)

	serviceStatusUAContains.mu.Lock()
	serviceStatusUAContains.rawMode = rawMode
	serviceStatusUAContains.rawKeywords = rawKeywords
	serviceStatusUAContains.mode = mode
	serviceStatusUAContains.keywords = keywords
	serviceStatusUAContains.hash = hash
	serviceStatusUAContains.mu.Unlock()

	return mode, keywords, hash
}

func normalizeServiceStatusUAKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case '\n', ',', ';':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return nil
	}

	keywords := make([]string, 0, len(parts))
	for _, p := range parts {
		kw := strings.ToLower(strings.TrimSpace(p))
		if kw == "" {
			continue
		}
		keywords = append(keywords, kw)
	}
	if len(keywords) == 0 {
		return nil
	}

	sort.Strings(keywords)

	out := keywords[:0]
	last := ""
	for _, kw := range keywords {
		if kw == last {
			continue
		}
		out = append(out, kw)
		last = kw
	}
	return out
}

func hashServiceStatusUAFilter(mode string, keywords []string) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(mode)))
	h.Write([]byte{'\n'})
	for i, kw := range keywords {
		if i > 0 {
			h.Write([]byte{'\n'})
		}
		h.Write([]byte(kw))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func escapeLikePattern(s string) string {
	// Use a stable single-character escape across MySQL/SQLite/PostgreSQL.
	// (MySQL treats backslash as an escape in string literals, SQLite doesn't.)
	s = strings.ReplaceAll(s, "!", "!!")
	s = strings.ReplaceAll(s, "%", "!%")
	s = strings.ReplaceAll(s, "_", "!_")
	return s
}

func buildServiceStatusUAFilterWhere(mode string, keywords []string) (string, []interface{}) {
	if len(keywords) == 0 {
		if mode == serviceStatusUAFilterModeExclude {
			// Exclude mode with empty keywords means "count everything".
			return "", nil
		}
		return "", nil
	}

	likeExpr := "other LIKE ? ESCAPE '!'"
	if common.LogSqlType == common.DatabaseTypePostgreSQL {
		likeExpr = "other ILIKE ? ESCAPE '!'"
	}

	conds := make([]string, 0, len(keywords))
	args := make([]interface{}, 0, len(keywords))
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		conds = append(conds, likeExpr)
		args = append(args, "%\"request_ua\":\"%"+escapeLikePattern(kw)+"%")
	}
	if len(conds) == 0 {
		return "", nil
	}
	inner := "(" + strings.Join(conds, " OR ") + ")"
	if mode == serviceStatusUAFilterModeExclude {
		return "NOT " + inner, args
	}
	return inner, args
}

func serviceStatusOtherContainsAnyUAKeyword(other string, keywords []string) bool {
	if len(keywords) == 0 || strings.TrimSpace(other) == "" {
		return false
	}

	lowerOther := strings.ToLower(other)
	const key = `"request_ua":"`
	if idx := strings.Index(lowerOther, key); idx >= 0 {
		start := idx + len(key)
		if start < len(lowerOther) {
			if end := strings.IndexByte(lowerOther[start:], '"'); end > 0 {
				ua := lowerOther[start : start+end]
				for _, kw := range keywords {
					if kw == "" {
						continue
					}
					if strings.Contains(ua, kw) {
						return true
					}
				}
				return false
			}
		}
	}

	// Fallback to substring match within the tail of JSON if the value cannot be extracted.
	if idx := strings.Index(lowerOther, `"request_ua":`); idx >= 0 {
		tail := lowerOther[idx:]
		for _, kw := range keywords {
			if kw == "" {
				continue
			}
			if strings.Contains(tail, kw) {
				return true
			}
		}
	}

	return false
}

func serviceStatusOtherMatchesUAFilter(other string, mode string, keywords []string) bool {
	mode = normalizeServiceStatusUAFilterMode(mode)

	switch mode {
	case serviceStatusUAFilterModeExclude:
		// Exclude mode: count everything except UAs that match the blacklist.
		if len(keywords) == 0 {
			return true
		}
		return !serviceStatusOtherContainsAnyUAKeyword(other, keywords)
	default:
		// Include mode: only count UAs that match the whitelist (legacy behavior).
		if len(keywords) == 0 {
			return false
		}
		return serviceStatusOtherContainsAnyUAKeyword(other, keywords)
	}
}

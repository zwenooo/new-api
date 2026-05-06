package operation_setting

import (
	"fmt"
	"strconv"
	"strings"
)

var DemoSiteEnabled = false
var SelfUseModeEnabled = false
var ChatCompletionsEnabled = false

var AutomaticDisableKeywords = []string{
	"your credit balance is too low",
	"this organization has been disabled.",
	"you exceeded your current quota",
	"permission denied",
	"the security token included in the request is invalid",
	"operation not allowed",
	"your account is not authorized",
}

var AutomaticDisableKeywordDurations = make(map[string]int64)

// AutomaticSwitchKeywords is kept for compatibility with historical option values.
// Automatic channel switch now uses status code whitelist instead.
var AutomaticSwitchKeywords = []string{}

var AutomaticSwitchStatusCodeWhitelist = []int{}
var AutomaticSwitchStatusCodeWhitelistSet = map[int]struct{}{}
var AutomaticSwitchMaxRetries = 5
var ResponsesCapacityRetryEnabled = false
var ResponsesCapacityRetryKeywords = []string{}

func AutomaticDisableKeywordsToString() string {
	entries := make([]string, 0, len(AutomaticDisableKeywords))
	for _, keyword := range AutomaticDisableKeywords {
		cleaned := strings.TrimSpace(keyword)
		if cleaned == "" {
			continue
		}
		if seconds, ok := AutomaticDisableKeywordDurations[cleaned]; ok && seconds > 0 {
			entries = append(entries, fmt.Sprintf("%s:%d", cleaned, seconds))
		} else {
			entries = append(entries, cleaned)
		}
	}
	return strings.Join(entries, "\n")
}

func AutomaticSwitchKeywordsToString() string {
	entries := make([]string, 0, len(AutomaticSwitchKeywords))
	for _, keyword := range AutomaticSwitchKeywords {
		cleaned := strings.TrimSpace(keyword)
		if cleaned == "" {
			continue
		}
		entries = append(entries, cleaned)
	}
	return strings.Join(entries, "\n")
}

func AutomaticSwitchStatusCodeWhitelistToString() string {
	entries := make([]string, 0, len(AutomaticSwitchStatusCodeWhitelist))
	for _, statusCode := range AutomaticSwitchStatusCodeWhitelist {
		entries = append(entries, strconv.Itoa(statusCode))
	}
	return strings.Join(entries, "\n")
}

func ResponsesCapacityRetryKeywordsToString() string {
	entries := make([]string, 0, len(ResponsesCapacityRetryKeywords))
	for _, keyword := range ResponsesCapacityRetryKeywords {
		cleaned := strings.TrimSpace(keyword)
		if cleaned == "" {
			continue
		}
		entries = append(entries, cleaned)
	}
	return strings.Join(entries, "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	AutomaticDisableKeywordDurations = make(map[string]int64)
	ak := strings.Split(s, "\n")
	for _, raw := range ak {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		keyword := lower
		var restoreSeconds int64

		if idx := strings.LastIndex(lower, ":"); idx != -1 {
			valueText := strings.TrimSpace(lower[idx+1:])
			if valueText != "" {
				if parsed, err := strconv.ParseInt(valueText, 10, 64); err == nil {
					if parsed > 0 {
						restoreSeconds = parsed
						keyword = strings.TrimSpace(lower[:idx])
					}
				}
			}
		}

		if keyword == "" {
			continue
		}

		AutomaticDisableKeywords = append(AutomaticDisableKeywords, keyword)
		if restoreSeconds > 0 {
			AutomaticDisableKeywordDurations[keyword] = restoreSeconds
		}
	}
}

func AutomaticSwitchKeywordsFromString(s string) {
	AutomaticSwitchKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, raw := range ak {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		keyword := lower

		// Compatibility: allow "keyword:60" style input (ignore the trailing duration).
		if idx := strings.LastIndex(lower, ":"); idx != -1 {
			valueText := strings.TrimSpace(lower[idx+1:])
			if valueText != "" {
				if parsed, err := strconv.ParseInt(valueText, 10, 64); err == nil && parsed > 0 {
					keyword = strings.TrimSpace(lower[:idx])
				}
			}
		}

		if keyword == "" {
			continue
		}

		AutomaticSwitchKeywords = append(AutomaticSwitchKeywords, keyword)
	}
}

func ResponsesCapacityRetryKeywordsFromString(s string) {
	ResponsesCapacityRetryKeywords = []string{}
	seen := make(map[string]struct{})
	for _, raw := range strings.Split(s, "\n") {
		keyword := strings.ToLower(strings.TrimSpace(raw))
		if keyword == "" {
			continue
		}
		if _, exists := seen[keyword]; exists {
			continue
		}
		seen[keyword] = struct{}{}
		ResponsesCapacityRetryKeywords = append(ResponsesCapacityRetryKeywords, keyword)
	}
}

func AutomaticSwitchStatusCodeWhitelistFromString(s string) error {
	statusCodes, statusCodeSet, err := ParseAutomaticSwitchStatusCodeWhitelist(s)
	if err != nil {
		return err
	}

	AutomaticSwitchStatusCodeWhitelist = statusCodes
	AutomaticSwitchStatusCodeWhitelistSet = statusCodeSet
	return nil
}

func ParseAutomaticSwitchStatusCodeWhitelist(s string) ([]int, map[int]struct{}, error) {
	statusCodes := make([]int, 0)
	statusCodeSet := make(map[int]struct{})
	lines := strings.Split(s, "\n")
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		statusCode, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, nil, fmt.Errorf("自动切换状态码白名单存在非法状态码：%q", trimmed)
		}
		if statusCode < 100 || statusCode > 599 {
			return nil, nil, fmt.Errorf("自动切换状态码白名单仅支持 100-599：%d", statusCode)
		}
		if _, exists := statusCodeSet[statusCode]; exists {
			continue
		}

		statusCodeSet[statusCode] = struct{}{}
		statusCodes = append(statusCodes, statusCode)
	}

	return statusCodes, statusCodeSet, nil
}

func ValidateAutomaticSwitchStatusCodeWhitelist(s string) error {
	_, _, err := ParseAutomaticSwitchStatusCodeWhitelist(s)
	return err
}

func ValidateAutomaticSwitchMaxRetries(value int) error {
	if value < 0 {
		return fmt.Errorf("AutomaticSwitchMaxRetries 必须大于等于 0")
	}
	return nil
}

func HasAutomaticSwitchStatusCodeWhitelist() bool {
	return len(AutomaticSwitchStatusCodeWhitelist) > 0
}

func IsAutomaticSwitchStatusCodeAllowed(statusCode int) bool {
	if len(AutomaticSwitchStatusCodeWhitelistSet) == 0 {
		return false
	}
	_, ok := AutomaticSwitchStatusCodeWhitelistSet[statusCode]
	return ok
}

func GetAutomaticDisableKeywordDuration(keyword string) int64 {
	cleaned := strings.ToLower(strings.TrimSpace(keyword))
	if cleaned == "" {
		return 0
	}
	if seconds, ok := AutomaticDisableKeywordDurations[cleaned]; ok {
		return seconds
	}
	return 0
}

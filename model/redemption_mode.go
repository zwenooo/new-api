package model

import "strings"

func jsonValueHasElements(value JSONValue) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != "[]"
}

func resolveCompatibleRedemptionModeFields(mode string, quota int, dailyQuotaLimit int, dailyRequestLimit int, quotaValidDays int, planValidDays int, channelIDs JSONValue, allowedGroupIDs JSONValue) string {
	normalizedMode := strings.TrimSpace(mode)
	if normalizedMode != "" {
		return normalizedMode
	}
	switch {
	case planValidDays > 0 || jsonValueHasElements(channelIDs):
		return "xiaotuan"
	case dailyRequestLimit > 0:
		return "request"
	case dailyQuotaLimit > 0 || quotaValidDays > 0:
		return "subscription"
	case jsonValueHasElements(allowedGroupIDs):
		return "payg"
	case quota > 0:
		return "free"
	default:
		return ""
	}
}

func ResolveCompatibleRedemptionMode(redemption *Redemption) string {
	if redemption == nil {
		return ""
	}
	return resolveCompatibleRedemptionModeFields(
		redemption.Mode,
		redemption.Quota,
		redemption.DailyQuotaLimit,
		redemption.DailyRequestLimit,
		redemption.QuotaValidDays,
		redemption.PlanValidDays,
		redemption.ChannelIds,
		redemption.AllowedGroupIds,
	)
}

func NormalizeCompatibleRedemptionMode(redemption *Redemption) {
	if redemption == nil {
		return
	}
	if resolved := ResolveCompatibleRedemptionMode(redemption); resolved != "" {
		redemption.Mode = resolved
	} else {
		redemption.Mode = strings.TrimSpace(redemption.Mode)
	}
}

func ResolveCompatibleRedemptionPresetMode(preset *RedemptionPreset) string {
	if preset == nil {
		return ""
	}
	return resolveCompatibleRedemptionModeFields(
		preset.Mode,
		preset.Quota,
		preset.DailyQuotaLimit,
		preset.DailyRequestLimit,
		preset.QuotaValidDays,
		preset.PlanValidDays,
		preset.ChannelIds,
		preset.AllowedGroupIds,
	)
}

func NormalizeCompatibleRedemptionPresetMode(preset *RedemptionPreset) {
	if preset == nil {
		return
	}
	if resolved := ResolveCompatibleRedemptionPresetMode(preset); resolved != "" {
		preset.Mode = resolved
	} else {
		preset.Mode = strings.TrimSpace(preset.Mode)
	}
}

func ResolveCompatibleRedemptionPresetRevisionMode(revision *RedemptionPresetRevision) string {
	if revision == nil {
		return ""
	}
	return resolveCompatibleRedemptionModeFields(
		revision.Mode,
		revision.Quota,
		revision.DailyQuotaLimit,
		revision.DailyRequestLimit,
		revision.QuotaValidDays,
		revision.PlanValidDays,
		revision.ChannelIds,
		revision.AllowedGroupIds,
	)
}

func NormalizeCompatibleRedemptionPresetRevisionMode(revision *RedemptionPresetRevision) {
	if revision == nil {
		return
	}
	if resolved := ResolveCompatibleRedemptionPresetRevisionMode(revision); resolved != "" {
		revision.Mode = resolved
	} else {
		revision.Mode = strings.TrimSpace(revision.Mode)
	}
}

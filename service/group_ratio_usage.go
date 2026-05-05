package service

import (
	"one-api/billing"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/setting/ratio_setting"

	"github.com/shopspring/decimal"
)

// ResolveEffectiveGroupRatio mirrors relay/helper.HandleGroupRatio without
// introducing a package cycle. It is used by billing paths that need to apply
// group ratio to non-currency buckets (tokens / requests).
func ResolveEffectiveGroupRatio(relayInfo *relaycommon.RelayInfo) float64 {
	if relayInfo == nil {
		return 1
	}
	if relayInfo.UsingGroupId <= 0 {
		return 1
	}
	if relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		return 0
	}
	publicGroupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroupId)
	info, _, err := model.ResolveUserGroupRatioInfoByID(relayInfo.UserId, relayInfo.UsingGroupId)
	if err != nil {
		return publicGroupRatio
	}
	return info.EffectiveGroupRatio
}

func ResolvePublicGroupRatio(relayInfo *relaycommon.RelayInfo) float64 {
	if relayInfo == nil {
		return 1
	}
	if relayInfo.UsingGroupId <= 0 {
		return 1
	}
	if relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		return 0
	}
	publicGroupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroupId)
	info, _, err := model.ResolveUserGroupRatioInfoByID(relayInfo.UserId, relayInfo.UsingGroupId)
	if err != nil {
		return publicGroupRatio
	}
	return info.PublicGroupRatio
}

func ComputeTokenBucketUsage(relayInfo *relaycommon.RelayInfo, tokens int) int {
	scaled := billing.ScaleTokensByGroupRatio(tokens, ResolveEffectiveGroupRatio(relayInfo))
	if scaled <= 0 {
		return 0
	}
	multiplier := 1.0
	if relayInfo != nil {
		multiplier = relayInfo.ServiceTierCostMultiplier()
	}
	if multiplier == 1 {
		return scaled
	}
	result := decimal.NewFromInt(int64(scaled)).Mul(decimal.NewFromFloat(multiplier)).Round(0).IntPart()
	if result <= 0 {
		return 1
	}
	return int(result)
}

func ComputeRequestBucketUsage(relayInfo *relaycommon.RelayInfo, requests int) int {
	scaled := billing.ScaleRequestsByGroupRatio(requests, ResolveEffectiveGroupRatio(relayInfo))
	if scaled <= 0 {
		return 0
	}
	return scaled
}

func ComputeTokenBucketPreConsumedUsage(relayInfo *relaycommon.RelayInfo) int {
	if relayInfo == nil {
		return 0
	}
	preConsumedTokens := relayInfo.PromptTokens
	if maxTokens := extractMaxTokens(relayInfo); maxTokens > 0 {
		preConsumedTokens += maxTokens
	}
	if preConsumedTokens <= 0 {
		preConsumedTokens = 1
	}
	return ComputeTokenBucketUsage(relayInfo, preConsumedTokens)
}

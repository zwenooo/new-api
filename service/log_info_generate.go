package service

import (
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

func extractPromptCacheKeyFromRelayInfo(relayInfo *relaycommon.RelayInfo) string {
	if relayInfo == nil || relayInfo.Request == nil {
		return ""
	}
	req, ok := relayInfo.Request.(*dto.OpenAIResponsesRequest)
	if !ok || req == nil || len(req.PromptCacheKey) == 0 {
		return ""
	}
	if common.GetJsonType(req.PromptCacheKey) != "string" {
		return ""
	}
	var key string
	if err := common.Unmarshal(req.PromptCacheKey, &key); err != nil {
		return ""
	}
	return strings.TrimSpace(key)
}

func appendConversationContext(other map[string]interface{}, ctx *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if other == nil || ctx == nil {
		return
	}
	promptCacheKey := extractPromptCacheKeyFromRelayInfo(relayInfo)
	if promptCacheKey != "" {
		other["prompt_cache_key"] = promptCacheKey
	}

	conversationID := strings.TrimSpace(ctx.GetHeader("conversation_id"))
	sessionID := strings.TrimSpace(ctx.GetHeader("session_id"))
	if conversationID == "" && promptCacheKey != "" {
		conversationID = promptCacheKey
	}
	if sessionID == "" && promptCacheKey != "" {
		sessionID = promptCacheKey
	}
	if conversationID != "" {
		other["conversation_id"] = conversationID
	}
	if sessionID != "" {
		other["session_id"] = sessionID
	}
}

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64, baseMultiplierApplied bool) map[string]interface{} {
	return GenerateTextOtherInfoWithQuotaMetrics(
		ctx, relayInfo,
		modelRatio, groupRatio, groupRatio, completionRatio,
		cacheTokens, cacheRatio,
		modelPrice, userGroupRatio, baseMultiplierApplied,
		0, 0, 0,
	)
}

func GenerateTextOtherInfoWithQuotaMetrics(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64, baseMultiplierApplied bool, settledQuota int, visibleQuota int, costQuota int) map[string]interface{} {
	other := make(map[string]interface{})
	if ctx != nil {
		if requestID := strings.TrimSpace(ctx.GetString(common.RequestIdKey)); requestID != "" {
			other["request_id"] = requestID
		}
	}
	if relayInfo != nil {
		other["quota_bucket"] = relayInfo.QuotaBucket
	}
	other["model_ratio"] = modelRatio
	other["group_ratio"] = groupRatio
	other["public_group_ratio"] = publicGroupRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	other["user_group_ratio"] = userGroupRatio
	other["base_multiplier_applied"] = baseMultiplierApplied
	other["group_ratio_source"] = relayInfo.PriceData.GroupRatioInfo.Source
	other["settled_quota"] = settledQuota
	other["visible_quota"] = visibleQuota
	other["cost_quota"] = costQuota
	other["base_multiplier"] = relayInfo.BaseMultiplier
	other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	if reason := common.GetContextKeyString(ctx, constant.ContextKeyStreamExitReason); reason != "" {
		other["stream_exit_reason"] = reason
	}
	if errMsg := common.GetContextKeyString(ctx, constant.ContextKeyStreamExitError); errMsg != "" {
		other["stream_exit_error"] = errMsg
	}
	if relayInfo.ReasoningEffort != "" {
		other["reasoning_effort"] = relayInfo.ReasoningEffort
	}
	if serviceTier := relayInfo.EffectiveServiceTier(); serviceTier != "" {
		other["service_tier"] = serviceTier
		other["service_tier_multiplier"] = relayInfo.ServiceTierCostMultiplier()
	}
	if relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}

	appendConversationContext(other, ctx, relayInfo)

	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}
	other["admin_info"] = adminInfo
	return other
}

func GenerateWssOtherInfoWithQuotaMetrics(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, publicGroupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64, settledQuota int, visibleQuota int, costQuota int) map[string]interface{} {
	info := GenerateTextOtherInfoWithQuotaMetrics(ctx, relayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio, relayInfo.PriceData.GroupRatioInfo.BaseMultiplierApplied, settledQuota, visibleQuota, costQuota)
	info["ws"] = true
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateAudioOtherInfoWithQuotaMetrics(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, publicGroupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64, settledQuota int, visibleQuota int, costQuota int) map[string]interface{} {
	info := GenerateTextOtherInfoWithQuotaMetrics(ctx, relayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio, relayInfo.PriceData.GroupRatioInfo.BaseMultiplierApplied, settledQuota, visibleQuota, costQuota)
	info["audio"] = true
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64, baseMultiplierApplied bool) map[string]interface{} {
	return GenerateClaudeOtherInfoWithQuotaMetrics(
		ctx, relayInfo,
		modelRatio, groupRatio, groupRatio, completionRatio,
		cacheTokens, cacheRatio,
		cacheCreationTokens, cacheCreationRatio,
		cacheCreationTokens5m, cacheCreationRatio5m,
		cacheCreationTokens1h, cacheCreationRatio1h,
		modelPrice, userGroupRatio, baseMultiplierApplied,
		0, 0, 0,
	)
}

func GenerateClaudeOtherInfoWithQuotaMetrics(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64, baseMultiplierApplied bool, settledQuota int, visibleQuota int, costQuota int) map[string]interface{} {
	info := GenerateTextOtherInfoWithQuotaMetrics(ctx, relayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio, baseMultiplierApplied, settledQuota, visibleQuota, costQuota)
	info["claude"] = true
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	if cacheCreationTokens5m != 0 {
		info["cache_creation_tokens_5m"] = cacheCreationTokens5m
		info["cache_creation_ratio_5m"] = cacheCreationRatio5m
	}
	if cacheCreationTokens1h != 0 {
		info["cache_creation_tokens_1h"] = cacheCreationTokens1h
		info["cache_creation_ratio_1h"] = cacheCreationRatio1h
	}
	return info
}

func GenerateMjOtherInfo(priceData types.PerCallPriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	other["group_ratio"] = priceData.GroupRatioInfo.EffectiveGroupRatio
	other["public_group_ratio"] = priceData.GroupRatioInfo.PublicGroupRatio
	other["private_group_ratio"] = priceData.GroupRatioInfo.PrivateGroupRatio
	other["base_multiplier_applied"] = priceData.GroupRatioInfo.BaseMultiplierApplied
	other["group_ratio_source"] = priceData.GroupRatioInfo.Source
	other["settled_quota"] = priceData.Quota
	other["visible_quota"] = priceData.VisibleQuota
	other["cost_quota"] = priceData.CostQuota
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
	}
	return other
}

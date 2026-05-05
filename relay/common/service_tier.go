package common

import (
	appcommon "one-api/common"
	"one-api/dto"
	relayconstant "one-api/relay/constant"
	"one-api/setting/model_setting"
	"one-api/setting/ratio_setting"
	"strings"
)

const (
	ServiceTierPriority = "priority"
	ServiceTierFlex     = "flex"
)

const (
	directCodexResponsesDefaultInstructions = "You are a helpful coding assistant."
	openAIGPT54LongContextInputThreshold    = 272000
	openAIGPT54LongContextInputMultiplier   = 2.0
	openAIGPT54LongContextOutputMultiplier  = 1.5
)

func NormalizeServiceTier(serviceTier string) string {
	switch strings.ToLower(strings.TrimSpace(serviceTier)) {
	case "fast", ServiceTierPriority:
		return ServiceTierPriority
	case ServiceTierFlex:
		return ServiceTierFlex
	default:
		return ""
	}
}

func ServiceTierCostMultiplier(serviceTier string) float64 {
	switch NormalizeServiceTier(serviceTier) {
	case ServiceTierPriority:
		return 2.0
	case ServiceTierFlex:
		return 0.5
	default:
		return 1.0
	}
}

func ApplyServiceTierPricing(usePrice bool, modelPrice, modelRatio float64, serviceTier string) (float64, float64, float64) {
	multiplier := ServiceTierCostMultiplier(serviceTier)
	if multiplier == 1.0 {
		return modelPrice, modelRatio, multiplier
	}
	if usePrice {
		modelPrice *= multiplier
	} else {
		modelRatio *= multiplier
	}
	return modelPrice, modelRatio, multiplier
}

func ResolveAllowedServiceTier(serviceTier string, channelOtherSettings dto.ChannelOtherSettings, channelPassThroughEnabled bool) string {
	serviceTier = NormalizeServiceTier(serviceTier)
	if serviceTier == "" {
		return ""
	}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || channelPassThroughEnabled {
		return serviceTier
	}
	if !channelOtherSettings.AllowServiceTier {
		return ""
	}
	return serviceTier
}

func normalizeServiceTierModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(ratio_setting.FormatMatchingModelName(modelName)))
}

func HasDedicatedPriorityServiceTierPricing(modelName string) bool {
	switch normalizeServiceTierModelName(modelName) {
	case "gpt-5.1",
		"gpt-5.2",
		"gpt-5.4",
		"gpt-5.1-codex",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"codex-mini-latest":
		return true
	default:
		return false
	}
}

func AdjustCacheCreationRatiosForServiceTier(modelName, serviceTier string, cacheCreationRatio, cacheCreationRatio5m, cacheCreationRatio1h float64) (float64, float64, float64) {
	if NormalizeServiceTier(serviceTier) != ServiceTierPriority || !HasDedicatedPriorityServiceTierPricing(modelName) {
		return cacheCreationRatio, cacheCreationRatio5m, cacheCreationRatio1h
	}
	return cacheCreationRatio / 2, cacheCreationRatio5m / 2, cacheCreationRatio1h / 2
}

func LongContextInputOutputMultipliers(modelName string, inputTokens, cacheReadTokens int) (float64, float64) {
	if normalizeServiceTierModelName(modelName) != "gpt-5.4" {
		return 1, 1
	}
	totalInputTokens := inputTokens + cacheReadTokens
	if totalInputTokens > openAIGPT54LongContextInputThreshold {
		return openAIGPT54LongContextInputMultiplier, openAIGPT54LongContextOutputMultiplier
	}
	return 1, 1
}

func (info *RelayInfo) RequestedServiceTier() string {
	if info == nil {
		return ""
	}
	req, ok := info.Request.(*dto.OpenAIResponsesRequest)
	if ok && req != nil {
		return NormalizeServiceTier(req.ServiceTier)
	}
	return NormalizeServiceTier(info.ServiceTier)
}

func (info *RelayInfo) ApplyChannelServiceTierPolicy() {
	if info == nil {
		return
	}
	if info.ChannelMeta == nil {
		info.ServiceTier = NormalizeServiceTier(info.RequestedServiceTier())
		return
	}
	info.ServiceTier = ResolveAllowedServiceTier(info.RequestedServiceTier(), info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
}

func IsDirectCodexResponsesUpstream(info *RelayInfo) bool {
	if info == nil {
		return false
	}
	baseURL := strings.ToLower(strings.TrimSpace(info.ChannelBaseUrl))
	return strings.Contains(baseURL, "/backend-api/codex")
}

func ApplyResponsesUpstreamRequirements(info *RelayInfo, payload []byte, isCompact bool) ([]byte, error) {
	if !IsDirectCodexResponsesUpstream(info) || len(payload) == 0 {
		return payload, nil
	}

	var data map[string]interface{}
	if err := appcommon.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = make(map[string]interface{})
	}

	modified := false
	if instructions, ok := data["instructions"].(string); !ok || strings.TrimSpace(instructions) == "" {
		data["instructions"] = directCodexResponsesDefaultInstructions
		modified = true
	}

	if isCompact {
		compact := make(map[string]interface{}, 4)
		for _, key := range []string{"model", "input", "instructions", "previous_response_id"} {
			if value, ok := data[key]; ok {
				compact[key] = value
			}
		}
		if len(compact) != len(data) {
			modified = true
		}
		for key := range data {
			delete(data, key)
		}
		for key, value := range compact {
			data[key] = value
		}
	} else {
		if stream, ok := data["stream"].(bool); !ok || !stream {
			data["stream"] = true
			modified = true
		}
		if store, ok := data["store"].(bool); !ok || store {
			data["store"] = false
			modified = true
		}
		if parallelToolCalls, ok := data["parallel_tool_calls"].(bool); !ok || !parallelToolCalls {
			data["parallel_tool_calls"] = true
			modified = true
		}
		if ensureDirectCodexResponsesInclude(data) {
			modified = true
		}
		if reasoning, ok := data["reasoning"].(map[string]interface{}); ok {
			if effort, ok := reasoning["effort"].(string); ok && strings.EqualFold(strings.TrimSpace(effort), "minimal") {
				reasoning["effort"] = "none"
				modified = true
			}
		}
		if serviceTier, ok := data["service_tier"].(string); ok {
			normalized := NormalizeServiceTier(serviceTier)
			switch normalized {
			case "":
				delete(data, "service_tier")
				modified = true
			case ServiceTierPriority:
				if serviceTier != ServiceTierPriority {
					data["service_tier"] = ServiceTierPriority
					modified = true
				}
			default:
				delete(data, "service_tier")
				modified = true
			}
		}
		for _, key := range []string{
			"max_output_tokens",
			"max_completion_tokens",
			"temperature",
			"top_p",
			"frequency_penalty",
			"presence_penalty",
			"truncation",
			"user",
			"context_management",
		} {
			if _, ok := data[key]; ok {
				delete(data, key)
				modified = true
			}
		}
	}

	if !modified {
		return payload, nil
	}
	return appcommon.Marshal(data)
}

func ensureDirectCodexResponsesInclude(data map[string]interface{}) bool {
	if data == nil {
		return false
	}

	const target = "reasoning.encrypted_content"
	value, exists := data["include"]
	if !exists || value == nil {
		data["include"] = []interface{}{target}
		return true
	}

	switch items := value.(type) {
	case []interface{}:
		for _, item := range items {
			if str, ok := item.(string); ok && strings.TrimSpace(str) == target {
				return false
			}
		}
		data["include"] = append(items, target)
		return true
	case []string:
		for _, item := range items {
			if strings.TrimSpace(item) == target {
				return false
			}
		}
		data["include"] = append(items, target)
		return true
	case string:
		if strings.TrimSpace(items) == target {
			return false
		}
		data["include"] = []interface{}{items, target}
		return true
	default:
		data["include"] = []interface{}{target}
		return true
	}
}

func ApplyResponsesRequestChannelPolicy(info *RelayInfo, request *dto.OpenAIResponsesRequest, payload []byte) ([]byte, error) {
	if info == nil {
		return payload, nil
	}
	if request != nil {
		info.Request = request
	}
	info.ApplyChannelServiceTierPolicy()
	if request != nil {
		request.ServiceTier = info.ServiceTier
	}

	updatePayloadServiceTier := func(payload []byte, serviceTier string) ([]byte, error) {
		if len(payload) == 0 {
			return payload, nil
		}
		var data map[string]interface{}
		if err := appcommon.Unmarshal(payload, &data); err != nil {
			return nil, err
		}
		if data == nil {
			data = make(map[string]interface{})
		}
		if serviceTier == "" {
			delete(data, "service_tier")
		} else {
			data["service_tier"] = serviceTier
		}
		return appcommon.Marshal(data)
	}

	payload, err := ApplyResponsesUpstreamRequirements(info, payload, false)
	if err != nil {
		return nil, err
	}

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || (info.ChannelMeta != nil && info.ChannelSetting.PassThroughBodyEnabled) {
		return payload, nil
	}
	payload, err = updatePayloadServiceTier(payload, info.ServiceTier)
	if err != nil {
		return nil, err
	}
	if info.ChannelMeta == nil {
		return payload, nil
	}
	return RemoveDisabledFields(payload, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
}

func (info *RelayInfo) EffectiveServiceTier() string {
	if info == nil || info.RelayMode != relayconstant.RelayModeResponses {
		return ""
	}
	return NormalizeServiceTier(info.ServiceTier)
}

func (info *RelayInfo) ServiceTierCostMultiplier() float64 {
	return ServiceTierCostMultiplier(info.EffectiveServiceTier())
}

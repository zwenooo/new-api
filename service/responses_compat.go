package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/setting/model_setting"
	"os"
	"strings"

	promptdef "codex-service-go/prompt"
	"github.com/gin-gonic/gin"
	cpatranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	cpabuiltin "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
	"github.com/tidwall/gjson"
)

const defaultRelayResponsesCompatInstructions = "You are a helpful coding assistant."

type RelayResponsesRequestNormalizationResult struct {
	Body                []byte
	Applied             bool
	ForceUpstreamStream bool
	ServiceTier         string
	ReasoningEffort     string
}

func NormalizeRelayResponsesRequestBodyByUA(body []byte, headers http.Header, isCompact bool) ([]byte, bool, error) {
	result, err := NormalizeRelayResponsesRequestByUA(body, headers, isCompact)
	if err != nil {
		return nil, false, err
	}
	return result.Body, result.ForceUpstreamStream, nil
}

func NormalizeRelayResponsesRequestByUA(body []byte, headers http.Header, isCompact bool) (RelayResponsesRequestNormalizationResult, error) {
	return normalizeRelayResponsesRequest(body, headers, isCompact)
}

func NormalizeChannelCompatResponsesRequest(body []byte, headers http.Header) (RelayResponsesRequestNormalizationResult, error) {
	return normalizeRelayResponsesRequest(body, headers, false)
}

func normalizeRelayResponsesRequest(body []byte, headers http.Header, isCompact bool) (RelayResponsesRequestNormalizationResult, error) {
	result := RelayResponsesRequestNormalizationResult{
		Body:            body,
		ServiceTier:     extractResponsesServiceTier(body),
		ReasoningEffort: extractResponsesReasoningEffort(body),
	}
	if isCompact {
		return result, nil
	}

	codexCLIUAContains, overrideInstructions, err := GetCxCompatResponsesInstructionsSettings()
	if err != nil {
		return RelayResponsesRequestNormalizationResult{}, err
	}

	userAgentLower := strings.ToLower(strings.TrimSpace(headers.Get("User-Agent")))
	if matchesUserAgentContains(userAgentLower, codexCLIUAContains) {
		return result, nil
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return result, nil
	}

	requestedServiceTier := extractResponsesServiceTier(trimmed)
	modelName := strings.TrimSpace(gjson.GetBytes(trimmed, "model").String())
	translated := cpabuiltin.Registry().TranslateRequest(
		cpatranslator.FormatOpenAIResponse,
		cpatranslator.FormatCodex,
		modelName,
		trimmed,
		true,
	)
	if requestedServiceTier == relaycommon.ServiceTierPriority {
		translated, err = setTopLevelJSONStringFieldCompat(translated, "service_tier", relaycommon.ServiceTierPriority)
		if err != nil {
			return RelayResponsesRequestNormalizationResult{}, err
		}
	}

	bodyPatch, err := readRelayResponsesCompatBodyPatch()
	if err != nil {
		return RelayResponsesRequestNormalizationResult{}, err
	}
	if len(bodyPatch) > 0 {
		translated, err = applyJSONBodyPatchCompat(translated, bodyPatch)
		if err != nil {
			return RelayResponsesRequestNormalizationResult{}, err
		}
	}
	translated, err = finalizeRelayResponsesCompatBody(translated, overrideInstructions)
	if err != nil {
		return RelayResponsesRequestNormalizationResult{}, err
	}

	result.Body = translated
	result.Applied = true
	result.ForceUpstreamStream = true
	result.ServiceTier = extractResponsesServiceTier(translated)
	result.ReasoningEffort = extractResponsesReasoningEffort(translated)
	return result, nil
}

func PredictRelayResponsesRequestBillingMeta(c *gin.Context, info *relaycommon.RelayInfo) (RelayResponsesRequestNormalizationResult, error) {
	if c == nil || info == nil {
		return RelayResponsesRequestNormalizationResult{}, nil
	}

	request, ok := info.Request.(*dto.OpenAIResponsesRequest)
	if !ok || request == nil {
		return RelayResponsesRequestNormalizationResult{}, nil
	}

	isCompact := strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact")
	var body []byte
	var err error
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || (info.ChannelMeta != nil && info.ChannelSetting.PassThroughBodyEnabled) {
		body, err = common.GetRequestBody(c)
		if err != nil {
			return RelayResponsesRequestNormalizationResult{}, err
		}
	} else {
		requestCopy, err := common.DeepCopy(request)
		if err != nil {
			return RelayResponsesRequestNormalizationResult{}, err
		}
		requestCopy.ServiceTier = info.ServiceTier
		body, err = common.Marshal(requestCopy)
		if err != nil {
			return RelayResponsesRequestNormalizationResult{}, err
		}
		if len(info.ParamOverride) > 0 {
			body, err = relaycommon.ApplyParamOverride(body, info.ParamOverride)
			if err != nil {
				return RelayResponsesRequestNormalizationResult{}, err
			}
		}
	}

	return NormalizeRelayResponsesRequestByUA(body, c.Request.Header, isCompact)
}

func matchesUserAgentContains(uaLower, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	items := strings.FieldsFunc(pattern, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if strings.Contains(uaLower, item) {
			return true
		}
	}
	return false
}

func finalizeRelayResponsesCompatBody(raw []byte, overrideInstructions bool) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unmarshal translated compat body: %w", err)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	if err := normalizeRelayResponsesCompatPayload(obj, overrideInstructions); err != nil {
		return nil, err
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized compat body: %w", err)
	}
	return out, nil
}

func normalizeRelayResponsesCompatPayload(raw map[string]any, overrideInstructions bool) error {
	if raw == nil {
		return nil
	}

	needsContinuation := needsRelayResponsesToolContinuation(raw)

	if stream, ok := raw["stream"].(bool); !ok || !stream {
		raw["stream"] = true
	}
	if store, ok := raw["store"].(bool); !ok || store {
		raw["store"] = false
	}
	if parallelToolCalls, ok := raw["parallel_tool_calls"].(bool); !ok || !parallelToolCalls {
		raw["parallel_tool_calls"] = true
	}

	ensureReasoningEncryptedContentIncludeCompat(raw)

	if input, ok := raw["input"].([]any); ok {
		raw["input"] = filterRelayResponsesCompatInput(input, needsContinuation)
	}

	if reasoning, ok := raw["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok && strings.EqualFold(strings.TrimSpace(effort), "minimal") {
			reasoning["effort"] = "none"
		}
	}

	if model, ok := raw["model"].(string); ok && !supportsRelayResponsesCompatVerbosity(model) {
		if text, ok := raw["text"].(map[string]any); ok {
			if _, exists := text["verbosity"]; exists {
				delete(text, "verbosity")
				if len(text) == 0 {
					delete(raw, "text")
				}
			}
		}
	}

	if value, ok := raw["service_tier"]; ok {
		if relaycommon.NormalizeServiceTier(common.Interface2String(value)) != relaycommon.ServiceTierPriority {
			delete(raw, "service_tier")
		} else {
			raw["service_tier"] = relaycommon.ServiceTierPriority
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
		"prompt_cache_retention",
		"safety_identifier",
	} {
		delete(raw, key)
	}
	if !needsContinuation {
		delete(raw, "previous_response_id")
	}

	instructions, _ := readTopLevelJSONStringFromMap(raw, "instructions")
	desiredInstructions, err := resolveResponsesCompatInstructionsText(instructions, overrideInstructions)
	if err != nil {
		return err
	}
	raw["instructions"] = desiredInstructions
	return nil
}

func resolveResponsesCompatInstructionsText(existingInstructions string, overrideInstructions bool) (string, error) {
	if overrideInstructions {
		instructions := strings.TrimSpace(os.Getenv("ONEAPI_CODEX_PROMPT_CHAT_COMPLETIONS_INSTRUCTIONS"))
		if instructions == "" {
			instructions = promptdef.GPT5Codex
		}
		instructions = strings.TrimSpace(instructions)
		if instructions == "" {
			return "", fmt.Errorf("codex instructions not configured")
		}
		return instructions, nil
	}

	existingInstructions = strings.TrimSpace(existingInstructions)
	if existingInstructions != "" {
		return existingInstructions, nil
	}
	return defaultRelayResponsesCompatInstructions, nil
}

func readTopLevelJSONStringFromMap(raw map[string]any, key string) (string, bool) {
	if raw == nil {
		return "", false
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return "", false
	}
	str, ok := value.(string)
	if !ok {
		return "", false
	}
	return str, true
}

func readTopLevelJSONStringField(body []byte, key string) (string, bool) {
	value := gjson.GetBytes(body, key)
	if !value.Exists() || value.Type != gjson.String {
		return "", false
	}
	return value.String(), true
}

func setTopLevelJSONStringFieldCompat(body []byte, key, value string) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("unmarshal top-level JSON string field %s: %w", key, err)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj[key] = value
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal top-level JSON string field %s: %w", key, err)
	}
	return out, nil
}

func readRelayResponsesCompatBodyPatch() (map[string]any, error) {
	raw := strings.TrimSpace(readOption(cxCompatResponsesBodyPatchJSONOpt))
	if raw == "" {
		return nil, nil
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		return nil, fmt.Errorf("%s parse failed: %w", cxCompatResponsesBodyPatchJSONOpt, err)
	}
	if patch == nil {
		patch = map[string]any{}
	}
	return patch, nil
}

func applyJSONBodyPatchCompat(raw []byte, patch map[string]any) ([]byte, error) {
	if len(patch) == 0 {
		return raw, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unmarshal body for patch: %w", err)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	mergeJSONObjectCompat(obj, patch)
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal patched body: %w", err)
	}
	return out, nil
}

func mergeJSONObjectCompat(dst map[string]any, patch map[string]any) {
	for key, value := range patch {
		patchObj, patchIsObj := value.(map[string]any)
		if !patchIsObj {
			dst[key] = value
			continue
		}
		existingObj, existingIsObj := dst[key].(map[string]any)
		if !existingIsObj {
			dst[key] = patchObj
			continue
		}
		mergeJSONObjectCompat(existingObj, patchObj)
		dst[key] = existingObj
	}
}

func extractResponsesServiceTier(body []byte) string {
	return relaycommon.NormalizeServiceTier(gjson.GetBytes(body, "service_tier").String())
}

func extractResponsesReasoningEffort(body []byte) string {
	if effort := normalizeResponsesReasoningEffort(gjson.GetBytes(body, "reasoning.effort").String()); effort != "" {
		return effort
	}
	return parseResponsesReasoningEffortFromModel(gjson.GetBytes(body, "model").String())
}

func normalizeResponsesReasoningEffort(effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "none" {
		return ""
	}
	return effort
}

func parseResponsesReasoningEffortFromModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	for _, suffix := range []string{"-xhigh", "-high", "-minimal", "-low", "-medium"} {
		if strings.HasSuffix(model, suffix) {
			return strings.TrimPrefix(suffix, "-")
		}
	}
	return ""
}

func ensureReasoningEncryptedContentIncludeCompat(raw map[string]any) {
	if raw == nil {
		return
	}

	const target = "reasoning.encrypted_content"
	value, exists := raw["include"]
	if !exists || value == nil {
		raw["include"] = []any{target}
		return
	}

	switch items := value.(type) {
	case []any:
		for _, item := range items {
			if str, ok := item.(string); ok && strings.TrimSpace(str) == target {
				return
			}
		}
		raw["include"] = append(items, target)
	case []string:
		for _, item := range items {
			if strings.TrimSpace(item) == target {
				return
			}
		}
		raw["include"] = append(items, target)
	case string:
		if strings.TrimSpace(items) == target {
			return
		}
		raw["include"] = []any{items, target}
	default:
		raw["include"] = []any{target}
	}
}

func supportsRelayResponsesCompatVerbosity(model string) bool {
	if !strings.HasPrefix(model, "gpt-") {
		return true
	}

	var major, minor int
	n, _ := fmt.Sscanf(model, "gpt-%d.%d", &major, &minor)
	if major > 5 {
		return true
	}
	if major < 5 {
		return false
	}
	if n == 1 {
		return true
	}
	return minor >= 3
}

func needsRelayResponsesToolContinuation(raw map[string]any) bool {
	if raw == nil {
		return false
	}
	if hasNonEmptyStringCompat(raw["previous_response_id"]) {
		return true
	}
	if hasToolsSignalCompat(raw) || hasToolChoiceSignalCompat(raw) {
		return true
	}
	input, ok := raw["input"].([]any)
	if !ok {
		return false
	}
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		if itemType == "function_call_output" || itemType == "item_reference" {
			return true
		}
	}
	return false
}

func hasNonEmptyStringCompat(value any) bool {
	str, ok := value.(string)
	return ok && strings.TrimSpace(str) != ""
}

func hasToolsSignalCompat(raw map[string]any) bool {
	tools, ok := raw["tools"].([]any)
	return ok && len(tools) > 0
}

func hasToolChoiceSignalCompat(raw map[string]any) bool {
	if raw == nil {
		return false
	}
	value, ok := raw["tool_choice"]
	if !ok || value == nil {
		return false
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str) != ""
	}
	if obj, ok := value.(map[string]any); ok {
		return len(obj) > 0
	}
	return false
}

func filterRelayResponsesCompatInput(input []any, preserveReferences bool) []any {
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		msg, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}

		itemType, _ := msg["type"].(string)
		fixCallIDPrefix := func(id string) string {
			if id == "" || strings.HasPrefix(id, "fc") {
				return id
			}
			if strings.HasPrefix(id, "call_") {
				return "fc" + strings.TrimPrefix(id, "call_")
			}
			return "fc_" + id
		}

		if itemType == "item_reference" {
			if !preserveReferences {
				continue
			}
			next := make(map[string]any, len(msg))
			for key, value := range msg {
				next[key] = value
			}
			if id, ok := next["id"].(string); ok && strings.HasPrefix(id, "call_") {
				next["id"] = fixCallIDPrefix(id)
			}
			filtered = append(filtered, next)
			continue
		}

		next := msg
		copied := false
		ensureCopy := func() {
			if copied {
				return
			}
			next = make(map[string]any, len(msg))
			for key, value := range msg {
				next[key] = value
			}
			copied = true
		}

		if isRelayResponsesCompatToolCallItemType(itemType) {
			callID, ok := msg["call_id"].(string)
			if !ok || strings.TrimSpace(callID) == "" {
				if id, ok := msg["id"].(string); ok && strings.TrimSpace(id) != "" {
					callID = id
					ensureCopy()
					next["call_id"] = callID
				}
			}
			if callID != "" {
				fixedCallID := fixCallIDPrefix(callID)
				if fixedCallID != callID {
					ensureCopy()
					next["call_id"] = fixedCallID
				}
			}
		}

		if !preserveReferences {
			ensureCopy()
			delete(next, "id")
			if !isRelayResponsesCompatToolCallItemType(itemType) {
				delete(next, "call_id")
			}
		}

		filtered = append(filtered, next)
	}
	return filtered
}

func isRelayResponsesCompatToolCallItemType(itemType string) bool {
	if itemType == "" {
		return false
	}
	return strings.HasSuffix(itemType, "_call") || strings.HasSuffix(itemType, "_call_output")
}

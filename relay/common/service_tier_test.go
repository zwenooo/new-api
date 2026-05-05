package common

import (
	"encoding/json"
	"net/http/httptest"
	"one-api/dto"
	relayconstant "one-api/relay/constant"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeServiceTier(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "fast maps to priority", raw: "fast", want: ServiceTierPriority},
		{name: "priority stays priority", raw: " priority ", want: ServiceTierPriority},
		{name: "flex stays flex", raw: "FLEX", want: ServiceTierFlex},
		{name: "unknown cleared", raw: "default", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeServiceTier(tc.raw); got != tc.want {
				t.Fatalf("NormalizeServiceTier(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestApplyServiceTierPricing(t *testing.T) {
	price, ratio, multiplier := ApplyServiceTierPricing(false, 0, 1.25, "fast")
	if price != 0 {
		t.Fatalf("unexpected price for ratio billing: got %v", price)
	}
	if ratio != 2.5 {
		t.Fatalf("unexpected ratio for priority tier: got %v want 2.5", ratio)
	}
	if multiplier != 2 {
		t.Fatalf("unexpected multiplier: got %v want 2", multiplier)
	}

	price, ratio, multiplier = ApplyServiceTierPricing(true, 3, 0, "flex")
	if price != 1.5 {
		t.Fatalf("unexpected price for flex tier: got %v want 1.5", price)
	}
	if ratio != 0 {
		t.Fatalf("unexpected ratio for price billing: got %v", ratio)
	}
	if multiplier != 0.5 {
		t.Fatalf("unexpected multiplier: got %v want 0.5", multiplier)
	}
}

func TestGenRelayInfoResponsesNormalizesServiceTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	req := &dto.OpenAIResponsesRequest{
		Model:       "gpt-5.4",
		ServiceTier: "FAST",
	}

	info := GenRelayInfoResponses(ctx, req, nil)
	if info.ServiceTier != ServiceTierPriority {
		t.Fatalf("info.ServiceTier = %q, want %q", info.ServiceTier, ServiceTierPriority)
	}
	if info.EffectiveServiceTier() != ServiceTierPriority {
		t.Fatalf("info.EffectiveServiceTier() = %q, want %q", info.EffectiveServiceTier(), ServiceTierPriority)
	}
	if info.ServiceTierCostMultiplier() != 2 {
		t.Fatalf("info.ServiceTierCostMultiplier() = %v, want 2", info.ServiceTierCostMultiplier())
	}

	info.RelayMode = relayconstant.RelayModeChatCompletions
	if info.EffectiveServiceTier() != "" {
		t.Fatalf("non-responses relay mode should ignore service tier, got %q", info.EffectiveServiceTier())
	}
}

func TestResolveAllowedServiceTier(t *testing.T) {
	settings := dto.ChannelOtherSettings{}
	if got := ResolveAllowedServiceTier("fast", settings, false); got != "" {
		t.Fatalf("ResolveAllowedServiceTier should block service tier when disallowed, got %q", got)
	}

	settings.AllowServiceTier = true
	if got := ResolveAllowedServiceTier("fast", settings, false); got != ServiceTierPriority {
		t.Fatalf("ResolveAllowedServiceTier should keep allowed tier, got %q", got)
	}
}

func TestAdjustCacheCreationRatiosForDedicatedPriorityPricing(t *testing.T) {
	cacheCreationRatio, cacheCreationRatio5m, cacheCreationRatio1h := AdjustCacheCreationRatiosForServiceTier("gpt-5.4", "priority", 1, 1, 1)
	if cacheCreationRatio != 0.5 || cacheCreationRatio5m != 0.5 || cacheCreationRatio1h != 0.5 {
		t.Fatalf("priority dedicated pricing should halve cache creation ratios, got %v %v %v", cacheCreationRatio, cacheCreationRatio5m, cacheCreationRatio1h)
	}
}

func TestLongContextInputOutputMultipliers(t *testing.T) {
	inputMultiplier, outputMultiplier := LongContextInputOutputMultipliers("gpt-5.4", 272001, 0)
	if inputMultiplier != 2 || outputMultiplier != 1.5 {
		t.Fatalf("LongContextInputOutputMultipliers() = %v,%v want 2,1.5", inputMultiplier, outputMultiplier)
	}
}

func TestApplyResponsesRequestChannelPolicy(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model:       "gpt-5.4",
		ServiceTier: "fast",
	}
	info := &RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		Request:   request,
		ChannelMeta: &ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{},
		},
	}
	payload := []byte(`{"type":"response.create","model":"gpt-5.4","service_tier":"fast","input":"hello"}`)

	updatedPayload, err := ApplyResponsesRequestChannelPolicy(info, request, payload)
	if err != nil {
		t.Fatalf("ApplyResponsesRequestChannelPolicy() error = %v", err)
	}
	if info.ServiceTier != "" || request.ServiceTier != "" {
		t.Fatalf("disallowed service tier should be cleared, got info=%q request=%q", info.ServiceTier, request.ServiceTier)
	}
	var updated map[string]any
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, exists := updated["service_tier"]; exists {
		t.Fatalf("service_tier should be removed when disallowed, got %v", updated["service_tier"])
	}

	info.ChannelOtherSettings.AllowServiceTier = true
	request.ServiceTier = "fast"
	payload = []byte(`{"type":"response.create","model":"gpt-5.4","service_tier":"fast","input":"hello"}`)
	updatedPayload, err = ApplyResponsesRequestChannelPolicy(info, request, payload)
	if err != nil {
		t.Fatalf("ApplyResponsesRequestChannelPolicy() allow case error = %v", err)
	}
	if info.ServiceTier != ServiceTierPriority || request.ServiceTier != ServiceTierPriority {
		t.Fatalf("allowed service tier should normalize to priority, got info=%q request=%q", info.ServiceTier, request.ServiceTier)
	}
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("json.Unmarshal() allow case error = %v", err)
	}
	if got := updated["service_tier"]; got != ServiceTierPriority {
		t.Fatalf("service_tier should be normalized to priority, got %v", got)
	}
}

func TestApplyResponsesUpstreamRequirements_DirectCodexAddsRequiredFields(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelBaseUrl: "https://chatgpt.com/backend-api/codex/responses",
		},
	}
	payload := []byte(`{"model":"gpt-5-codex","input":"hello","stream":false,"store":true,"parallel_tool_calls":false,"include":["foo"],"max_output_tokens":256,"max_completion_tokens":128,"temperature":0.2,"top_p":0.5,"frequency_penalty":0.1,"presence_penalty":0.2,"truncation":"auto","user":"u1","context_management":{"mode":"auto"},"reasoning":{"effort":"minimal"},"service_tier":"fast"}`)

	updatedPayload, err := ApplyResponsesUpstreamRequirements(info, payload, false)
	if err != nil {
		t.Fatalf("ApplyResponsesUpstreamRequirements() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := updated["instructions"]; got != directCodexResponsesDefaultInstructions {
		t.Fatalf("instructions = %v, want %q", got, directCodexResponsesDefaultInstructions)
	}
	if got, ok := updated["stream"].(bool); !ok || !got {
		t.Fatalf("stream = %#v, want true", updated["stream"])
	}
	if got, ok := updated["store"].(bool); !ok || got {
		t.Fatalf("store = %#v, want false", updated["store"])
	}
	if got, ok := updated["parallel_tool_calls"].(bool); !ok || !got {
		t.Fatalf("parallel_tool_calls = %#v, want true", updated["parallel_tool_calls"])
	}
	include, ok := updated["include"].([]any)
	if !ok || len(include) != 2 || include[0] != "foo" || include[1] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want [foo reasoning.encrypted_content]", updated["include"])
	}
	reasoning, ok := updated["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "none" {
		t.Fatalf("reasoning = %#v, want effort=none", updated["reasoning"])
	}
	if got := updated["service_tier"]; got != ServiceTierPriority {
		t.Fatalf("service_tier = %#v, want %q", got, ServiceTierPriority)
	}
	if _, exists := updated["max_output_tokens"]; exists {
		t.Fatalf("max_output_tokens should be removed, got %v", updated["max_output_tokens"])
	}
	if _, exists := updated["max_completion_tokens"]; exists {
		t.Fatalf("max_completion_tokens should be removed, got %v", updated["max_completion_tokens"])
	}
	if _, exists := updated["temperature"]; exists {
		t.Fatalf("temperature should be removed, got %v", updated["temperature"])
	}
	if _, exists := updated["top_p"]; exists {
		t.Fatalf("top_p should be removed, got %v", updated["top_p"])
	}
	if _, exists := updated["frequency_penalty"]; exists {
		t.Fatalf("frequency_penalty should be removed, got %v", updated["frequency_penalty"])
	}
	if _, exists := updated["presence_penalty"]; exists {
		t.Fatalf("presence_penalty should be removed, got %v", updated["presence_penalty"])
	}
	if _, exists := updated["truncation"]; exists {
		t.Fatalf("truncation should be removed, got %v", updated["truncation"])
	}
	if _, exists := updated["user"]; exists {
		t.Fatalf("user should be removed, got %v", updated["user"])
	}
	if _, exists := updated["context_management"]; exists {
		t.Fatalf("context_management should be removed, got %v", updated["context_management"])
	}
}

func TestApplyResponsesUpstreamRequirements_DirectCodexCompactOnlyBackfillsInstructions(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelBaseUrl: "https://chatgpt.com/backend-api/codex/responses/compact",
		},
	}
	payload := []byte(`{"model":"gpt-5-codex","input":"hello","stream":true,"store":true,"max_output_tokens":256,"prompt_cache_key":"pcache_123","temperature":0.2,"parallel_tool_calls":true,"service_tier":"priority"}`)

	updatedPayload, err := ApplyResponsesUpstreamRequirements(info, payload, true)
	if err != nil {
		t.Fatalf("ApplyResponsesUpstreamRequirements() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := updated["instructions"]; got != directCodexResponsesDefaultInstructions {
		t.Fatalf("instructions = %v, want %q", got, directCodexResponsesDefaultInstructions)
	}
	if got := updated["model"]; got != "gpt-5-codex" {
		t.Fatalf("compact request should preserve model, got %#v", updated["model"])
	}
	if got := updated["input"]; got != "hello" {
		t.Fatalf("compact request should preserve input, got %#v", updated["input"])
	}
	if len(updated) != 3 {
		t.Fatalf("compact request should keep only model/input/instructions when previous_response_id is absent, got %#v", updated)
	}
	for _, key := range []string{
		"stream",
		"store",
		"max_output_tokens",
		"prompt_cache_key",
		"temperature",
		"parallel_tool_calls",
		"service_tier",
	} {
		if _, exists := updated[key]; exists {
			t.Fatalf("compact request should drop %s, got %#v", key, updated[key])
		}
	}
}

func TestApplyResponsesUpstreamRequirements_DirectCodexCompactPreservesPreviousResponseID(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelBaseUrl: "https://chatgpt.com/backend-api/codex/responses/compact",
		},
	}
	payload := []byte(`{"model":"gpt-5-codex","input":"hello","instructions":"keep","previous_response_id":"resp_123","store":true}`)

	updatedPayload, err := ApplyResponsesUpstreamRequirements(info, payload, true)
	if err != nil {
		t.Fatalf("ApplyResponsesUpstreamRequirements() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := updated["previous_response_id"]; got != "resp_123" {
		t.Fatalf("previous_response_id = %#v, want resp_123", got)
	}
	if len(updated) != 4 {
		t.Fatalf("compact request should keep only model/input/instructions/previous_response_id, got %#v", updated)
	}
}

func TestApplyResponsesUpstreamRequirements_DirectCodexDropsNonPriorityServiceTier(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelBaseUrl: "https://chatgpt.com/backend-api/codex/responses",
		},
	}
	payload := []byte(`{"model":"gpt-5-codex","input":"hello","service_tier":"flex"}`)

	updatedPayload, err := ApplyResponsesUpstreamRequirements(info, payload, false)
	if err != nil {
		t.Fatalf("ApplyResponsesUpstreamRequirements() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedPayload, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, exists := updated["service_tier"]; exists {
		t.Fatalf("service_tier should be removed for non-priority direct codex requests, got %#v", updated["service_tier"])
	}
}

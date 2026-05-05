package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestNormalizeRelayResponsesRequestBodyByUA_NormalizesMatchingUA(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "codex_cli_rs/",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"model":"gpt-5.2-codex","input":"hello","max_output_tokens":512,"max_completion_tokens":256,"temperature":0.7,"top_p":0.8,"frequency_penalty":0.5,"presence_penalty":0.2,"truncation":"auto","user":"u1","context_management":{"mode":"auto"},"service_tier":"flex","previous_response_id":"resp_123","prompt_cache_retention":{"ttl":"5m"},"safety_identifier":"sid_123","reasoning":{"effort":"minimal"},"text":{"verbosity":"high"}}`)
	headers := http.Header{"User-Agent": []string{"OpenCode/1.0.0"}}

	got, forceStream, err := NormalizeRelayResponsesRequestBodyByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestBodyByUA error: %v", err)
	}
	if !forceStream {
		t.Fatalf("forceStream = false, want true")
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if stream, _ := payload["stream"].(bool); !stream {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
	if store, ok := payload["store"].(bool); !ok || store {
		t.Fatalf("store = %#v, want false", payload["store"])
	}
	if parallel, ok := payload["parallel_tool_calls"].(bool); !ok || !parallel {
		t.Fatalf("parallel_tool_calls = %#v, want true", payload["parallel_tool_calls"])
	}
	if gotInstructions, _ := payload["instructions"].(string); gotInstructions == "" {
		t.Fatalf("instructions should be populated, got empty string")
	}
	if _, exists := payload["max_output_tokens"]; exists {
		t.Fatalf("max_output_tokens should be removed: %#v", payload["max_output_tokens"])
	}
	if _, exists := payload["max_completion_tokens"]; exists {
		t.Fatalf("max_completion_tokens should be removed: %#v", payload["max_completion_tokens"])
	}
	if _, exists := payload["temperature"]; exists {
		t.Fatalf("temperature should be removed: %#v", payload["temperature"])
	}
	if _, exists := payload["top_p"]; exists {
		t.Fatalf("top_p should be removed: %#v", payload["top_p"])
	}
	if _, exists := payload["frequency_penalty"]; exists {
		t.Fatalf("frequency_penalty should be removed: %#v", payload["frequency_penalty"])
	}
	if _, exists := payload["presence_penalty"]; exists {
		t.Fatalf("presence_penalty should be removed: %#v", payload["presence_penalty"])
	}
	if _, exists := payload["truncation"]; exists {
		t.Fatalf("truncation should be removed: %#v", payload["truncation"])
	}
	if _, exists := payload["user"]; exists {
		t.Fatalf("user should be removed: %#v", payload["user"])
	}
	if _, exists := payload["context_management"]; exists {
		t.Fatalf("context_management should be removed: %#v", payload["context_management"])
	}
	if _, exists := payload["service_tier"]; exists {
		t.Fatalf("service_tier should be removed when not priority: %#v", payload["service_tier"])
	}
	if got := payload["previous_response_id"]; got != "resp_123" {
		t.Fatalf("previous_response_id = %#v, want resp_123", payload["previous_response_id"])
	}
	if _, exists := payload["prompt_cache_retention"]; exists {
		t.Fatalf("prompt_cache_retention should be removed: %#v", payload["prompt_cache_retention"])
	}
	if _, exists := payload["safety_identifier"]; exists {
		t.Fatalf("safety_identifier should be removed: %#v", payload["safety_identifier"])
	}
	if reasoning, ok := payload["reasoning"].(map[string]any); !ok || reasoning["effort"] != "none" {
		t.Fatalf("reasoning = %#v, want effort=none", payload["reasoning"])
	}
	if _, exists := payload["text"]; exists {
		t.Fatalf("text.verbosity should be removed for gpt-5.2-codex: %#v", payload["text"])
	}

	include, ok := payload["include"].([]any)
	if !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want [reasoning.encrypted_content]", payload["include"])
	}

	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		t.Fatalf("input = %#v, want at least one message item", payload["input"])
	}
	last, ok := input[len(input)-1].(map[string]any)
	if !ok {
		t.Fatalf("input[last] = %#v, want object", input[len(input)-1])
	}
	if last["role"] != "user" {
		t.Fatalf("input[last].role = %#v, want user", last["role"])
	}
}

func TestNormalizeRelayResponsesRequestBodyByUA_SkipsDirectPassUA(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "codex_vscode,codex_exec,Codex Desktop,codex_cli_rs",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"model":"gpt-5-codex","input":"hello"}`)
	headers := http.Header{"User-Agent": []string{"Codex Desktop/1.0.0"}}

	got, forceStream, err := NormalizeRelayResponsesRequestBodyByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestBodyByUA error: %v", err)
	}
	if forceStream {
		t.Fatalf("forceStream = true, want false")
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body changed for direct-pass UA: got %s want %s", string(got), string(body))
	}
}

func TestNormalizeRelayResponsesRequestBodyByUA_LegacyDirectPassDefaultExpandsToMoreUAs(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "codex_cli_rs/",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"model":"gpt-5-codex","input":"hello"}`)
	headers := http.Header{"User-Agent": []string{"codex_exec/1.2.3"}}

	got, forceStream, err := NormalizeRelayResponsesRequestBodyByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestBodyByUA error: %v", err)
	}
	if forceStream {
		t.Fatalf("forceStream = true, want false")
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body changed for expanded direct-pass UA: got %s want %s", string(got), string(body))
	}
}

func TestNormalizeRelayResponsesRequestBodyByUA_AppliesOverrideInstructionsAndBodyPatch(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "true",
		cxCompatResponsesBodyPatchJSONOpt:        `{"service_tier":"priority","metadata":{"from":"patch"}}`,
	})
	t.Setenv("ONEAPI_CODEX_PROMPT_CHAT_COMPLETIONS_INSTRUCTIONS", "custom codex instructions")

	body := []byte(`{"model":"gpt-5-codex","input":"hello","instructions":"client instructions"}`)
	headers := http.Header{"User-Agent": []string{"some-third-party-client/1.0"}}

	got, forceStream, err := NormalizeRelayResponsesRequestBodyByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestBodyByUA error: %v", err)
	}
	if !forceStream {
		t.Fatalf("forceStream = false, want true")
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if gotInstructions, _ := payload["instructions"].(string); gotInstructions != "custom codex instructions" {
		t.Fatalf("instructions = %q, want custom codex instructions", gotInstructions)
	}
	if tier, _ := payload["service_tier"].(string); tier != "priority" {
		t.Fatalf("service_tier = %#v, want priority", payload["service_tier"])
	}
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok || metadata["from"] != "patch" {
		t.Fatalf("metadata = %#v, want patch merge", payload["metadata"])
	}
}

func TestNormalizeRelayResponsesRequestByUA_ReturnsEffectiveBillingMeta(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"model":"gpt-5-high","input":"hello","service_tier":"flex"}`)
	headers := http.Header{"User-Agent": []string{"some-third-party-client/1.0"}}

	result, err := NormalizeRelayResponsesRequestByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestByUA error: %v", err)
	}
	if !result.Applied || !result.ForceUpstreamStream {
		t.Fatalf("normalization result = %#v, want applied=true and force stream=true", result)
	}
	if result.ServiceTier != "" {
		t.Fatalf("ServiceTier = %q, want empty after compat strips flex", result.ServiceTier)
	}
	if result.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort = %q, want high", result.ReasoningEffort)
	}
}

func TestNormalizeRelayResponsesRequestByUA_PopulatesEmptyInstructions(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"model":"gpt-5","input":"hello","instructions":""}`)
	headers := http.Header{"User-Agent": []string{"some-third-party-client/1.0"}}

	result, err := NormalizeRelayResponsesRequestByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestByUA error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got, _ := payload["instructions"].(string); got == "" {
		t.Fatalf("instructions = %q, want non-empty", got)
	}
}

func TestNormalizeRelayResponsesRequestByUA_ReplacesPatchedEmptyInstructionsAndInvalidFields(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        `{"instructions":"","stream":false,"store":true,"parallel_tool_calls":false,"temperature":0.3,"service_tier":"flex"}`,
	})

	body := []byte(`{"model":"gpt-5","input":"hello","instructions":"client instructions"}`)
	headers := http.Header{"User-Agent": []string{"some-third-party-client/1.0"}}

	result, err := NormalizeRelayResponsesRequestByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestByUA error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got, _ := payload["instructions"].(string); got != defaultRelayResponsesCompatInstructions {
		t.Fatalf("instructions = %q, want %q", got, defaultRelayResponsesCompatInstructions)
	}
	if stream, _ := payload["stream"].(bool); !stream {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
	if store, ok := payload["store"].(bool); !ok || store {
		t.Fatalf("store = %#v, want false", payload["store"])
	}
	if parallel, ok := payload["parallel_tool_calls"].(bool); !ok || !parallel {
		t.Fatalf("parallel_tool_calls = %#v, want true", payload["parallel_tool_calls"])
	}
	if _, exists := payload["temperature"]; exists {
		t.Fatalf("temperature should be removed after body patch: %#v", payload["temperature"])
	}
	if _, exists := payload["service_tier"]; exists {
		t.Fatalf("service_tier should be removed after body patch when not priority: %#v", payload["service_tier"])
	}
}

func TestNormalizeRelayResponsesRequestByUA_EmptyUAStillAppliesCompat(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"input":[{"content":[{"text":"Reply with pong only.","type":"input_text"}],"role":"user","type":"message"}],"model":"gpt-5.4","stream":true,"text":{"verbosity":"low"}}`)
	headers := http.Header{}

	result, err := NormalizeRelayResponsesRequestByUA(body, headers, false)
	if err != nil {
		t.Fatalf("NormalizeRelayResponsesRequestByUA error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true for ordinary /v1/responses compat even when mode=off")
	}
	if !result.ForceUpstreamStream {
		t.Fatalf("ForceUpstreamStream = false, want true for ordinary /v1/responses compat even when mode=off")
	}

	var payload map[string]any
	if err := json.Unmarshal(result.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got, _ := payload["instructions"].(string); got == "" {
		t.Fatalf("instructions = %q, want non-empty", got)
	}
	if stream, _ := payload["stream"].(bool); !stream {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
}

func TestNormalizeChannelCompatResponsesRequest_SkipsDirectPassUA(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "codex_cli_rs/",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"model":"gpt-5-codex","input":"hello","instructions":"","stream":false}`)
	headers := http.Header{"User-Agent": []string{"codex_cli_rs/0.1.0"}}

	result, err := NormalizeChannelCompatResponsesRequest(body, headers)
	if err != nil {
		t.Fatalf("NormalizeChannelCompatResponsesRequest error: %v", err)
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false for direct-pass UA")
	}
	if result.ForceUpstreamStream {
		t.Fatalf("ForceUpstreamStream = true, want false for direct-pass UA")
	}
	if !bytes.Equal(result.Body, body) {
		t.Fatalf("body changed for direct-pass UA: got %s want %s", string(result.Body), string(body))
	}
}

func TestNormalizeChannelCompatResponsesRequest_EmptyUAStillAppliesCompat(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	body := []byte(`{"input":[{"content":[{"text":"Reply with pong only.","type":"input_text"}],"role":"user","type":"message"}],"model":"gpt-5.4","stream":true,"text":{"verbosity":"low"}}`)
	headers := http.Header{}

	result, err := NormalizeChannelCompatResponsesRequest(body, headers)
	if err != nil {
		t.Fatalf("NormalizeChannelCompatResponsesRequest error: %v", err)
	}
	if !result.Applied || !result.ForceUpstreamStream {
		t.Fatalf("normalization result = %#v, want applied=true and force stream=true", result)
	}

	var payload map[string]any
	if err := json.Unmarshal(result.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got, _ := payload["instructions"].(string); got == "" {
		t.Fatalf("instructions = %q, want non-empty", got)
	}
	if stream, _ := payload["stream"].(bool); !stream {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
}

func TestPredictRelayResponsesRequestBillingMeta_UsesParamOverride(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"hello"}`))
	c.Request.Header.Set("User-Agent", "third-party-client/1.0")

	info := &relaycommon.RelayInfo{
		Request: &dto.OpenAIResponsesRequest{
			Model:     "gpt-5.4",
			Input:     json.RawMessage(`"hello"`),
			Reasoning: &dto.Reasoning{Effort: "medium"},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]interface{}{
				"service_tier": "priority",
			},
		},
	}

	result, err := PredictRelayResponsesRequestBillingMeta(c, info)
	if err != nil {
		t.Fatalf("PredictRelayResponsesRequestBillingMeta error: %v", err)
	}
	if !result.Applied || !result.ForceUpstreamStream {
		t.Fatalf("normalization result = %#v, want applied=true and force stream=true", result)
	}
	if result.ServiceTier != relaycommon.ServiceTierPriority {
		t.Fatalf("ServiceTier = %q, want %q", result.ServiceTier, relaycommon.ServiceTierPriority)
	}
	if result.ReasoningEffort != "medium" {
		t.Fatalf("ReasoningEffort = %q, want medium", result.ReasoningEffort)
	}
}

func TestPredictRelayResponsesRequestBillingMeta_EmptyUAStillUsesCompat(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"hello"}`))

	info := &relaycommon.RelayInfo{
		Request: &dto.OpenAIResponsesRequest{
			Model: "gpt-5.4",
			Input: json.RawMessage(`"hello"`),
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	result, err := PredictRelayResponsesRequestBillingMeta(c, info)
	if err != nil {
		t.Fatalf("PredictRelayResponsesRequestBillingMeta error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true for ordinary /v1/responses compat even when mode=off")
	}
	if !result.ForceUpstreamStream {
		t.Fatalf("ForceUpstreamStream = false, want true for ordinary /v1/responses compat even when mode=off")
	}

	var payload map[string]any
	if err := json.Unmarshal(result.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got, _ := payload["instructions"].(string); got == "" {
		t.Fatalf("instructions = %q, want non-empty", got)
	}
	if stream, _ := payload["stream"].(bool); !stream {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
}

func TestPredictRelayResponsesRequestBillingMeta_AppliesCompat(t *testing.T) {
	restoreResponsesCompatOptions(t, map[string]string{
		cxCompatResponsesCodexCLIUAContainsOpt:   "",
		cxCompatResponsesOverrideInstructionsOpt: "false",
		cxCompatResponsesBodyPatchJSONOpt:        "",
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"hello","service_tier":"flex"}`))

	info := &relaycommon.RelayInfo{
		Request: &dto.OpenAIResponsesRequest{
			Model: "gpt-5.4",
			Input: json.RawMessage(`"hello"`),
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	result, err := PredictRelayResponsesRequestBillingMeta(c, info)
	if err != nil {
		t.Fatalf("PredictRelayResponsesRequestBillingMeta error: %v", err)
	}
	if !result.Applied || !result.ForceUpstreamStream {
		t.Fatalf("normalization result = %#v, want applied=true and force stream=true", result)
	}

	var payload map[string]any
	if err := json.Unmarshal(result.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if got, _ := payload["instructions"].(string); got == "" {
		t.Fatalf("instructions = %q, want non-empty", got)
	}
	if _, exists := payload["service_tier"]; exists {
		t.Fatalf("service_tier should be removed when not priority: %#v", payload["service_tier"])
	}
}

func TestNormalizeRelayResponsesCompatPayload_PreservesPreviousResponseIDForContinuation(t *testing.T) {
	raw := map[string]any{
		"model":                "gpt-5",
		"previous_response_id": "resp_123",
		"input": []any{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "tool_1",
				"output":  "done",
			},
		},
	}

	if err := normalizeRelayResponsesCompatPayload(raw, false); err != nil {
		t.Fatalf("normalizeRelayResponsesCompatPayload error: %v", err)
	}

	if got := raw["previous_response_id"]; got != "resp_123" {
		t.Fatalf("previous_response_id = %#v, want resp_123", got)
	}

	input, ok := raw["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v, want single continuation item", raw["input"])
	}
	item, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("input[0] = %#v, want object", input[0])
	}
	if got := item["call_id"]; got != "fc_tool_1" {
		t.Fatalf("call_id = %#v, want fc_tool_1", got)
	}
}

func restoreResponsesCompatOptions(t *testing.T, options map[string]string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	backup := cloneResponsesCompatOptionMap(common.OptionMap)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for _, key := range []string{
		cxCompatResponsesCodexCLIUAContainsOpt,
		cxCompatResponsesOverrideInstructionsOpt,
		cxCompatResponsesBodyPatchJSONOpt,
	} {
		delete(common.OptionMap, key)
	}
	for key, value := range options {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = backup
		common.OptionMapRWMutex.Unlock()
	})
}

func cloneResponsesCompatOptionMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

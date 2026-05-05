package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestIsCodexOfficialClientByHeaders_MatchesKnownFamilies(t *testing.T) {
	if !isCodexOfficialClientByHeaders("codex_vscode/1.2.3", "") {
		t.Fatalf("codex_vscode user-agent should be treated as official client")
	}
	if !isCodexOfficialClientByHeaders("", "codex_exec") {
		t.Fatalf("codex_exec originator should be treated as official client")
	}
	if isCodexOfficialClientByHeaders("openai-node/4.0.0", "opencode") {
		t.Fatalf("generic SDK headers must not be treated as official codex clients")
	}
}

func TestApplyOverrides_ChatGPTResponses_ShapesHeadersLikeCodexGateway(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept-Language", "zh-CN")
	headers.Set("Accept", "application/json")
	headers.Set("Accept-Encoding", "gzip")
	headers.Set("User-Agent", "codex_vscode/1.2.3")
	headers.Set("X-Forwarded-For", "203.0.113.1")
	headers.Set("X-Codex-Turn-State", "turn-1")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "pcache_123", requestOptions{
		Accept:           "text/event-stream",
		PrepareResponses: true,
	}, false)

	if got := out.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("expected OpenAI-Beta override, got %q", got)
	}
	if got := out.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("expected SSE accept header, got %q", got)
	}
	if got := out.Get("Originator"); got != "codex_cli_rs" {
		t.Fatalf("expected official client originator, got %q", got)
	}
	if got := out.Get("User-Agent"); got != "codex_vscode/1.2.3" {
		t.Fatalf("expected caller user-agent to be preserved, got %q", got)
	}
	if got := out.Get("Version"); got != "1.2.3" {
		t.Fatalf("expected version to follow codex client user-agent, got %q", got)
	}
	if got := out.Get("conversation_id"); got != "pcache_123" {
		t.Fatalf("expected conversation_id to follow prompt_cache_key, got %q", got)
	}
	if got := out.Get("session_id"); got != "pcache_123" {
		t.Fatalf("expected session_id to follow prompt_cache_key, got %q", got)
	}
	if got := out.Get("X-Forwarded-For"); got != "" {
		t.Fatalf("unexpected forwarded header passthrough: %q", got)
	}
	if got := out.Get("Accept-Encoding"); got != "" {
		t.Fatalf("unexpected accept-encoding passthrough: %q", got)
	}
	if got := out.Get("X-Codex-Turn-State"); got != "turn-1" {
		t.Fatalf("expected codex turn state to survive filtering, got %q", got)
	}
}

func TestApplyOverrides_ChatGPTResponses_PreservesBetaAndCodexExtras(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("OpenAI-Beta", "assistants=v2")
	headers.Set("X-Codex-Beta-Features", "feature-a")
	headers.Set("X-Responsesapi-Include-Timing-Metrics", "1")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		Accept:           "text/event-stream",
		PrepareResponses: true,
	}, false)

	if got := out.Get("OpenAI-Beta"); got != "assistants=v2,responses=experimental" {
		t.Fatalf("expected OpenAI-Beta to preserve existing entries and append responses beta, got %q", got)
	}
	if got := out.Get("X-Codex-Beta-Features"); got != "feature-a" {
		t.Fatalf("expected x-codex-beta-features to survive filtering, got %q", got)
	}
	if got := out.Get("X-Responsesapi-Include-Timing-Metrics"); got != "1" {
		t.Fatalf("expected timing metrics header to survive filtering, got %q", got)
	}
}

func TestApplyOverrides_ChatGPTResponsesWebSocket_UsesWebSocketBetaOnly(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("OpenAI-Beta", "assistants=v2,responses=experimental")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		Accept:             "text/event-stream",
		PrepareResponses:   true,
		ResponsesWebSocket: true,
	}, false)

	if got := out.Get("OpenAI-Beta"); got != "assistants=v2,"+openAIResponsesWSBetaV2 {
		t.Fatalf("expected websocket beta header to replace responses=experimental, got %q", got)
	}
	if got := out.Get("OpenAI-Beta"); strings.Contains(got, "responses=experimental") {
		t.Fatalf("unexpected HTTP responses beta on websocket path: %q", got)
	}
}

func TestApplyOverrides_ChatGPTResponses_UsesOpencodeOriginatorForNonOfficialClients(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "openai-node/4.0.0")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		Accept:           "text/event-stream",
		PrepareResponses: true,
	}, false)

	if got := out.Get("Originator"); got != "opencode" {
		t.Fatalf("expected non-official client originator to fall back to opencode, got %q", got)
	}
}

func TestApplyOverrides_ChatGPTResponses_SetsVersionAndSessionDefaults(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		Accept:           "text/event-stream",
		PrepareResponses: true,
	}, false)

	if got := out.Get("Version"); got != defaultCodexClientVersion {
		t.Fatalf("expected generic client to get default codex version, got %q", got)
	}
	if got := out.Get("session_id"); got == "" {
		t.Fatal("expected session_id to be generated when caller did not provide one")
	}
}

func TestApplyOverrides_ChatGPTResponses_ForcesCodexUserAgentForNonOfficialClients(t *testing.T) {
	svc := NewService(Options{UserAgent: "codex_cli_rs/9.9.9 (linux; x86_64) codex-service-go"})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "openai-node/4.0.0")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		Accept:           "text/event-stream",
		PrepareResponses: true,
	}, false)

	if got := out.Get("User-Agent"); got != "codex_cli_rs/9.9.9 (linux; x86_64) codex-service-go" {
		t.Fatalf("expected non-official client user-agent to be pinned to codex-style default, got %q", got)
	}
	if got := out.Get("Originator"); got != "opencode" {
		t.Fatalf("expected non-official client originator to remain opencode, got %q", got)
	}
}

func TestApplyOverrides_ChatGPTCompact_SetsVersionAndSessionFallback(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "codex_vscode/1.2.3")
	headers.Set("conversation_id", "conv-123")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		PrepareResponses: true,
		ResponsesCompact: true,
	}, false)

	if got := out.Get("Accept"); got != "application/json" {
		t.Fatalf("expected compact accept header, got %q", got)
	}
	if got := out.Get("Version"); got != "1.2.3" {
		t.Fatalf("expected compact version header, got %q", got)
	}
	if got := out.Get("session_id"); got != "conv-123" {
		t.Fatalf("expected compact session_id fallback from conversation_id, got %q", got)
	}
}

func TestApplyOverrides_ChatGPTCompact_UsesDefaultVersionForGenericClients(t *testing.T) {
	svc := NewService(Options{UserAgent: "custom-client"})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	out := svc.applyOverrides(headers, &authContext{mode: "chatgpt"}, "", requestOptions{
		PrepareResponses: true,
		ResponsesCompact: true,
	}, false)

	if got := out.Get("Version"); got != defaultCodexClientVersion {
		t.Fatalf("expected compact generic client to use default version, got %q", got)
	}
}

func TestMergeOpenAIBetaHeader_PreservesExistingFamilyValues(t *testing.T) {
	got := mergeOpenAIBetaHeader("assistants=v2,responses_websockets=2026-01-01", "responses=experimental", openAIResponsesWSBetaV2)
	if got != "assistants=v2,responses_websockets=2026-01-01,responses=experimental" {
		t.Fatalf("unexpected merged OpenAI-Beta header: %q", got)
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_OpencodeLocksFieldsLikeSub2API(t *testing.T) {
	svc := NewService(Options{})
	if err := svc.SetResponsesCompatBodyPatchJSON(`{"instructions":"patched","store":true,"debug_flag":"patched"}`); err != nil {
		t.Fatalf("set body patch: %v", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "opencode/1.0")

	body := []byte(`{"model":"gpt-5-codex","input":[{"content":"first instruction"},{"content":"second message"}],"stream":false,"store":true,"previous_response_id":"resp_123","temperature":1}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true to be enforced, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected store=false to be enforced, got %#v", parsed["store"])
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved on direct HTTP path, got %#v", parsed["previous_response_id"])
	}
	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected opencode request to use default instructions, got %#v", got)
	}
	if _, ok := parsed["temperature"]; ok {
		t.Fatalf("expected unsupported sampling params to be stripped")
	}
	if _, ok := parsed["debug_flag"]; ok {
		t.Fatalf("expected direct responses request to skip body patching")
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_GenericSDKUsesSub2APINormalization(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`{"model":"gpt-5","input":"hello","stream":false,"store":true,"previous_response_id":"resp_123","functions":[{"name":"shell","description":"run","parameters":{"type":"object"}}],"function_call":"auto","max_output_tokens":512,"reasoning":{"effort":"minimal"},"prompt_cache_retention":{"ttl":10},"safety_identifier":"sid","text":{"verbosity":"high"}}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["model"]; got != "gpt-5.1" {
		t.Fatalf("expected model to be normalized, got %#v", got)
	}
	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true to be enforced, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected store=false to be enforced, got %#v", parsed["store"])
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved on direct HTTP path, got %#v", parsed["previous_response_id"])
	}
	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected missing instructions to use default instructions, got %#v", got)
	}
	input, ok := parsed["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected string input to be converted into message array, got %#v", parsed["input"])
	}
	if _, ok := parsed["functions"]; ok {
		t.Fatalf("expected legacy functions field to be removed")
	}
	if _, ok := parsed["function_call"]; ok {
		t.Fatalf("expected legacy function_call field to be removed")
	}
	if got := parsed["tool_choice"]; got != "auto" {
		t.Fatalf("expected function_call to be rewritten into tool_choice, got %#v", got)
	}
	if _, ok := parsed["tools"]; !ok {
		t.Fatalf("expected functions to be rewritten into tools")
	}
	if _, ok := parsed["max_output_tokens"]; ok {
		t.Fatalf("expected unsupported max_output_tokens to be stripped")
	}
	reasoning, ok := parsed["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "none" {
		t.Fatalf("expected reasoning.effort minimal to normalize to none, got %#v", parsed["reasoning"])
	}
	if _, ok := parsed["prompt_cache_retention"]; ok {
		t.Fatalf("expected prompt_cache_retention to be stripped for non-official client")
	}
	if _, ok := parsed["safety_identifier"]; ok {
		t.Fatalf("expected safety_identifier to be stripped for non-official client")
	}
	text, ok := parsed["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text object to remain present")
	}
	if _, ok := text["verbosity"]; ok {
		t.Fatalf("expected unsupported text.verbosity to be stripped for low-version mapped model")
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_GenericSDKUsesDefaultInstructionsWhenInputEmpty(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`{"model":"gpt-5-codex","input":[],"stream":false,"store":true,"previous_response_id":"resp_123"}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected empty input to still get default instructions, got %#v", got)
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved on direct HTTP path, got %#v", parsed["previous_response_id"])
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_MissingContentTypeStillAddsInstructions(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`{"model":"gpt-5-codex","input":[],"stream":false,"store":true,"previous_response_id":"resp_123"}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected missing content-type JSON request to still get default instructions, got %#v", got)
	}
	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected missing content-type JSON request to still be normalized to stream=true, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected missing content-type JSON request to still be normalized to store=false, got %#v", parsed["store"])
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved on missing content-type JSON request, got %#v", parsed["previous_response_id"])
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_MissingContentTypeNonJSONPassesThrough(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`not-json`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}
	if string(out) != string(body) {
		t.Fatalf("expected non-JSON body without content-type to pass through unchanged, got %q", string(out))
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_OfficialCodexClientAlsoLocksFields(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "codex_vscode/1.2.3")

	body := []byte(`{"model":"gpt-5-codex","input":[],"stream":false,"store":true,"temperature":1,"prompt_cache_retention":{"ttl":10},"safety_identifier":"sid","previous_response_id":"resp_123"}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["model"]; got != "gpt-5.1-codex" {
		t.Fatalf("expected official codex client model to be normalized, got %#v", got)
	}
	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected official codex client stream=true, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected official codex client store=false, got %#v", parsed["store"])
	}
	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected official codex client to get default instructions when missing, got %#v", got)
	}
	if _, ok := parsed["temperature"]; ok {
		t.Fatalf("expected unsupported params to be stripped for official codex client too")
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected official codex client previous_response_id to be preserved on direct HTTP path, got %#v", parsed["previous_response_id"])
	}
	if _, ok := parsed["prompt_cache_retention"]; !ok {
		t.Fatalf("expected official codex client to preserve prompt_cache_retention like sub2api")
	}
	if _, ok := parsed["safety_identifier"]; !ok {
		t.Fatalf("expected official codex client to preserve safety_identifier like sub2api")
	}
}

func TestPrepareResponsesBody_FromChatCompat_StillLocksCodexFields(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`{"model":"gpt-5.2","instructions":"BASE\n\nSYS","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false,"store":true,"previous_response_id":"resp_123"}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true, FromChatCompat: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected chat compat request to still be forced to stream=true, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected chat compat request to still be forced to store=false, got %#v", parsed["store"])
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected chat compat request to preserve previous_response_id before codex upstream, got %#v", parsed["previous_response_id"])
	}
	if got := parsed["instructions"]; got != "BASE\n\nSYS" {
		t.Fatalf("expected chat compat instructions to be preserved, got %#v", got)
	}
}

func TestPrepareResponsesBody_FromChatCompat_ReplacesExplicitEmptyInstructionsWithDefault(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`{"model":"gpt-5.2","instructions":"","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"SYS"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false,"store":true}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true, FromChatCompat: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected explicit empty instructions to be replaced by default instructions, got %#v", got)
	}
}

func TestPrepareResponsesBody_ResponsesPassthrough_InvalidInstructionsUseDefaultInstructions(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "openclaw/1.0")

	body := []byte(`{"model":"gpt-5-codex","instructions":{"bad":true},"input":[{"content":"first instruction"},{"content":"second message"}],"stream":false,"store":true}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected invalid instructions to be replaced by default instructions, got %#v", got)
	}
}

func TestPrepareResponsesBody_ResponsesCompact_KeepsOnlyCompactFields(t *testing.T) {
	svc := NewService(Options{})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "OpenAI/JS 6.26.0")

	body := []byte(`{"model":"gpt-5","input":"hello","stream":false,"store":true,"previous_response_id":"resp_123","prompt_cache_key":"pcache_123","temperature":1}`)
	out, promptKey, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true, ResponsesCompact: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}
	if promptKey != "pcache_123" {
		t.Fatalf("expected prompt cache key to still be extracted before compact normalization, got %q", promptKey)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["model"]; got != "gpt-5.1" {
		t.Fatalf("expected compact request model to be normalized, got %#v", got)
	}
	if got := parsed["instructions"]; got != defaultCompatResponsesInstructions {
		t.Fatalf("expected compact request to get default instructions, got %#v", got)
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected compact direct HTTP request to preserve previous_response_id, got %#v", parsed["previous_response_id"])
	}
	if _, ok := parsed["stream"]; ok {
		t.Fatalf("expected compact request to drop stream")
	}
	if _, ok := parsed["store"]; ok {
		t.Fatalf("expected compact request to drop store")
	}
	if _, ok := parsed["prompt_cache_key"]; ok {
		t.Fatalf("expected compact request to drop prompt_cache_key from body")
	}
	if _, ok := parsed["temperature"]; ok {
		t.Fatalf("expected compact request to drop unsupported params")
	}
	if len(parsed) != 4 {
		t.Fatalf("expected compact body to keep only model/input/instructions/previous_response_id after HTTP normalization, got %#v", parsed)
	}
}

func TestPrepareResponsesBody_InternalCompatBridge_LocksCodexRequiredFields(t *testing.T) {
	svc := NewService(Options{})

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "undici")

	body := []byte(`{"model":"gpt-5-codex","input":[],"stream":false,"store":true,"previous_response_id":"resp_123","instructions":"keep me","max_output_tokens":512}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true to be enforced, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected store=false to be enforced, got %#v", parsed["store"])
	}
	if got, ok := parsed["parallel_tool_calls"].(bool); !ok || !got {
		t.Fatalf("expected parallel_tool_calls=true to be enforced, got %#v", parsed["parallel_tool_calls"])
	}
	include, ok := parsed["include"].([]any)
	if !ok || len(include) == 0 || include[len(include)-1] != "reasoning.encrypted_content" {
		t.Fatalf("expected include to contain reasoning.encrypted_content, got %#v", parsed["include"])
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved for internal compat passthrough, got %#v", parsed["previous_response_id"])
	}
	if got := parsed["instructions"]; got != "keep me" {
		t.Fatalf("expected existing instructions to be preserved, got %#v", got)
	}
	if _, ok := parsed["max_output_tokens"]; ok {
		t.Fatalf("expected known-bad max_output_tokens to be stripped for ChatGPT backend")
	}
}

func TestPrepareResponsesBody_InternalCompatBridge_WebSocketLocksCodexRequiredFields(t *testing.T) {
	svc := NewService(Options{})

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "undici")

	body := []byte(`{"model":"gpt-5-codex","input":[],"stream":true,"store":false,"previous_response_id":"resp_123","instructions":"keep me","max_output_tokens":512}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{
		PrepareResponses:   true,
		ResponsesWebSocket: true,
	})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := parsed["type"]; got != "response.create" {
		t.Fatalf("expected websocket payload to add response.create envelope, got %#v", got)
	}
	if got := parsed["model"]; got != "gpt-5-codex" {
		t.Fatalf("expected internal compat bridge model to survive websocket path, got %#v", got)
	}
	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true to be enforced, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected store=false to be enforced, got %#v", parsed["store"])
	}
	if got, ok := parsed["parallel_tool_calls"].(bool); !ok || !got {
		t.Fatalf("expected parallel_tool_calls=true to be enforced, got %#v", parsed["parallel_tool_calls"])
	}
	include, ok := parsed["include"].([]any)
	if !ok || len(include) == 0 || include[len(include)-1] != "reasoning.encrypted_content" {
		t.Fatalf("expected include to contain reasoning.encrypted_content, got %#v", parsed["include"])
	}
	if got := parsed["previous_response_id"]; got != "resp_123" {
		t.Fatalf("expected previous_response_id to be preserved for websocket compat passthrough, got %#v", parsed["previous_response_id"])
	}
	if got := parsed["instructions"]; got != "keep me" {
		t.Fatalf("expected existing instructions to be preserved, got %#v", got)
	}
	if _, ok := parsed["max_output_tokens"]; ok {
		t.Fatalf("expected known-bad max_output_tokens to be stripped for ChatGPT backend")
	}
}

func TestPrepareResponsesBody_InternalCompatBridge_AddsEmptyInstructionsWhenMissing(t *testing.T) {
	svc := NewService(Options{})

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "undici")

	body := []byte(`{"model":"gpt-5-codex","input":[],"stream":false,"store":true}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got, ok := parsed["instructions"].(string); !ok || got != defaultCompatResponsesInstructions {
		t.Fatalf("expected missing instructions to be normalized to default instructions, got %#v", parsed["instructions"])
	}
}

func TestPrepareResponsesBody_InternalCompatBridge_ReplacesEmptyInstructionsAfterBodyPatch(t *testing.T) {
	svc := NewService(Options{})
	if err := svc.SetResponsesCompatBodyPatchJSON(`{"instructions":"","stream":false,"store":true,"parallel_tool_calls":false}`); err != nil {
		t.Fatalf("set body patch: %v", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "undici")

	body := []byte(`{"model":"gpt-5-codex","input":[],"instructions":"","stream":true,"store":false,"parallel_tool_calls":true}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got, ok := parsed["instructions"].(string); !ok || got != defaultCompatResponsesInstructions {
		t.Fatalf("expected empty instructions to be normalized to default instructions after body patch, got %#v", parsed["instructions"])
	}
	if got, ok := parsed["stream"].(bool); !ok || !got {
		t.Fatalf("expected stream=true after body patch, got %#v", parsed["stream"])
	}
	if got, ok := parsed["store"].(bool); !ok || got {
		t.Fatalf("expected store=false after body patch, got %#v", parsed["store"])
	}
	if got, ok := parsed["parallel_tool_calls"].(bool); !ok || !got {
		t.Fatalf("expected parallel_tool_calls=true after body patch, got %#v", parsed["parallel_tool_calls"])
	}
}

func TestPrepareResponsesBody_InternalCompatBridge_NormalizesToolContinuationInput(t *testing.T) {
	svc := NewService(Options{})

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "undici")

	body := []byte(`{"model":"gpt-5-codex","tool_choice":"auto","input":[{"type":"message","id":"msg_1","role":"user","content":[{"type":"input_text","text":"hi"}]},{"type":"item_reference","id":"toolu_1"},{"type":"function_call","call_id":"toolu_1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"toolu_1","output":"done"}]}`)
	out, _, err := svc.prepareResponsesBody(body, headers, "chatgpt", requestOptions{PrepareResponses: true})
	if err != nil {
		t.Fatalf("prepareResponsesBody returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	input, ok := parsed["input"].([]any)
	if !ok || len(input) != 4 {
		t.Fatalf("expected normalized input to keep 4 items, got %#v", parsed["input"])
	}

	msg, ok := input[0].(map[string]any)
	if !ok || msg["id"] != "msg_1" {
		t.Fatalf("expected native message id to be preserved, got %#v", input[0])
	}

	ref, ok := input[1].(map[string]any)
	if !ok || ref["id"] != "fc_toolu_1" {
		t.Fatalf("expected item_reference id to be normalized, got %#v", input[1])
	}

	call, ok := input[2].(map[string]any)
	if !ok || call["call_id"] != "fc_toolu_1" {
		t.Fatalf("expected function_call call_id to be normalized, got %#v", input[2])
	}

	output, ok := input[3].(map[string]any)
	if !ok || output["call_id"] != "fc_toolu_1" {
		t.Fatalf("expected function_call_output call_id to be normalized, got %#v", input[3])
	}
}

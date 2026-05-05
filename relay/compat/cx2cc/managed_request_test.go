package cx2cc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/service"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrepareChannelMessagesToResponsesCompatRequest_SetsResponsesModeAndStripsBillingHeader(t *testing.T) {
	restoreCx2ccOptions(t, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-5-sonnet"},
	}

	req := &dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		System: []any{
			map[string]any{"type": "text", "text": "x-anthropic-billing-header: tenant-1"},
			map[string]any{"type": "text", "text": "SYS"},
		},
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Metadata: json.RawMessage(`{"session_id":"sess-123"}`),
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	if !info.DisablePing {
		t.Fatalf("expected DisablePing=true")
	}
	if info.RelayMode != relayconstant.RelayModeResponses {
		t.Fatalf("RelayMode = %d, want %d", info.RelayMode, relayconstant.RelayModeResponses)
	}
	if info.RequestURLPath != "/v1/responses" {
		t.Fatalf("RequestURLPath = %q, want %q", info.RequestURLPath, "/v1/responses")
	}
	if got := c.Request.Header.Get("session_id"); got != "sess-123" {
		t.Fatalf("session_id header = %q, want %q", got, "sess-123")
	}
	if info.PromptCacheKey != "sess-123" {
		t.Fatalf("PromptCacheKey = %q, want %q", info.PromptCacheKey, "sess-123")
	}

	var promptCacheKey string
	if err := json.Unmarshal(out.PromptCacheKey, &promptCacheKey); err != nil {
		t.Fatalf("unmarshal prompt_cache_key: %v", err)
	}
	if promptCacheKey != "sess-123" {
		t.Fatalf("prompt_cache_key = %q, want %q", promptCacheKey, "sess-123")
	}

	var instructions string
	if err := json.Unmarshal(out.Instructions, &instructions); err != nil {
		t.Fatalf("unmarshal instructions: %v", err)
	}
	if instructions != "SYS" {
		t.Fatalf("instructions = %q, want %q", instructions, "SYS")
	}
}

func TestPrepareChannelMessagesToResponsesCompatRequest_UsesChannelMappedUpstreamModel(t *testing.T) {
	restoreCx2ccOptions(t, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-7-sonnet-20250219",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.2", IsModelMapped: true},
	}

	req := &dto.ClaudeRequest{
		Model: "claude-3-7-sonnet-20250219",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	if out.Model != "gpt-5.2" {
		t.Fatalf("model = %q, want %q", out.Model, "gpt-5.2")
	}
	if info.UpstreamModelName != "gpt-5.2" {
		t.Fatalf("info.UpstreamModelName = %q, want %q", info.UpstreamModelName, "gpt-5.2")
	}
}

func TestPrepareChannelMessagesToResponsesCompatRequest_PopulatesResponsesUsageTools(t *testing.T) {
	restoreCx2ccOptions(t, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.2-codex"},
	}

	req := &dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "search golang"},
		},
		Tools: []any{
			map[string]any{
				"type": "web_search_20250305",
				"name": "web_search",
			},
		},
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	if len(out.GetToolsMap()) != 1 {
		t.Fatalf("expected converted responses request to keep one tool, got %#v", out.GetToolsMap())
	}
	webSearchTool := relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolWebSearch)
	if webSearchTool == nil {
		t.Fatalf("expected relay info to track canonical web_search tool, got %#v", info.ResponsesUsageInfo)
	}
	if webSearchTool.SearchContextSize != "medium" {
		t.Fatalf("expected default search_context_size=medium, got %#v", webSearchTool.SearchContextSize)
	}
}

func TestPrepareChannelMessagesToResponsesCompatRequest_AppliesCodexCompatNormalization(t *testing.T) {
	restoreCx2ccOptions(t, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-5-sonnet"},
	}

	req := &dto.ClaudeRequest{
		Model:       "claude-3-5-sonnet",
		ServiceTier: "fast",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	var instructions string
	if err := json.Unmarshal(out.Instructions, &instructions); err != nil {
		t.Fatalf("unmarshal instructions: %v", err)
	}
	if instructions != "You are a helpful coding assistant." {
		t.Fatalf("instructions = %q, want default non-empty instructions", instructions)
	}
	if got := string(out.ParallelToolCalls); got != "true" {
		t.Fatalf("parallel_tool_calls = %s, want true", got)
	}
	if got := string(out.Store); got != "false" {
		t.Fatalf("store = %s, want false", got)
	}
	if out.MaxOutputTokens != 0 {
		t.Fatalf("max_output_tokens = %d, want omitted/0", out.MaxOutputTokens)
	}
	if out.ServiceTier != "priority" {
		t.Fatalf("service_tier = %q, want %q", out.ServiceTier, "priority")
	}

	var include []string
	if err := json.Unmarshal(out.Include, &include); err != nil {
		t.Fatalf("unmarshal include: %v", err)
	}
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want reasoning.encrypted_content", include)
	}
}

func TestPrepareChannelMessagesToResponsesCompatRequest_StoresResponsesToolNameMapping(t *testing.T) {
	restoreCx2ccOptions(t, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.2-codex"},
	}

	originalListDir := "mcp__" + strings.Repeat("workspace_alpha__", 5) + "list_dir"
	req := &dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Tools: []map[string]any{
			{
				"name":        originalListDir,
				"description": "List directory",
			},
		},
		ToolChoice: map[string]any{
			"type": "tool",
			"name": originalListDir,
		},
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":  "tool_use",
						"id":    "toolu_1",
						"name":  originalListDir,
						"input": map[string]any{"relative_path": "internal"},
					},
				},
			},
		},
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	normalizedListDir := info.ClaudeConvertInfo.ResponsesToolNameByOriginal[originalListDir]
	if normalizedListDir == "" || normalizedListDir == originalListDir {
		t.Fatalf("expected relay info to store normalized tool name, got %#v", normalizedListDir)
	}
	if got := info.ClaudeConvertInfo.ResponsesToolNameByNormalized[normalizedListDir]; got != originalListDir {
		t.Fatalf("expected relay info reverse map to restore original tool name, got %#v", got)
	}

	tools := out.GetToolsMap()
	if len(tools) != 1 {
		t.Fatalf("expected one converted tool, got %#v", tools)
	}
	if got := tools[0]["name"]; got != normalizedListDir {
		t.Fatalf("expected upstream tool name to be normalized, got %#v", got)
	}
}

func TestPrepareChannelMessagesToResponsesCompatRequest_FinalBodyEnforcesCompatFields(t *testing.T) {
	restoreCx2ccOptions(t, map[string]string{
		"cx_compat.responses.codex_cli_rs_ua_contains": "",
		"cx_compat.responses.override_instructions":    "false",
		"cx_compat.responses.body_patch_json":          "",
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "third-party-client/1.0")

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.2-codex"},
	}

	req := &dto.ClaudeRequest{
		Model:       "claude-3-5-sonnet",
		ServiceTier: "fast",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	out.Stream = false
	out.PreviousResponseID = "resp_123"
	out.Temperature = 0.7
	out.Truncation = "auto"
	out.User = "u1"
	out.Text = json.RawMessage(`{"verbosity":"high"}`)
	out.Reasoning = &dto.Reasoning{Effort: "minimal"}

	body, err := common.Marshal(out)
	if err != nil {
		t.Fatalf("marshal responses request: %v", err)
	}

	normalized, err := service.NormalizeChannelCompatResponsesRequest(body, c.Request.Header)
	if err != nil {
		t.Fatalf("NormalizeChannelCompatResponsesRequest error: %v", err)
	}
	if !normalized.ForceUpstreamStream {
		t.Fatalf("ForceUpstreamStream = false, want true")
	}

	var payload map[string]any
	if err := json.Unmarshal(normalized.Body, &payload); err != nil {
		t.Fatalf("unmarshal normalized body: %v", err)
	}
	if instructions, _ := payload["instructions"].(string); strings.TrimSpace(instructions) == "" {
		t.Fatalf("instructions = %q, want non-empty instructions", instructions)
	}
	if stream, _ := payload["stream"].(bool); !stream {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
	if got := payload["previous_response_id"]; got != "resp_123" {
		t.Fatalf("previous_response_id = %#v, want resp_123", got)
	}
	if _, exists := payload["temperature"]; exists {
		t.Fatalf("temperature should be removed: %#v", payload["temperature"])
	}
	if _, exists := payload["truncation"]; exists {
		t.Fatalf("truncation should be removed: %#v", payload["truncation"])
	}
	if _, exists := payload["user"]; exists {
		t.Fatalf("user should be removed: %#v", payload["user"])
	}
	if _, exists := payload["text"]; exists {
		t.Fatalf("text.verbosity should be removed for gpt-5.2-codex: %#v", payload["text"])
	}
	if tier, _ := payload["service_tier"].(string); tier != "priority" {
		t.Fatalf("service_tier = %#v, want priority", payload["service_tier"])
	}
	if reasoning, ok := payload["reasoning"].(map[string]any); !ok || reasoning["effort"] != "none" {
		t.Fatalf("reasoning = %#v, want effort=none", payload["reasoning"])
	}
}

func TestPrepareChannelMessagesToResponsesCompatRequest_DerivesStableConversationIDFromSessionKey(t *testing.T) {
	restoreCx2ccOptions(t, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-5-sonnet"},
	}

	req := &dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Metadata: json.RawMessage(`{"session_key":"stable-session-key"}`),
	}

	out, err := PrepareChannelMessagesToResponsesCompatRequest(c, info, req)
	if err != nil {
		t.Fatalf("PrepareChannelMessagesToResponsesCompatRequest error: %v", err)
	}

	wantConversationID := "1e4b9d6a-cf5a-4c0e-aa13-508725d521ef"
	if info.ConversationId != wantConversationID {
		t.Fatalf("ConversationId = %q, want %q", info.ConversationId, wantConversationID)
	}
	if info.SessionId != wantConversationID {
		t.Fatalf("SessionId = %q, want %q", info.SessionId, wantConversationID)
	}
	if got := c.Request.Header.Get("session_id"); got != wantConversationID {
		t.Fatalf("session_id header = %q, want %q", got, wantConversationID)
	}

	var promptCacheKey string
	if err := json.Unmarshal(out.PromptCacheKey, &promptCacheKey); err != nil {
		t.Fatalf("unmarshal prompt_cache_key: %v", err)
	}
	if promptCacheKey != wantConversationID {
		t.Fatalf("prompt_cache_key = %q, want %q", promptCacheKey, wantConversationID)
	}
}

func restoreCx2ccOptions(t *testing.T, updates map[string]string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	backup := cloneOptionMap(common.OptionMap)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for k, v := range updates {
		common.OptionMap[k] = v
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = backup
		common.OptionMapRWMutex.Unlock()
	})
}

func cloneOptionMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

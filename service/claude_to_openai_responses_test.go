package service

import (
	"encoding/json"
	"one-api/common"
	"one-api/dto"
	"strings"
	"testing"
)

func TestClaudeToOpenAIResponsesRequest_InsertsDefaultInstructionsWhenSystemEmpty(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var instructions string
	if err := json.Unmarshal(out.Instructions, &instructions); err != nil {
		t.Fatalf("unmarshal instructions: %v", err)
	}
	if instructions != defaultRelayResponsesCompatInstructions {
		t.Fatalf("expected default instructions %q, got %q", defaultRelayResponsesCompatInstructions, instructions)
	}
}

func TestClaudeToOpenAIResponsesRequest_PreservesExplicitSystemInstructions(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model:  "claude-3-5-sonnet",
		System: "SYS",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var instructions string
	if err := json.Unmarshal(out.Instructions, &instructions); err != nil {
		t.Fatalf("unmarshal instructions: %v", err)
	}
	if instructions != "SYS" {
		t.Fatalf("expected explicit system instructions to win, got %q", instructions)
	}
}

func TestClaudeToOpenAIResponsesRequest_AppliesOverrideInstructionsWhenConfigured(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")
	restoreResponsesCompatOverrideOption(t, "true")
	t.Setenv("ONEAPI_CODEX_PROMPT_CHAT_COMPLETIONS_INSTRUCTIONS", "OVERRIDE")

	req := dto.ClaudeRequest{
		Model:  "claude-3-5-sonnet",
		System: "SYS",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var instructions string
	if err := json.Unmarshal(out.Instructions, &instructions); err != nil {
		t.Fatalf("unmarshal instructions: %v", err)
	}
	if instructions != "OVERRIDE" {
		t.Fatalf("expected override instructions to win, got %q", instructions)
	}
}

func TestClaudeToOpenAIResponsesRequest_AppliesCodexCompatFieldsAndNormalizesTools(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model:       "claude-3-5-sonnet",
		ServiceTier: "fast",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []map[string]any{
			{
				"type": "web_search_20260301",
				"name": "web_search",
			},
			{
				"name":        "Read",
				"description": "Read a file",
			},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
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

	if got := string(out.ToolChoice); got != `"auto"` {
		t.Fatalf("tool_choice = %s, want auto", got)
	}

	var tools []map[string]any
	if err := json.Unmarshal(out.Tools, &tools); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %#v", len(tools), tools)
	}

	if got := tools[0]["type"]; got != "web_search" {
		t.Fatalf("expected first tool to be web_search, got %#v", got)
	}
	if _, exists := tools[0]["name"]; exists {
		t.Fatalf("expected built-in web_search tool to drop unsupported name field, got %#v", tools[0]["name"])
	}

	if got := tools[1]["type"]; got != "function" {
		t.Fatalf("expected second tool to be function, got %#v", got)
	}
	if got := tools[1]["name"]; got != "Read" {
		t.Fatalf("expected second tool name to be Read, got %#v", got)
	}
	if got := tools[1]["description"]; got != "Read a file" {
		t.Fatalf("expected second tool description to be preserved, got %#v", got)
	}
	if got, ok := tools[1]["strict"].(bool); !ok || got {
		t.Fatalf("expected second tool strict=false, got %#v", tools[1]["strict"])
	}

	parameters, ok := tools[1]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected second tool parameters to be an object, got %#v", tools[1]["parameters"])
	}
	if got := parameters["type"]; got != "object" {
		t.Fatalf("expected parameters.type=object, got %#v", got)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok || len(properties) != 0 {
		t.Fatalf("expected parameters.properties to default to empty object, got %#v", parameters["properties"])
	}
	if got, ok := parameters["additionalProperties"].(bool); !ok || got {
		t.Fatalf("expected parameters.additionalProperties=false, got %#v", parameters["additionalProperties"])
	}
}

func TestClaudeToOpenAIResponsesRequest_NormalizesExplicitBuiltinAliasesWithoutBreakingFunctionNames(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []map[string]any{
			{
				"type": "google_search",
				"name": "google_search",
			},
			{
				"name":        "google_search",
				"description": "custom function, not built-in",
			},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var tools []map[string]any
	if err := json.Unmarshal(out.Tools, &tools); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %#v", len(tools), tools)
	}

	if got := tools[0]["type"]; got != "web_search" {
		t.Fatalf("expected google_search alias to normalize to web_search, got %#v", got)
	}
	if got := tools[1]["type"]; got != "function" {
		t.Fatalf("expected name-only function tool to stay function, got %#v", got)
	}
	if got := tools[1]["name"]; got != "google_search" {
		t.Fatalf("expected name-only function tool name to be preserved, got %#v", got)
	}
}

func TestClaudeToOpenAIResponsesRequest_DisablesParallelToolCallsWhenClaudeRequestsIt(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		ToolChoice: map[string]any{
			"type":                      "auto",
			"disable_parallel_tool_use": true,
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}
	if got := string(out.ParallelToolCalls); got != "false" {
		t.Fatalf("parallel_tool_calls = %s, want false", got)
	}
	if got := string(out.ToolChoice); got != `"auto"` {
		t.Fatalf("tool_choice = %s, want auto", got)
	}
}

func TestClaudeToOpenAIResponsesRequest_RemovesUnsupportedServiceTier(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model:       "claude-3-5-sonnet",
		ServiceTier: "flex",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}
	if out.ServiceTier != "" {
		t.Fatalf("expected unsupported service_tier to be removed, got %q", out.ServiceTier)
	}
}

func TestClaudeToolUseIDResponsesCallIDConversions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		back string
	}{
		{name: "call", in: "call_1", want: "fc_call_1", back: "call_1"},
		{name: "toolu", in: "toolu_1", want: "fc_toolu_1", back: "toolu_1"},
		{name: "already_fc", in: "fc_plain", want: "fc_plain", back: "fc_plain"},
	}

	for _, tc := range cases {
		if got := ClaudeToolUseIDToResponsesCallID(tc.in); got != tc.want {
			t.Fatalf("%s: expected responses call id %q, got %q", tc.name, tc.want, got)
		}
		if got := ResponsesCallIDToClaudeToolUseID(tc.want); got != tc.back {
			t.Fatalf("%s: expected claude tool id %q, got %q", tc.name, tc.back, got)
		}
	}
}

func TestClaudeToOpenAIResponsesRequest_NormalizesToolUseIDsAndExtractsToolResultImages(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":  "tool_use",
						"id":    "call_1",
						"name":  "Read",
						"input": map[string]any{"file_path": "/tmp/screen.png"},
					},
				},
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "call_1",
						"content": []any{
							map[string]any{"type": "text", "text": "done"},
							map[string]any{
								"type": "image",
								"source": map[string]any{
									"type":       "base64",
									"media_type": "image/png",
									"data":       "AAAA",
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal(out.Input, &items); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 input items, got %d: %#v", len(items), items)
	}

	if got := items[0]["type"]; got != "function_call" {
		t.Fatalf("expected first item to be function_call, got %#v", got)
	}
	if got := items[0]["call_id"]; got != "fc_call_1" {
		t.Fatalf("expected function_call call_id to be normalized, got %#v", got)
	}

	if got := items[1]["type"]; got != "function_call_output" {
		t.Fatalf("expected second item to be function_call_output, got %#v", got)
	}
	if got := items[1]["call_id"]; got != "fc_call_1" {
		t.Fatalf("expected function_call_output call_id to be normalized, got %#v", got)
	}
	if got := items[1]["output"]; got != "done" {
		t.Fatalf("expected tool result text output to be preserved, got %#v", got)
	}

	if got := items[2]["type"]; got != "message" {
		t.Fatalf("expected third item to be a user message for extracted images, got %#v", got)
	}
	if got := items[2]["role"]; got != "user" {
		t.Fatalf("expected extracted image message to stay on user role, got %#v", got)
	}
	content, ok := items[2]["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("expected one extracted image content part, got %#v", items[2]["content"])
	}
	part, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("expected image content part to be an object, got %#v", content[0])
	}
	if got := part["type"]; got != "input_image" {
		t.Fatalf("expected extracted content part to be input_image, got %#v", got)
	}
	if got := part["image_url"]; got != "data:image/png;base64,AAAA" {
		t.Fatalf("expected extracted image_url to be preserved, got %#v", got)
	}
}

func TestClaudeToOpenAIResponsesRequest_ErrorsOnEmptyAssistantToolUseID(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":  "tool_use",
						"id":    "   ",
						"name":  "Read",
						"input": map[string]any{"file_path": "/tmp/a"},
					},
				},
			},
		},
	}

	_, err := ClaudeToOpenAIResponsesRequest(req)
	if err == nil || err.Error() != "assistant tool_use id is empty" {
		t.Fatalf("expected assistant tool_use id validation error, got %v", err)
	}
}

func TestClaudeToOpenAIResponsesRequest_ErrorsOnEmptyToolResultToolUseID(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "",
						"content": []any{
							map[string]any{"type": "text", "text": "done"},
						},
					},
				},
			},
		},
	}

	_, err := ClaudeToOpenAIResponsesRequest(req)
	if err == nil || err.Error() != "user tool_result tool_use_id is empty" {
		t.Fatalf("expected tool_result tool_use_id validation error, got %v", err)
	}
}

func TestClaudeToOpenAIResponsesRequest_ToolChoiceSpecificUsesResponsesFunctionShape(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		ToolChoice: map[string]any{
			"type": "tool",
			"name": "Read",
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var toolChoice map[string]any
	if err := json.Unmarshal(out.ToolChoice, &toolChoice); err != nil {
		t.Fatalf("unmarshal tool_choice: %v", err)
	}
	if got := toolChoice["type"]; got != "function" {
		t.Fatalf("tool_choice.type = %#v, want function", got)
	}
	fn, ok := toolChoice["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice.function = %#v, want object", toolChoice["function"])
	}
	if got := fn["name"]; got != "Read" {
		t.Fatalf("tool_choice.function.name = %#v, want Read", got)
	}
	if _, exists := toolChoice["name"]; exists {
		t.Fatalf("tool_choice must not use flat name field: %#v", toolChoice["name"])
	}
}

func TestClaudeToOpenAIResponsesRequest_BuiltinWebSearchToolChoiceUsesResponsesBuiltinShape(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []map[string]any{
			{
				"type": "google_search",
				"name": "google_search",
			},
		},
		ToolChoice: map[string]any{
			"type": "tool",
			"name": "google_search",
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var toolChoice map[string]any
	if err := json.Unmarshal(out.ToolChoice, &toolChoice); err != nil {
		t.Fatalf("unmarshal tool_choice: %v", err)
	}
	if got := toolChoice["type"]; got != "web_search" {
		t.Fatalf("tool_choice.type = %#v, want web_search", got)
	}
	if _, exists := toolChoice["function"]; exists {
		t.Fatalf("builtin web_search tool_choice must not use function shape: %#v", toolChoice["function"])
	}
}

func TestClaudeToOpenAIResponsesRequest_ToolChoiceKeepsFunctionShapeForNameOnlyGoogleSearchTool(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []map[string]any{
			{
				"name":        "google_search",
				"description": "custom function, not built-in",
			},
		},
		ToolChoice: map[string]any{
			"type": "tool",
			"name": "google_search",
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var toolChoice map[string]any
	if err := json.Unmarshal(out.ToolChoice, &toolChoice); err != nil {
		t.Fatalf("unmarshal tool_choice: %v", err)
	}
	if got := toolChoice["type"]; got != "function" {
		t.Fatalf("tool_choice.type = %#v, want function", got)
	}
	if _, exists := toolChoice["function"]; !exists {
		t.Fatalf("name-only function tool_choice must keep function shape: %#v", toolChoice)
	}
}

func TestClaudeToOpenAIResponsesRequestWithToolNameMapping_NormalizesLongMcpToolNames(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	originalListDirA := "mcp__" + strings.Repeat("workspace_alpha__", 5) + "list_dir"
	originalListDirB := "mcp__" + strings.Repeat("workspace_beta__", 5) + "list_dir"

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Tools: []map[string]any{
			{
				"name":        originalListDirA,
				"description": "List directory A",
			},
			{
				"name":        originalListDirB,
				"description": "List directory B",
			},
		},
		ToolChoice: map[string]any{
			"type": "tool",
			"name": originalListDirA,
		},
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":  "tool_use",
						"id":    "toolu_1",
						"name":  originalListDirB,
						"input": map[string]any{"relative_path": "internal"},
					},
				},
			},
		},
	}

	out, originalToNormalized, normalizedToOriginal, err := ClaudeToOpenAIResponsesRequestWithToolNameMapping(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequestWithToolNameMapping error: %v", err)
	}

	normalizedListDirA := originalToNormalized[originalListDirA]
	normalizedListDirB := originalToNormalized[originalListDirB]
	if normalizedListDirA == "" || normalizedListDirA == originalListDirA {
		t.Fatalf("expected long MCP tool A to be normalized, got %#v", normalizedListDirA)
	}
	if normalizedListDirB == "" || normalizedListDirB == originalListDirB {
		t.Fatalf("expected long MCP tool B to be normalized, got %#v", normalizedListDirB)
	}
	if len(normalizedListDirA) > 64 || len(normalizedListDirB) > 64 {
		t.Fatalf("expected normalized MCP tool names to respect 64-char limit, got %q and %q", normalizedListDirA, normalizedListDirB)
	}
	if normalizedListDirA == normalizedListDirB {
		t.Fatalf("expected colliding MCP tool names to remain unique, got %q", normalizedListDirA)
	}
	if got := normalizedToOriginal[normalizedListDirA]; got != originalListDirA {
		t.Fatalf("expected reverse map for tool A, got %#v", got)
	}
	if got := normalizedToOriginal[normalizedListDirB]; got != originalListDirB {
		t.Fatalf("expected reverse map for tool B, got %#v", got)
	}

	var tools []map[string]any
	if err := json.Unmarshal(out.Tools, &tools); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %#v", tools)
	}
	if got := tools[0]["name"]; got != normalizedListDirA {
		t.Fatalf("expected first tool name to be normalized, got %#v", got)
	}
	if got := tools[1]["name"]; got != normalizedListDirB {
		t.Fatalf("expected second tool name to be normalized, got %#v", got)
	}

	var toolChoice map[string]any
	if err := json.Unmarshal(out.ToolChoice, &toolChoice); err != nil {
		t.Fatalf("unmarshal tool_choice: %v", err)
	}
	function, ok := toolChoice["function"].(map[string]any)
	if !ok {
		t.Fatalf("expected function tool_choice, got %#v", toolChoice)
	}
	if got := function["name"]; got != normalizedListDirA {
		t.Fatalf("expected tool_choice.function.name to be normalized, got %#v", got)
	}

	var items []map[string]any
	if err := json.Unmarshal(out.Input, &items); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 input item, got %#v", items)
	}
	if got := items[0]["type"]; got != "function_call" {
		t.Fatalf("expected function_call item, got %#v", got)
	}
	if got := items[0]["name"]; got != normalizedListDirB {
		t.Fatalf("expected assistant tool_use name to be normalized, got %#v", got)
	}
}

func TestClaudeToOpenAIResponsesRequest_EmptyToolUseInputUsesEmptyJSONObject(t *testing.T) {
	restoreCodexInstructionsOption(t, "BASE")

	req := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type": "tool_use",
						"id":   "toolu_1",
						"name": "Read",
					},
				},
			},
		},
	}

	out, err := ClaudeToOpenAIResponsesRequest(req)
	if err != nil {
		t.Fatalf("ClaudeToOpenAIResponsesRequest error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal(out.Input, &items); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 input item, got %d: %#v", len(items), items)
	}
	if got := items[0]["type"]; got != "function_call" {
		t.Fatalf("expected function_call item, got %#v", got)
	}
	if got := items[0]["arguments"]; got != "{}" {
		t.Fatalf("arguments = %#v, want {}", got)
	}
}

func TestNormalizeClaudeResponsesBuiltInToolType_WebSearchTypeAliases(t *testing.T) {
	cases := []struct {
		name     string
		toolType string
	}{
		{name: "typePrefix", toolType: "web_search_preview"},
		{name: "typeGoogle", toolType: "google_search"},
	}

	for _, tc := range cases {
		if got := normalizeClaudeResponsesBuiltInToolType(tc.toolType); got != dto.BuildInToolWebSearch {
			t.Fatalf("%s: expected alias to normalize to %q, got %q", tc.name, dto.BuildInToolWebSearch, got)
		}
	}
}

func TestClaudeToolChoiceToOpenAIResponses_BuiltInWebSearchProfiles(t *testing.T) {
	tools := []map[string]any{
		{"type": "web_search_preview", "name": "web_search_preview"},
		{"type": "google_search", "name": "google_search"},
	}

	cases := []struct {
		name  string
		input any
	}{
		{name: "stringPrefix", input: "web_search_preview"},
		{name: "stringGoogle", input: "google_search"},
		{name: "objectNameAlias", input: dto.ClaudeToolChoice{Type: "tool", Name: "google_search"}},
		{name: "objectTypeAlias", input: dto.ClaudeToolChoice{Type: "web_search_preview"}},
	}

	for _, tc := range cases {
		got, err := claudeToolChoiceToOpenAIResponses(tc.input, tools)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		toolChoice, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s: expected map output, got %#v", tc.name, got)
		}
		if gotType, _ := toolChoice["type"].(string); gotType != dto.BuildInToolWebSearch {
			t.Fatalf("%s: expected type %q, got %#v", tc.name, dto.BuildInToolWebSearch, toolChoice["type"])
		}
		if _, exists := toolChoice["function"]; exists {
			t.Fatalf("%s: expected built-in tool_choice to avoid function wrapper, got %#v", tc.name, toolChoice)
		}
	}
}

func restoreCodexInstructionsOption(t *testing.T, value string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	backup := cloneOptionMap(common.OptionMap)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["codex.prompt.chat_completions.instructions"] = value
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = backup
		common.OptionMapRWMutex.Unlock()
	})
}

func restoreResponsesCompatOverrideOption(t *testing.T, value string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	backup := cloneOptionMap(common.OptionMap)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[cxCompatResponsesOverrideInstructionsOpt] = value
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

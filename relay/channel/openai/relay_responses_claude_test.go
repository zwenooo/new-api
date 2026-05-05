package openai

import (
	"encoding/json"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"testing"
)

func TestTranslateResponsesToClaudeMessage_RestoresAnthropicToolUseID(t *testing.T) {
	raw := openAIResponsesRaw{
		Status: "completed",
		Output: []json.RawMessage{
			json.RawMessage(`{"type":"function_call","call_id":"fc_toolu_1","name":"Read","arguments":"{\"file_path\":\"/tmp/a\"}"}`),
		},
	}

	resp := translateResponsesToClaudeMessage(raw, "claude-3-5-sonnet")
	if len(resp.Content) != 1 {
		t.Fatalf("expected one content block, got %#v", resp.Content)
	}

	block, ok := resp.Content[0].(oaiCcClaudeResponseToolUseBlock)
	if !ok {
		t.Fatalf("expected tool_use block, got %#v", resp.Content[0])
	}
	if block.Id != "toolu_1" {
		t.Fatalf("expected tool_use id to be restored for Claude clients, got %#v", block.Id)
	}
}

func TestTranslateResponsesToClaudeMessageWithToolNameMap_RestoresNormalizedToolName(t *testing.T) {
	originalName := "mcp__workspace_alpha__workspace_alpha__workspace_alpha__list_dir"
	raw := openAIResponsesRaw{
		Status: "completed",
		Output: []json.RawMessage{
			json.RawMessage(`{"type":"function_call","call_id":"fc_toolu_1","name":"mcp__list_dir","arguments":"{\"file_path\":\"/tmp/a\"}"}`),
		},
	}

	resp := translateResponsesToClaudeMessageWithToolNameMap(raw, "claude-3-5-sonnet", map[string]string{
		"mcp__list_dir": originalName,
	})
	if len(resp.Content) != 1 {
		t.Fatalf("expected one content block, got %#v", resp.Content)
	}

	block, ok := resp.Content[0].(oaiCcClaudeResponseToolUseBlock)
	if !ok {
		t.Fatalf("expected tool_use block, got %#v", resp.Content[0])
	}
	if block.Name != originalName {
		t.Fatalf("expected normalized tool name to be restored, got %#v", block.Name)
	}
}

func TestTranslateResponsesToClaudeMessage_MapsWebSearchCallToClaudeServerToolBlocks(t *testing.T) {
	raw := openAIResponsesRaw{
		Status: "completed",
		Output: []json.RawMessage{
			json.RawMessage(`{"type":"web_search_call","id":"ws_1","action":{"query":"golang"}}`),
		},
	}

	resp := translateResponsesToClaudeMessage(raw, "claude-3-5-sonnet")
	if len(resp.Content) != 2 {
		t.Fatalf("expected two content blocks, got %#v", resp.Content)
	}

	serverToolUse, ok := resp.Content[0].(map[string]any)
	if !ok {
		t.Fatalf("expected server_tool_use block, got %#v", resp.Content[0])
	}
	if got := serverToolUse["type"]; got != "server_tool_use" {
		t.Fatalf("expected server_tool_use type, got %#v", got)
	}
	if got := serverToolUse["id"]; got != "srvtoolu_ws_1" {
		t.Fatalf("expected synthesized server tool id, got %#v", got)
	}
	if got := serverToolUse["name"]; got != "web_search" {
		t.Fatalf("expected web_search name, got %#v", got)
	}
	input, ok := serverToolUse["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected server_tool_use input object, got %#v", serverToolUse["input"])
	}
	if got := input["query"]; got != "golang" {
		t.Fatalf("expected query to be preserved, got %#v", got)
	}

	resultBlock, ok := resp.Content[1].(map[string]any)
	if !ok {
		t.Fatalf("expected web_search_tool_result block, got %#v", resp.Content[1])
	}
	if got := resultBlock["type"]; got != "web_search_tool_result" {
		t.Fatalf("expected web_search_tool_result type, got %#v", got)
	}
	if got := resultBlock["tool_use_id"]; got != "srvtoolu_ws_1" {
		t.Fatalf("expected matching tool_use_id, got %#v", got)
	}
}

func TestResponsesToClaudeStreamTranslator_RestoresAnthropicToolUseID(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)
	outs, _, ok := translator.processEvent("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "fc_call_1",
			"name":    "lookup",
		},
	})
	if !ok {
		t.Fatal("expected stream translator to continue")
	}
	if len(outs) != 1 {
		t.Fatalf("expected one outbound event, got %#v", outs)
	}

	start, ok := outs[0].Payload.(oaiCcClaudeStreamContentBlockStart)
	if !ok {
		t.Fatalf("expected content block start payload, got %#v", outs[0].Payload)
	}
	contentBlock, ok := start.ContentBlock.(map[string]any)
	if !ok {
		t.Fatalf("expected content block object, got %#v", start.ContentBlock)
	}
	if got := contentBlock["id"]; got != "call_1" {
		t.Fatalf("expected stream tool_use id to be restored, got %#v", got)
	}
}

func TestResponsesToClaudeStreamTranslatorWithToolNameMap_RestoresNormalizedToolName(t *testing.T) {
	originalName := "mcp__workspace_alpha__workspace_alpha__workspace_alpha__list_dir"
	translator := newResponsesToClaudeStreamTranslatorWithToolNameMap("claude-3-5-sonnet", nil, map[string]string{
		"mcp__list_dir": originalName,
	})
	outs, _, ok := translator.processEvent("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "fc_call_1",
			"name":    "mcp__list_dir",
		},
	})
	if !ok {
		t.Fatal("expected stream translator to continue")
	}
	if len(outs) != 1 {
		t.Fatalf("expected one outbound event, got %#v", outs)
	}

	start, ok := outs[0].Payload.(oaiCcClaudeStreamContentBlockStart)
	if !ok {
		t.Fatalf("expected content block start payload, got %#v", outs[0].Payload)
	}
	contentBlock, ok := start.ContentBlock.(map[string]any)
	if !ok {
		t.Fatalf("expected content block object, got %#v", start.ContentBlock)
	}
	if got := contentBlock["name"]; got != originalName {
		t.Fatalf("expected normalized tool name to be restored, got %#v", got)
	}
}

func TestResponsesToClaudeStreamTranslator_MapsWebSearchCallToClaudeServerToolEvents(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)
	outs, _, ok := translator.processEvent("response.output_item.done", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":   "web_search_call",
			"status": "completed",
			"id":     "ws_1",
			"action": map[string]any{
				"query": "golang",
			},
		},
	})
	if !ok {
		t.Fatal("expected stream translator to continue")
	}
	if len(outs) != 4 {
		t.Fatalf("expected four outbound events, got %#v", outs)
	}

	start1, ok := outs[0].Payload.(oaiCcClaudeStreamContentBlockStart)
	if !ok {
		t.Fatalf("expected first payload to be content block start, got %#v", outs[0].Payload)
	}
	block1, ok := start1.ContentBlock.(map[string]any)
	if !ok {
		t.Fatalf("expected first content block object, got %#v", start1.ContentBlock)
	}
	if got := block1["type"]; got != "server_tool_use" {
		t.Fatalf("expected server_tool_use block, got %#v", got)
	}
	if got := block1["id"]; got != "srvtoolu_ws_1" {
		t.Fatalf("expected synthesized server tool id, got %#v", got)
	}

	start2, ok := outs[2].Payload.(oaiCcClaudeStreamContentBlockStart)
	if !ok {
		t.Fatalf("expected third payload to be content block start, got %#v", outs[2].Payload)
	}
	block2, ok := start2.ContentBlock.(map[string]any)
	if !ok {
		t.Fatalf("expected second content block object, got %#v", start2.ContentBlock)
	}
	if got := block2["type"]; got != "web_search_tool_result" {
		t.Fatalf("expected web_search_tool_result block, got %#v", got)
	}
	if got := block2["tool_use_id"]; got != "srvtoolu_ws_1" {
		t.Fatalf("expected matching tool_use_id, got %#v", got)
	}
}

func TestResponsesToClaudeStreamTranslator_FunctionCallArgumentsDoneEmitsFallbackDelta(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)

	if _, _, ok := translator.processEvent("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "fc_call_1",
			"name":    "lookup",
		},
	}); !ok {
		t.Fatal("expected translator to continue after function_call start")
	}

	outs, _, ok := translator.processEvent("response.function_call_arguments.done", map[string]any{
		"output_index": 0,
		"arguments":    `{"city":"Paris"}`,
	})
	if !ok {
		t.Fatal("expected translator to continue after function_call_arguments.done")
	}
	if len(outs) != 1 {
		t.Fatalf("expected one fallback delta event, got %#v", outs)
	}

	delta, ok := outs[0].Payload.(oaiCcClaudeStreamContentBlockDelta)
	if !ok {
		t.Fatalf("expected content block delta payload, got %#v", outs[0].Payload)
	}
	deltaBody, ok := delta.Delta.(map[string]any)
	if !ok {
		t.Fatalf("expected delta object, got %#v", delta.Delta)
	}
	if got := deltaBody["type"]; got != "input_json_delta" {
		t.Fatalf("expected input_json_delta type, got %#v", got)
	}
	if got := deltaBody["partial_json"]; got != `{"city":"Paris"}` {
		t.Fatalf("expected fallback partial_json to use done arguments, got %#v", got)
	}
}

func TestResponsesToClaudeStreamTranslator_FunctionCallArgumentsDoneSkipsFallbackAfterDelta(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)

	if _, _, ok := translator.processEvent("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "fc_call_1",
			"name":    "lookup",
		},
	}); !ok {
		t.Fatal("expected translator to continue after function_call start")
	}

	if _, _, ok := translator.processEvent("response.function_call_arguments.delta", map[string]any{
		"output_index": 0,
		"call_id":      "fc_call_1",
		"name":         "lookup",
		"delta":        `{"city":"Pa`,
	}); !ok {
		t.Fatal("expected translator to continue after function_call_arguments.delta")
	}

	outs, _, ok := translator.processEvent("response.function_call_arguments.done", map[string]any{
		"output_index": 0,
		"arguments":    `{"city":"Paris"}`,
	})
	if !ok {
		t.Fatal("expected translator to continue after function_call_arguments.done")
	}
	if len(outs) != 0 {
		t.Fatalf("expected done event to skip duplicate fallback delta, got %#v", outs)
	}
}

func TestResponsesToClaudeStreamTranslator_EmptyArgumentsDeltaStillAllowsDoneFallback(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)

	if _, _, ok := translator.processEvent("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "fc_call_1",
			"name":    "lookup",
		},
	}); !ok {
		t.Fatal("expected translator to continue after function_call start")
	}

	outs, _, ok := translator.processEvent("response.function_call_arguments.delta", map[string]any{
		"output_index": 0,
		"call_id":      "fc_call_1",
		"name":         "lookup",
		"delta":        "",
	})
	if !ok {
		t.Fatal("expected translator to continue after empty function_call_arguments.delta")
	}
	if len(outs) != 0 {
		t.Fatalf("expected empty delta to produce no outbound events, got %#v", outs)
	}

	outs, _, ok = translator.processEvent("response.function_call_arguments.done", map[string]any{
		"output_index": 0,
		"arguments":    `{"city":"Paris"}`,
	})
	if !ok {
		t.Fatal("expected translator to continue after function_call_arguments.done")
	}
	if len(outs) != 1 {
		t.Fatalf("expected done fallback delta after empty delta, got %#v", outs)
	}

	delta, ok := outs[0].Payload.(oaiCcClaudeStreamContentBlockDelta)
	if !ok {
		t.Fatalf("expected content block delta payload, got %#v", outs[0].Payload)
	}
	deltaBody, ok := delta.Delta.(map[string]any)
	if !ok {
		t.Fatalf("expected delta object, got %#v", delta.Delta)
	}
	if got := deltaBody["partial_json"]; got != `{"city":"Paris"}` {
		t.Fatalf("expected done fallback partial_json to be preserved, got %#v", got)
	}
}

func TestRecordClaudeWebSearchRequestsFromContent_UpdatesResponsesUsageInfo(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	content := []any{
		map[string]any{
			"type": "server_tool_use",
			"name": dto.BuildInToolWebSearch,
		},
		map[string]any{
			"type": "server_tool_use",
			"name": dto.BuildInToolWebSearch,
		},
	}

	callCount := recordClaudeWebSearchRequestsFromContent(info, content)
	if callCount != 2 {
		t.Fatalf("expected two web search calls, got %d", callCount)
	}

	webSearchTool := relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolWebSearch)
	if webSearchTool == nil {
		t.Fatalf("expected canonical web_search tool entry, got %#v", info.ResponsesUsageInfo)
	}
	if webSearchTool.CallCount != 2 {
		t.Fatalf("expected web_search call count to be recorded, got %d", webSearchTool.CallCount)
	}
}

func TestResponsesToClaudeStreamTranslator_FinalizeIncompleteClosesMessage(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)
	if outs, usage := translator.finalizeIncomplete(""); len(outs) != 0 || usage != nil {
		t.Fatalf("expected no synthetic finalization before message start, got outs=%#v usage=%#v", outs, usage)
	}

	createdOuts, _, ok := translator.processEvent("response.created", map[string]any{
		"response": map[string]any{
			"id": "resp_1",
		},
	})
	if !ok {
		t.Fatal("expected translator to continue after response.created")
	}
	if len(createdOuts) != 1 {
		t.Fatalf("expected one message_start event, got %#v", createdOuts)
	}

	textOuts, _, ok := translator.processEvent("response.output_text.delta", map[string]any{
		"output_index":  0,
		"content_index": 0,
		"delta":         "partial",
	})
	if !ok {
		t.Fatal("expected translator to continue after text delta")
	}
	if len(textOuts) != 2 {
		t.Fatalf("expected content_block_start + content_block_delta, got %#v", textOuts)
	}

	finalOuts, finalUsage := translator.finalizeIncomplete("done")
	if len(finalOuts) != 3 {
		t.Fatalf("expected content_block_stop + message_delta + message_stop, got %#v", finalOuts)
	}
	if finalOuts[0].EventType != "content_block_stop" {
		t.Fatalf("expected first synthetic event to close the open block, got %#v", finalOuts[0])
	}
	if finalOuts[1].EventType != "message_delta" {
		t.Fatalf("expected second synthetic event to be message_delta, got %#v", finalOuts[1])
	}
	if finalOuts[2].EventType != "message_stop" {
		t.Fatalf("expected third synthetic event to be message_stop, got %#v", finalOuts[2])
	}
	if finalUsage == nil {
		t.Fatal("expected synthetic finalization to return usage")
	}

	repeatedOuts, repeatedUsage := translator.finalizeIncomplete("done")
	if len(repeatedOuts) != 0 || repeatedUsage != nil {
		t.Fatalf("expected finalization to be idempotent after stop, got outs=%#v usage=%#v", repeatedOuts, repeatedUsage)
	}
}

func TestResponsesToClaudeStreamTranslator_FinalizeIncompleteSkipsFailedTermination(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)
	if _, _, ok := translator.processEvent("response.created", map[string]any{
		"response": map[string]any{"id": "resp_1"},
	}); !ok {
		t.Fatal("expected translator to continue after response.created")
	}

	if _, _, ok := translator.processEvent("response.failed", map[string]any{
		"response": map[string]any{
			"error": map[string]any{
				"type":    "server_error",
				"message": "boom",
			},
		},
	}); ok {
		t.Fatal("expected translator to stop after response.failed")
	}

	if outs, usage := translator.finalizeIncomplete("failed"); len(outs) != 0 || usage != nil {
		t.Fatalf("expected no synthetic finalization after failed termination, got outs=%#v usage=%#v", outs, usage)
	}
}

func TestRecordResponsesNonWebSearchToolCalls_TracksFileSearchWithoutDoubleCountingWebSearch(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	recordResponsesNonWebSearchToolCalls(info, []map[string]any{
		{
			"type": dto.BuildInToolWebSearch,
		},
		{
			"type": dto.BuildInToolFileSearch,
		},
	})

	webSearchTool := relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolWebSearch)
	if webSearchTool == nil {
		t.Fatalf("expected canonical web_search tool entry, got %#v", info.ResponsesUsageInfo)
	}
	if webSearchTool.CallCount != 0 {
		t.Fatalf("expected web_search count to stay at 0, got %d", webSearchTool.CallCount)
	}

	fileSearchTool := relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolFileSearch)
	if fileSearchTool == nil {
		t.Fatalf("expected file_search tool entry, got %#v", info.ResponsesUsageInfo)
	}
	if fileSearchTool.CallCount != 1 {
		t.Fatalf("expected file_search count 1, got %d", fileSearchTool.CallCount)
	}
}

func TestResponsesToClaudeStreamTranslator_FinalizeIncompleteSkipsAfterErrorEvent(t *testing.T) {
	translator := newResponsesToClaudeStreamTranslator("claude-3-5-sonnet", nil)
	if _, _, ok := translator.processEvent("response.created", map[string]any{
		"response": map[string]any{"id": "resp_1"},
	}); !ok {
		t.Fatal("expected translator to continue after response.created")
	}

	if outs, _, ok := translator.processEvent("error", map[string]any{
		"code":    "server_error",
		"message": "boom",
	}); !ok {
		t.Fatal("expected translator to continue after error event")
	} else if len(outs) != 1 || outs[0].EventType != "error" {
		t.Fatalf("expected error event to be forwarded, got %#v", outs)
	}

	if outs, usage := translator.finalizeIncomplete("done"); len(outs) != 0 || usage != nil {
		t.Fatalf("expected no synthetic finalization after error event, got outs=%#v usage=%#v", outs, usage)
	}
}

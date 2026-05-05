package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildResponsesRequest_IncludesChatHistory(t *testing.T) {
	h := &APIHandler{}
	req := &chatCompletionsRequest{
		Model: "gpt-5.2",
		Messages: []chatMessageInput{
			{Role: "system", Content: "SYS"},
			{Role: "user", Content: "你叫什么名字？"},
			{Role: "assistant", Content: "我叫 ChatGPT。"},
			{Role: "user", Content: "我叫小王"},
		},
	}

	resp, err := h.buildResponsesRequest(req, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest error: %v", err)
	}

	if resp.Instructions != "" {
		t.Fatalf("unexpected instructions: %q", resp.Instructions)
	}

	if got := len(resp.Input); got != 4 {
		t.Fatalf("unexpected input length: %d", got)
	}

	if resp.Input[0].Role != "developer" || len(resp.Input[0].Content) != 1 || resp.Input[0].Content[0].Type != "input_text" || resp.Input[0].Content[0].Text != "SYS" {
		t.Fatalf("unexpected first input: %+v", resp.Input[0])
	}
	if resp.Input[1].Role != "user" || len(resp.Input[1].Content) != 1 || resp.Input[1].Content[0].Type != "input_text" || resp.Input[1].Content[0].Text != "你叫什么名字？" {
		t.Fatalf("unexpected second input: %+v", resp.Input[1])
	}
	if resp.Input[2].Role != "assistant" || len(resp.Input[2].Content) != 1 || resp.Input[2].Content[0].Type != "output_text" || resp.Input[2].Content[0].Text != "我叫 ChatGPT。" {
		t.Fatalf("unexpected third input: %+v", resp.Input[2])
	}
	if resp.Input[3].Role != "user" || len(resp.Input[3].Content) != 1 || resp.Input[3].Content[0].Type != "input_text" || resp.Input[3].Content[0].Text != "我叫小王" {
		t.Fatalf("unexpected fourth input: %+v", resp.Input[3])
	}
	if resp.Stream {
		t.Fatalf("expected non-stream responses request")
	}
	if resp.Reasoning == nil || resp.Reasoning.Effort != "medium" || resp.Reasoning.Summary != "auto" {
		t.Fatalf("expected default reasoning medium, got %+v", resp.Reasoning)
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal responses request: %v", err)
	}
	if !strings.Contains(string(encoded), `"instructions":""`) {
		t.Fatalf("expected encoded payload to keep empty instructions, got %s", string(encoded))
	}
}

func TestGatherMessages_AllowsAssistantTail(t *testing.T) {
	inputs, err := gatherMessages([]chatMessageInput{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inputs) != 2 || inputs[1].Role != "assistant" {
		t.Fatalf("unexpected inputs: %+v", inputs)
	}
}

func TestGatherMessages_RejectsUnknownRole(t *testing.T) {
	_, err := gatherMessages([]chatMessageInput{
		{Role: "user", Content: "hi"},
		{Role: "critic", Content: "result"},
		{Role: "user", Content: "next"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildResponsesRequest_StreamMatchesTargetMode(t *testing.T) {
	h := &APIHandler{}
	req := &chatCompletionsRequest{
		Model: "gpt-5.2",
		Messages: []chatMessageInput{
			{Role: "user", Content: "hi"},
		},
	}

	resp, err := h.buildResponsesRequest(req, true)
	if err != nil {
		t.Fatalf("buildResponsesRequest error: %v", err)
	}
	if !resp.Stream {
		t.Fatalf("expected stream=true for streaming compat request")
	}
	if len(resp.Tools) != 0 {
		t.Fatalf("expected no synthetic tools, got %+v", resp.Tools)
	}
}

func TestShouldTreatAsResponsesFormat(t *testing.T) {
	if !shouldTreatAsResponsesFormat(map[string]interface{}{"input": []interface{}{}}) {
		t.Fatalf("expected responses format when input exists")
	}
	if !shouldTreatAsResponsesFormat(map[string]interface{}{"instructions": "SYS"}) {
		t.Fatalf("expected responses format when instructions exists")
	}
	if shouldTreatAsResponsesFormat(map[string]interface{}{"messages": []interface{}{map[string]interface{}{"role": "user", "content": "hi"}}, "input": []interface{}{}}) {
		t.Fatalf("messages should win over responses markers")
	}
}

func TestConvertResponsesRequestToChatCompletions(t *testing.T) {
	req, err := convertResponsesRequestToChatCompletions(map[string]interface{}{
		"model":               "gpt-5.1-codex",
		"instructions":        "SYS",
		"stream":              true,
		"parallel_tool_calls": true,
		"tool_choice": map[string]interface{}{
			"type": "function",
			"name": "lookup",
		},
		"input": []interface{}{
			map[string]interface{}{
				"type": "message",
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "input_text",
						"text": "hello",
					},
				},
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "lookup",
				"arguments": `{"id":1}`,
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output":  "ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("convertResponsesRequestToChatCompletions error: %v", err)
	}
	if req.Model != "gpt-5.1-codex" {
		t.Fatalf("unexpected model: %q", req.Model)
	}
	if req.Stream == nil || !*req.Stream {
		t.Fatalf("expected stream=true")
	}
	if req.ParallelToolCalls == nil || !*req.ParallelToolCalls {
		t.Fatalf("expected parallel_tool_calls=true")
	}
	if len(req.Messages) != 4 {
		t.Fatalf("unexpected messages length: %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "SYS" {
		t.Fatalf("unexpected system message: %+v", req.Messages[0])
	}
	if req.Messages[2].Role != "assistant" || len(req.Messages[2].ToolCalls) != 1 || req.Messages[2].ToolCalls[0].Function == nil || req.Messages[2].ToolCalls[0].Function.Name != "lookup" {
		t.Fatalf("unexpected assistant tool call message: %+v", req.Messages[2])
	}
	if req.Messages[3].Role != "tool" || req.Messages[3].ToolCallID != "call_1" || req.Messages[3].Content != "ok" {
		t.Fatalf("unexpected tool message: %+v", req.Messages[3])
	}
}

func TestBuildResponsesRequest_PreservesToolCallsAndToolOutputs(t *testing.T) {
	h := &APIHandler{}
	req := &chatCompletionsRequest{
		Model: "gpt-5.2",
		Messages: []chatMessageInput{
			{Role: "system", Content: "SYS"},
			{Role: "user", Content: "run"},
			{
				Role: "assistant",
				ToolCalls: []chatToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: &chatToolFunction{
							Name:      "lookup",
							Arguments: `{"q":"x"}`,
						},
					},
				},
			},
			{Role: "tool", ToolCallID: "call_1", Content: "done"},
		},
		Tools: []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "lookup",
					"description": "desc",
					"parameters": map[string]interface{}{
						"type": "object",
					},
				},
			},
		},
		ToolChoice: map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": "lookup",
			},
		},
	}

	resp, err := h.buildResponsesRequest(req, true)
	if err != nil {
		t.Fatalf("buildResponsesRequest error: %v", err)
	}
	if !resp.Stream {
		t.Fatalf("expected stream=true")
	}
	if !resp.ParallelToolCalls {
		t.Fatalf("expected parallel_tool_calls default to true")
	}
	if len(resp.Input) != 4 {
		t.Fatalf("unexpected input length: %d", len(resp.Input))
	}
	if resp.Input[0].Role != "developer" || len(resp.Input[0].Content) != 1 || resp.Input[0].Content[0].Text != "SYS" {
		t.Fatalf("unexpected developer input: %+v", resp.Input[0])
	}
	if resp.Input[2].Type != "function_call" || resp.Input[2].CallID != "call_1" || resp.Input[2].Name != "lookup" || resp.Input[2].Arguments != `{"q":"x"}` {
		t.Fatalf("unexpected function_call input: %+v", resp.Input[2])
	}
	if resp.Input[3].Type != "function_call_output" || resp.Input[3].CallID != "call_1" || resp.Input[3].Output != "done" {
		t.Fatalf("unexpected function_call_output input: %+v", resp.Input[3])
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("unexpected tools length: %d", len(resp.Tools))
	}
	toolMap, ok := resp.Tools[0].(map[string]interface{})
	if !ok || toolMap["name"] != "lookup" {
		t.Fatalf("unexpected normalized tool: %+v", resp.Tools[0])
	}
	toolChoice, ok := resp.ToolChoice.(map[string]interface{})
	if !ok || toolChoice["name"] != "lookup" {
		t.Fatalf("unexpected tool choice: %+v", resp.ToolChoice)
	}
	if resp.Instructions != "" {
		t.Fatalf("expected empty instructions, got %q", resp.Instructions)
	}
	if resp.Reasoning == nil || resp.Reasoning.Effort != "medium" {
		t.Fatalf("expected default reasoning medium, got %+v", resp.Reasoning)
	}
}

func TestBuildResponsesRequest_MapsSystemToDeveloperInput(t *testing.T) {
	h := &APIHandler{}
	req := &chatCompletionsRequest{
		Model: "gpt-5.2",
		Messages: []chatMessageInput{
			{Role: "system", Content: "SYS"},
			{Role: "user", Content: "hi"},
		},
	}

	resp, err := h.buildResponsesRequest(req, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest error: %v", err)
	}
	if resp.Instructions != "" {
		t.Fatalf("unexpected instructions: %q", resp.Instructions)
	}
	if len(resp.Input) != 2 || resp.Input[0].Role != "developer" || len(resp.Input[0].Content) != 1 || resp.Input[0].Content[0].Text != "SYS" {
		t.Fatalf("unexpected input: %+v", resp.Input)
	}
}

func TestGatherMessages_RejectsMissingAssistantToolCallID(t *testing.T) {
	_, err := gatherMessages([]chatMessageInput{
		{Role: "user", Content: "hi"},
		{
			Role: "assistant",
			ToolCalls: []chatToolCall{
				{
					Type: "function",
					Function: &chatToolFunction{
						Name:      "lookup",
						Arguments: `{}`,
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestGatherMessages_RejectsUnknownToolCallReference(t *testing.T) {
	_, err := gatherMessages([]chatMessageInput{
		{Role: "user", Content: "hi"},
		{Role: "tool", ToolCallID: "call_1", Content: "done"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildResponsesRequest_RejectsInvalidReasoningEffort(t *testing.T) {
	h := &APIHandler{}
	req := &chatCompletionsRequest{
		Model:           "gpt-5.2",
		ReasoningEffort: "invalid",
		Messages: []chatMessageInput{
			{Role: "user", Content: "hi"},
		},
	}

	if _, err := h.buildResponsesRequest(req, false); err == nil {
		t.Fatalf("expected error")
	}
}

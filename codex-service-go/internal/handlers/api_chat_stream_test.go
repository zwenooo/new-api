package handlers

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func makeSSE(body string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
	}
}

func makeJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
	}
}

func TestAggregateResponsesToChat_ThinkTags(t *testing.T) {
	// Minimal SSE with output_text deltas + reasoning.summary + completed usage
	sse := "" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello \",\"response\":{\"id\":\"resp_1\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\",\"response\":{\"id\":\"resp_1\"}}\n\n" +
		"event: reasoning.summary\n" +
		"data: {\"type\":\"reasoning.summary\",\"summary\":\"thinking...\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n"

	resp := makeSSE(sse)
	h := &APIHandler{}
	combined, err := h.aggregateResponsesToChat(resp, "gpt-5", "think-tags")
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	if combined == nil || len(combined.Choices) == 0 {
		t.Fatalf("no choices in aggregated response")
	}
	msg := combined.Choices[0].Message
	if msg.Content == nil {
		t.Fatalf("content is nil")
	}
	got := *msg.Content
	if got != "<think>thinking...</think>Hello world" {
		t.Fatalf("unexpected content: %q", got)
	}
	if combined.Usage == nil || combined.Usage.PromptTokens != 10 || combined.Usage.CompletionTokens != 5 || combined.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", combined.Usage)
	}
}

func TestAggregateResponsesToChat_ReasoningField_Both(t *testing.T) {
	sse := "" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\",\"response\":{\"id\":\"resp_2\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"!\",\"response\":{\"id\":\"resp_2\"}}\n\n" +
		"event: reasoning.summary\n" +
		"data: {\"type\":\"reasoning.summary\",\"summary\":\"think\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n"

	resp := makeSSE(sse)
	h := &APIHandler{}
	combined, err := h.aggregateResponsesToChat(resp, "gpt-5", "both")
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	msg := combined.Choices[0].Message
	if msg.Content == nil || *msg.Content != "<think>think</think>Hello!" {
		t.Fatalf("unexpected content: %v", msg.Content)
	}
	if strings.Contains(msg.Reasoning, "<think>") {
		t.Fatalf("reasoning field should not include think tags")
	}
	if msg.Reasoning != "think" {
		t.Fatalf("unexpected reasoning: %q", msg.Reasoning)
	}
}

func TestAggregateResponsesToChat_NoDuplicateOutputTextDone(t *testing.T) {
	sse := "" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hello \",\"response\":{\"id\":\"resp_3\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"world\",\"response\":{\"id\":\"resp_3\"}}\n\n" +
		"event: response.output_text.done\n" +
		"data: {\"type\":\"response.output_text.done\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"text\":\"Hello world\",\"response\":{\"id\":\"resp_3\"}}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_3\"}}\n\n"

	resp := makeSSE(sse)
	h := &APIHandler{}
	combined, err := h.aggregateResponsesToChat(resp, "gpt-5", "")
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	if combined == nil || len(combined.Choices) == 0 {
		t.Fatalf("no choices in aggregated response")
	}
	msg := combined.Choices[0].Message
	if msg.Content == nil {
		t.Fatalf("content is nil")
	}
	if got := *msg.Content; got != "Hello world" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestAggregateResponsesToChat_MessageItemDoneFallback(t *testing.T) {
	sse := "" +
		"event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_4\"}}\n\n" +
		"event: response.output_item.done\n" +
		"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"id\":\"msg_4\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello from item\"}]}}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_4\"}}\n\n"

	resp := makeSSE(sse)
	h := &APIHandler{}
	combined, err := h.aggregateResponsesToChat(resp, "gpt-5", "")
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	if combined == nil || len(combined.Choices) == 0 {
		t.Fatalf("no choices in aggregated response")
	}
	msg := combined.Choices[0].Message
	if msg.Content == nil {
		t.Fatalf("content is nil")
	}
	if got := *msg.Content; got != "Hello from item" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestAggregateResponsesResultToChat_JSONResponse(t *testing.T) {
	body := `{
		"id":"resp_json_1",
		"output":[
			{"type":"reasoning","summary":[{"text":"think-json"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello json"}]},
			{"type":"function_call","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"pwd\"}"}
		],
		"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}
	}`

	resp := makeJSONResponse(body)
	h := &APIHandler{}
	combined, err := h.aggregateResponsesResultToChat(resp, "gpt-5", "both")
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	if combined == nil || len(combined.Choices) == 0 {
		t.Fatalf("no choices in aggregated response")
	}
	msg := combined.Choices[0].Message
	if msg.Content == nil || *msg.Content != "<think>think-json</think>Hello json" {
		t.Fatalf("unexpected content: %v", msg.Content)
	}
	if msg.Reasoning != "think-json" {
		t.Fatalf("unexpected reasoning: %q", msg.Reasoning)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_1" || msg.ToolCalls[0].Function == nil || msg.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("unexpected tool calls: %+v", msg.ToolCalls)
	}
	if combined.Usage == nil || combined.Usage.PromptTokens != 2 || combined.Usage.CompletionTokens != 3 || combined.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %+v", combined.Usage)
	}
}

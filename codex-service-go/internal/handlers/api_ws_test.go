package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestBuildResponsesWebSocketSyntheticPrewarm(t *testing.T) {
	state, payloads, err := buildResponsesWebSocketSyntheticPrewarm([]byte(`{"type":"response.create","model":"gpt-5.2-codex","generate":false,"instructions":"SYS"}`))
	if err != nil {
		t.Fatalf("buildResponsesWebSocketSyntheticPrewarm error: %v", err)
	}
	if state == nil {
		t.Fatal("expected synthetic prewarm state")
	}
	if state.responseID == "" {
		t.Fatal("expected synthetic response id")
	}
	if state.model != "gpt-5.2-codex" {
		t.Fatalf("unexpected prewarm model: %q", state.model)
	}
	if got := string(state.instructions); got != `"SYS"` {
		t.Fatalf("unexpected prewarm instructions: %s", got)
	}
	if len(payloads) != 2 {
		t.Fatalf("unexpected synthetic payload count: %d", len(payloads))
	}

	var created struct {
		Type     string `json:"type"`
		Response struct {
			ID     string `json:"id"`
			Model  string `json:"model"`
			Status string `json:"status"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payloads[0], &created); err != nil {
		t.Fatalf("unmarshal created payload: %v", err)
	}
	if created.Type != "response.created" {
		t.Fatalf("unexpected created payload type: %q", created.Type)
	}
	if created.Response.ID != state.responseID {
		t.Fatalf("unexpected created response id: %q", created.Response.ID)
	}
	if created.Response.Model != state.model {
		t.Fatalf("unexpected created response model: %q", created.Response.Model)
	}
	if created.Response.Status != "in_progress" {
		t.Fatalf("unexpected created response status: %q", created.Response.Status)
	}

	var completed struct {
		Type     string `json:"type"`
		Response struct {
			ID     string `json:"id"`
			Model  string `json:"model"`
			Status string `json:"status"`
			Usage  struct {
				TotalTokens int `json:"total_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payloads[1], &completed); err != nil {
		t.Fatalf("unmarshal completed payload: %v", err)
	}
	if completed.Type != "response.completed" {
		t.Fatalf("unexpected completed payload type: %q", completed.Type)
	}
	if completed.Response.ID != state.responseID {
		t.Fatalf("unexpected completed response id: %q", completed.Response.ID)
	}
	if completed.Response.Model != state.model {
		t.Fatalf("unexpected completed response model: %q", completed.Response.Model)
	}
	if completed.Response.Status != "completed" {
		t.Fatalf("unexpected completed response status: %q", completed.Response.Status)
	}
	if completed.Response.Usage.TotalTokens != 0 {
		t.Fatalf("unexpected completed total tokens: %d", completed.Response.Usage.TotalTokens)
	}
}

func TestRewriteResponsesWebSocketSyntheticPrewarmFollowUp(t *testing.T) {
	state := &responsesWebSocketSyntheticPrewarm{
		responseID:   "resp_prewarm_test",
		model:        "gpt-5.2-codex",
		instructions: json.RawMessage(`"SYS"`),
	}

	rewritten, matched, err := rewriteResponsesWebSocketSyntheticPrewarmFollowUp([]byte(`{"type":"response.create","previous_response_id":"resp_prewarm_test","input":[]}`), state)
	if err != nil {
		t.Fatalf("rewriteResponsesWebSocketSyntheticPrewarmFollowUp error: %v", err)
	}
	if !matched {
		t.Fatal("expected synthetic prewarm follow-up to match")
	}

	var raw map[string]any
	if err := json.Unmarshal(rewritten, &raw); err != nil {
		t.Fatalf("unmarshal rewritten payload: %v", err)
	}
	if _, ok := raw["previous_response_id"]; ok {
		t.Fatalf("unexpected previous_response_id in rewritten payload: %s", string(rewritten))
	}
	if _, ok := raw["generate"]; ok {
		t.Fatalf("unexpected generate in rewritten payload: %s", string(rewritten))
	}
	if got, _ := raw["model"].(string); got != state.model {
		t.Fatalf("unexpected rewritten model: %q", got)
	}
	if got, _ := raw["instructions"].(string); got != "SYS" {
		t.Fatalf("unexpected rewritten instructions: %#v", raw["instructions"])
	}
}

func TestRewriteResponsesWebSocketSyntheticPrewarmFollowUp_NoMatch(t *testing.T) {
	state := &responsesWebSocketSyntheticPrewarm{
		responseID:   "resp_prewarm_test",
		model:        "gpt-5.2-codex",
		instructions: json.RawMessage(`"SYS"`),
	}

	original := []byte(`{"type":"response.create","previous_response_id":"resp_real","input":[]}`)
	rewritten, matched, err := rewriteResponsesWebSocketSyntheticPrewarmFollowUp(original, state)
	if err != nil {
		t.Fatalf("rewriteResponsesWebSocketSyntheticPrewarmFollowUp error: %v", err)
	}
	if matched {
		t.Fatal("expected non-synthetic follow-up to pass through")
	}
	if string(rewritten) != string(original) {
		t.Fatalf("unexpected rewritten payload: %s", string(rewritten))
	}
}

func TestShouldIgnoreResponsesWebSocketProxyTargetReadError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/v1/responses", nil)

	if !shouldIgnoreResponsesWebSocketProxyTargetReadError(c, io.EOF, false) {
		t.Fatal("expected io.EOF to be ignored")
	}
	if !shouldIgnoreResponsesWebSocketProxyTargetReadError(c, errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"), true) {
		t.Fatal("expected terminal-event close to be ignored")
	}
	if shouldIgnoreResponsesWebSocketProxyTargetReadError(c, errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"), false) {
		t.Fatal("did not expect non-terminal close to be ignored")
	}
}

func TestIsResponsesWebSocketTerminalPayload(t *testing.T) {
	if !isResponsesWebSocketTerminalPayload(websocket.TextMessage, []byte(`{"type":"response.completed"}`)) {
		t.Fatal("expected response.completed to be terminal")
	}
	if isResponsesWebSocketTerminalPayload(websocket.TextMessage, []byte(`{"type":"response.in_progress"}`)) {
		t.Fatal("did not expect response.in_progress to be terminal")
	}
}

func TestNewResponsesWebSocketProxyError_PreservesUpstreamStatus(t *testing.T) {
	err := newResponsesWebSocketProxyError(http.StatusTooManyRequests, []byte(`{"error":{"type":"rate_limit_error","message":"The usage limit has been reached"}}`))
	if err == nil {
		t.Fatal("expected proxy error")
	}
	if err.status != http.StatusTooManyRequests {
		t.Fatalf("unexpected status: %d", err.status)
	}
	if err.errType != "rate_limit_error" {
		t.Fatalf("unexpected error type: %q", err.errType)
	}
	if err.message != "The usage limit has been reached" {
		t.Fatalf("unexpected message: %q", err.message)
	}
}

package relay

import (
	"encoding/json"
	"testing"
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

func TestRewriteResponsesWebSocketSyntheticPrewarmFollowUpNoMatch(t *testing.T) {
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

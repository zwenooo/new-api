package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pruntime "codex-service-go/internal/proxy/runtime"
	instsvc "codex-service-go/internal/services/instances"

	"github.com/gorilla/websocket"
)

func TestBuildResponsesWebSocketSSEFrame(t *testing.T) {
	payload := []byte("{\n\"type\":\"response.completed\"\n}")
	got := string(buildResponsesWebSocketSSEFrame("response.completed", payload))
	want := "event: response.completed\ndata: {\ndata: \"type\":\"response.completed\"\ndata: }\n\n"
	if got != want {
		t.Fatalf("buildResponsesWebSocketSSEFrame() = %q, want %q", got, want)
	}
}

func TestNormalizeResponsesWebSocketRuntimePayload_RateLimitFailedEvent(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"rate_limit_exceeded","message":"Rate limit reached. Please try again in 11.054s."}}}`)

	normalized, status, ok, err := normalizeResponsesWebSocketRuntimePayload("response.failed", payload)
	if err != nil {
		t.Fatalf("normalizeResponsesWebSocketRuntimePayload returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected normalized payload")
	}
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", status, http.StatusTooManyRequests)
	}

	var body map[string]any
	if err := json.Unmarshal(normalized, &body); err != nil {
		t.Fatalf("unmarshal normalized payload: %v", err)
	}
	errorObj, ok := body["error"].(map[string]any)
	if !ok || errorObj == nil {
		t.Fatalf("normalized payload missing error object: %#v", body)
	}
	if got, _ := errorObj["type"].(string); got != "usage_limit_reached" {
		t.Fatalf("error.type = %q, want %q", got, "usage_limit_reached")
	}
	if got, _ := errorObj["resets_in_seconds"].(float64); got != 12 {
		t.Fatalf("error.resets_in_seconds = %v, want 12", errorObj["resets_in_seconds"])
	}
}

func TestNormalizeResponsesWebSocketRuntimePayload_UsageLimitMessageOnly(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"error","error":{"message":"The usage limit has been reached"}}`)

	normalized, status, ok, err := normalizeResponsesWebSocketRuntimePayload("error", payload)
	if err != nil {
		t.Fatalf("normalizeResponsesWebSocketRuntimePayload returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected normalized payload")
	}
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", status, http.StatusTooManyRequests)
	}

	var body map[string]any
	if err := json.Unmarshal(normalized, &body); err != nil {
		t.Fatalf("unmarshal normalized payload: %v", err)
	}
	errorObj, ok := body["error"].(map[string]any)
	if !ok || errorObj == nil {
		t.Fatalf("normalized payload missing error object: %#v", body)
	}
	if got, _ := errorObj["type"].(string); got != "usage_limit_reached" {
		t.Fatalf("error.type = %q, want %q", got, "usage_limit_reached")
	}
}

func TestForwardResponses_UsesHTTPByDefault(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if websocket.IsWebSocketUpgrade(r) {
			t.Errorf("unexpected websocket upgrade")
			http.Error(w, "unexpected websocket upgrade", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body failed", http.StatusInternalServerError)
			return
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("unmarshal request body: %v", err)
			http.Error(w, "unmarshal request body failed", http.StatusBadRequest)
			return
		}
		if got, ok := request["stream"].(bool); !ok || !got {
			t.Errorf("expected stream=true, got %#v", request["stream"])
			http.Error(w, "stream not normalized", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_http\"}}\n\n"))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "openai-node/4.0.0")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if got := string(body); !strings.Contains(got, "resp_http") {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func TestUseResponsesUpstreamWebSocketHonorsGlobalAndHeader(t *testing.T) {
	t.Parallel()

	svc := NewService(Options{})
	if svc.UseResponsesUpstreamWebSocket(http.Header{}) {
		t.Fatal("expected upstream websocket to be disabled by default")
	}

	headers := http.Header{}
	headers.Set(internalCx2ccUpstreamResponsesWSHeader, "true")
	if !svc.UseResponsesUpstreamWebSocket(headers) {
		t.Fatal("expected internal header to force upstream websocket")
	}

	svc.SetResponsesUpstreamWebSocketAllEnabled(true)
	if !svc.UseResponsesUpstreamWebSocket(http.Header{}) {
		t.Fatal("expected global setting to force upstream websocket")
	}
}

func TestForwardResponses_UsesUpstreamWebSocketWhenGlobalSettingEnabled(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}

		var request map[string]any
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Errorf("unmarshal websocket payload: %v", err)
			return
		}
		if got, _ := request["type"].(string); got != "response.create" {
			t.Errorf("unexpected websocket request type: %q", got)
			return
		}

		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_global_ws"}}`))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})
	svc.SetResponsesUpstreamWebSocketAllEnabled(true)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "openai-node/4.0.0")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if got := string(body); !strings.Contains(got, "resp_global_ws") {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func TestForwardResponses_UsesUpstreamWebSocketEventStreamWhenInternalHeaderSet(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get(internalCx2ccUpstreamResponsesWSHeader); got != "" {
			http.Error(w, "internal websocket header leaked upstream", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("OpenAI-Beta"); !strings.Contains(got, openAIResponsesWSBetaV2) {
			http.Error(w, "missing websocket beta header", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("OpenAI-Beta"); strings.Contains(got, "responses=experimental") {
			http.Error(w, "unexpected HTTP responses beta on websocket request", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}

		var request map[string]any
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Errorf("unmarshal websocket payload: %v", err)
			return
		}
		if got, _ := request["type"].(string); got != "response.create" {
			t.Errorf("unexpected websocket request type: %q", got)
			return
		}
		if got, ok := request["stream"].(bool); !ok || !got {
			t.Errorf("expected stream=true, got %#v", request["stream"])
			return
		}
		if got, ok := request["store"].(bool); !ok || got {
			t.Errorf("expected store=false, got %#v", request["store"])
			return
		}

		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"id":"resp_1"}}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.output_text.delta","delta":"hi"}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_1"}}`))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")
	req.Header.Set(internalCx2ccUpstreamResponsesWSHeader, "true")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read synthetic event stream: %v", err)
	}
	stream := string(body)
	for _, want := range []string{
		"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n",
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n",
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\"}}\n\n",
	} {
		if !strings.Contains(stream, want) {
			t.Fatalf("synthetic event stream missing %q in %q", want, stream)
		}
	}
}

func TestForwardResponses_UsesUpstreamWebSocketEventStreamWithV1BaseURL(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_v1_base"}}`))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL + "/v1",
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")
	req.Header.Set(internalCx2ccUpstreamResponsesWSHeader, "true")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read event stream body: %v", err)
	}
	if got := string(body); !strings.Contains(got, "resp_v1_base") {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func TestForwardResponses_DoesNotFallBackToHTTPWhenInternalHeaderSetAndHandshakeFails(t *testing.T) {
	t.Parallel()

	var websocketAttempts atomic.Int32
	var httpAttempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if websocket.IsWebSocketUpgrade(r) {
			websocketAttempts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found","success":false}`))
			return
		}

		httpAttempts.Add(1)
		if got := r.Header.Get(internalCx2ccUpstreamResponsesWSHeader); got != "" {
			t.Errorf("internal websocket header leaked upstream on fallback request: %q", got)
			http.Error(w, "internal websocket header leaked upstream", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_http_fallback\"}}\n\n"))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")
	req.Header.Set(internalCx2ccUpstreamResponsesWSHeader, "true")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	if websocketAttempts.Load() != 1 {
		t.Fatalf("websocket attempts = %d, want 1", websocketAttempts.Load())
	}
	if httpAttempts.Load() != 0 {
		t.Fatalf("http attempts = %d, want 0", httpAttempts.Load())
	}
	if got := resp.StatusCode; got != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", got, http.StatusNotFound)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read handshake response body: %v", err)
	}
	if string(body) != `{"message":"not found","success":false}` {
		t.Fatalf("unexpected handshake response body: %s", string(body))
	}
}

func TestForwardResponses_ReturnsWebSocketReadErrorWhenInternalHeaderSetAndUpstreamClosesBeforeFirstEvent(t *testing.T) {
	t.Parallel()

	var websocketAttempts atomic.Int32

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if websocket.IsWebSocketUpgrade(r) {
			websocketAttempts.Add(1)
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("upgrade websocket: %v", err)
				return
			}
			defer conn.Close()

			if _, _, err := conn.ReadMessage(); err != nil {
				t.Errorf("read websocket payload: %v", err)
				return
			}
			if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, ""), time.Now().Add(time.Second)); err != nil {
				t.Errorf("write close control: %v", err)
			}
			return
		}
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")
	req.Header.Set(internalCx2ccUpstreamResponsesWSHeader, "true")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err == nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("ForwardResponses returned nil error")
	}

	if websocketAttempts.Load() != 1 {
		t.Fatalf("websocket attempts = %d, want 1", websocketAttempts.Load())
	}
	if !strings.Contains(err.Error(), "read upstream responses websocket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForwardResponses_DoesNotFallBackToHTTPWhenGlobalSettingEnabled(t *testing.T) {
	t.Parallel()

	var websocketAttempts atomic.Int32
	var httpAttempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if websocket.IsWebSocketUpgrade(r) {
			websocketAttempts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found","success":false}`))
			return
		}

		httpAttempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_http_unexpected\"}}\n\n"))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})
	svc.SetResponsesUpstreamWebSocketAllEnabled(true)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	if websocketAttempts.Load() != 1 {
		t.Fatalf("websocket attempts = %d, want 1", websocketAttempts.Load())
	}
	if httpAttempts.Load() != 0 {
		t.Fatalf("http attempts = %d, want 0", httpAttempts.Load())
	}
	if got := resp.StatusCode; got != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", got, http.StatusNotFound)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read handshake response body: %v", err)
	}
	if string(body) != `{"message":"not found","success":false}` {
		t.Fatalf("unexpected handshake response body: %s", string(body))
	}
}

func TestOpenResponsesWebSocketEventStream_ReturnsHandshakeResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found","success":false}`))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	resp, err := svc.OpenResponsesWebSocketEventStream(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, http.Header{
		"Content-Type": []string{"application/json"},
	}, "", []byte(`{"model":"gpt-5.2","input":"hello"}`))
	if err != nil {
		t.Fatalf("OpenResponsesWebSocketEventStream returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.StatusCode; got != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", got, http.StatusNotFound)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read handshake response body: %v", err)
	}
	if string(body) != `{"message":"not found","success":false}` {
		t.Fatalf("unexpected handshake response body: %s", string(body))
	}
}

func TestOpenResponsesWebSocketEventStream_HandshakeResponseUpdatesRuntime(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"detail":{"code":"deactivated_workspace"}}`))
	}))
	defer server.Close()

	svc := newResponsesWebSocketRuntimeTestService(t, server.URL)

	resp, err := svc.OpenResponsesWebSocketEventStream(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, http.Header{
		"Content-Type": []string{"application/json"},
	}, "", []byte(`{"model":"gpt-5.2","input":"hello"}`))
	if err != nil {
		t.Fatalf("OpenResponsesWebSocketEventStream returned error: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.StatusCode; got != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want %d", got, http.StatusPaymentRequired)
	}

	block, err := svc.ShouldBlock(context.Background(), 1)
	if err != nil {
		t.Fatalf("ShouldBlock returned error: %v", err)
	}
	if !block.Blocked {
		t.Fatalf("expected runtime blocked, got %+v", block)
	}
	if got := block.Reason; got != "deactivated_workspace" {
		t.Fatalf("blocked reason = %q, want %q", got, "deactivated_workspace")
	}
}

func TestOpenResponsesWebSocketEventStream_IgnoresCloseAfterTerminalEvent(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_terminal"}}`)); err != nil {
			t.Errorf("write response.completed: %v", err)
			return
		}
		if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, ""), time.Now().Add(time.Second)); err != nil {
			t.Errorf("write close control: %v", err)
			return
		}
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})

	resp, err := svc.OpenResponsesWebSocketEventStream(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, http.Header{
		"Content-Type": []string{"application/json"},
	}, "", []byte(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	if err != nil {
		t.Fatalf("OpenResponsesWebSocketEventStream returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read event stream body: %v", err)
	}
	if got := string(body); !strings.Contains(got, "resp_terminal") {
		t.Fatalf("unexpected event stream body: %s", got)
	}
}

func TestForwardResponses_ResponseFailedUpdatesRuntime(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"deactivated_workspace","message":"workspace disabled"}}}`))
	}))
	defer server.Close()

	svc := newResponsesWebSocketRuntimeTestService(t, server.URL)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")
	req.Header.Set(internalCx2ccUpstreamResponsesWSHeader, "true")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read synthetic event stream: %v", err)
	}
	if got := string(body); !strings.Contains(got, "response.failed") {
		t.Fatalf("unexpected response body: %s", got)
	}

	block, err := svc.ShouldBlock(context.Background(), 1)
	if err != nil {
		t.Fatalf("ShouldBlock returned error: %v", err)
	}
	if !block.Blocked {
		t.Fatalf("expected runtime blocked, got %+v", block)
	}
	if got := block.Reason; got != "deactivated_workspace" {
		t.Fatalf("blocked reason = %q, want %q", got, "deactivated_workspace")
	}
}

func TestForwardResponses_ResponseCompletedClearsSleepRuntime(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_completed"}}`))
	}))
	defer server.Close()

	svc := newResponsesWebSocketRuntimeTestService(t, server.URL)
	runtime := svc.runtimeForInstance(1)
	if runtime == nil {
		t.Fatalf("expected runtime manager")
	}
	if err := runtime.RecordUsageLimit(pruntime.UsageLimitInfo{ResetsInSeconds: 60, Message: "seed"}); err != nil {
		t.Fatalf("RecordUsageLimit returned error: %v", err)
	}

	before, err := svc.ShouldBlock(context.Background(), 1)
	if err != nil {
		t.Fatalf("ShouldBlock before call returned error: %v", err)
	}
	if !before.Blocked {
		t.Fatalf("expected runtime blocked before success, got %+v", before)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "undici")
	req.Header.Set(internalCx2ccUpstreamResponsesWSHeader, "true")

	resp, err := svc.ForwardResponses(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, req)
	if err != nil {
		t.Fatalf("ForwardResponses returned error: %v", err)
	}
	defer resp.Body.Close()

	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read synthetic event stream: %v", err)
	}

	after, err := svc.ShouldBlock(context.Background(), 1)
	if err != nil {
		t.Fatalf("ShouldBlock after call returned error: %v", err)
	}
	if after.Blocked {
		t.Fatalf("expected runtime recovered after success, got %+v", after)
	}
}

func TestCallResponsesJSON_UsesUpstreamWebSocketWhenGlobalSettingEnabled(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_json_ws"}}`))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})
	svc.SetResponsesUpstreamWebSocketAllEnabled(true)

	resp, err := svc.CallResponsesJSON(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, []byte(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	if err != nil {
		t.Fatalf("CallResponsesJSON returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if got := string(body); !strings.Contains(got, "resp_json_ws") {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func TestCallResponsesStream_UsesUpstreamWebSocketWhenGlobalSettingEnabled(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_stream_ws"}}`))
	}))
	defer server.Close()

	svc := NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: server.URL,
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})
	svc.SetResponsesUpstreamWebSocketAllEnabled(true)

	resp, err := svc.CallResponsesStream(context.Background(), instsvc.InstanceWithPaths{
		Instance: instsvc.Instance{
			ID:       1,
			AuthMode: "chatgpt",
		},
		AuthPath: writeResponsesWebSocketTestAuth(t),
	}, []byte(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	if err != nil {
		t.Fatalf("CallResponsesStream returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if got := string(body); !strings.Contains(got, "resp_stream_ws") {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func writeResponsesWebSocketTestAuth(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{
  "OPENAI_API_KEY": null,
  "tokens": {
    "access_token": "test-access-token",
    "refresh_token": null,
    "id_token": "test-id-token"
  }
}
`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	return authPath
}

func newResponsesWebSocketRuntimeTestService(t *testing.T, responsesBaseURL string) *Service {
	t.Helper()

	dir := t.TempDir()
	return NewService(Options{
		ChatGPTClientID:  "app_test",
		ResponsesBaseURL: responsesBaseURL,
		RuntimeFile:      filepath.Join(dir, "runtime.json"),
		UserAgent:        "codex_cli_rs/0.0.0 (test) codex-service-go",
	})
}

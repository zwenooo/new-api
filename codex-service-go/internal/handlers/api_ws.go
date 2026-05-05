package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	instsvc "codex-service-go/internal/services/instances"
)

var responsesWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type responsesWebSocketRequestEnvelope struct {
	Type               string          `json:"type,omitempty"`
	Model              string          `json:"model,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Instructions       json.RawMessage `json:"instructions,omitempty"`
	Generate           *bool           `json:"generate,omitempty"`
}

type responsesWebSocketSyntheticPrewarm struct {
	responseID   string
	model        string
	instructions json.RawMessage
}

type responsesWebSocketProxyError struct {
	status  int
	errType string
	message string
}

func (e *responsesWebSocketProxyError) Error() string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.message)
	if msg == "" {
		msg = http.StatusText(e.status)
	}
	if msg == "" {
		msg = "upstream responses request failed"
	}
	if e.status > 0 {
		return fmt.Sprintf("upstream responses http status %d: %s", e.status, msg)
	}
	return msg
}

func (h *APIHandler) HandleResponsesWebSocket(c *gin.Context) {
	inst, ok := h.authorize(c)
	if !ok {
		return
	}
	if h.shouldBlock(c, inst) {
		return
	}

	clientConn, err := responsesWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	closeCode := websocket.CloseNormalClosure
	defer func() {
		closeResponsesWebSocketConn(clientConn, closeCode)
	}()

	var prewarm *responsesWebSocketSyntheticPrewarm
	for {
		_ = clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		firstMessageType, firstPayload, err := clientConn.ReadMessage()
		_ = clientConn.SetReadDeadline(time.Time{})
		if err != nil {
			if isResponsesWebSocketClientClose(err) {
				return
			}
			closeCode = websocket.ClosePolicyViolation
			h.writeResponsesWSError(clientConn, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		trimmedPayload, err := normalizeResponsesWebSocketPayload(firstPayload)
		if err != nil {
			closeCode = websocket.ClosePolicyViolation
			h.writeResponsesWSError(clientConn, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		if prewarm == nil && shouldHandleResponsesWebSocketPrewarmLocally(trimmedPayload) {
			nextPrewarm, syntheticPayloads, err := buildResponsesWebSocketSyntheticPrewarm(trimmedPayload)
			if err != nil {
				closeCode = websocket.ClosePolicyViolation
				h.writeResponsesWSError(clientConn, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			prewarm = nextPrewarm
			for i := 0; i < len(syntheticPayloads); i++ {
				if err := clientConn.WriteMessage(websocket.TextMessage, syntheticPayloads[i]); err != nil {
					return
				}
			}
			continue
		}

		if prewarm != nil {
			rewrittenPayload, rewritten, err := rewriteResponsesWebSocketSyntheticPrewarmFollowUp(trimmedPayload, prewarm)
			if err != nil {
				closeCode = websocket.ClosePolicyViolation
				h.writeResponsesWSError(clientConn, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			if rewritten {
				trimmedPayload = rewrittenPayload
				prewarm = nil
			}
		}

		if h.proxy.UseResponsesUpstreamWebSocket(c.Request.Header) {
			targetConn, normalizedFirstPayload, err := h.proxy.OpenResponsesWebSocket(c.Request.Context(), *inst, c.Request.Header, c.Request.URL.RawQuery, trimmedPayload)
			if err != nil {
				closeCode = websocket.CloseInternalServerErr
				h.writeResponsesWSError(clientConn, http.StatusBadGateway, "upstream_error", err.Error())
				return
			}
			defer targetConn.Close()

			if isLocalCxPoolTestRequest(c) && h.proxy != nil {
				_ = h.proxy.RecoverInstanceForLocalTestSuccess(inst.ID)
			}

			if err := targetConn.WriteMessage(firstMessageType, normalizedFirstPayload); err != nil {
				closeCode = websocket.CloseInternalServerErr
				h.writeResponsesWSError(clientConn, http.StatusBadGateway, "upstream_error", err.Error())
				return
			}

			h.appendLog(inst.LogPath, "["+time.Now().Format(time.RFC3339)+"] WS /v1/responses connected (upstream=websocket)")
			if err := h.proxyResponsesWebSocket(c, inst, clientConn, targetConn); err != nil {
				closeCode = websocket.CloseInternalServerErr
				h.appendLog(inst.LogPath, "["+time.Now().Format(time.RFC3339)+"] WS /v1/responses error: "+err.Error())
				writeResponsesWebSocketProxyError(clientConn, err)
				return
			}
			h.appendLog(inst.LogPath, "["+time.Now().Format(time.RFC3339)+"] WS /v1/responses closed")
			return
		}

		h.appendLog(inst.LogPath, "["+time.Now().Format(time.RFC3339)+"] WS /v1/responses connected (upstream=https)")
		if err := h.proxyResponsesWebSocketViaHTTP(c, inst, clientConn, trimmedPayload); err != nil {
			closeCode = websocket.CloseInternalServerErr
			h.appendLog(inst.LogPath, "["+time.Now().Format(time.RFC3339)+"] WS /v1/responses error: "+err.Error())
			writeResponsesWebSocketProxyError(clientConn, err)
			return
		}
		if isLocalCxPoolTestRequest(c) && h.proxy != nil {
			_ = h.proxy.RecoverInstanceForLocalTestSuccess(inst.ID)
		}
		h.appendLog(inst.LogPath, "["+time.Now().Format(time.RFC3339)+"] WS /v1/responses round closed")
	}
}

func (h *APIHandler) proxyResponsesWebSocket(c *gin.Context, inst *instsvc.InstanceWithPaths, clientConn *websocket.Conn, targetConn *websocket.Conn) error {
	errChan := make(chan error, 2)
	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	var terminalEventSeen atomic.Bool
	var closeClientOnce sync.Once
	var closeTargetOnce sync.Once

	go func() {
		for {
			messageType, payload, err := clientConn.ReadMessage()
			if err != nil {
				closeClientOnce.Do(func() {
					close(clientClosed)
				})
				return
			}
			normalizedPayload, err := h.proxy.PrepareResponsesWebSocketPayload(*inst, c.Request.Header, payload)
			if err != nil {
				errChan <- err
				return
			}
			if err := targetConn.WriteMessage(messageType, normalizedPayload); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		for {
			messageType, payload, err := targetConn.ReadMessage()
			if err != nil {
				if shouldIgnoreResponsesWebSocketProxyTargetReadError(c, err, terminalEventSeen.Load()) {
					closeTargetOnce.Do(func() {
						close(targetClosed)
					})
					return
				}
				errChan <- err
				return
			}
			if isResponsesWebSocketTerminalPayload(messageType, payload) {
				terminalEventSeen.Store(true)
			}
			if err := clientConn.WriteMessage(messageType, payload); err != nil {
				closeClientOnce.Do(func() {
					close(clientClosed)
				})
				return
			}
		}
	}()

	for {
		select {
		case err := <-errChan:
			return err
		case <-clientClosed:
			return nil
		case <-targetClosed:
			return nil
		case <-c.Request.Context().Done():
			return nil
		}
	}
}

func (h *APIHandler) proxyResponsesWebSocketViaHTTP(c *gin.Context, inst *instsvc.InstanceWithPaths, clientConn *websocket.Conn, firstPayload []byte) error {
	normalizedPayload, err := h.proxy.PrepareResponsesWebSocketPayload(*inst, c.Request.Header, firstPayload)
	if err != nil {
		return err
	}

	reqURL := &url.URL{
		Path:     "/v1/responses",
		RawQuery: c.Request.URL.RawQuery,
	}
	headers := c.Request.Header.Clone()
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "text/event-stream")
	req := &http.Request{
		Method: http.MethodPost,
		URL:    reqURL,
		Header: headers,
		Body:   io.NopCloser(bytes.NewReader(normalizedPayload)),
	}

	resp, err := h.proxy.ForwardResponses(c.Request.Context(), *inst, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return newResponsesWebSocketProxyError(resp.StatusCode, body)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var payloadBuffer bytes.Buffer
	flushPayload := func() error {
		trimmed := bytes.TrimSpace(payloadBuffer.Bytes())
		payloadBuffer.Reset()
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
			return nil
		}
		if err := clientConn.WriteMessage(websocket.TextMessage, trimmed); err != nil {
			if isResponsesWebSocketClientClose(err) {
				return nil
			}
			return err
		}
		return nil
	}

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			if err := flushPayload(); err != nil {
				return err
			}
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		part := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if payloadBuffer.Len() > 0 {
			_ = payloadBuffer.WriteByte('\n')
		}
		_, _ = payloadBuffer.Write(part)
		if isResponsesWebSocketTerminalPayload(websocket.TextMessage, bytes.TrimSpace(payloadBuffer.Bytes())) {
			if err := flushPayload(); err != nil {
				return err
			}
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if c.Request.Context().Err() != nil || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return flushPayload()
}

func (h *APIHandler) writeResponsesWSError(ws *websocket.Conn, status int, errType string, message string) {
	if ws == nil {
		return
	}
	payload, _ := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
			"code":    status,
		},
	})
	_ = ws.WriteMessage(websocket.TextMessage, payload)
}

func writeResponsesWebSocketProxyError(ws *websocket.Conn, err error) {
	if ws == nil || err == nil {
		return
	}
	var proxyErr *responsesWebSocketProxyError
	if errors.As(err, &proxyErr) && proxyErr != nil {
		status := proxyErr.status
		if status <= 0 {
			status = http.StatusBadGateway
		}
		errType := strings.TrimSpace(proxyErr.errType)
		if errType == "" {
			errType = defaultResponsesWebSocketErrorType(status)
		}
		message := strings.TrimSpace(proxyErr.message)
		if message == "" {
			message = proxyErr.Error()
		}
		payload, _ := json.Marshal(gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": message,
				"code":    status,
			},
		})
		_ = ws.WriteMessage(websocket.TextMessage, payload)
		return
	}
	payload, _ := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "upstream_error",
			"message": strings.TrimSpace(err.Error()),
			"code":    http.StatusBadGateway,
		},
	})
	_ = ws.WriteMessage(websocket.TextMessage, payload)
}

func newResponsesWebSocketProxyError(status int, body []byte) *responsesWebSocketProxyError {
	errType, message := parseResponsesWebSocketProxyErrorDetails(status, body)
	return &responsesWebSocketProxyError{
		status:  status,
		errType: errType,
		message: message,
	}
}

func parseResponsesWebSocketProxyErrorDetails(status int, body []byte) (string, string) {
	errType := defaultResponsesWebSocketErrorType(status)
	message := strings.TrimSpace(string(bytes.TrimSpace(body)))

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		if message == "" {
			message = strings.TrimSpace(http.StatusText(status))
		}
		return errType, message
	}

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		if message == "" {
			message = strings.TrimSpace(http.StatusText(status))
		}
		return errType, message
	}

	if nested, ok := payload["error"].(map[string]any); ok && nested != nil {
		if candidate := strings.TrimSpace(responsesWebSocketAnyString(nested["type"])); candidate != "" {
			errType = candidate
		}
		if candidate := strings.TrimSpace(responsesWebSocketAnyString(nested["message"])); candidate != "" {
			message = candidate
		}
		if message == "" {
			if candidate := strings.TrimSpace(responsesWebSocketAnyString(nested["code"])); candidate != "" {
				message = candidate
			}
		}
		return errType, message
	}

	if candidate := strings.TrimSpace(responsesWebSocketAnyString(payload["message"])); candidate != "" {
		message = candidate
	}
	if message == "" {
		if candidate := strings.TrimSpace(responsesWebSocketAnyString(payload["msg"])); candidate != "" {
			message = candidate
		}
	}
	if message == "" {
		if candidate := strings.TrimSpace(responsesWebSocketAnyString(payload["error_msg"])); candidate != "" {
			message = candidate
		}
	}
	if message == "" {
		message = strings.TrimSpace(http.StatusText(status))
	}
	return errType, message
}

func defaultResponsesWebSocketErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func responsesWebSocketAnyString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func normalizeResponsesWebSocketPayload(payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, errors.New("websocket request payload is empty")
	}
	if !json.Valid(trimmed) {
		return nil, errors.New("websocket request payload is invalid")
	}
	return trimmed, nil
}

func shouldHandleResponsesWebSocketPrewarmLocally(payload []byte) bool {
	var envelope responsesWebSocketRequestEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return false
	}
	switch strings.TrimSpace(envelope.Type) {
	case "", "response.create":
	default:
		return false
	}
	return envelope.Generate != nil && !*envelope.Generate
}

func buildResponsesWebSocketSyntheticPrewarm(payload []byte) (*responsesWebSocketSyntheticPrewarm, [][]byte, error) {
	var envelope responsesWebSocketRequestEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, nil, err
	}

	modelName := strings.TrimSpace(envelope.Model)
	if modelName == "" {
		return nil, nil, errors.New("model is required")
	}

	state := &responsesWebSocketSyntheticPrewarm{
		responseID: "resp_prewarm_" + uuid.NewString(),
		model:      modelName,
	}
	if trimmedInstructions := bytes.TrimSpace(envelope.Instructions); len(trimmedInstructions) > 0 {
		state.instructions = bytes.Clone(trimmedInstructions)
	}

	createdAt := time.Now().Unix()
	createdPayload, err := json.Marshal(gin.H{
		"type":            "response.created",
		"sequence_number": 0,
		"response": gin.H{
			"id":                  state.responseID,
			"object":              "response",
			"created_at":          createdAt,
			"model":               modelName,
			"status":              "in_progress",
			"background":          false,
			"error":               nil,
			"output":              []any{},
			"parallel_tool_calls": true,
		},
	})
	if err != nil {
		return nil, nil, err
	}

	completedPayload, err := json.Marshal(gin.H{
		"type":            "response.completed",
		"sequence_number": 1,
		"response": gin.H{
			"id":                  state.responseID,
			"object":              "response",
			"created_at":          createdAt,
			"model":               modelName,
			"status":              "completed",
			"background":          false,
			"error":               nil,
			"output":              []any{},
			"parallel_tool_calls": true,
			"usage": gin.H{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	return state, [][]byte{createdPayload, completedPayload}, nil
}

func rewriteResponsesWebSocketSyntheticPrewarmFollowUp(payload []byte, state *responsesWebSocketSyntheticPrewarm) ([]byte, bool, error) {
	if state == nil {
		return payload, false, nil
	}

	var envelope responsesWebSocketRequestEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(envelope.PreviousResponseID) != strings.TrimSpace(state.responseID) {
		return payload, false, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, false, err
	}

	delete(raw, "previous_response_id")
	delete(raw, "generate")

	if strings.TrimSpace(envelope.Model) == "" {
		raw["model"] = state.model
	}
	if len(bytes.TrimSpace(envelope.Instructions)) == 0 && len(state.instructions) > 0 {
		var instructions any
		if err := json.Unmarshal(state.instructions, &instructions); err != nil {
			return nil, false, err
		}
		raw["instructions"] = instructions
	}

	rewritten, err := json.Marshal(raw)
	if err != nil {
		return nil, false, err
	}
	return rewritten, true, nil
}

func closeResponsesWebSocketConn(ws *websocket.Conn, closeCode int) {
	if ws == nil {
		return
	}
	if closeCode == 0 {
		closeCode = websocket.CloseNormalClosure
	}
	_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, ""), time.Now().Add(time.Second))
	_ = ws.Close()
}

func shouldIgnoreResponsesWebSocketProxyTargetReadError(c *gin.Context, err error, terminalEventSeen bool) bool {
	if terminalEventSeen {
		return true
	}
	if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived)
}

func isResponsesWebSocketTerminalPayload(messageType int, payload []byte) bool {
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return false
	}

	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return false
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return false
	}

	switch strings.TrimSpace(envelope.Type) {
	case "response.completed", "response.done", "response.incomplete", "response.failed", "error":
		return true
	default:
		return false
	}
}

func isResponsesWebSocketClientClose(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived)
}

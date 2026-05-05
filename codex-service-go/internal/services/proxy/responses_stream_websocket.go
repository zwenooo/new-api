package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	instsvc "codex-service-go/internal/services/instances"

	"github.com/gorilla/websocket"
)

var responsesWebSocketRetryAfterPattern = regexp.MustCompile(`(?i)try again in\s*(\d+(?:\.\d+)?)\s*(s|ms|seconds?)`)

func (s *Service) OpenResponsesWebSocketEventStream(ctx context.Context, inst instsvc.InstanceWithPaths, original http.Header, rawQuery string, firstPayload []byte) (*http.Response, error) {
	finishDial := s.beginRequestStage(ctx, "upstream", "CONNECT", "responses_websocket")
	targetConn, handshakeResp, normalizedFirstPayload, err := s.openResponsesWebSocketWithHandshake(ctx, inst, original, rawQuery, firstPayload)
	if err != nil {
		status := http.StatusBadGateway
		if handshakeResp != nil {
			status = handshakeResp.StatusCode
		}
		finishDial(status, err, nil)
		if handshakeResp != nil {
			return s.postProcessResponse(handshakeResp, inst.ID)
		}
		return nil, err
	}
	finishDial(http.StatusSwitchingProtocols, nil, nil)

	finishWrite := s.beginRequestStage(ctx, "upstream", "WRITE", "responses_websocket.create")
	if err := targetConn.WriteMessage(websocket.TextMessage, normalizedFirstPayload); err != nil {
		finishWrite(http.StatusBadGateway, err, nil)
		_ = targetConn.Close()
		return nil, err
	}
	finishWrite(http.StatusOK, nil, map[string]any{"bytes": len(normalizedFirstPayload)})

	firstFrame, firstEventType, firstEventPayload, terminal, err := readFirstResponsesWebSocketEvent(ctx, targetConn)
	if err != nil {
		_ = targetConn.Close()
		return nil, err
	}
	s.syncRuntimeFromResponsesWebSocketEvent(inst.ID, firstEventType, firstEventPayload)

	pipeReader, pipeWriter := io.Pipe()
	if terminal {
		_ = targetConn.Close()
		_ = pipeWriter.Close()
	} else {
		go func() {
			<-ctx.Done()
			_ = targetConn.Close()
		}()
		go s.bridgeResponsesWebSocketEventStream(ctx, inst.ID, targetConn, pipeWriter)
	}

	body := io.NopCloser(io.MultiReader(bytes.NewReader(firstFrame), pipeReader))
	return buildSyntheticResponsesEventStreamResponse(body), nil
}

func (s *Service) bridgeResponsesWebSocketEventStream(ctx context.Context, instanceID int64, targetConn *websocket.Conn, pipeWriter *io.PipeWriter) {
	var closeErr error
	terminalEventSeen := false
	defer func() {
		_ = targetConn.Close()
		if closeErr != nil {
			_ = pipeWriter.CloseWithError(closeErr)
			return
		}
		_ = pipeWriter.Close()
	}()

	for {
		messageType, message, err := targetConn.ReadMessage()
		if err != nil {
			if shouldIgnoreResponsesWebSocketReadError(ctx, err, terminalEventSeen) {
				return
			}
			closeErr = fmt.Errorf("read upstream responses websocket: %w", err)
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		eventType := extractResponsesWebSocketEventType(message)
		if isResponsesWebSocketTerminalEvent(eventType) {
			terminalEventSeen = true
			s.syncRuntimeFromResponsesWebSocketEvent(instanceID, eventType, message)
		}
		frame := buildResponsesWebSocketSSEFrame(eventType, message)
		if len(frame) > 0 {
			if _, err := pipeWriter.Write(frame); err != nil {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
				closeErr = fmt.Errorf("write synthetic responses event stream: %w", err)
				return
			}
		}
		if isResponsesWebSocketTerminalEvent(eventType) {
			return
		}
	}
}

func shouldIgnoreResponsesWebSocketReadError(ctx context.Context, err error, terminalEventSeen bool) bool {
	if terminalEventSeen {
		return true
	}
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	return websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	)
}

func readFirstResponsesWebSocketEvent(ctx context.Context, targetConn *websocket.Conn) ([]byte, string, []byte, bool, error) {
	for {
		messageType, message, err := targetConn.ReadMessage()
		if err != nil {
			if shouldIgnoreResponsesWebSocketReadError(ctx, err, false) {
				return nil, "", nil, false, io.EOF
			}
			return nil, "", nil, false, fmt.Errorf("read upstream responses websocket: %w", err)
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		eventType := extractResponsesWebSocketEventType(message)
		frame := buildResponsesWebSocketSSEFrame(eventType, message)
		if len(frame) == 0 {
			if isResponsesWebSocketTerminalEvent(eventType) {
				return nil, eventType, message, true, io.EOF
			}
			continue
		}
		return frame, eventType, message, isResponsesWebSocketTerminalEvent(eventType), nil
	}
}

func buildSyntheticResponsesEventStreamResponse(body io.ReadCloser) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Header:        header,
		Body:          body,
		ContentLength: -1,
	}
}

func extractResponsesWebSocketEventType(payload []byte) string {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return ""
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Type)
}

func buildResponsesWebSocketSSEFrame(eventType string, payload []byte) []byte {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}

	var frame bytes.Buffer
	if strings.TrimSpace(eventType) != "" {
		frame.WriteString("event: ")
		frame.WriteString(strings.TrimSpace(eventType))
		frame.WriteByte('\n')
	}

	lines := bytes.Split(trimmed, []byte("\n"))
	for i := 0; i < len(lines); i++ {
		frame.WriteString("data: ")
		frame.Write(bytes.TrimRight(lines[i], "\r"))
		frame.WriteByte('\n')
	}
	frame.WriteByte('\n')
	return frame.Bytes()
}

func isResponsesWebSocketTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.incomplete", "response.failed", "error":
		return true
	default:
		return false
	}
}

func (s *Service) syncRuntimeFromResponsesWebSocketEvent(instanceID int64, eventType string, payload []byte) {
	if s == nil || instanceID <= 0 {
		return
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return
	}

	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done":
		if err := runtime.ClearSleepStatus(); err != nil {
			s.logDebug("runtime clear error: %v", err)
		}
		return
	}

	normalizedPayload, status, ok, err := normalizeResponsesWebSocketRuntimePayload(eventType, payload)
	if err != nil {
		s.logDebug("runtime websocket normalize error: %v", err)
	}
	if ok {
		if err := runtime.SyncFromPayload(status, "application/json", normalizedPayload); err != nil {
			s.logDebug("runtime websocket sync error: %v", err)
		}
	}
	s.syncRuntimeFromCxPoolStateKeywords(runtime, nil, payload)
}

func normalizeResponsesWebSocketRuntimePayload(eventType string, payload []byte) ([]byte, int, bool, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, 0, false, nil
	}
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done":
		return nil, 0, false, nil
	}

	var event map[string]interface{}
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return nil, 0, false, err
	}

	errorObj := extractResponsesWebSocketErrorObject(event)
	if errorObj == nil {
		return nil, 0, false, nil
	}

	normalizedError := cloneResponsesWebSocketJSONObject(errorObj)
	code := strings.ToLower(strings.TrimSpace(responsesWebSocketString(normalizedError["code"])))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(responsesWebSocketString(normalizedError["error_code"])))
	}
	typ := strings.ToLower(strings.TrimSpace(responsesWebSocketString(normalizedError["type"])))
	if typ == "" {
		typ = strings.ToLower(strings.TrimSpace(responsesWebSocketString(normalizedError["error_type"])))
	}
	messageLower := strings.ToLower(strings.TrimSpace(responsesWebSocketString(normalizedError["message"])))
	if code == "rate_limit_exceeded" {
		if seconds := parseResponsesWebSocketRetryAfterSeconds(responsesWebSocketString(normalizedError["message"])); seconds > 0 {
			normalizedError["type"] = "usage_limit_reached"
			normalizedError["resets_in_seconds"] = seconds
			typ = "usage_limit_reached"
		}
	}
	if (typ == "" || code == "") && isResponsesWebSocketUsageLimitMessage(messageLower) {
		if typ == "" {
			normalizedError["type"] = "usage_limit_reached"
			typ = "usage_limit_reached"
		}
		if code == "" {
			normalizedError["code"] = "usage_limit_reached"
			code = "usage_limit_reached"
		}
	}

	normalized := map[string]interface{}{
		"error": normalizedError,
	}
	status := http.StatusBadRequest
	switch {
	case code == "payment_required" || code == "deactivated_workspace":
		normalized["detail"] = map[string]interface{}{"code": code}
		status = http.StatusPaymentRequired
	case typ == "usage_limit_reached" || code == "rate_limit_exceeded":
		status = http.StatusTooManyRequests
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, 0, false, err
	}
	return data, status, true, nil
}

func isResponsesWebSocketUsageLimitMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	return strings.Contains(message, "usage limit") || strings.Contains(message, "rate limit")
}

func extractResponsesWebSocketErrorObject(event map[string]interface{}) map[string]interface{} {
	if event == nil {
		return nil
	}
	if errObj, ok := event["error"].(map[string]interface{}); ok && errObj != nil {
		return errObj
	}
	response, ok := event["response"].(map[string]interface{})
	if !ok || response == nil {
		return nil
	}
	if errObj, ok := response["error"].(map[string]interface{}); ok && errObj != nil {
		return errObj
	}
	return nil
}

func cloneResponsesWebSocketJSONObject(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func responsesWebSocketString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func parseResponsesWebSocketRetryAfterSeconds(message string) int {
	message = strings.TrimSpace(message)
	if message == "" {
		return 0
	}
	matches := responsesWebSocketRetryAfterPattern.FindStringSubmatch(message)
	if len(matches) != 3 {
		return 0
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil || value <= 0 {
		return 0
	}
	unit := strings.ToLower(strings.TrimSpace(matches[2]))
	switch {
	case unit == "ms":
		return int(math.Ceil(value / 1000))
	case unit == "s", strings.HasPrefix(unit, "second"):
		return int(math.Ceil(value))
	default:
		return 0
	}
}

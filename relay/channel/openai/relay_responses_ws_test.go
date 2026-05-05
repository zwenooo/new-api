package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/types"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestShouldIgnoreResponsesWebSocketTargetReadErrorAfterTerminalEvent(t *testing.T) {
	if !shouldIgnoreResponsesWebSocketTargetReadError(nil, io.ErrUnexpectedEOF, true) {
		t.Fatal("expected terminal-event EOF to be ignored")
	}
}

func TestShouldIgnoreResponsesWebSocketTargetReadErrorWhenTerminalReasonAlreadySet(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")

	if !shouldIgnoreResponsesWebSocketTargetReadError(c, errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"), false) {
		t.Fatal("expected close after terminal stream_exit_reason to be ignored")
	}
}

func TestIsResponsesWebSocketTerminalEvent(t *testing.T) {
	if !isResponsesWebSocketTerminalEvent(websocket.TextMessage, []byte(`{"type":"response.completed"}`)) {
		t.Fatal("expected response.completed to be terminal")
	}
	if isResponsesWebSocketTerminalEvent(websocket.TextMessage, []byte(`{"type":"response.in_progress"}`)) {
		t.Fatal("did not expect response.in_progress to be terminal")
	}
}

func TestGetResponsesWSRoundSettleFnAcceptsAnonymousFunc(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(responsesWSRoundSettleFnCtxKey, func(info *relaycommon.RelayInfo, usage *dto.Usage) *types.NewAPIError {
		return nil
	})

	if getResponsesWSRoundSettleFn(c) == nil {
		t.Fatal("expected anonymous settle callback to be accepted")
	}
}

func TestResponsesWSSessionRejectsConcurrentRounds(t *testing.T) {
	session := &responsesWSSession{
		baseInfo: &relaycommon.RelayInfo{},
		current: &responsesWSRound{
			info: &relaycommon.RelayInfo{},
		},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	_, apiErr := session.startNextRound(
		c,
		&dto.OpenAIResponsesRequest{Model: "gpt-5.2", Stream: true},
		websocket.TextMessage,
		[]byte(`{"type":"response.create"}`),
	)
	if apiErr == nil {
		t.Fatal("expected concurrent round to be rejected")
	}
	if apiErr.StatusCode != 400 {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
}

func TestResponsesWSSessionFinishCurrentRoundClearsRoundOnSettlementError(t *testing.T) {
	session := &responsesWSSession{
		baseInfo: &relaycommon.RelayInfo{},
		current: &responsesWSRound{
			info: &relaycommon.RelayInfo{},
		},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	apiErr := session.finishCurrentRound(c, func(info *relaycommon.RelayInfo, usage *dto.Usage) *types.NewAPIError {
		return types.NewError(errors.New("boom"), types.ErrorCodeDoRequestFailed)
	})
	if apiErr == nil {
		t.Fatal("expected settlement error")
	}
	if session.currentRound() != nil {
		t.Fatal("expected current round to be cleared after settlement error")
	}
}

func TestBuildResponsesWebSocketTerminalAPIError_UsageLimitMessage(t *testing.T) {
	apiErr := buildResponsesWebSocketTerminalAPIError(
		websocket.TextMessage,
		[]byte(`{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"message":"The usage limit has been reached"}}}`),
	)
	if apiErr == nil {
		t.Fatal("expected terminal websocket error")
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
	if apiErr.Error() != "The usage limit has been reached" {
		t.Fatalf("unexpected error message: %q", apiErr.Error())
	}
}

func TestBuildResponsesWebSocketTerminalAPIError_PreservesWrappedStatus(t *testing.T) {
	apiErr := buildResponsesWebSocketTerminalAPIError(
		websocket.TextMessage,
		[]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"The usage limit has been reached","code":429}}`),
	)
	if apiErr == nil {
		t.Fatal("expected terminal websocket error")
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
}

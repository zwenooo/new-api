package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	responsesWSFirstMessageTypeCtxKey = "responses_ws_first_message_type"
	responsesWSFirstMessageBodyCtxKey = "responses_ws_first_message"
	responsesWSRoundSettleFnCtxKey    = "responses_ws_round_settle_fn"
)

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

func WssResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo, firstMessageType int, firstMessage []byte) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if messageType, message, ok := getResponsesWebSocketRoundRequestFromContext(c); ok {
		firstMessageType = messageType
		firstMessage = message
	}

	firstMessageType, firstMessage, err := prepareResponsesWebSocketFirstUpstreamMessage(info.ClientWs, firstMessageType, firstMessage)
	if err != nil {
		if errors.Is(err, websocket.ErrCloseSent) || isResponsesWebSocketClientClose(err) {
			return nil
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if request, _, reqErr := helper.GetAndValidateResponsesWebSocketRequestBytes(firstMessage); reqErr == nil && request != nil {
		info.Request = request
		firstMessage, err = relaycommon.ApplyResponsesRequestChannelPolicy(info, request, firstMessage)
		if err != nil {
			return types.NewErrorWithStatusCode(fmt.Errorf("invalid websocket request body: %w", err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}

	c.Set(responsesWSFirstMessageTypeCtxKey, firstMessageType)
	c.Set(responsesWSFirstMessageBodyCtxKey, append([]byte(nil), firstMessage...))
	c.Set(responsesWSRoundSettleFnCtxKey, func(roundInfo *relaycommon.RelayInfo, usage *dto.Usage) *types.NewAPIError {
		if strings.HasPrefix(roundInfo.OriginModelName, "gpt-4o-audio") {
			return service.PostAudioConsumeQuota(c, roundInfo, usage, "")
		}
		return postConsumeQuota(c, roundInfo, usage, "")
	})

	statusCodeMappingStr := c.GetString("status_code_mapping")
	resp, err := adaptor.DoRequest(c, info, bytes.NewReader(firstMessage))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		info.TargetWs = resp.(*websocket.Conn)
		defer info.TargetWs.Close()
	}

	_, apiErr := adaptor.DoResponse(c, nil, info)
	if apiErr != nil {
		service.ResetStatusCode(apiErr, statusCodeMappingStr)
		return apiErr
	}
	return nil
}

func getResponsesWebSocketRoundRequestFromContext(c *gin.Context) (int, []byte, bool) {
	if c == nil {
		return 0, nil, false
	}

	messageType := websocket.TextMessage
	if value, ok := c.Get(responsesWSFirstMessageTypeCtxKey); ok {
		if storedType, ok := value.(int); ok && storedType != 0 {
			messageType = storedType
		}
	}

	value, ok := c.Get(responsesWSFirstMessageBodyCtxKey)
	if !ok {
		return 0, nil, false
	}
	message, ok := value.([]byte)
	if !ok || len(message) == 0 {
		return 0, nil, false
	}
	return messageType, append([]byte(nil), message...), true
}

func prepareResponsesWebSocketFirstUpstreamMessage(clientConn *websocket.Conn, firstMessageType int, firstPayload []byte) (int, []byte, error) {
	if clientConn == nil {
		return firstMessageType, firstPayload, nil
	}

	prewarmState := (*responsesWebSocketSyntheticPrewarm)(nil)
	messageType := firstMessageType
	payload := firstPayload

	for {
		trimmedPayload, err := normalizeResponsesWebSocketPayload(payload)
		if err != nil {
			return 0, nil, err
		}

		if prewarmState == nil && shouldHandleResponsesWebSocketPrewarmLocally(trimmedPayload) {
			nextState, syntheticPayloads, err := buildResponsesWebSocketSyntheticPrewarm(trimmedPayload)
			if err != nil {
				return 0, nil, err
			}
			prewarmState = nextState
			for i := 0; i < len(syntheticPayloads); i++ {
				if err := clientConn.WriteMessage(websocket.TextMessage, syntheticPayloads[i]); err != nil {
					return 0, nil, err
				}
			}

			_ = clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
			messageType, payload, err = clientConn.ReadMessage()
			_ = clientConn.SetReadDeadline(time.Time{})
			if err != nil {
				return 0, nil, err
			}
			continue
		}

		if prewarmState != nil {
			rewrittenPayload, rewritten, err := rewriteResponsesWebSocketSyntheticPrewarmFollowUp(trimmedPayload, prewarmState)
			if err != nil {
				return 0, nil, err
			}
			if rewritten {
				payload = rewrittenPayload
			} else {
				payload = trimmedPayload
			}
			prewarmState = nil
			return messageType, payload, nil
		}

		return messageType, trimmedPayload, nil
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

func isResponsesWebSocketClientClose(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived)
}

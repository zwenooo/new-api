package openai

import (
	"bytes"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const responsesWSRoundSettleFnCtxKey = "responses_ws_round_settle_fn"

type responsesWSRoundSettleFn = func(info *relaycommon.RelayInfo, usage *dto.Usage) *types.NewAPIError

type responsesWSRound struct {
	info               *relaycommon.RelayInfo
	usage              dto.Usage
	outputText         strings.Builder
	requestMessageType int
	requestPayload     []byte
	clientFramesSent   int
}

type responsesWSSession struct {
	mu       sync.Mutex
	baseInfo *relaycommon.RelayInfo
	current  *responsesWSRound
}

func cloneResponsesWSProductAllocations(allocations []relaycommon.ProductQuotaAllocation) []relaycommon.ProductQuotaAllocation {
	if len(allocations) == 0 {
		return nil
	}
	out := make([]relaycommon.ProductQuotaAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.ProductId == 0 || allocation.Quota <= 0 {
			continue
		}
		out = append(out, relaycommon.ProductQuotaAllocation{
			ProductId: allocation.ProductId,
			Quota:     allocation.Quota,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneSubscriptionAllocations(allocations []relaycommon.SubscriptionUnitAllocation) []relaycommon.SubscriptionUnitAllocation {
	if len(allocations) == 0 {
		return nil
	}
	out := make([]relaycommon.SubscriptionUnitAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.SubscriptionId == 0 || allocation.Amount <= 0 {
			continue
		}
		out = append(out, relaycommon.SubscriptionUnitAllocation{
			SubscriptionId:      allocation.SubscriptionId,
			GroupId:             allocation.GroupId,
			StatDate:            allocation.StatDate,
			Amount:              allocation.Amount,
			UsesGroupDailyLimit: allocation.UsesGroupDailyLimit,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func OpenaiResponsesWebSocketHandler(c *gin.Context, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return nil, types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse)
	}
	settleRound := getResponsesWSRoundSettleFn(c)
	if settleRound == nil {
		return nil, types.NewError(fmt.Errorf("missing websocket round settle callback"), types.ErrorCodeBadResponse)
	}

	firstMessageAny, ok := c.Get("responses_ws_first_message")
	if !ok {
		return nil, types.NewError(fmt.Errorf("missing first websocket payload"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	firstMessage, ok := firstMessageAny.([]byte)
	if !ok || len(firstMessage) == 0 {
		return nil, types.NewError(fmt.Errorf("invalid first websocket payload"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	firstMessageType := websocket.TextMessage
	if v, ok := c.Get("responses_ws_first_message_type"); ok {
		if t, ok := v.(int); ok && t != 0 {
			firstMessageType = t
		}
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs
	session, apiErr := newResponsesWSSession(c, info)
	if apiErr != nil {
		return nil, apiErr
	}

	if err := targetConn.WriteMessage(firstMessageType, firstMessage); err != nil {
		session.abortCurrentRound(c)
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
	}

	errChan := make(chan error, 2)
	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	var terminalEventSeen atomic.Bool
	var closeClientOnce sync.Once
	var closeTargetOnce sync.Once

	go func() {
		for {
			select {
			case <-c.Done():
				return
			default:
				messageType, message, err := clientConn.ReadMessage()
				if err != nil {
					closeClientOnce.Do(func() {
						close(clientClosed)
					})
					return
				}

				request, normalizedMessage, apiErr := parseResponsesWSClientRoundRequest(message)
				if apiErr != nil {
					helper.ResponsesWssError(c, clientConn, apiErr.ToOpenAIError())
					continue
				}
				normalizedMessage, apiErr = session.startNextRound(c, request, messageType, normalizedMessage)
				if apiErr != nil {
					helper.ResponsesWssError(c, clientConn, apiErr.ToOpenAIError())
					continue
				}

				if err := targetConn.WriteMessage(messageType, normalizedMessage); err != nil {
					session.abortCurrentRound(c)
					errChan <- fmt.Errorf("error writing to target: %w", err)
					return
				}
				terminalEventSeen.Store(false)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-c.Done():
				return
			default:
				messageType, message, err := targetConn.ReadMessage()
				if err != nil {
					if shouldIgnoreResponsesWebSocketTargetReadError(c, err, terminalEventSeen.Load()) {
						closeTargetOnce.Do(func() {
							close(targetClosed)
						})
						return
					}
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
						errChan <- fmt.Errorf("error reading from target: %w", err)
					}
					closeTargetOnce.Do(func() {
						close(targetClosed)
					})
					return
				}
				currentRound := session.currentRound()
				if currentRound == nil {
					if err := clientConn.WriteMessage(messageType, message); err != nil {
						closeClientOnce.Do(func() {
							close(clientClosed)
						})
						return
					}
					continue
				}
				currentRound.info.SetFirstResponseTime()
				if isResponsesWebSocketTerminalEvent(messageType, message) {
					terminalEventSeen.Store(true)
				}
				terminalErr := buildResponsesWebSocketTerminalAPIError(messageType, message)
				if terminalErr != nil {
					applyResponsesWebSocketTerminalErrorContext(c, terminalErr)
					if currentRound.clientFramesSent == 0 {
						errChan <- terminalErr
						return
					}
				}
				outMessage := trackAndRewriteResponsesWSMessage(c, currentRound.info, &currentRound.usage, &currentRound.outputText, messageType, message)
				if err := clientConn.WriteMessage(messageType, outMessage); err != nil {
					closeClientOnce.Do(func() {
						close(clientClosed)
					})
					return
				}
				currentRound.clientFramesSent++
				if isResponsesWebSocketTerminalEvent(messageType, message) {
					if terminalErr != nil {
						session.abortCurrentRound(c)
						continue
					}
					if apiErr := session.finishCurrentRound(c, settleRound); apiErr != nil {
						errChan <- apiErr
						return
					}
				}
			}
		}
	}()

	for {
		select {
		case err := <-errChan:
			if apiErr, ok := err.(*types.NewAPIError); ok {
				service.ResetStatusCode(apiErr, c.GetString("status_code_mapping"))
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "error")
				}
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitError, apiErr.Error())
				}
				session.prepareRetryCurrentRound(c)
				session.abortCurrentRound(c)
				return nil, apiErr
			}
			session.abortCurrentRound(c)
			return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		case <-clientClosed:
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "client_closed")
			}
			session.abortCurrentRound(c)
			return nil, nil
		case <-targetClosed:
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "target_closed")
			}
			session.abortCurrentRound(c)
			return nil, nil
		case <-c.Done():
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "client_closed")
			}
			session.abortCurrentRound(c)
			return nil, nil
		}
	}
}

func getResponsesWSRoundSettleFn(c *gin.Context) responsesWSRoundSettleFn {
	if c == nil {
		return nil
	}
	if value, ok := c.Get(responsesWSRoundSettleFnCtxKey); ok {
		if fn, ok := value.(responsesWSRoundSettleFn); ok {
			return fn
		}
	}
	return nil
}

func newResponsesWSSession(c *gin.Context, baseInfo *relaycommon.RelayInfo) (*responsesWSSession, *types.NewAPIError) {
	request, ok := baseInfo.Request.(*dto.OpenAIResponsesRequest)
	if !ok || request == nil {
		return nil, types.NewError(fmt.Errorf("invalid websocket round request type: %T", baseInfo.Request), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	roundInfo, apiErr := prepareResponsesWSRoundInfo(c, baseInfo, request, true)
	if apiErr != nil {
		return nil, apiErr
	}
	clearResponsesWSRoundBilling(baseInfo)
	return &responsesWSSession{
		baseInfo: baseInfo,
		current: &responsesWSRound{
			info:               roundInfo,
			requestMessageType: getResponsesWSRetryMessageType(c),
			requestPayload:     getResponsesWSRetryPayload(c),
		},
	}, nil
}

func (s *responsesWSSession) currentRound() *responsesWSRound {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *responsesWSSession) startNextRound(c *gin.Context, request *dto.OpenAIResponsesRequest, messageType int, payload []byte) ([]byte, *types.NewAPIError) {
	if s == nil {
		return nil, types.NewError(fmt.Errorf("websocket session is nil"), types.ErrorCodeBadResponse)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("another response round is still in progress on this websocket"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	roundInfo, apiErr := prepareResponsesWSRoundInfo(c, s.baseInfo, request, false)
	if apiErr != nil {
		return nil, apiErr
	}
	payload, err := relaycommon.ApplyResponsesRequestChannelPolicy(roundInfo, request, payload)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid websocket request body: %w", err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	s.current = &responsesWSRound{
		info:               roundInfo,
		requestMessageType: messageType,
		requestPayload:     append([]byte(nil), payload...),
	}
	setResponsesWSRetryRoundRequest(c, messageType, payload)
	return payload, nil
}

func (s *responsesWSSession) finishCurrentRound(c *gin.Context, settleRound responsesWSRoundSettleFn) *types.NewAPIError {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	current := s.current
	s.mu.Unlock()
	if current == nil {
		return nil
	}
	finalizeResponsesWSUsage(current.info, &current.usage, current.outputText.String())
	apiErr := settleRound(current.info, &current.usage)
	if apiErr != nil {
		service.ReturnPreConsumedQuota(c, current.info)
		clearResponsesWSRoundBilling(current.info)
		s.mu.Lock()
		s.current = nil
		s.mu.Unlock()
		return apiErr
	}
	clearResponsesWSRoundBilling(current.info)
	s.mu.Lock()
	s.current = nil
	s.mu.Unlock()
	return nil
}

func (s *responsesWSSession) abortCurrentRound(c *gin.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	current := s.current
	s.current = nil
	s.mu.Unlock()
	if current == nil {
		return
	}
	service.ReturnPreConsumedQuota(c, current.info)
	clearResponsesWSRoundBilling(current.info)
}

func (s *responsesWSSession) prepareRetryCurrentRound(c *gin.Context) {
	if s == nil || c == nil {
		return
	}
	s.mu.Lock()
	current := s.current
	s.mu.Unlock()
	if current == nil {
		return
	}
	setResponsesWSRetryRoundRequest(c, current.requestMessageType, current.requestPayload)
}

func parseResponsesWSClientRoundRequest(payload []byte) (*dto.OpenAIResponsesRequest, []byte, *types.NewAPIError) {
	request, normalizedPayload, err := helper.GetAndValidateResponsesWebSocketRequestBytes(payload)
	if err != nil {
		return nil, nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return request, normalizedPayload, nil
}

func prepareResponsesWSRoundInfo(
	c *gin.Context,
	baseInfo *relaycommon.RelayInfo,
	request *dto.OpenAIResponsesRequest,
	reuseExistingReservation bool,
) (*relaycommon.RelayInfo, *types.NewAPIError) {
	if baseInfo == nil || request == nil {
		return nil, types.NewError(fmt.Errorf("invalid websocket round context"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	roundInfo := cloneResponsesWSRoundInfo(baseInfo)
	roundInfo.Request = request
	roundInfo.IsStream = true
	roundInfo.StartTime = time.Now()
	roundInfo.FirstResponseTime = roundInfo.StartTime.Add(-time.Second)
	roundInfo.LogId = 0
	roundInfo.LogCreatedAt = 0
	roundInfo.ResponsesUsageInfo = nil
	roundInfo.ServiceTier = relaycommon.ResolveAllowedServiceTier(relaycommon.NormalizeServiceTier(request.ServiceTier), roundInfo.ChannelOtherSettings, roundInfo.ChannelSetting.PassThroughBodyEnabled)
	baseInfo.ServiceTier = roundInfo.ServiceTier
	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !roundInfo.ChannelSetting.PassThroughBodyEnabled {
		request.ServiceTier = roundInfo.ServiceTier
	}

	modelName := strings.TrimSpace(request.Model)
	if modelName != "" {
		roundInfo.OriginModelName = modelName
		baseInfo.OriginModelName = modelName
		common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	}
	baseInfo.Request = request
	resetResponsesWSRoundContext(c)

	meta := request.GetTokenCountMeta()
	tokens, err := service.CountRequestToken(c, meta, roundInfo)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeCountTokenFailed)
	}
	roundInfo.SetPromptTokens(tokens)
	baseInfo.SetPromptTokens(tokens)

	if _, err := helper.ModelPriceHelper(c, roundInfo, tokens, meta); err != nil {
		return nil, types.NewError(err, types.ErrorCodeModelPriceError)
	}

	if reuseExistingReservation {
		roundInfo.FinalPreConsumedQuota = baseInfo.FinalPreConsumedQuota
		roundInfo.FinalPreConsumedTokens = baseInfo.FinalPreConsumedTokens
		roundInfo.FinalPreConsumedRequests = baseInfo.FinalPreConsumedRequests
		roundInfo.RequestSubscriptionId = baseInfo.RequestSubscriptionId
		roundInfo.FinalPreConsumedPayRequests = baseInfo.FinalPreConsumedPayRequests
		roundInfo.PayRequestProductId = baseInfo.PayRequestProductId
		roundInfo.PayRequestProductAllocations = cloneResponsesWSProductAllocations(baseInfo.PayRequestProductAllocations)
		roundInfo.SubscriptionAllocations = cloneSubscriptionAllocations(baseInfo.SubscriptionAllocations)
		roundInfo.PaygProductAllocations = cloneResponsesWSProductAllocations(baseInfo.PaygProductAllocations)
		roundInfo.PayTokenProductAllocations = cloneResponsesWSProductAllocations(baseInfo.PayTokenProductAllocations)
	} else {
		if apiErr := service.PreConsumeQuota(c, roundInfo.PriceData.ShouldPreConsumedQuota, roundInfo); apiErr != nil {
			return nil, apiErr
		}
	}

	if err := ensureResponsesWSRoundInitialConsumeLog(c, roundInfo); err != nil {
		return nil, err
	}
	return roundInfo, nil
}

func cloneResponsesWSRoundInfo(baseInfo *relaycommon.RelayInfo) *relaycommon.RelayInfo {
	clone := *baseInfo
	if baseInfo.ChannelMeta != nil {
		metaCopy := *baseInfo.ChannelMeta
		clone.ChannelMeta = &metaCopy
	}
	clone.FinalPreConsumedQuota = 0
	clone.FinalPreConsumedTokens = 0
	clone.FinalPreConsumedRequests = 0
	clone.RequestSubscriptionId = 0
	clone.FinalPreConsumedPayRequests = 0
	clone.PayRequestProductId = 0
	clone.PayRequestProductAllocations = nil
	clone.SubscriptionAllocations = nil
	clone.PaygProductId = baseInfo.PaygProductId
	clone.PaygProductAllocations = nil
	clone.PayTokenProductId = baseInfo.PayTokenProductId
	clone.PayTokenProductAllocations = nil
	clone.ResponsesUsageInfo = nil
	return &clone
}

func resetResponsesWSRoundContext(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyStreamExitReason, "")
	common.SetContextKey(c, constant.ContextKeyStreamExitError, "")
	c.Set("image_generation_call", false)
	c.Set("image_generation_call_quality", "")
	c.Set("image_generation_call_size", "")
}

func setResponsesWSRetryRoundRequest(c *gin.Context, messageType int, payload []byte) {
	if c == nil || len(payload) == 0 {
		return
	}
	c.Set("responses_ws_first_message_type", messageType)
	c.Set("responses_ws_first_message", append([]byte(nil), payload...))
}

func getResponsesWSRetryMessageType(c *gin.Context) int {
	if c == nil {
		return websocket.TextMessage
	}
	if value, ok := c.Get("responses_ws_first_message_type"); ok {
		if messageType, ok := value.(int); ok && messageType != 0 {
			return messageType
		}
	}
	return websocket.TextMessage
}

func getResponsesWSRetryPayload(c *gin.Context) []byte {
	if c == nil {
		return nil
	}
	if value, ok := c.Get("responses_ws_first_message"); ok {
		if payload, ok := value.([]byte); ok && len(payload) > 0 {
			return append([]byte(nil), payload...)
		}
	}
	return nil
}

func clearResponsesWSRoundBilling(info *relaycommon.RelayInfo) {
	if info == nil {
		return
	}
	info.FinalPreConsumedQuota = 0
	info.FinalPreConsumedTokens = 0
	info.FinalPreConsumedRequests = 0
	info.RequestSubscriptionId = 0
	info.FinalPreConsumedPayRequests = 0
	info.PayRequestProductId = 0
	info.PayRequestProductAllocations = nil
	info.SubscriptionAllocations = nil
	info.PaygProductAllocations = nil
	info.PayTokenProductAllocations = nil
}

func ensureResponsesWSRoundInitialConsumeLog(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil {
		return nil
	}
	if info.LogCreatedAt == 0 {
		info.LogCreatedAt = common.GetTimestamp()
	}
	if info.LogId != 0 || !common.LogConsumeInProgressEnabled {
		return nil
	}

	recordInitialLogStart := time.Now()
	logId := model.RecordInitialConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		CreatedAt:        info.LogCreatedAt,
		ChannelId:        info.ChannelId,
		PromptTokens:     info.PromptTokens,
		CompletionTokens: 0,
		ModelName:        info.OriginModelName,
		TokenName:        c.GetString("token_name"),
		Quota:            info.FinalPreConsumedQuota,
		VisibleQuota:     info.PriceData.VisiblePreConsumedQuota,
		CostQuota:        0,
		Content:          "",
		TokenId:          info.TokenId,
		UseTimeSeconds:   0,
		IsStream:         true,
		Group:            fmt.Sprintf("%d", info.UsingGroupId),
		Other:            nil,
	})
	service.RecordRequestTraceSpan(
		c.Request.Context(),
		"db",
		"DB",
		"model.RecordInitialConsumeLog",
		recordInitialLogStart,
		time.Now(),
		func() int {
			if logId > 0 || !common.LogConsumeEnabled {
				return http.StatusOK
			}
			return http.StatusInternalServerError
		}(),
		func() error {
			if logId > 0 || !common.LogConsumeEnabled {
				return nil
			}
			return fmt.Errorf("record initial consume log failed (log_id=0)")
		}(),
		map[string]any{
			"user_id":    info.UserId,
			"channel_id": info.ChannelId,
			"log_id":     logId,
		},
	)
	info.LogId = logId
	return nil
}

func shouldIgnoreResponsesWebSocketTargetReadError(c *gin.Context, err error, terminalEventSeen bool) bool {
	if terminalEventSeen || responsesWebSocketTerminalReasonSeen(c) {
		return true
	}
	if c != nil && c.Err() != nil {
		return true
	}
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived)
}

func responsesWebSocketTerminalReasonSeen(c *gin.Context) bool {
	if c == nil {
		return false
	}
	switch strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyStreamExitReason)) {
	case "done", "incomplete", "failed", "error":
		return true
	default:
		return false
	}
}

func buildResponsesWebSocketTerminalAPIError(messageType int, payload []byte) *types.NewAPIError {
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return nil
	}

	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}

	var event map[string]any
	if err := common.Unmarshal(trimmed, &event); err != nil {
		return nil
	}

	eventType := strings.TrimSpace(common.Interface2String(event["type"]))
	switch eventType {
	case "response.failed", "error":
	default:
		return nil
	}

	openAIError := extractResponsesWebSocketEventOpenAIError(event)
	if openAIError == nil {
		return nil
	}
	normalized := normalizeResponsesWebSocketTerminalOpenAIError(*openAIError)
	return types.WithOpenAIError(normalized, inferResponsesWebSocketTerminalStatus(normalized))
}

func extractResponsesWebSocketEventOpenAIError(event map[string]any) *types.OpenAIError {
	if event == nil {
		return nil
	}
	if openAIError := dto.GetOpenAIError(event["error"]); openAIError != nil {
		return openAIError
	}
	response, ok := event["response"].(map[string]any)
	if !ok || response == nil {
		return nil
	}
	return dto.GetOpenAIError(response["error"])
}

func normalizeResponsesWebSocketTerminalOpenAIError(openAIError types.OpenAIError) types.OpenAIError {
	message := strings.TrimSpace(openAIError.Message)
	code := strings.ToLower(strings.TrimSpace(common.Interface2String(openAIError.Code)))
	errType := strings.ToLower(strings.TrimSpace(openAIError.Type))
	messageLower := strings.ToLower(message)

	if message == "" {
		switch {
		case code != "":
			message = code
		case errType != "":
			message = errType
		default:
			message = "upstream responses websocket error"
		}
		openAIError.Message = message
	}

	if isResponsesWebSocketUsageLimitError(code, errType, messageLower) {
		if errType == "" {
			openAIError.Type = "usage_limit_reached"
			errType = "usage_limit_reached"
		}
		if code == "" {
			openAIError.Code = "usage_limit_reached"
		}
		return openAIError
	}

	if code == "" {
		switch {
		case errType == "authentication_error":
			openAIError.Code = "401"
		case errType == "permission_error":
			openAIError.Code = "403"
		case errType == "invalid_request_error":
			openAIError.Code = "400"
		}
	}

	return openAIError
}

func inferResponsesWebSocketTerminalStatus(openAIError types.OpenAIError) int {
	code := strings.ToLower(strings.TrimSpace(common.Interface2String(openAIError.Code)))
	errType := strings.ToLower(strings.TrimSpace(openAIError.Type))
	messageLower := strings.ToLower(strings.TrimSpace(openAIError.Message))

	if status, ok := parseResponsesWebSocketStatusCode(code); ok {
		return status
	}
	switch {
	case isResponsesWebSocketUsageLimitError(code, errType, messageLower):
		return http.StatusTooManyRequests
	case code == "payment_required" || code == "deactivated_workspace":
		return http.StatusPaymentRequired
	case errType == "authentication_error" || strings.Contains(messageLower, "unauthorized") || strings.Contains(messageLower, "invalid api key"):
		return http.StatusUnauthorized
	case errType == "permission_error":
		return http.StatusForbidden
	case errType == "invalid_request_error":
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func parseResponsesWebSocketStatusCode(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	status, err := strconv.Atoi(raw)
	if err != nil || status < 100 || status > 599 {
		return 0, false
	}
	return status, true
}

func isResponsesWebSocketUsageLimitError(code string, errType string, messageLower string) bool {
	switch {
	case code == "429", code == "rate_limit_exceeded", code == "usage_limit_reached":
		return true
	case errType == "usage_limit_reached" || errType == "rate_limit_error":
		return true
	case strings.Contains(messageLower, "usage limit"):
		return true
	case strings.Contains(messageLower, "rate limit"):
		return true
	default:
		return false
	}
}

func applyResponsesWebSocketTerminalErrorContext(c *gin.Context, apiErr *types.NewAPIError) {
	if c == nil || apiErr == nil {
		return
	}
	if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
		common.SetContextKey(c, constant.ContextKeyStreamExitReason, "error")
	}
	if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" {
		common.SetContextKey(c, constant.ContextKeyStreamExitError, strings.TrimSpace(apiErr.Error()))
	}
}

func isResponsesWebSocketTerminalEvent(messageType int, payload []byte) bool {
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
	if err := common.Unmarshal(trimmed, &envelope); err != nil {
		return false
	}

	switch strings.TrimSpace(envelope.Type) {
	case "response.completed", "response.done", "response.incomplete", "response.failed", "error":
		return true
	default:
		return false
	}
}

func trackAndRewriteResponsesWSMessage(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, outputText *strings.Builder, messageType int, message []byte) []byte {
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return message
	}

	trimmed := bytes.TrimSpace(message)
	if len(trimmed) == 0 {
		return message
	}

	var streamResponse dto.ResponsesStreamResponse
	if err := common.Unmarshal(trimmed, &streamResponse); err != nil {
		return message
	}

	var bodyMap map[string]any
	bodyMapOk := common.Unmarshal(trimmed, &bodyMap) == nil
	changedForClient := false
	if bodyMapOk && !helper.IsAdminUser(c) {
		if responseObj, ok := bodyMap["response"].(map[string]any); ok && responseObj != nil {
			responseObj["model"] = info.OriginModelName
			changedForClient = true
		}
	}

	switch strings.TrimSpace(streamResponse.Type) {
	case "":
		if bodyMapOk {
			if _, ok := bodyMap["error"]; ok && common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "error")
			}
		}
	case "response.completed", "response.done", "response.incomplete":
		switch streamResponse.Type {
		case "response.completed", "response.done":
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")
			}
		case "response.incomplete":
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "incomplete")
			}
		}
		if streamResponse.Response != nil {
			accumulateResponsesUsage(usage, streamResponse.Response.Usage)
			if streamResponse.Response.HasImageGenerationCall() {
				c.Set("image_generation_call", true)
				c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
				c.Set("image_generation_call_size", streamResponse.Response.GetSize())
			}
		}
	case "response.failed":
		if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
			common.SetContextKey(c, constant.ContextKeyStreamExitReason, "failed")
		}
	case "error":
		if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
			common.SetContextKey(c, constant.ContextKeyStreamExitReason, "error")
		}
		if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" && bodyMapOk {
			if nested, ok := bodyMap["error"].(map[string]any); ok && nested != nil {
				message := strings.TrimSpace(common.Interface2String(nested["message"]))
				code := strings.TrimSpace(common.Interface2String(nested["code"]))
				switch {
				case code != "" && message != "":
					common.SetContextKey(c, constant.ContextKeyStreamExitError, code+": "+message)
				case message != "":
					common.SetContextKey(c, constant.ContextKeyStreamExitError, message)
				case code != "":
					common.SetContextKey(c, constant.ContextKeyStreamExitError, code)
				}
			}
		}
	case "response.output_text.delta":
		outputText.WriteString(streamResponse.Delta)
	case dto.ResponsesOutputTypeItemDone:
		if streamResponse.Item != nil {
			switch streamResponse.Item.Type {
			case dto.BuildInCallWebSearchCall:
				relaycommon.IncrementResponsesBuiltInToolCall(info, dto.BuildInToolWebSearch)
			}
		}
	}

	if !changedForClient {
		return message
	}
	rewritten, err := common.Marshal(bodyMap)
	if err != nil {
		return message
	}
	return rewritten
}

func accumulateResponsesUsage(total *dto.Usage, current *dto.Usage) {
	if total == nil || current == nil {
		return
	}
	total.PromptTokens += current.InputTokens
	total.CompletionTokens += current.OutputTokens
	total.TotalTokens += current.TotalTokens
	if current.InputTokensDetails != nil {
		total.PromptTokensDetails.CachedTokens += current.InputTokensDetails.CachedTokens
	}
}

func finalizeResponsesWSUsage(info *relaycommon.RelayInfo, usage *dto.Usage, outputText string) {
	if usage == nil {
		return
	}
	if usage.CompletionTokens == 0 {
		text := strings.TrimSpace(outputText)
		if text != "" {
			usage.CompletionTokens = service.CountTextToken(text, info.UpstreamModelName)
		}
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.PromptTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}

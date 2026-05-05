package controller

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/middleware"
	"one-api/model"
	"one-api/relay"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting"
	"one-api/setting/operation_setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func closeRelayWebSocket(ws *websocket.Conn, closeCode int) {
	if ws == nil {
		return
	}
	if closeCode == 0 {
		closeCode = websocket.CloseNormalClosure
	}
	_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, ""), time.Now().Add(time.Second))
	_ = ws.Close()
}

type relayRetryMode int

const (
	relayRetryModeStop relayRetryMode = iota
	relayRetryModeSwitchPreferred
	relayRetryModeSwitchRequired
)

type relayRetryAction int

const (
	relayRetryActionStop relayRetryAction = iota
	relayRetryActionSameChannel
	relayRetryActionSwitchChannel
)

type relayRetryState struct {
	singleChannelBudget int
	switchBudget        int
	singleChannelCount  int
	switchCount         int
}

func getSingleChannelRetryBudget(c *gin.Context) int {
	return common.RetryTimes
}

func getChannelSwitchRetryBudget() int {
	return operation_setting.AutomaticSwitchMaxRetries
}

func (s *relayRetryState) canRetrySameChannel() bool {
	return s.singleChannelCount < s.singleChannelBudget
}

func (s *relayRetryState) canSwitchChannel() bool {
	return s.switchCount < s.switchBudget
}

func (s *relayRetryState) recordSameChannelRetry() {
	s.singleChannelCount++
}

func (s *relayRetryState) recordChannelSwitch() {
	s.switchCount++
	s.singleChannelCount = 0
}

func (s *relayRetryState) addForcedSwitchRetry() {
	s.switchBudget++
}

func decideRelayRetryAction(mode relayRetryMode, state relayRetryState, hasAlternativeChannel bool) relayRetryAction {
	if mode == relayRetryModeStop {
		return relayRetryActionStop
	}
	if hasAlternativeChannel && state.canSwitchChannel() {
		return relayRetryActionSwitchChannel
	}
	if mode != relayRetryModeSwitchRequired && state.canRetrySameChannel() {
		return relayRetryActionSameChannel
	}
	return relayRetryActionStop
}

func formatGroupLabels(groupIDs []int) []string {
	if len(groupIDs) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(groupIDs))
	labels := make([]string, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		label, ok := model.GetGroupLabelByID(gid)
		if !ok || strings.TrimSpace(label) == "" {
			label = "未知分组"
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	return labels
}

func specificChannelSupportsBillingGroup(specificChannelGroupSet map[int]struct{}, contextSelectedGroupID int, candidateGroupID int) bool {
	if candidateGroupID <= 0 {
		return false
	}
	if len(specificChannelGroupSet) > 0 {
		_, ok := specificChannelGroupSet[candidateGroupID]
		return ok
	}
	if contextSelectedGroupID > 0 {
		return candidateGroupID == contextSelectedGroupID
	}
	return true
}

func takeContextSelectedChannelForBillingCandidate(
	contextSelectedChannel *model.Channel,
	hasSpecificChannelBinding bool,
	isMessagesRequest bool,
	contextSelectedGroupID int,
	specificChannelGroupSet map[int]struct{},
	candidateGroupID int,
) (channel *model.Channel, nextContextSelectedChannel *model.Channel) {
	if contextSelectedChannel == nil {
		return nil, nil
	}
	if hasSpecificChannelBinding {
		if !specificChannelSupportsBillingGroup(specificChannelGroupSet, contextSelectedGroupID, candidateGroupID) {
			return nil, contextSelectedChannel
		}
		// Specific-channel binding must survive billing-bucket fallback within the same request.
		return contextSelectedChannel, contextSelectedChannel
	}
	if !isMessagesRequest && candidateGroupID == contextSelectedGroupID {
		return contextSelectedChannel, nil
	}
	return nil, contextSelectedChannel
}

func prepareRelayBillingCandidateAttempt(relayInfo *relaycommon.RelayInfo, cand billingCandidate) {
	if relayInfo == nil {
		return
	}
	relayInfo.UsingGroupId = cand.GroupID
	relayInfo.QuotaBucket = cand.Bucket
}

func commitRelayBillingCandidateSelection(c *gin.Context, relayInfo *relaycommon.RelayInfo, selectionAuthority *middleware.RuntimeSelectionAuthority, cand billingCandidate) {
	prepareRelayBillingCandidateAttempt(relayInfo, cand)
	if selectionAuthority != nil {
		selectionAuthority.SetSelectedGroup(cand.GroupID)
		selectionAuthority.ApplyContext(c)
		return
	}
	if c != nil && cand.GroupID > 0 {
		common.SetContextKey(c, constant.ContextKeyUsingGroupId, cand.GroupID)
	}
}

func stripReasoningEffortSuffix(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	effortSuffixes := []string{"-xhigh", "-high", "-minimal", "-low", "-medium"}
	for _, suffix := range effortSuffixes {
		if strings.HasSuffix(model, suffix) {
			return strings.TrimSuffix(model, suffix)
		}
	}
	return model
}

func getCurrentRelayChannel(c *gin.Context) *model.Channel {
	autoBan := c.GetBool("auto_ban")
	autoBanInt := 1
	if !autoBan {
		autoBanInt = 0
	}
	return &model.Channel{
		Id:      common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		Type:    common.GetContextKeyInt(c, constant.ContextKeyChannelType),
		Name:    common.GetContextKeyString(c, constant.ContextKeyChannelName),
		AutoBan: &autoBanInt,
	}
}

func getCurrentRelayChannelError(c *gin.Context, channel *model.Channel) types.ChannelError {
	if channel == nil {
		channel = getCurrentRelayChannel(c)
	}
	return *types.NewChannelError(
		channel.Id,
		channel.Type,
		channel.Name,
		common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
		common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		channel.GetAutoBan(),
	)
}

func getRelayRetryMode(c *gin.Context, openaiErr *types.NewAPIError) relayRetryMode {
	if openaiErr == nil {
		return relayRetryModeStop
	}
	if types.IsSkipRetryError(openaiErr) {
		return relayRetryModeStop
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return relayRetryModeStop
	}
	if operation_setting.HasAutomaticSwitchStatusCodeWhitelist() &&
		operation_setting.IsAutomaticSwitchStatusCodeAllowed(openaiErr.StatusCode) {
		return relayRetryModeStop
	}

	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	if shouldDisable, _, _ := service.ShouldDisableChannel(channelType, openaiErr); shouldDisable {
		return relayRetryModeSwitchRequired
	}
	if shouldSwitch, _ := service.ShouldSwitchChannel(channelType, openaiErr); shouldSwitch {
		return relayRetryModeSwitchPreferred
	}
	if types.IsChannelError(openaiErr) {
		return relayRetryModeSwitchPreferred
	}
	if openaiErr.StatusCode == http.StatusTooManyRequests {
		return relayRetryModeSwitchPreferred
	}
	if openaiErr.StatusCode == 307 {
		return relayRetryModeSwitchPreferred
	}
	if openaiErr.StatusCode/100 == 5 {
		if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
			return relayRetryModeStop
		}
		return relayRetryModeSwitchPreferred
	}
	if openaiErr.StatusCode == http.StatusBadRequest {
		return relayRetryModeStop
	}
	if openaiErr.StatusCode == 408 {
		return relayRetryModeStop
	}
	if openaiErr.StatusCode/100 == 2 {
		return relayRetryModeStop
	}
	return relayRetryModeSwitchPreferred
}

func newRelayServiceBusyError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		fmt.Errorf("服务繁忙，请稍后再试"),
		types.ErrorCodeGetChannelFailed,
		http.StatusServiceUnavailable,
	)
}

func newRelayRequestBodyReadError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		err,
		types.ErrorCodeReadRequestBodyFailed,
		common.RequestBodyErrorStatusCode(err),
		types.ErrOptionWithSkipRetry(),
	)
}

func clearRelayBillingReservationState(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil {
		return
	}
	relayInfo.FinalPreConsumedQuota = 0
	relayInfo.FinalPreConsumedTokens = 0
	relayInfo.FinalPreConsumedRequests = 0
	relayInfo.RequestSubscriptionId = 0
	relayInfo.FinalPreConsumedPayRequests = 0
	relayInfo.PayRequestProductId = 0
	relayInfo.SubscriptionAllocations = nil
	relayInfo.PaygProductId = 0
	relayInfo.PaygProductAllocations = nil
	relayInfo.PayTokenProductId = 0
}

func hasRelayBillingReservationState(relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil {
		return false
	}
	return relayInfo.FinalPreConsumedQuota != 0 ||
		relayInfo.FinalPreConsumedTokens != 0 ||
		relayInfo.FinalPreConsumedRequests != 0 ||
		relayInfo.FinalPreConsumedPayRequests != 0
}

func newRelayBillingRollbackError(err error) *types.NewAPIError {
	if err != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("计费回滚失败，请联系管理员: %w", err),
			types.ErrorCodeUpdateDataError,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	return types.NewErrorWithStatusCode(
		fmt.Errorf("计费回滚失败，请联系管理员"),
		types.ErrorCodeUpdateDataError,
		http.StatusInternalServerError,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

func canUseFastTokenCountMetaForPricing(request dto.Request, info *relaycommon.RelayInfo) bool {
	if request == nil || info == nil {
		return false
	}
	switch request.(type) {
	case *dto.GeneralOpenAIRequest, *dto.OpenAIResponsesRequest, *dto.ClaudeRequest, *dto.ImageRequest:
		// Keep this list intentionally narrow. The fast-path helper only preserves
		// pricing-critical fields for these request types.
	default:
		return false
	}
	if !constant.GetMediaToken {
		return true
	}
	if !constant.GetMediaTokenNotStream && !info.IsStream {
		return true
	}
	return info.RelayFormat == types.RelayFormatOpenAIRealtime
}

// fastTokenCountMetaForPricing preserves pricing-critical fields without building large CombineText payloads.
// It is only safe when prompt sensitive checking is disabled and CountRequestToken will short-circuit to zero.
func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	}

	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}

	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if r.MaxCompletionTokens > r.MaxTokens {
			meta.MaxTokens = int(r.MaxCompletionTokens)
		} else {
			meta.MaxTokens = int(r.MaxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(r.MaxOutputTokens)
	case *dto.ClaudeRequest:
		if r.MaxTokens != nil {
			meta.MaxTokens = int(*r.MaxTokens)
		}
	case *dto.ImageRequest:
		return r.GetTokenCountMeta()
	}

	return meta
}

func seedOriginalModelFromRequestIfMissing(c *gin.Context, request dto.Request) {
	if c == nil || request == nil {
		return
	}
	if strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyOriginalModel)) != "" {
		return
	}
	modelName := ""
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		modelName = r.Model
	case *dto.OpenAIResponsesRequest:
		modelName = r.Model
	case *dto.ClaudeRequest:
		modelName = r.Model
	case *dto.EmbeddingRequest:
		modelName = r.Model
	case *dto.ImageRequest:
		modelName = r.Model
	case *dto.RerankRequest:
		modelName = r.Model
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return
	}
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	groupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		relayInfo   *relaycommon.RelayInfo
		ws          *websocket.Conn
		request     dto.Request
		firstWSBody []byte
		firstWSType int
		err         error
	)

	if relayFormat == types.RelayFormatOpenAIRealtime ||
		(relayFormat == types.RelayFormatOpenAIResponses && websocket.IsWebSocketUpgrade(c.Request)) {
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			openaiErr := types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError()
			if relayFormat == types.RelayFormatOpenAIResponses {
				helper.ResponsesWssError(c, ws, openaiErr)
			} else {
				helper.WssError(c, ws, openaiErr)
			}
			return
		}
		defer func() {
			closeCode := websocket.CloseNormalClosure
			if newAPIError != nil {
				closeCode = websocket.CloseInternalServerErr
			}
			closeRelayWebSocket(ws, closeCode)
		}()

		if relayFormat == types.RelayFormatOpenAIResponses {
			_ = ws.SetReadDeadline(time.Now().Add(30 * time.Second))
			firstWSType, firstWSBody, err = ws.ReadMessage()
			_ = ws.SetReadDeadline(time.Time{})
			if err != nil {
				newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				return
			}

			wsRequest, normalizedBody, reqErr := helper.GetAndValidateResponsesWebSocketRequestBytes(firstWSBody)
			if reqErr != nil {
				newAPIError = types.NewErrorWithStatusCode(reqErr, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				return
			}
			request = wsRequest
			firstWSBody = normalizedBody

			if middleware.IsDeferredResponsesWSDistribute(c) {
				if common.GetContextKeyInt(c, constant.ContextKeyChannelId) <= 0 {
					if selectErr := middleware.SelectChannelForModelRequest(c, &middleware.ModelRequest{Model: wsRequest.Model}); selectErr != nil {
						newAPIError = selectErr
						return
					}
				} else {
					common.SetContextKey(c, constant.ContextKeyOriginalModel, wsRequest.Model)
				}
			}
		}
	}

	defer func() {
		if newAPIError != nil {
			// Some local/Windows run configs only capture stdout; make sure 5xx errors are visible there.
			if newAPIError.StatusCode >= 500 {
				common.SysLog(fmt.Sprintf(
					"[relay_5xx] request_id=%s method=%s path=%s status=%d error_type=%s error_code=%s user_id=%d token_id=%d group_id=%d channel_id=%d channel_type=%d msg=%s",
					requestId,
					c.Request.Method,
					c.Request.URL.Path,
					newAPIError.StatusCode,
					newAPIError.GetErrorType(),
					newAPIError.GetErrorCode(),
					c.GetInt("id"),
					common.GetContextKeyInt(c, constant.ContextKeyTokenId),
					common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId),
					c.GetInt("channel_id"),
					c.GetInt("channel_type"),
					newAPIError.MaskSensitiveError(),
				))
			}
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatOpenAIResponses:
				if ws != nil {
					helper.ResponsesWssError(c, ws, newAPIError.ToOpenAIError())
					return
				}
				if shouldReturnResponsesQuotaFailedSSE(c, relayInfo, newAPIError) {
					helper.ResponsesFailed(c, "insufficient_quota", newAPIError.Error())
					return
				}
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			case types.RelayFormatClaude:
				statusCode := newAPIError.StatusCode
				if statusCode <= 0 {
					statusCode = http.StatusInternalServerError
				}
				c.JSON(statusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				if shouldReturnResponsesQuotaFailedSSE(c, relayInfo, newAPIError) {
					helper.ResponsesFailed(c, "insufficient_quota", newAPIError.Error())
					return
				}
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	if request == nil {
		request, err = helper.GetAndValidateRequest(c, relayFormat)
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) {
				newAPIError = newRelayRequestBodyReadError(err)
				return
			}
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			return
		}
	}
	seedOriginalModelFromRequestIfMissing(c, request)

	groupID = common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	originalModel = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	relayInfo, err = relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	if relayFormat == types.RelayFormatOpenAIResponses && ws != nil {
		relayInfo.IsStream = true
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	var meta *types.TokenCountMeta
	if needSensitiveCheck || !canUseFastTokenCountMetaForPricing(request, relayInfo) {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewErrorWithStatusCode(
				fmt.Errorf("prompt contains sensitive words"),
				types.ErrorCodeSensitiveWordsDetected,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
			return
		}
	}

	tokens, err := service.CountRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetPromptTokens(tokens)
	rawPreConsumedTokens := rawPreConsumedTokenUnits(tokens, meta.MaxTokens)

	tokenAllowedGroupIDs, _ := common.GetContextKeyType[[]int](c, constant.ContextKeyTokenAllowedGroupIds)
	selectionAuthority, selectionErr := middleware.ResolveRuntimeSelectionAuthority(c, c.Request.URL.Path, relayInfo.OriginModelName, 0)
	if selectionErr != nil {
		newAPIError = selectionErr
		return
	}

	loadBillingCandidates := func() (*relayBillingCandidateSources, []billingCandidate, *types.NewAPIError) {
		sources, err := loadRelayBillingCandidateSources(
			relayInfo,
			tokenAllowedGroupIDs,
			selectionAuthority,
			rawPreConsumedTokens,
		)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if sources == nil {
			sources = &relayBillingCandidateSources{NoBillingGroupSet: make(map[int]struct{})}
		}
		return sources, sources.buildCandidates(), nil
	}

	billingSources, candidates, sourceErr := loadBillingCandidates()
	if sourceErr != nil {
		newAPIError = sourceErr
		return
	}

	if len(billingSources.TokenGroupCandidates) == 0 {
		newAPIError = types.NewErrorWithStatusCode(fmt.Errorf("当前请求分组为空，无法计费"), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		return
	}

	if len(candidates) == 0 {
		renderLabelList := func(groupIDs []int) string {
			labels := formatGroupLabels(groupIDs)
			if len(labels) == 0 {
				return "[]"
			}
			return "[" + strings.Join(labels, ",") + "]"
		}

		userBillableGroupIDs := billingSources.billableGroupIDs()

		newAPIError = types.NewErrorWithStatusCode(
			fmt.Errorf("无可用计费分组：当前请求分组=%s，用户当前可扣费分组=%s", renderLabelList(billingSources.TokenGroupCandidates), renderLabelList(userBillableGroupIDs)),
			types.ErrorCodeAccessDenied,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
		return
	}

	var (
		selected      bool
		lastErr       *types.NewAPIError
		lastErrBucket string
	)

	isMessagesRequest := strings.HasPrefix(c.Request.URL.Path, "/v1/messages")
	hasSpecificChannelBinding := false
	if _, ok := c.Get("specific_channel_id"); ok {
		hasSpecificChannelBinding = true
	}
	contextSelectedGroupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	contextSelectedChannelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	var contextSelectedChannel *model.Channel
	var specificChannelGroupSet map[int]struct{}
	restoreContextSelectedChannel := func() {
		contextSelectedChannel = nil
		if contextSelectedChannelID <= 0 {
			return
		}
		if cachedChannel, err := model.CacheGetChannel(contextSelectedChannelID); err == nil && cachedChannel != nil && cachedChannel.Status == common.ChannelStatusEnabled {
			contextSelectedChannel = cachedChannel
		}
	}
	restoreContextSelectedChannel()
	if hasSpecificChannelBinding && contextSelectedChannelID > 0 {
		if groupIDs, err := model.GetChannelGroupIDs(contextSelectedChannelID); err == nil && len(groupIDs) > 0 {
			specificChannelGroupSet = make(map[int]struct{}, len(groupIDs))
			for _, gid := range groupIDs {
				if gid <= 0 {
					continue
				}
				specificChannelGroupSet[gid] = struct{}{}
			}
		}
	}

	uaRejectedGroupIDs := make([]int, 0)
	uaAcceptedAny := false
	selectBillingCandidateFrom := func(startIdx int) *types.NewAPIError {
		if startIdx < 0 {
			startIdx = 0
		}
		for idx := startIdx; idx < len(candidates); idx++ {
			cand := candidates[idx]
			// Reset PAYG product selection per candidate attempt.
			relayInfo.SubscriptionAllocations = nil
			relayInfo.PaygProductId = 0
			relayInfo.PaygProductAllocations = nil
			relayInfo.PayTokenProductId = 0
			prepareRelayBillingCandidateAttempt(relayInfo, cand)

			if !selectionAuthority.AllowsUserAgent(cand.GroupID) {
				uaRejectedGroupIDs = append(uaRejectedGroupIDs, cand.GroupID)
				continue
			}
			uaAcceptedAny = true

			var channel *model.Channel
			var chErr error
			lookupStep := ""
			channel, contextSelectedChannel = takeContextSelectedChannelForBillingCandidate(
				contextSelectedChannel,
				hasSpecificChannelBinding,
				isMessagesRequest,
				contextSelectedGroupID,
				specificChannelGroupSet,
				cand.GroupID,
			)
			if hasSpecificChannelBinding && channel == nil {
				continue
			}
			if channel == nil {
				channel, _, lookupStep, chErr = selectionAuthority.LookupChannelForGroup(c, cand.GroupID)
			}
			if chErr != nil {
				if model.IsChannelConcurrencyLimitReachedErr(chErr) {
					groupLabel := middleware.FormatGroupLabelForRuntimeSelection(cand.GroupID)
					message := fmt.Sprintf("分组 %s 下模型 %s 的所有可用渠道已达到最大并行度（billing）", groupLabel, relayInfo.OriginModelName)
					if lookupStep == "messages_to_responses_compat" {
						message = fmt.Sprintf("分组 %s 下模型 %s 的所有可用兼容渠道已达到最大并行度（billing）", groupLabel, relayInfo.OriginModelName)
					}
					lastErr = types.NewErrorWithStatusCode(
						fmt.Errorf("%s", message),
						types.ErrorCodeChannelConcurrencyExceeded,
						http.StatusTooManyRequests,
						types.ErrOptionWithSkipRetry(),
					)
					continue
				}
				if lookupStep == "messages_to_responses_compat" {
					return types.NewErrorWithStatusCode(
						fmt.Errorf("渠道级 messages->responses 模型映射配置错误"),
						types.ErrorCodeGetChannelFailed,
						http.StatusInternalServerError,
						types.ErrOptionWithSkipRetry(),
					)
				}
				lastErr = types.NewError(chErr, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
				continue
			}
			if channel == nil {
				continue
			}
			commitRelayBillingCandidateSelection(c, relayInfo, selectionAuthority, cand)
			if err := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); err != nil {
				lastErr = err
				continue
			}
			relayInfo.InitChannelMeta(c)
			relayInfo.ApplyChannelServiceTierPolicy()
			if relayInfo.RelayMode == relayconstant.RelayModeResponses && relayInfo.ClientWs == nil {
				normalized, err := service.PredictRelayResponsesRequestBillingMeta(c, relayInfo)
				if err != nil {
					return types.NewError(err, types.ErrorCodeConvertRequestFailed)
				}
				relayInfo.ServiceTier = normalized.ServiceTier
				relayInfo.ReasoningEffort = normalized.ReasoningEffort
			}

			pd, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
			if err != nil {
				return types.NewError(err, types.ErrorCodeModelPriceError)
			}

			preConsumeStart := time.Now()
			preErr := service.PreConsumeQuota(c, pd.ShouldPreConsumedQuota, relayInfo)
			service.RecordRequestTraceSpan(
				c.Request.Context(),
				"db",
				"DB",
				"service.PreConsumeQuota",
				preConsumeStart,
				time.Now(),
				func() int {
					if preErr == nil {
						return http.StatusOK
					}
					if preErr.StatusCode > 0 {
						return preErr.StatusCode
					}
					return http.StatusInternalServerError
				}(),
				func() error {
					if preErr == nil {
						return nil
					}
					return fmt.Errorf("%s", preErr.MaskSensitiveError())
				}(),
				map[string]any{
					"user_id":            relayInfo.UserId,
					"group_id":           relayInfo.UsingGroupId,
					"bucket":             relayInfo.QuotaBucket,
					"pre_consumed_quota": pd.ShouldPreConsumedQuota,
				},
			)
				if preErr == nil {
					recordTmpRelayBillingDecision(c, relayInfo.QuotaBucket, relayInfo.UsingGroupId)
					selected = true
					return nil
				}

			switch preErr.GetErrorCode() {
			case types.ErrorCodeTokenDailyQuotaExceeded,
				types.ErrorCodePreConsumeTokenQuotaFailed:
				return preErr
			default:
				lastErr, lastErrBucket = preferBillingAttemptError(lastErr, lastErrBucket, preErr, cand.Bucket)
				if hasRelayBillingReservationState(relayInfo) {
					return newRelayBillingRollbackError(preErr)
				}
				clearRelayBillingReservationState(relayInfo)
				continue
			}
		}
		return nil
	}
	if selectionErr := selectBillingCandidateFrom(0); selectionErr != nil {
		newAPIError = selectionErr
		return
	}

	if !selected {
		if lastErr != nil {
			newAPIError = lastErr
		} else if !uaAcceptedAny {
			newAPIError = types.NewErrorWithStatusCode(
				fmt.Errorf("无可用计费分组：UA 不被允许。UA=%q, 允许扣费分组=%v", selectionAuthority.UserAgent, formatGroupLabels(normalizePositiveGroupCandidatesKeepOrder(uaRejectedGroupIDs))),
				types.ErrorCodeAccessDenied,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		} else {
			// No channel was found in any eligible (bucket, group) candidates.
			// Differentiate "not eligible to consume this model's groups" from "channel truly unavailable".
			eligibleGroupIDs := make([]int, 0, len(candidates))
			eligibleGroupSet := make(map[int]struct{}, len(candidates))
			for _, cand := range candidates {
				gid := cand.GroupID
				if gid <= 0 {
					continue
				}
				if _, ok := eligibleGroupSet[gid]; ok {
					continue
				}
				eligibleGroupSet[gid] = struct{}{}
				eligibleGroupIDs = append(eligibleGroupIDs, gid)
			}
			enabledGroupIDs, gErr := model.GetModelEnabledGroupIDs(relayInfo.OriginModelName)
			if gErr == nil && len(enabledGroupIDs) > 0 {
				hasIntersection := false
				for _, gid := range enabledGroupIDs {
					if _, ok := eligibleGroupSet[gid]; ok {
						hasIntersection = true
						break
					}
				}
				if !hasIntersection {
					newAPIError = types.NewErrorWithStatusCode(
						fmt.Errorf("无可用计费分组：模型 %s 可用分组=%v，本次请求允许扣费的分组=%v", relayInfo.OriginModelName, formatGroupLabels(enabledGroupIDs), formatGroupLabels(eligibleGroupIDs)),
						types.ErrorCodeAccessDenied,
						http.StatusBadRequest,
						types.ErrOptionWithSkipRetry(),
					)
					return
				}
			}

			showGroup := "-"
			eligibleLabels := formatGroupLabels(eligibleGroupIDs)
			if len(eligibleLabels) == 1 {
				showGroup = eligibleLabels[0]
			} else if len(eligibleLabels) > 1 {
				showGroup = fmt.Sprintf("[%s]", strings.Join(eligibleLabels, ","))
			} else if relayInfo.UsingGroupId > 0 {
				usingLabels := formatGroupLabels([]int{relayInfo.UsingGroupId})
				if len(usingLabels) == 1 {
					showGroup = usingLabels[0]
				}
			}
			newAPIError = types.NewErrorWithStatusCode(
				fmt.Errorf("分组 %s 下模型 %s 无可用渠道（billing）", showGroup, relayInfo.OriginModelName),
				types.ErrorCodeModelNotFound,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
		return
	}

	// The initial billing candidate selection may override group for this request.
	// Once a concrete billing group/channel has been chosen, all downstream retries
	// must stay inside that same selected group. Otherwise one logical request can
	// silently drift into another group and multiply retry budget unexpectedly.
	groupID = relayInfo.UsingGroupId
	originalModel = relayInfo.OriginModelName

	defer func() {
		// Only return quota/requests if downstream failed and something was actually pre-consumed
		if newAPIError != nil && (relayInfo.FinalPreConsumedQuota != 0 || relayInfo.FinalPreConsumedTokens != 0 || relayInfo.FinalPreConsumedRequests != 0 || relayInfo.FinalPreConsumedPayRequests != 0) {
			if refundErr := service.ReturnPreConsumedQuota(c, relayInfo); refundErr != nil {
				logger.LogError(c, fmt.Sprintf("return pre-consumed quota failed: %s", refundErr.Error()))
			}
		}
	}()

	rateLimitGuard, rlErr := middleware.AcquireModelRequestRateLimitGuard(c, relayInfo.UsingGroupId)
	if rlErr != nil {
		newAPIError = rlErr
		return
	}
	if rateLimitGuard != nil {
		defer func() {
			if newAPIError == nil {
				rateLimitGuard.RecordSuccess()
			}
		}()
	}

	retryState := relayRetryState{
		singleChannelBudget: getSingleChannelRetryBudget(c),
		switchBudget:        getChannelSwitchRetryBudget(),
	}
	for {
		if relayFormat == types.RelayFormatOpenAIResponses && ws != nil {
			if currentModel := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyOriginalModel)); currentModel != "" {
				originalModel = currentModel
			}
		}
		channel := getCurrentRelayChannel(c)

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			newAPIError = newRelayRequestBodyReadError(bodyErr)
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)
		common.SetContextKey(c, constant.ContextKeyChannelAttemptStartTime, time.Now())

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatOpenAIResponses:
			if ws != nil {
				newAPIError = relay.WssResponsesHelper(c, relayInfo, firstWSType, firstWSBody)
			} else {
				newAPIError = relayHandler(c, relayInfo)
			}
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			return
		}

		shouldForceRetry := processChannelError(c, relayInfo, getCurrentRelayChannelError(c, channel), newAPIError)
		if shouldForceRetry {
			retryState.addForcedSwitchRetry()
		}

		retryMode := getRelayRetryMode(c, newAPIError)
		if retryMode != relayRetryModeStop {
			if retryState.canSwitchChannel() {
				nextChannel, retryErr := selectRetryChannel(c, groupID, originalModel)
				if retryErr != nil {
					logger.LogError(c, retryErr.Error())
				} else if decideRelayRetryAction(retryMode, retryState, nextChannel != nil) == relayRetryActionSwitchChannel {
					retryState.recordChannelSwitch()
					continue
				}
			}

			if decideRelayRetryAction(retryMode, retryState, false) == relayRetryActionSameChannel {
				retryState.recordSameChannelRetry()
				continue
			}
		}

		if retryMode == relayRetryModeStop {
			break
		}

		if newAPIError == nil {
			newAPIError = newRelayServiceBusyError()
		}
		break
	}

	if newAPIError != nil && relayInfo != nil && relayInfo.IsStream && relayInfo.LogId > 0 {
		content := newAPIError.LogMessage()
		if strings.TrimSpace(content) == "" {
			content = fmt.Sprintf("%s/%s status=%d", newAPIError.GetErrorType(), newAPIError.GetErrorCode(), newAPIError.StatusCode)
		}

		useTimeSeconds := 0
		if !relayInfo.StartTime.IsZero() {
			useTimeSeconds = int(time.Since(relayInfo.StartTime).Seconds())
		}

		other := make(map[string]interface{})
		other["error_type"] = newAPIError.GetErrorType()
		other["error_code"] = newAPIError.GetErrorCode()
		other["status_code"] = newAPIError.StatusCode

		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		other["admin_info"] = adminInfo

		other["prompt_tokens"] = relayInfo.PromptTokens
		if !relayInfo.StartTime.IsZero() {
			other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
		}
		if reason := common.GetContextKeyString(c, constant.ContextKeyStreamExitReason); reason != "" {
			other["stream_exit_reason"] = reason
		}
		if errMsg := common.GetContextKeyString(c, constant.ContextKeyStreamExitError); errMsg != "" {
			other["stream_exit_error"] = errMsg
		}

		updateConsumeLogStart := time.Now()
		model.UpdateConsumeLogAsError(c, relayInfo.LogId, model.RecordConsumeLogParams{
			CreatedAt:        relayInfo.LogCreatedAt,
			ChannelId:        relayInfo.ChannelId,
			PromptTokens:     relayInfo.PromptTokens,
			CompletionTokens: 0,
			ModelName:        relayInfo.OriginModelName,
			TokenName:        c.GetString("token_name"),
			Quota:            0,
			Content:          content,
			TokenId:          relayInfo.TokenId,
			UseTimeSeconds:   useTimeSeconds,
			IsStream:         relayInfo.IsStream,
			Group:            fmt.Sprintf("%d", relayInfo.UsingGroupId),
			Other:            other,
		})
		service.RecordRequestTraceSpan(
			c.Request.Context(),
			"db",
			"DB",
			"model.UpdateConsumeLogAsError",
			updateConsumeLogStart,
			time.Now(),
			http.StatusOK,
			nil,
			map[string]any{
				"log_id":     relayInfo.LogId,
				"user_id":    relayInfo.UserId,
				"channel_id": relayInfo.ChannelId,
			},
		)
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
}

func shouldReturnResponsesQuotaFailedSSE(
	c *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	newAPIError *types.NewAPIError,
) bool {
	if relayInfo == nil || newAPIError == nil {
		return false
	}
	if relayInfo.RelayMode != relayconstant.RelayModeResponses || !relayInfo.IsStream {
		return false
	}
	if !strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
		return false
	}
	switch newAPIError.GetErrorCode() {
	case "insufficient_quota",
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed,
		types.ErrorCodeUserDailyQuotaExceeded,
		types.ErrorCodeTokenDailyQuotaExceeded:
		return true
	default:
		return false
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func formatRetryGroupLabel(groupIDs []int) string {
	if len(groupIDs) == 0 {
		return "未知分组"
	}
	labels := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if v, ok := model.GetGroupLabelByID(groupID); ok {
			labels = append(labels, v)
			continue
		}
		labels = append(labels, fmt.Sprintf("#%d", groupID))
	}
	if len(labels) == 0 {
		return "未知分组"
	}
	if len(labels) == 1 {
		return labels[0]
	}
	return "[" + strings.Join(labels, ",") + "]"
}

func selectRetryChannelFromGroup(c *gin.Context, groupID int, originalModel string) (*model.Channel, *types.NewAPIError) {
	var (
		channel *model.Channel
		err     error
	)
	if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
		channel, err = model.CacheGetRandomSatisfiedChannel(c, groupID, originalModel, 0)
	} else {
		channel, err = model.CacheGetRandomSatisfiedChannel(c, groupID, originalModel, 0)
	}
	if err != nil {
		groupLabel := "未知分组"
		if v, ok := model.GetGroupLabelByID(groupID); ok {
			groupLabel = v
		}
		if model.IsChannelConcurrencyLimitReachedErr(err) {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("分组 %s 下模型 %s 的所有可用渠道已达到最大并行度（retry）", groupLabel, originalModel),
				types.ErrorCodeChannelConcurrencyExceeded,
				http.StatusTooManyRequests,
				types.ErrOptionWithSkipRetry(),
			)
		}
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", groupLabel, originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil &&
		strings.HasPrefix(c.Request.URL.Path, "/v1/messages") &&
		model.GroupAllowsModel(groupID, originalModel) {
		channel, err = model.CacheGetRandomSatisfiedMessagesToResponsesCompatChannel(c, groupID, originalModel, 0)
		if err != nil {
			groupLabel := "未知分组"
			if v, ok := model.GetGroupLabelByID(groupID); ok {
				groupLabel = v
			}
			if model.IsChannelConcurrencyLimitReachedErr(err) {
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("分组 %s 下模型 %s 的所有可用兼容渠道已达到最大并行度（retry）", groupLabel, originalModel),
					types.ErrorCodeChannelConcurrencyExceeded,
					http.StatusTooManyRequests,
					types.ErrOptionWithSkipRetry(),
				)
			}
			return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用兼容渠道失败（retry）: %s", groupLabel, originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
	}
	return channel, nil
}

func getChannel(c *gin.Context, groupID int, originalModel string, retryCount int) (*model.Channel, *types.NewAPIError) {
	if retryCount == 0 {
		return getCurrentRelayChannel(c), nil
	}

	channel, newAPIError := selectRetryChannel(c, groupID, originalModel)
	if newAPIError != nil || channel != nil {
		return channel, newAPIError
	}

	currentChannelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	retryGroupIDs, err := model.GetChannelRetryGroupIDs(currentChannelID, groupID)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("构建模型 %s 的重试分组失败: %s", originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if len(retryGroupIDs) == 0 {
		retryGroupIDs = []int{groupID}
	}
	groupLabel := formatRetryGroupLabel(retryGroupIDs)
	return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 无可用渠道（retry）", groupLabel, originalModel), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
}

func selectRetryChannel(c *gin.Context, groupID int, originalModel string) (*model.Channel, *types.NewAPIError) {
	currentChannelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	retryGroupIDs, err := model.GetChannelRetryGroupIDs(currentChannelID, groupID)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("构建模型 %s 的重试分组失败: %s", originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if len(retryGroupIDs) == 0 {
		retryGroupIDs = []int{groupID}
	}

	var lastErr *types.NewAPIError
	for _, retryGroupID := range retryGroupIDs {
		channel, chErr := selectRetryChannelFromGroup(c, retryGroupID, originalModel)
		if chErr != nil {
			lastErr = chErr
			continue
		}
		if channel == nil {
			continue
		}

		newAPIError := middleware.SetupContextForSelectedChannel(c, channel, originalModel)
		if newAPIError != nil {
			lastErr = newAPIError
			continue
		}
		return channel, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

func processChannelError(c *gin.Context, relayInfo *relaycommon.RelayInfo, channelError types.ChannelError, err *types.NewAPIError) bool {
	logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.MaskSensitiveError()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	service.RecordChannelAbnormalConsume(c, relayInfo, channelError, err)
	shouldDisable, disableReason, restoreSeconds := service.ShouldDisableChannel(channelError.ChannelType, err)
	shouldForceRetry := shouldDisable
	if shouldDisable && channelError.AutoBan {
		clearSpecificChannelBinding(c)
		reasonToUse := err.Error()
		if disableReason != "" {
			reasonToUse = disableReason
		}
		service.DisableChannel(channelError, reasonToUse, restoreSeconds)
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		usingGroupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
		userGroup := ""
		if usingGroupID > 0 {
			userGroup = fmt.Sprintf("%d", usingGroupID)
		}
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		other["request_id"] = c.GetString(common.RequestIdKey)

		promptCacheKey := ""
		if relayInfo != nil && relayInfo.Request != nil {
			if req, ok := relayInfo.Request.(*dto.OpenAIResponsesRequest); ok && req != nil && len(req.PromptCacheKey) > 0 {
				if common.GetJsonType(req.PromptCacheKey) == "string" {
					var parsed string
					if unmarshalErr := common.Unmarshal(req.PromptCacheKey, &parsed); unmarshalErr == nil {
						promptCacheKey = strings.TrimSpace(parsed)
					}
				}
			}
		}
		if promptCacheKey != "" {
			other["prompt_cache_key"] = promptCacheKey
		}
		conversationID := strings.TrimSpace(c.GetHeader("conversation_id"))
		sessionID := strings.TrimSpace(c.GetHeader("session_id"))
		if conversationID == "" && promptCacheKey != "" {
			conversationID = promptCacheKey
		}
		if sessionID == "" && promptCacheKey != "" {
			sessionID = promptCacheKey
		}
		if conversationID != "" {
			other["conversation_id"] = conversationID
		}
		if sessionID != "" {
			other["session_id"] = sessionID
		}

		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		other["admin_info"] = adminInfo

		useTimeSeconds := 0
		isStream := false
		if relayInfo != nil {
			isStream = relayInfo.IsStream
			if !relayInfo.StartTime.IsZero() {
				useTimeSeconds = int(time.Since(relayInfo.StartTime).Seconds())
			}
			other["prompt_tokens"] = relayInfo.PromptTokens
			other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
		}
		if reason := common.GetContextKeyString(c, constant.ContextKeyStreamExitReason); reason != "" {
			other["stream_exit_reason"] = reason
		}
		if errMsg := common.GetContextKeyString(c, constant.ContextKeyStreamExitError); errMsg != "" {
			other["stream_exit_error"] = errMsg
		}

		content := err.LogMessage()
		if strings.TrimSpace(content) == "" {
			content = fmt.Sprintf("%s/%s status=%d", err.GetErrorType(), err.GetErrorCode(), err.StatusCode)
		}
		recordErrorLogStart := time.Now()
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, content, tokenId, useTimeSeconds, isStream, userGroup, other)
		service.RecordRequestTraceSpan(
			c.Request.Context(),
			"db",
			"DB",
			"model.RecordErrorLog",
			recordErrorLogStart,
			time.Now(),
			http.StatusOK,
			nil,
			map[string]any{
				"user_id":    userId,
				"channel_id": channelId,
				"log_type":   "error",
			},
		)
	}

	return shouldForceRetry
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := dto.OpenAIError{
		Message: "API not implemented",
		Type:    "transfer_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayChatCompletionsDisabled(c *gin.Context) {
	err := dto.OpenAIError{
		Message: "API temporarily disabled (/v1/chat/completions)",
		Type:    "transfer_api_error",
		Param:   "",
		Code:    "chat_completions_disabled",
	}
	c.JSON(http.StatusForbidden, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	err := dto.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTask(c *gin.Context) {
	retryTimes := common.RetryTimes
	channelId := c.GetInt("channel_id")
	groupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	originalModel := c.GetString("original_model")
	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return
	}
	taskErr := taskRelayHandler(c, relayInfo)
	if taskErr == nil {
		retryTimes = 0
	}
	for i := 0; shouldRetryTaskRelay(c, channelId, taskErr, retryTimes) && i < retryTimes; i++ {
		channel, newAPIError := getChannel(c, groupID, originalModel, i)
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
			taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			break
		}
		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		logger.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, i))
		//middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", common.RequestBodyErrorStatusCode(bodyErr))
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)
		taskErr = taskRelayHandler(c, relayInfo)
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError {
	var err *dto.TaskError
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeVideoFetchByID:
		err = relay.RelayTaskFetch(c, relayInfo.RelayMode)
	default:
		err = relay.RelayTaskSubmit(c, relayInfo)
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode == http.StatusRequestEntityTooLarge {
		return false
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

func clearSpecificChannelBinding(c *gin.Context) {
	if c == nil {
		return
	}
	if c.Keys == nil {
		return
	}
	delete(c.Keys, string(constant.ContextKeyTokenSpecificChannelId))
}

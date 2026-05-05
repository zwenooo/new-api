package relay

import (
	"bytes"
	"fmt"
	"io"
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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	info.ApplyChannelServiceTierPolicy()

	isCompact := strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact")
	if isCompact && info.IsStream {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("Streaming not supported for compact responses"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	responsesReq, ok := info.Request.(*dto.OpenAIResponsesRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(responsesReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	request.ServiceTier = info.ServiceTier

	var forceUpstreamStream bool

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return newRequestBodyReadError(err)
		}
		if isCompact {
			body, err = normalizeOpenAIResponsesCompactBody(body)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		} else {
			body, err = ensureOpenAIResponsesStreamField(body, info.IsStream)
			if err != nil {
				return types.NewErrorWithStatusCode(fmt.Errorf("invalid responses request body: %w", err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		}
		if info.ClientWs == nil {
			normalized, err := service.NormalizeRelayResponsesRequestByUA(body, c.Request.Header, isCompact)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			body = normalized.Body
			forceUpstreamStream = normalized.ForceUpstreamStream
			applyResponsesRequestBillingMeta(info, normalized)
		}
		body, err = relaycommon.ApplyResponsesUpstreamRequirements(info, body, isCompact)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
			}
		}

		if isCompact {
			jsonData, err = normalizeOpenAIResponsesCompactBody(jsonData)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		} else {
			jsonData, err = ensureOpenAIResponsesStreamField(jsonData, info.IsStream)
			if err != nil {
				return types.NewError(fmt.Errorf("failed to normalize responses stream flag: %w", err), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
		}
		if info.ClientWs == nil {
			normalized, err := service.NormalizeRelayResponsesRequestByUA(jsonData, c.Request.Header, isCompact)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			jsonData = normalized.Body
			forceUpstreamStream = normalized.ForceUpstreamStream
			applyResponsesRequestBillingMeta(info, normalized)
		}
		jsonData, err = relaycommon.ApplyResponsesUpstreamRequirements(info, jsonData, isCompact)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	if forceUpstreamStream {
		common.SetContextKey(c, constant.ContextKeyResponsesForceUpstreamStream, true)
	}

	// Prepare a stable created_at for streaming logs even when we skip the "in progress" row.
	if info.IsStream && info.LogCreatedAt == 0 {
		info.LogCreatedAt = common.GetTimestamp()
	}

	// 在发起请求前创建初始日志条目（用于流式请求实时显示）
	if info.IsStream && info.LogId == 0 && common.LogConsumeInProgressEnabled {
		createdAt := info.LogCreatedAt
		initialLogParams := model.RecordConsumeLogParams{
			CreatedAt:        createdAt,
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
			Other:            nil, // 暂不设置详细信息
		}
		recordInitialLogStart := time.Now()
		logId := model.RecordInitialConsumeLog(c, info.UserId, initialLogParams)
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
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)

		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
	}
	if httpResp != nil {
		// Note: some upstreams may return SSE even when stream=false.
		// For non-stream callers we still need to return JSON, so we avoid flipping info.IsStream here.
		// The non-stream handler is responsible for aggregating SSE when needed.
	}

	usage, newApiErr := adaptor.DoResponse(c, httpResp, info)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		if apiErr := service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), ""); apiErr != nil {
			return apiErr
		}
	} else {
		if apiErr := postConsumeQuota(c, info, usage.(*dto.Usage), ""); apiErr != nil {
			return apiErr
		}
	}
	return nil
}

func applyResponsesRequestBillingMeta(info *relaycommon.RelayInfo, normalized service.RelayResponsesRequestNormalizationResult) {
	if info == nil {
		return
	}
	info.ServiceTier = normalized.ServiceTier
	info.ReasoningEffort = normalized.ReasoningEffort
}

func ensureOpenAIResponsesStreamField(body []byte, isStream bool) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body, nil
	}

	var m map[string]any
	if err := common.Unmarshal(trimmed, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return body, nil
	}

	if v, ok := m["stream"]; ok {
		if v == nil {
			m["stream"] = isStream
		} else if _, ok := v.(bool); ok {
			return body, nil
		} else {
			return nil, fmt.Errorf("invalid stream field type: %T", v)
		}
	} else {
		m["stream"] = isStream
	}

	return common.Marshal(m)
}

// normalizeOpenAIResponsesCompactBody aligns with OpenAI /v1/responses/compact behavior:
// - streaming is not supported
// - if "stream" is present, remove it (even when false)
func normalizeOpenAIResponsesCompactBody(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body, nil
	}

	var m map[string]any
	if err := common.Unmarshal(trimmed, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return body, nil
	}

	changed := false

	if v, ok := m["stream"]; ok {
		if v == nil {
			delete(m, "stream")
			changed = true
		} else if b, ok := v.(bool); ok {
			if b {
				return nil, fmt.Errorf("Streaming not supported for compact responses")
			}
			delete(m, "stream")
			changed = true
		} else {
			return nil, fmt.Errorf("invalid stream field type: %T", v)
		}
	}

	// /v1/responses/compact does not accept store; drop it for compatibility (Codex upstream also rejects it).
	if _, ok := m["store"]; ok {
		delete(m, "store")
		changed = true
	}

	if !changed {
		return body, nil
	}
	return common.Marshal(m)
}

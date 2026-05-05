package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/setting/operation_setting"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	info.ConversationId = request.ConversationId
	info.SessionId = request.SessionId
	info.PromptCacheKey = request.PromptCacheKey

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !request.Stream {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		requestBody, newAPIError = getPassThroughRequestBody(c)
		if newAPIError != nil {
			return newAPIError
		}
		if common.DebugEnabled {
			if body, err := common.GetRequestBody(c); err == nil {
				println("requestBody: ", string(body))
			}
		}
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if info.ChannelSetting.SystemPrompt != "" {
			// 如果有系统提示，则将其添加到请求中
			request, ok := convertedRequest.(*dto.GeneralOpenAIRequest)
			if ok {
				containSystemPrompt := false
				for _, message := range request.Messages {
					if message.Role == request.GetSystemRoleName() {
						containSystemPrompt = true
						break
					}
				}
				if !containSystemPrompt {
					// 如果没有系统提示，则添加系统提示
					systemMessage := dto.Message{
						Role:    request.GetSystemRoleName(),
						Content: info.ChannelSetting.SystemPrompt,
					}
					request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
				} else if info.ChannelSetting.SystemPromptOverride {
					common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
					// 如果有系统提示，且允许覆盖，则拼接到前面
					for i, message := range request.Messages {
						if message.Role == request.GetSystemRoleName() {
							if message.IsStringContent() {
								request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
							} else {
								contents := message.ParseContent()
								contents = append([]dto.MediaContent{
									{
										Type: dto.ContentTypeText,
										Text: info.ChannelSetting.SystemPrompt,
									},
								}, contents...)
								request.Messages[i].Content = contents
							}
							break
						}
					}
				}
			}
		}

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
			}
		}

		logger.LogDebug(c, fmt.Sprintf("text request body: %s", string(jsonData)))

		requestBody = bytes.NewBuffer(jsonData)
	}

	// Prepare a stable created_at for streaming logs even when we skip the "in progress" row.
	if info.IsStream && info.LogCreatedAt == 0 {
		info.LogCreatedAt = common.GetTimestamp()
	}

	// 在发起请求前创建初始日志条目（用于流式请求实时显示）
	if info.IsStream && info.LogId == 0 && common.LogConsumeInProgressEnabled {
		createdAt := info.LogCreatedAt
		initialOther := make(map[string]interface{})
		if info.ConversationId != "" {
			initialOther["conversation_id"] = info.ConversationId
		}
		if info.SessionId != "" {
			initialOther["session_id"] = info.SessionId
		}
		if info.PromptCacheKey != "" {
			initialOther["prompt_cache_key"] = info.PromptCacheKey
		}

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
			Other:            initialOther,
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
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
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

func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) *types.NewAPIError {
	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.PromptTokens,
			CompletionTokens: 0,
			TotalTokens:      relayInfo.PromptTokens,
		}
		extraContent += "（可能是请求出错）"
	}
	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	cacheTokens := usage.PromptTokensDetails.CachedTokens
	imageTokens := usage.PromptTokensDetails.ImageTokens
	audioTokens := usage.PromptTokensDetails.AudioTokens
	completionTokens := usage.CompletionTokens
	cachedCreationTokens := usage.PromptTokensDetails.CachedCreationTokens
	cachedCreationTokens5m := usage.ClaudeCacheCreation5mTokens
	cachedCreationTokens1h := usage.ClaudeCacheCreation1hTokens

	modelName := relayInfo.OriginModelName

	tokenName := ctx.GetString("token_name")
	completionRatio := relayInfo.PriceData.CompletionRatio
	cacheRatio := relayInfo.PriceData.CacheRatio
	imageRatio := relayInfo.PriceData.ImageRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.EffectiveGroupRatio
	publicGroupRatio := relayInfo.PriceData.GroupRatioInfo.PublicGroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	cachedCreationRatio := relayInfo.PriceData.CacheCreationRatio
	cachedCreationRatio5m := relayInfo.PriceData.CacheCreation5mRatio
	cachedCreationRatio1h := relayInfo.PriceData.CacheCreation1hRatio
	longContextInputMultiplier, longContextOutputMultiplier := relaycommon.LongContextInputOutputMultipliers(modelName, promptTokens, cacheTokens)

	upstreamModelName := strings.TrimSpace(relayInfo.UpstreamModelName)
	if upstreamModelName == "" {
		upstreamModelName = modelName
	}
	costModelPrice, costUsePrice := ratio_setting.GetModelPrice(upstreamModelName, false)
	costModelRatio, _, _ := ratio_setting.GetModelRatio(upstreamModelName)
	costModelPrice, costModelRatio, _ = relaycommon.ApplyServiceTierPricing(costUsePrice, costModelPrice, costModelRatio, relayInfo.EffectiveServiceTier())
	costCompletionRatio := ratio_setting.GetCompletionRatio(upstreamModelName)
	costCacheRatio, _ := ratio_setting.GetCacheRatio(upstreamModelName)
	costCachedCreationRatio, _ := ratio_setting.GetCreateCacheRatio(upstreamModelName)
	costCachedCreationRatio5m := costCachedCreationRatio
	costCachedCreationRatio1h := costCachedCreationRatio * claudeCacheCreation1hMultiplier
	costCachedCreationRatio, costCachedCreationRatio5m, costCachedCreationRatio1h = relaycommon.AdjustCacheCreationRatiosForServiceTier(upstreamModelName, relayInfo.EffectiveServiceTier(), costCachedCreationRatio, costCachedCreationRatio5m, costCachedCreationRatio1h)
	costImageRatio, _ := ratio_setting.GetImageRatio(upstreamModelName)
	costLongContextInputMultiplier, costLongContextOutputMultiplier := relaycommon.LongContextInputOutputMultipliers(upstreamModelName, promptTokens, cacheTokens)

	// Convert values to decimal for precise calculation
	dPromptTokens := decimal.NewFromInt(int64(promptTokens))
	dCacheTokens := decimal.NewFromInt(int64(cacheTokens))
	dImageTokens := decimal.NewFromInt(int64(imageTokens))
	dAudioTokens := decimal.NewFromInt(int64(audioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(completionTokens))
	dCachedCreationTokens := decimal.NewFromInt(int64(cachedCreationTokens))
	dCachedCreationTokens5m := decimal.NewFromInt(int64(cachedCreationTokens5m))
	dCachedCreationTokens1h := decimal.NewFromInt(int64(cachedCreationTokens1h))
	dCompletionRatio := decimal.NewFromFloat(completionRatio)
	dCacheRatio := decimal.NewFromFloat(cacheRatio)
	dImageRatio := decimal.NewFromFloat(imageRatio)
	dModelRatio := decimal.NewFromFloat(modelRatio)
	dGroupRatio := decimal.NewFromFloat(groupRatio)
	dPublicGroupRatio := decimal.NewFromFloat(publicGroupRatio)
	dModelPrice := decimal.NewFromFloat(modelPrice)
	dCachedCreationRatio := decimal.NewFromFloat(cachedCreationRatio)
	dCachedCreationRatio5m := decimal.NewFromFloat(cachedCreationRatio5m)
	dCachedCreationRatio1h := decimal.NewFromFloat(cachedCreationRatio1h)
	dLongContextInputMultiplier := decimal.NewFromFloat(longContextInputMultiplier)
	dLongContextOutputMultiplier := decimal.NewFromFloat(longContextOutputMultiplier)
	dCostCompletionRatio := decimal.NewFromFloat(costCompletionRatio)
	dCostCacheRatio := decimal.NewFromFloat(costCacheRatio)
	dCostImageRatio := decimal.NewFromFloat(costImageRatio)
	dCostModelRatio := decimal.NewFromFloat(costModelRatio)
	dCostModelPrice := decimal.NewFromFloat(costModelPrice)
	dCostCachedCreationRatio := decimal.NewFromFloat(costCachedCreationRatio)
	dCostCachedCreationRatio5m := decimal.NewFromFloat(costCachedCreationRatio5m)
	dCostCachedCreationRatio1h := decimal.NewFromFloat(costCachedCreationRatio1h)
	dCostLongContextInputMultiplier := decimal.NewFromFloat(costLongContextInputMultiplier)
	dCostLongContextOutputMultiplier := decimal.NewFromFloat(costLongContextOutputMultiplier)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio)

	// openai web search 工具计费
	var dWebSearchQuota decimal.Decimal
	var dVisibleWebSearchQuota decimal.Decimal
	var dCostWebSearchQuota decimal.Decimal
	var webSearchPrice float64
	// response api 格式工具计费
	if relayInfo.ResponsesUsageInfo != nil {
		webSearchTool := relaycommon.EnsureResponsesBuiltInTool(relayInfo, dto.BuildInToolWebSearch)
		if webSearchTool != nil && webSearchTool.CallCount > 0 {
			// 计算 web search 调用的配额 (配额 = 价格 * 调用次数 / 1000 * 分组倍率)
			searchContextSize := strings.TrimSpace(webSearchTool.SearchContextSize)
			if searchContextSize == "" {
				searchContextSize = "medium"
			}
			webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, searchContextSize)
			dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			dVisibleWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dPublicGroupRatio).Mul(dQuotaPerUnit)
			costWebSearchPrice := operation_setting.GetWebSearchPricePerThousand(upstreamModelName, searchContextSize)
			dCostWebSearchQuota = decimal.NewFromFloat(costWebSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("Web Search 调用 %d 次，上下文大小 %s，调用花费 %s",
				webSearchTool.CallCount, searchContextSize, dWebSearchQuota.String())
		}
	} else if strings.HasSuffix(modelName, "search-preview") {
		// search-preview 模型不支持 response api
		searchContextSize := ctx.GetString("chat_completion_web_search_context_size")
		if searchContextSize == "" {
			searchContextSize = "medium"
		}
		webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, searchContextSize)
		dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		dVisibleWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dPublicGroupRatio).Mul(dQuotaPerUnit)
		costWebSearchPrice := operation_setting.GetWebSearchPricePerThousand(upstreamModelName, searchContextSize)
		dCostWebSearchQuota = decimal.NewFromFloat(costWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Web Search 调用 1 次，上下文大小 %s，调用花费 %s",
			searchContextSize, dWebSearchQuota.String())
	}
	// claude web search tool 计费
	var dClaudeWebSearchQuota decimal.Decimal
	var dVisibleClaudeWebSearchQuota decimal.Decimal
	var dCostClaudeWebSearchQuota decimal.Decimal
	var claudeWebSearchPrice float64
	claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
	if claudeWebSearchCallCount > 0 {
		claudeWebSearchPrice = operation_setting.GetClaudeWebSearchPricePerThousand()
		dClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		dVisibleClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dPublicGroupRatio).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		dCostClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		extraContent += fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s",
			claudeWebSearchCallCount, dClaudeWebSearchQuota.String())
	}
	// file search tool 计费
	var dFileSearchQuota decimal.Decimal
	var dVisibleFileSearchQuota decimal.Decimal
	var dCostFileSearchQuota decimal.Decimal
	var fileSearchPrice float64
	if relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool.CallCount > 0 {
			fileSearchPrice = operation_setting.GetFileSearchPricePerThousand()
			dFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			dVisibleFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dPublicGroupRatio).Mul(dQuotaPerUnit)
			dCostFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("File Search 调用 %d 次，调用花费 %s",
				fileSearchTool.CallCount, dFileSearchQuota.String())
		}
	}
	var dImageGenerationCallQuota decimal.Decimal
	var dVisibleImageGenerationCallQuota decimal.Decimal
	var dCostImageGenerationCallQuota decimal.Decimal
	var imageGenerationCallPrice float64
	if ctx.GetBool("image_generation_call") {
		imageGenerationCallPrice = operation_setting.GetGPTImage1PriceOnceCall(ctx.GetString("image_generation_call_quality"), ctx.GetString("image_generation_call_size"))
		dImageGenerationCallQuota = decimal.NewFromFloat(imageGenerationCallPrice).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		dVisibleImageGenerationCallQuota = decimal.NewFromFloat(imageGenerationCallPrice).Mul(dPublicGroupRatio).Mul(dQuotaPerUnit)
		dCostImageGenerationCallQuota = decimal.NewFromFloat(imageGenerationCallPrice).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Image Generation Call 花费 %s", dImageGenerationCallQuota.String())
	}

	var quotaCalculateDecimal decimal.Decimal
	var visibleQuotaCalculateDecimal decimal.Decimal

	var audioInputQuota decimal.Decimal
	var visibleAudioInputQuota decimal.Decimal
	var audioInputPrice float64
	var visibleRatio decimal.Decimal
	isAnthropic := relayInfo.ApiType == constant.APITypeAnthropic
	if !relayInfo.PriceData.UsePrice {
		baseTokens := dPromptTokens
		// 减去 cached tokens
		var cachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			// Anthropic usage.PromptTokens 表示非缓存 input_tokens，cache_read/cache_creation 不包含在内，
			// 因此不能从 PromptTokens 中再减去 cache tokens，否则会导致有效输入为负数并被钳成 1，造成严重少扣费。
			if !isAnthropic {
				baseTokens = baseTokens.Sub(dCacheTokens)
			}
			cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
		}
		var dCachedCreationTokensWithRatio decimal.Decimal
		if !dCachedCreationTokens.IsZero() || !dCachedCreationTokens5m.IsZero() || !dCachedCreationTokens1h.IsZero() {
			if !isAnthropic {
				if !dCachedCreationTokens.IsZero() {
					baseTokens = baseTokens.Sub(dCachedCreationTokens)
				} else {
					baseTokens = baseTokens.Sub(dCachedCreationTokens5m).Sub(dCachedCreationTokens1h)
				}
			}
			dCachedCreationTokensWithRatio = dCachedCreationTokens5m.Mul(dCachedCreationRatio5m).
				Add(dCachedCreationTokens1h.Mul(dCachedCreationRatio1h))
			remainingCachedCreationTokens := dCachedCreationTokens.Sub(dCachedCreationTokens5m).Sub(dCachedCreationTokens1h)
			if remainingCachedCreationTokens.GreaterThan(decimal.Zero) {
				dCachedCreationTokensWithRatio = dCachedCreationTokensWithRatio.Add(remainingCachedCreationTokens.Mul(dCachedCreationRatio))
			} else if dCachedCreationTokensWithRatio.IsZero() {
				dCachedCreationTokensWithRatio = dCachedCreationTokens.Mul(dCachedCreationRatio)
			}
		}

		// 减去 image tokens
		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			baseTokens = baseTokens.Sub(dImageTokens)
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		// 减去 Gemini audio tokens
		if !dAudioTokens.IsZero() {
			audioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(modelName)
			if audioInputPrice > 0 {
				// 重新计算 base tokens
				baseTokens = baseTokens.Sub(dAudioTokens)
				audioInputQuota = decimal.NewFromFloat(audioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dQuotaPerUnit)
				visibleAudioInputQuota = decimal.NewFromFloat(audioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dPublicGroupRatio).Mul(dQuotaPerUnit)
				extraContent += fmt.Sprintf("Audio Input 花费 %s", audioInputQuota.String())
			}
		}
		promptQuota := baseTokens.Mul(dLongContextInputMultiplier).Add(cachedTokensWithRatio).
			Add(imageTokensWithRatio).
			Add(dCachedCreationTokensWithRatio)

		completionQuota := dCompletionTokens.Mul(dCompletionRatio).Mul(dLongContextOutputMultiplier)

		quotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(ratio)
		visibleRatio = dModelRatio.Mul(dPublicGroupRatio)
		visibleQuotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(visibleRatio)

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
		if !visibleRatio.IsZero() && visibleQuotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			visibleQuotaCalculateDecimal = decimal.NewFromInt(1)
		}
	} else {
		quotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio)
		visibleQuotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dPublicGroupRatio)
	}

	// cost side (ignore group/base multiplier; use actual upstream model pricing)
	var costQuotaCalculateDecimal decimal.Decimal
	var costAudioInputQuota decimal.Decimal
	if costUsePrice {
		costQuotaCalculateDecimal = dCostModelPrice.Mul(dQuotaPerUnit)
	} else {
		costBaseTokens := dPromptTokens
		// 减去 cached tokens
		var costCachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			if !isAnthropic {
				costBaseTokens = costBaseTokens.Sub(dCacheTokens)
			}
			costCachedTokensWithRatio = dCacheTokens.Mul(dCostCacheRatio)
		}
		var costCachedCreationTokensWithRatio decimal.Decimal
		if !dCachedCreationTokens.IsZero() || !dCachedCreationTokens5m.IsZero() || !dCachedCreationTokens1h.IsZero() {
			if !isAnthropic {
				if !dCachedCreationTokens.IsZero() {
					costBaseTokens = costBaseTokens.Sub(dCachedCreationTokens)
				} else {
					costBaseTokens = costBaseTokens.Sub(dCachedCreationTokens5m).Sub(dCachedCreationTokens1h)
				}
			}
			costCachedCreationTokensWithRatio = dCachedCreationTokens5m.Mul(dCostCachedCreationRatio5m).
				Add(dCachedCreationTokens1h.Mul(dCostCachedCreationRatio1h))
			costRemainingCachedCreationTokens := dCachedCreationTokens.Sub(dCachedCreationTokens5m).Sub(dCachedCreationTokens1h)
			if costRemainingCachedCreationTokens.GreaterThan(decimal.Zero) {
				costCachedCreationTokensWithRatio = costCachedCreationTokensWithRatio.Add(costRemainingCachedCreationTokens.Mul(dCostCachedCreationRatio))
			} else if costCachedCreationTokensWithRatio.IsZero() {
				costCachedCreationTokensWithRatio = dCachedCreationTokens.Mul(dCostCachedCreationRatio)
			}
		}

		// 减去 image tokens
		var costImageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			costBaseTokens = costBaseTokens.Sub(dImageTokens)
			costImageTokensWithRatio = dImageTokens.Mul(dCostImageRatio)
		}

		// Gemini audio input 独立计费（成本侧）
		if !dAudioTokens.IsZero() {
			costAudioInputPrice := operation_setting.GetGeminiInputAudioPricePerMillionTokens(upstreamModelName)
			if costAudioInputPrice > 0 {
				costBaseTokens = costBaseTokens.Sub(dAudioTokens)
				costAudioInputQuota = decimal.NewFromFloat(costAudioInputPrice).
					Div(decimal.NewFromInt(1000000)).
					Mul(dAudioTokens).
					Mul(dQuotaPerUnit)
			}
		}

		costPromptQuota := costBaseTokens.Mul(dCostLongContextInputMultiplier).Add(costCachedTokensWithRatio).
			Add(costImageTokensWithRatio).
			Add(costCachedCreationTokensWithRatio)

		costCompletionQuota := dCompletionTokens.Mul(dCostCompletionRatio).Mul(dCostLongContextOutputMultiplier)

		costQuotaCalculateDecimal = costPromptQuota.Add(costCompletionQuota).Mul(dCostModelRatio)

		if !dCostModelRatio.IsZero() && costQuotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			costQuotaCalculateDecimal = decimal.NewFromInt(1)
		}
	}
	// 添加 responses tools call 调用的配额
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dWebSearchQuota)
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dClaudeWebSearchQuota)
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dFileSearchQuota)
	visibleQuotaCalculateDecimal = visibleQuotaCalculateDecimal.Add(dVisibleWebSearchQuota)
	visibleQuotaCalculateDecimal = visibleQuotaCalculateDecimal.Add(dVisibleClaudeWebSearchQuota)
	visibleQuotaCalculateDecimal = visibleQuotaCalculateDecimal.Add(dVisibleFileSearchQuota)
	// 添加 audio input 独立计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)
	visibleQuotaCalculateDecimal = visibleQuotaCalculateDecimal.Add(visibleAudioInputQuota)
	// 添加 image generation call 计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dImageGenerationCallQuota)
	visibleQuotaCalculateDecimal = visibleQuotaCalculateDecimal.Add(dVisibleImageGenerationCallQuota)

	costQuotaCalculateDecimal = costQuotaCalculateDecimal.Add(dCostWebSearchQuota)
	costQuotaCalculateDecimal = costQuotaCalculateDecimal.Add(dCostClaudeWebSearchQuota)
	costQuotaCalculateDecimal = costQuotaCalculateDecimal.Add(dCostFileSearchQuota)
	costQuotaCalculateDecimal = costQuotaCalculateDecimal.Add(costAudioInputQuota)
	costQuotaCalculateDecimal = costQuotaCalculateDecimal.Add(dCostImageGenerationCallQuota)

	quota := int(quotaCalculateDecimal.Round(0).IntPart())
	visibleQuota := int(visibleQuotaCalculateDecimal.Round(0).IntPart())
	costQuota := int(costQuotaCalculateDecimal.Round(0).IntPart())
	totalTokens := promptTokens + completionTokens

	var logContent string

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		visibleQuota = 0
		costQuota = 0
		if reason := common.GetContextKeyString(ctx, constant.ContextKeyStreamExitReason); reason != "" {
			logContent += fmt.Sprintf("（usage统计失败，stream_exit_reason=%s）", reason)
		} else {
			logContent += fmt.Sprintf("（usage统计失败）")
		}
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		if !ratio.IsZero() && quota == 0 {
			quota = 1
		}
		if !visibleRatio.IsZero() && visibleQuota == 0 {
			visibleQuota = 1
		}
		if !costUsePrice && !dCostModelRatio.IsZero() && costQuota == 0 {
			costQuota = 1
		}
		model.UpdateUserUsageAndRequestCount(relayInfo.UserId, quota, visibleQuota, costQuota)
		model.UpdateChannelUsageQuotas(relayInfo.ChannelId, quota, visibleQuota, costQuota)
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

	//logger.LogInfo(ctx, fmt.Sprintf("request quota delta: %s", logger.FormatQuota(quotaDelta)))

	if quotaDelta > 0 {
		logger.LogRequestInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	} else if quotaDelta < 0 {
		logger.LogRequestInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(-quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	}

	logModel := modelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	var other map[string]interface{}
	if isAnthropic {
		other = service.GenerateClaudeOtherInfoWithQuotaMetrics(ctx, relayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio,
			cacheTokens, cacheRatio,
			cachedCreationTokens, cachedCreationRatio,
			cachedCreationTokens5m, cachedCreationRatio5m,
			cachedCreationTokens1h, cachedCreationRatio1h,
			modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio, relayInfo.PriceData.GroupRatioInfo.BaseMultiplierApplied, quota, visibleQuota, costQuota)
	} else {
		other = service.GenerateTextOtherInfoWithQuotaMetrics(ctx, relayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio,
			cacheTokens, cacheRatio, modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio, relayInfo.PriceData.GroupRatioInfo.BaseMultiplierApplied, quota, visibleQuota, costQuota)
		if cachedCreationTokens != 0 {
			other["cache_creation_tokens"] = cachedCreationTokens
			other["cache_creation_ratio"] = cachedCreationRatio
		}
	}
	if imageTokens != 0 {
		other["image"] = true
		other["image_ratio"] = imageRatio
		other["image_output"] = imageTokens
	}
	if !dWebSearchQuota.IsZero() {
		if relayInfo.ResponsesUsageInfo != nil {
			webSearchTool := relaycommon.EnsureResponsesBuiltInTool(relayInfo, dto.BuildInToolWebSearch)
			if webSearchTool != nil {
				other["web_search"] = true
				other["web_search_call_count"] = webSearchTool.CallCount
				other["web_search_price"] = webSearchPrice
			}
		} else if strings.HasSuffix(modelName, "search-preview") {
			other["web_search"] = true
			other["web_search_call_count"] = 1
			other["web_search_price"] = webSearchPrice
		}
	} else if !dClaudeWebSearchQuota.IsZero() {
		other["web_search"] = true
		other["web_search_call_count"] = claudeWebSearchCallCount
		other["web_search_price"] = claudeWebSearchPrice
	}
	if !dFileSearchQuota.IsZero() && relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists {
			other["file_search"] = true
			other["file_search_call_count"] = fileSearchTool.CallCount
			other["file_search_price"] = fileSearchPrice
		}
	}
	if !audioInputQuota.IsZero() {
		other["audio_input_seperate_price"] = true
		other["audio_input_token_count"] = audioTokens
		other["audio_input_price"] = audioInputPrice
	}
	if !dImageGenerationCallQuota.IsZero() {
		other["image_generation_call"] = true
		other["image_generation_call_price"] = imageGenerationCallPrice
	}

	if relayInfo.ConversationId != "" {
		other["conversation_id"] = relayInfo.ConversationId
	}
	if relayInfo.SessionId != "" {
		other["session_id"] = relayInfo.SessionId
	}
	if relayInfo.PromptCacheKey != "" {
		other["prompt_cache_key"] = relayInfo.PromptCacheKey
	}

	userDelta := quotaDelta
	if relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken {
		userDelta = service.ComputeTokenBucketUsage(relayInfo, totalTokens) - relayInfo.FinalPreConsumedTokens
	}

	if quotaDelta != 0 || userDelta != 0 {
		err := service.PostConsumeQuota(relayInfo, userDelta, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			// rollback user/channel used quota to keep invariant when final consume fails
			model.UpdateUserUsageQuotas(relayInfo.UserId, -quota, -visibleQuota, -costQuota)
			model.UpdateChannelUsageQuotas(relayInfo.ChannelId, -quota, -visibleQuota, -costQuota)
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
			if relayInfo.LogId > 0 {
				errorContent := logContent
				if errorContent != "" {
					errorContent += ", "
				}
				errorContent += "扣费失败: " + err.Error()
				model.UpdateConsumeLogAsError(ctx, relayInfo.LogId, model.RecordConsumeLogParams{
					CreatedAt:        relayInfo.LogCreatedAt,
					PromptTokens:     promptTokens,
					CompletionTokens: completionTokens,
					Quota:            quota,
					VisibleQuota:     visibleQuota,
					CostQuota:        costQuota,
					Content:          errorContent,
					UseTimeSeconds:   int(useTimeSeconds),
					Group:            fmt.Sprintf("%d", relayInfo.UsingGroupId),
					Other:            other,
				})
			}
			return service.ConvertPostConsumeError(err)
		}
	}

	// 如果已有初始日志条目（流式请求），则更新；否则新建
	if relayInfo.LogId > 0 {
		params := model.RecordConsumeLogParams{
			CreatedAt:        relayInfo.LogCreatedAt,
			ChannelId:        relayInfo.ChannelId,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			ModelName:        logModel,
			TokenName:        tokenName,
			Quota:            quota,
			VisibleQuota:     visibleQuota,
			CostQuota:        costQuota,
			Content:          logContent,
			TokenId:          relayInfo.TokenId,
			UseTimeSeconds:   int(useTimeSeconds),
			IsStream:         relayInfo.IsStream,
			Group:            fmt.Sprintf("%d", relayInfo.UsingGroupId),
			Other:            other,
		}
		model.UpdateConsumeLog(ctx, relayInfo.LogId, params)
	} else {
		model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
			CreatedAt:        relayInfo.LogCreatedAt,
			ChannelId:        relayInfo.ChannelId,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			ModelName:        logModel,
			TokenName:        tokenName,
			Quota:            quota,
			VisibleQuota:     visibleQuota,
			CostQuota:        costQuota,
			Content:          logContent,
			TokenId:          relayInfo.TokenId,
			UseTimeSeconds:   int(useTimeSeconds),
			IsStream:         relayInfo.IsStream,
			Group:            fmt.Sprintf("%d", relayInfo.UsingGroupId),
			Other:            other,
		})
	}

	return nil
}

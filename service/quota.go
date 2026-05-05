package service

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/setting/ratio_setting"
	"one-api/setting/system_setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) int {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return int(quota.IntPart())
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return int(quota.Round(0).IntPart())
}

func CalculateAudioQuotaProfile(settledInfo QuotaInfo, publicGroupRatio float64, upstreamModelName string, serviceTier string) types.QuotaProfile {
	if strings.TrimSpace(upstreamModelName) == "" {
		upstreamModelName = strings.TrimSpace(settledInfo.ModelName)
	}
	settledQuota := calculateAudioQuota(settledInfo)
	visibleInfo := settledInfo
	visibleInfo.GroupRatio = publicGroupRatio
	visibleQuota := calculateAudioQuota(visibleInfo)

	costModelPrice, costUsePrice := ratio_setting.GetModelPrice(upstreamModelName, false)
	costModelRatio, _, _ := ratio_setting.GetModelRatio(upstreamModelName)
	costModelPrice, costModelRatio, _ = relaycommon.ApplyServiceTierPricing(costUsePrice, costModelPrice, costModelRatio, serviceTier)
	costInfo := QuotaInfo{
		InputDetails:  settledInfo.InputDetails,
		OutputDetails: settledInfo.OutputDetails,
		ModelName:     upstreamModelName,
		UsePrice:      costUsePrice,
		ModelPrice:    costModelPrice,
		ModelRatio:    costModelRatio,
		GroupRatio:    1,
	}
	costQuota := calculateAudioQuota(costInfo)
	return types.QuotaProfile{
		SettledQuota: settledQuota,
		VisibleQuota: visibleQuota,
		CostQuota:    costQuota,
	}
}

// PreWssConsumeQuota 仅做额度与日限校验，不实际扣费，真正扣费由 PostWssConsumeQuota 统一完成。
func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) *types.NewAPIError {
	if relayInfo.UsePrice {
		return nil
	}
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketRequest {
		requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
		// Request-count subscription: reserve the current completed response round's
		// request units after group-ratio scaling,
		// then continue to token quota validation.
		if requestUnits > 0 && relayInfo.FinalPreConsumedRequests == 0 {
			subId, err := model.PreConsumeUserRequestSubscription(relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			relayInfo.FinalPreConsumedRequests = requestUnits
			relayInfo.RequestSubscriptionId = subId
		}
	}
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketPayRequest {
		requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
		// Pay-per-request: reserve the current completed response round's request units
		// after group-ratio scaling,
		// then continue to token quota validation.
		if requestUnits > 0 && relayInfo.FinalPreConsumedPayRequests == 0 {
			productId, err := model.PreConsumeUserPayRequestQuotaWithProduct(relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			relayInfo.FinalPreConsumedPayRequests = requestUnits
			relayInfo.PayRequestProductId = productId
		}
	}
	token, err := model.GetTokenByKey(strings.TrimLeft(relayInfo.TokenKey, "sk-"), false)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	actualGroupRatio := ResolveEffectiveGroupRatio(relayInfo)

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   relayInfo.UsePrice,
		ModelRatio: modelRatio,
		GroupRatio: actualGroupRatio,
	}

	quota := calculateAudioQuota(quotaInfo)

	// 日限与订阅日限前置校验，避免在当日可用额度不足时继续消费
	prevPreConsumed := relayInfo.FinalPreConsumedQuota
	prevPreConsumedTokens := relayInfo.FinalPreConsumedTokens
	relayInfo.FinalPreConsumedQuota = quota
	if relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken {
		relayInfo.FinalPreConsumedTokens = ComputeTokenBucketUsage(relayInfo, usage.TotalTokens)
	}
	if derr := ensureDailyQuotaAvailability(relayInfo); derr != nil {
		relayInfo.FinalPreConsumedQuota = prevPreConsumed
		relayInfo.FinalPreConsumedTokens = prevPreConsumedTokens
		return types.NewErrorWithStatusCode(derr, mapDailyQuotaErrorCode(derr), http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	relayInfo.FinalPreConsumedQuota = prevPreConsumed
	relayInfo.FinalPreConsumedTokens = prevPreConsumedTokens

	// 总额度校验：
	// - 新分桶逻辑：桶内余额校验已在 ensureDailyQuotaAvailability 完成；
	// - 旧逻辑：仍保留 users.quota 校验，但允许“无限订阅额度(total_quota=0)”放行。
	if relayInfo.QuotaBucket == "" {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota < quota {
			allow := false
			if relayInfo.UsingGroupId > 0 {
				_, _, totalUnlimited, _, capErr := model.GetUserSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
				if capErr == nil && totalUnlimited {
					allow = true
				}
			}
			if !allow {
				return types.NewErrorWithStatusCode(
					fmt.Errorf("用户额度不足, 剩余额度: %s, 需要消费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota)),
					types.ErrorCodeInsufficientUserQuota,
					http.StatusForbidden,
					types.ErrOptionWithSkipRetry(),
					types.ErrOptionWithNoRecordErrorLog(),
				)
			}
		}
	}

	// 令牌额度校验
	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota)),
			types.ErrorCodePreConsumeTokenQuotaFailed,
			http.StatusForbidden,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}

	// 这里只做校验，不真正扣费；实际扣费在 PostWssConsumeQuota 中一次性完成
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) *types.NewAPIError {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.EffectiveGroupRatio
	publicGroupRatio := relayInfo.PriceData.GroupRatioInfo.PublicGroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	upstreamModelName := strings.TrimSpace(relayInfo.UpstreamModelName)
	if upstreamModelName == "" {
		upstreamModelName = strings.TrimSpace(modelName)
	}
	if upstreamModelName == "" {
		upstreamModelName = relayInfo.OriginModelName
	}
	quotaProfile := CalculateAudioQuotaProfile(quotaInfo, publicGroupRatio, upstreamModelName, relayInfo.EffectiveServiceTier())
	quota := quotaProfile.SettledQuota
	visibleQuota := quotaProfile.VisibleQuota
	costQuota := quotaProfile.CostQuota

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		visibleQuota = 0
		costQuota = 0
		if reason := common.GetContextKeyString(ctx, constant.ContextKeyStreamExitReason); reason != "" {
			if errMsg := common.GetContextKeyString(ctx, constant.ContextKeyStreamExitError); errMsg != "" {
				logContent += fmt.Sprintf("（usage统计失败，stream_exit_reason=%s，stream_exit_error=%s）", reason, errMsg)
			} else {
				logContent += fmt.Sprintf("（usage统计失败，stream_exit_reason=%s）", reason)
			}
		} else {
			if errMsg := common.GetContextKeyString(ctx, constant.ContextKeyStreamExitError); errMsg != "" {
				logContent += fmt.Sprintf("（usage统计失败，stream_exit_error=%s）", errMsg)
			} else {
				logContent += fmt.Sprintf("（usage统计失败）")
			}
		}
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsageAndRequestCount(relayInfo.UserId, quota, visibleQuota, costQuota)
		model.UpdateChannelUsageQuotas(relayInfo.ChannelId, quota, visibleQuota, costQuota)
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

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

	userDelta := quotaDelta
	if relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken {
		userDelta = ComputeTokenBucketUsage(relayInfo, totalTokens) - relayInfo.FinalPreConsumedTokens
	}

	if quotaDelta != 0 || userDelta != 0 {
		err := PostConsumeQuota(relayInfo, userDelta, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			// rollback used quota when final consume fails to keep user total (used+remain) stable
			model.UpdateUserUsageQuotas(relayInfo.UserId, -quota, -visibleQuota, -costQuota)
			model.UpdateChannelUsageQuotas(relayInfo.ChannelId, -quota, -visibleQuota, -costQuota)
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
			return ConvertPostConsumeError(err)
		}
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfoWithQuotaMetrics(ctx, relayInfo, usage, modelRatio, groupRatio,
		publicGroupRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio, quota, visibleQuota, costQuota)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
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

	return nil
}

func PostClaudeConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) *types.NewAPIError {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens
	modelName := relayInfo.OriginModelName
	upstreamModelName := strings.TrimSpace(relayInfo.UpstreamModelName)
	if upstreamModelName == "" {
		upstreamModelName = modelName
	}

	tokenName := ctx.GetString("token_name")
	completionRatio := relayInfo.PriceData.CompletionRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.EffectiveGroupRatio
	publicGroupRatio := relayInfo.PriceData.GroupRatioInfo.PublicGroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	cacheRatio := relayInfo.PriceData.CacheRatio
	cacheTokens := usage.PromptTokensDetails.CachedTokens

	cacheCreationRatio := relayInfo.PriceData.CacheCreationRatio
	cacheCreationRatio5m := relayInfo.PriceData.CacheCreation5mRatio
	cacheCreationRatio1h := relayInfo.PriceData.CacheCreation1hRatio
	cacheCreationTokens := usage.PromptTokensDetails.CachedCreationTokens
	cacheCreationTokens5m := usage.ClaudeCacheCreation5mTokens
	cacheCreationTokens1h := usage.ClaudeCacheCreation1hTokens

	if relayInfo.ChannelType == constant.ChannelTypeOpenRouter {
		promptTokens -= cacheTokens
		isUsingCustomSettings := relayInfo.PriceData.UsePrice || hasCustomModelRatio(modelName, relayInfo.PriceData.ModelRatio)
		if cacheCreationTokens == 0 && relayInfo.PriceData.CacheCreationRatio != 1 && usage.Cost != 0 && !isUsingCustomSettings {
			maybeCacheCreationTokens := CalcOpenRouterCacheCreateTokens(*usage, relayInfo.PriceData)
			if maybeCacheCreationTokens >= 0 && promptTokens >= maybeCacheCreationTokens {
				cacheCreationTokens = maybeCacheCreationTokens
			}
		}
		promptTokens -= cacheCreationTokens
	}

	toolCharges := calculateBuiltInToolChargeSummary(ctx, relayInfo, modelName, upstreamModelName, groupRatio, publicGroupRatio)

	calculateQuota := 0.0
	if !relayInfo.PriceData.UsePrice {
		calculateQuota = float64(promptTokens)
		calculateQuota += float64(cacheTokens) * cacheRatio
		calculateQuota += float64(cacheCreationTokens5m) * cacheCreationRatio5m
		calculateQuota += float64(cacheCreationTokens1h) * cacheCreationRatio1h
		remainingCacheCreationTokens := cacheCreationTokens - cacheCreationTokens5m - cacheCreationTokens1h
		if remainingCacheCreationTokens > 0 {
			calculateQuota += float64(remainingCacheCreationTokens) * cacheCreationRatio
		}
		calculateQuota += float64(completionTokens) * completionRatio
		calculateQuota = calculateQuota * groupRatio * modelRatio
	} else {
		calculateQuota = modelPrice * common.QuotaPerUnit * groupRatio
	}
	visibleCalculateQuota := 0.0
	if !relayInfo.PriceData.UsePrice {
		visibleCalculateQuota = float64(promptTokens)
		visibleCalculateQuota += float64(cacheTokens) * cacheRatio
		visibleCalculateQuota += float64(cacheCreationTokens5m) * cacheCreationRatio5m
		visibleCalculateQuota += float64(cacheCreationTokens1h) * cacheCreationRatio1h
		remainingVisibleCacheCreationTokens := cacheCreationTokens - cacheCreationTokens5m - cacheCreationTokens1h
		if remainingVisibleCacheCreationTokens > 0 {
			visibleCalculateQuota += float64(remainingVisibleCacheCreationTokens) * cacheCreationRatio
		}
		visibleCalculateQuota += float64(completionTokens) * completionRatio
		visibleCalculateQuota = visibleCalculateQuota * publicGroupRatio * modelRatio
	} else {
		visibleCalculateQuota = modelPrice * common.QuotaPerUnit * publicGroupRatio
	}
	calculateQuota += toolCharges.ResponsesWebSearchQuota + toolCharges.ClaudeWebSearchQuota + toolCharges.FileSearchQuota
	visibleCalculateQuota += toolCharges.ResponsesWebSearchVisibleQuota + toolCharges.ClaudeWebSearchVisibleQuota + toolCharges.FileSearchVisibleQuota

	if modelRatio != 0 && calculateQuota <= 0 {
		calculateQuota = 1
	}
	if modelRatio != 0 && visibleCalculateQuota <= 0 {
		visibleCalculateQuota = 1
	}

	costModelPrice, costUsePrice := ratio_setting.GetModelPrice(upstreamModelName, false)
	costModelRatio, _, _ := ratio_setting.GetModelRatio(upstreamModelName)
	costModelPrice, costModelRatio, _ = relaycommon.ApplyServiceTierPricing(costUsePrice, costModelPrice, costModelRatio, relayInfo.EffectiveServiceTier())
	costCompletionRatio := ratio_setting.GetCompletionRatio(upstreamModelName)
	costCacheRatio, _ := ratio_setting.GetCacheRatio(upstreamModelName)
	costCacheCreationRatio, _ := ratio_setting.GetCreateCacheRatio(upstreamModelName)
	costCacheCreationRatio5m := costCacheCreationRatio
	costCacheCreationRatio1h := costCacheCreationRatio * claudeCacheCreation1hMultiplier

	costCalculateQuota := 0.0
	if !costUsePrice {
		costCalculateQuota = float64(promptTokens)
		costCalculateQuota += float64(cacheTokens) * costCacheRatio
		costCalculateQuota += float64(cacheCreationTokens5m) * costCacheCreationRatio5m
		costCalculateQuota += float64(cacheCreationTokens1h) * costCacheCreationRatio1h
		costRemainingCacheCreationTokens := cacheCreationTokens - cacheCreationTokens5m - cacheCreationTokens1h
		if costRemainingCacheCreationTokens > 0 {
			costCalculateQuota += float64(costRemainingCacheCreationTokens) * costCacheCreationRatio
		}
		costCalculateQuota += float64(completionTokens) * costCompletionRatio
		costCalculateQuota = costCalculateQuota * costModelRatio
	} else {
		costCalculateQuota = costModelPrice * common.QuotaPerUnit
	}
	if !costUsePrice && costModelRatio != 0 && costCalculateQuota <= 0 {
		costCalculateQuota = 1
	}
	costCalculateQuota += toolCharges.ResponsesWebSearchCostQuota + toolCharges.ClaudeWebSearchCostQuota + toolCharges.FileSearchCostQuota

	quota := int(calculateQuota)
	visibleQuota := int(visibleCalculateQuota)
	costQuota := int(costCalculateQuota)

	totalTokens := promptTokens + completionTokens

	var logContent string
	if toolExtraContent := toolCharges.ExtraContent(); toolExtraContent != "" {
		logContent += toolExtraContent
	}
	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		visibleQuota = 0
		costQuota = 0
		if logContent != "" {
			logContent += "，"
		}
		logContent += fmt.Sprintf("（可能是上游出错）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsageAndRequestCount(relayInfo.UserId, quota, visibleQuota, costQuota)
		model.UpdateChannelUsageQuotas(relayInfo.ChannelId, quota, visibleQuota, costQuota)
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

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

	userDelta := quotaDelta
	if relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken {
		userDelta = ComputeTokenBucketUsage(relayInfo, totalTokens) - relayInfo.FinalPreConsumedTokens
	}

	if quotaDelta != 0 || userDelta != 0 {
		err := PostConsumeQuota(relayInfo, userDelta, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			// rollback used quota when final consume fails to keep user total (used+remain) stable
			model.UpdateUserUsageQuotas(relayInfo.UserId, -quota, -visibleQuota, -costQuota)
			model.UpdateChannelUsageQuotas(relayInfo.ChannelId, -quota, -visibleQuota, -costQuota)
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
			return ConvertPostConsumeError(err)
		}
	}

	other := GenerateClaudeOtherInfoWithQuotaMetrics(ctx, relayInfo, modelRatio, groupRatio, publicGroupRatio, completionRatio,
		cacheTokens, cacheRatio,
		cacheCreationTokens, cacheCreationRatio,
		cacheCreationTokens5m, cacheCreationRatio5m,
		cacheCreationTokens1h, cacheCreationRatio1h,
		modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio, relayInfo.PriceData.GroupRatioInfo.BaseMultiplierApplied, quota, visibleQuota, costQuota)
	if toolCharges.ResponsesWebSearchCallCount > 0 {
		other["web_search"] = true
		other["web_search_call_count"] = toolCharges.ResponsesWebSearchCallCount
		other["web_search_price"] = toolCharges.ResponsesWebSearchPrice
	} else if toolCharges.ClaudeWebSearchCallCount > 0 {
		other["web_search"] = true
		other["web_search_call_count"] = toolCharges.ClaudeWebSearchCallCount
		other["web_search_price"] = toolCharges.ClaudeWebSearchPrice
	}
	if toolCharges.FileSearchCallCount > 0 {
		other["file_search"] = true
		other["file_search_call_count"] = toolCharges.FileSearchCallCount
		other["file_search_price"] = toolCharges.FileSearchPrice
	}
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        modelName,
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

	return nil
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) *types.NewAPIError {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.EffectiveGroupRatio
	publicGroupRatio := relayInfo.PriceData.GroupRatioInfo.PublicGroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  relayInfo.OriginModelName,
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	upstreamModelName := strings.TrimSpace(relayInfo.UpstreamModelName)
	if upstreamModelName == "" {
		upstreamModelName = relayInfo.OriginModelName
	}
	quotaProfile := CalculateAudioQuotaProfile(quotaInfo, publicGroupRatio, upstreamModelName, relayInfo.EffectiveServiceTier())
	quota := quotaProfile.SettledQuota
	visibleQuota := quotaProfile.VisibleQuota
	costQuota := quotaProfile.CostQuota

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

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
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsageAndRequestCount(relayInfo.UserId, quota, visibleQuota, costQuota)
		model.UpdateChannelUsageQuotas(relayInfo.ChannelId, quota, visibleQuota, costQuota)
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

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

	userDelta := quotaDelta
	if relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken {
		userDelta = ComputeTokenBucketUsage(relayInfo, totalTokens) - relayInfo.FinalPreConsumedTokens
	}

	if quotaDelta != 0 || userDelta != 0 {
		err := PostConsumeQuota(relayInfo, userDelta, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			// rollback used quota when final consume fails to keep user total (used+remain) stable
			model.UpdateUserUsageQuotas(relayInfo.UserId, -quota, -visibleQuota, -costQuota)
			model.UpdateChannelUsageQuotas(relayInfo.ChannelId, -quota, -visibleQuota, -costQuota)
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
			return ConvertPostConsumeError(err)
		}
	}

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfoWithQuotaMetrics(ctx, relayInfo, usage, modelRatio, groupRatio,
		publicGroupRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio, quota, visibleQuota, costQuota)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
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

	return nil
}

func ConvertPostConsumeError(err error) *types.NewAPIError {
	if errors.Is(err, model.ErrUserDailyQuotaExceeded) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeUserDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if errors.Is(err, model.ErrTokenDailyQuotaExceeded) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeTokenDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
}

func combineBillingRollbackError(primary error, rollbackErr error, action string) error {
	if rollbackErr == nil {
		return primary
	}
	if primary == nil {
		return rollbackErr
	}
	if strings.TrimSpace(action) == "" {
		action = "回滚"
	}
	return fmt.Errorf("%w; %s失败: %v", primary, action, rollbackErr)
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	//if relayInfo.TokenUnlimited {
	//	return nil
	//}
	token, err := model.GetTokenByKey(relayInfo.TokenKey, false)
	if err != nil {
		return err
	}
	if !relayInfo.TokenUnlimited && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}
	err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
	if err != nil {
		return err
	}
	return nil
}

func cloneProductQuotaAllocations(allocations []relaycommon.ProductQuotaAllocation) []relaycommon.ProductQuotaAllocation {
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

func appendSubscriptionAllocations(base []relaycommon.SubscriptionUnitAllocation, extra []relaycommon.SubscriptionUnitAllocation) []relaycommon.SubscriptionUnitAllocation {
	out := cloneSubscriptionAllocations(base)
	for _, allocation := range extra {
		if allocation.SubscriptionId == 0 || allocation.Amount <= 0 {
			continue
		}
		if n := len(out); n > 0 &&
			out[n-1].SubscriptionId == allocation.SubscriptionId &&
			out[n-1].GroupId == allocation.GroupId &&
			out[n-1].StatDate == allocation.StatDate &&
			out[n-1].UsesGroupDailyLimit == allocation.UsesGroupDailyLimit {
			out[n-1].Amount += allocation.Amount
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

func splitTrailingSubscriptionAllocations(allocations []relaycommon.SubscriptionUnitAllocation, amount int) (remaining []relaycommon.SubscriptionUnitAllocation, refund []relaycommon.SubscriptionUnitAllocation, ok bool) {
	if amount < 0 {
		return nil, nil, false
	}
	remaining = cloneSubscriptionAllocations(allocations)
	if amount == 0 {
		return remaining, nil, true
	}
	left := amount
	refund = make([]relaycommon.SubscriptionUnitAllocation, 0, len(remaining))
	for i := len(remaining) - 1; i >= 0 && left > 0; i-- {
		current := remaining[i]
		if current.SubscriptionId == 0 || current.Amount <= 0 {
			continue
		}
		useAmount := current.Amount
		if useAmount > left {
			useAmount = left
		}
		if useAmount <= 0 {
			continue
		}
		refund = append(refund, relaycommon.SubscriptionUnitAllocation{
			SubscriptionId:      current.SubscriptionId,
			GroupId:             current.GroupId,
			StatDate:            current.StatDate,
			Amount:              useAmount,
			UsesGroupDailyLimit: current.UsesGroupDailyLimit,
		})
		remaining[i].Amount -= useAmount
		left -= useAmount
	}
	if left > 0 {
		return nil, nil, false
	}
	compacted := make([]relaycommon.SubscriptionUnitAllocation, 0, len(remaining))
	for _, allocation := range remaining {
		if allocation.SubscriptionId == 0 || allocation.Amount <= 0 {
			continue
		}
		compacted = append(compacted, allocation)
	}
	for i, j := 0, len(refund)-1; i < j; i, j = i+1, j-1 {
		refund[i], refund[j] = refund[j], refund[i]
	}
	return compacted, refund, true
}

func appendProductQuotaAllocations(base []relaycommon.ProductQuotaAllocation, extra []relaycommon.ProductQuotaAllocation) []relaycommon.ProductQuotaAllocation {
	out := cloneProductQuotaAllocations(base)
	for _, allocation := range extra {
		if allocation.ProductId == 0 || allocation.Quota <= 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].ProductId == allocation.ProductId {
			out[n-1].Quota += allocation.Quota
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

func firstProductIDFromAllocations(allocations []relaycommon.ProductQuotaAllocation) int {
	for _, allocation := range allocations {
		if allocation.ProductId != 0 && allocation.Quota > 0 {
			return allocation.ProductId
		}
	}
	return 0
}

func splitTrailingProductQuotaAllocations(allocations []relaycommon.ProductQuotaAllocation, quota int) (remaining []relaycommon.ProductQuotaAllocation, refund []relaycommon.ProductQuotaAllocation, ok bool) {
	if quota < 0 {
		return nil, nil, false
	}
	remaining = cloneProductQuotaAllocations(allocations)
	if quota == 0 {
		return remaining, nil, true
	}
	left := quota
	refund = make([]relaycommon.ProductQuotaAllocation, 0, len(remaining))
	for i := len(remaining) - 1; i >= 0 && left > 0; i-- {
		current := remaining[i]
		if current.ProductId == 0 || current.Quota <= 0 {
			continue
		}
		useQuota := current.Quota
		if useQuota > left {
			useQuota = left
		}
		if useQuota <= 0 {
			continue
		}
		refund = append(refund, relaycommon.ProductQuotaAllocation{
			ProductId: current.ProductId,
			Quota:     useQuota,
		})
		remaining[i].Quota -= useQuota
		left -= useQuota
	}
	if left > 0 {
		return nil, nil, false
	}
	compacted := make([]relaycommon.ProductQuotaAllocation, 0, len(remaining))
	for _, allocation := range remaining {
		if allocation.ProductId == 0 || allocation.Quota <= 0 {
			continue
		}
		compacted = append(compacted, allocation)
	}
	for i, j := 0, len(refund)-1; i < j; i, j = i+1, j-1 {
		refund[i], refund[j] = refund[j], refund[i]
	}
	return compacted, refund, true
}

func setRelayPaygAllocations(relayInfo *relaycommon.RelayInfo, allocations []relaycommon.ProductQuotaAllocation) {
	if relayInfo == nil {
		return
	}
	relayInfo.PaygProductAllocations = cloneProductQuotaAllocations(allocations)
	relayInfo.PaygProductId = firstProductIDFromAllocations(relayInfo.PaygProductAllocations)
}

func setRelaySubscriptionAllocations(relayInfo *relaycommon.RelayInfo, allocations []relaycommon.SubscriptionUnitAllocation) {
	if relayInfo == nil {
		return
	}
	relayInfo.SubscriptionAllocations = cloneSubscriptionAllocations(allocations)
}

func currentRelayPaygAllocations(relayInfo *relaycommon.RelayInfo) []relaycommon.ProductQuotaAllocation {
	if relayInfo == nil {
		return nil
	}
	if len(relayInfo.PaygProductAllocations) > 0 {
		return cloneProductQuotaAllocations(relayInfo.PaygProductAllocations)
	}
	if relayInfo.PaygProductId != 0 && relayInfo.FinalPreConsumedQuota > 0 {
		return []relaycommon.ProductQuotaAllocation{
			{ProductId: relayInfo.PaygProductId, Quota: relayInfo.FinalPreConsumedQuota},
		}
	}
	return nil
}

func currentRelaySubscriptionAllocations(relayInfo *relaycommon.RelayInfo) []relaycommon.SubscriptionUnitAllocation {
	if relayInfo == nil {
		return nil
	}
	return cloneSubscriptionAllocations(relayInfo.SubscriptionAllocations)
}

func consumeRelayPaygQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo == nil {
		return errors.New("relayInfo is nil")
	}
	if quota <= 0 {
		return nil
	}
	allocations, err := model.DecreaseUserPaygQuotaWithAllocations(relayInfo.UserId, relayInfo.UsingGroupId, quota)
	if err != nil {
		return err
	}
	setRelayPaygAllocations(relayInfo, appendProductQuotaAllocations(currentRelayPaygAllocations(relayInfo), allocations))
	return nil
}

func consumeRelaySubscriptionQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo == nil {
		return errors.New("relayInfo is nil")
	}
	if quota <= 0 {
		return nil
	}
	switch relayInfo.QuotaBucket {
	case model.UserQuotaBucketSubscription, model.UserQuotaBucketTokens:
	default:
		return errors.New("当前计费桶不支持订阅分配快照")
	}
	_, allocations, err := model.DecreaseUserQuotaByBucketWithAllocations(
		relayInfo.UserId,
		quota,
		relayInfo.QuotaBucket,
		relayInfo.UsingGroupId,
		0,
	)
	if err != nil {
		return err
	}
	setRelaySubscriptionAllocations(relayInfo, appendSubscriptionAllocations(currentRelaySubscriptionAllocations(relayInfo), allocations))
	return nil
}

func returnRelayPaygQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo == nil {
		return errors.New("relayInfo is nil")
	}
	if quota <= 0 {
		return nil
	}
	remaining, refund, ok := splitTrailingProductQuotaAllocations(currentRelayPaygAllocations(relayInfo), quota)
	if !ok {
		if relayInfo.PaygProductId == 0 {
			return errors.New("按量付费商品未指定，无法返还额度")
		}
		refund = []relaycommon.ProductQuotaAllocation{
			{ProductId: relayInfo.PaygProductId, Quota: quota},
		}
		remaining = nil
	}
	if err := model.ReturnUserPaygQuotaWithAllocations(relayInfo.UserId, refund); err != nil {
		return err
	}
	setRelayPaygAllocations(relayInfo, remaining)
	return nil
}

func returnRelaySubscriptionQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo == nil {
		return errors.New("relayInfo is nil")
	}
	if quota <= 0 {
		return nil
	}
	switch relayInfo.QuotaBucket {
	case model.UserQuotaBucketSubscription, model.UserQuotaBucketTokens:
	default:
		return errors.New("当前计费桶不支持订阅分配快照")
	}
	current := currentRelaySubscriptionAllocations(relayInfo)
	if len(current) == 0 {
		return model.ReturnUserQuotaByBucket(relayInfo.UserId, quota, relayInfo.QuotaBucket, relayInfo.UsingGroupId, 0)
	}
	remaining, refund, ok := splitTrailingSubscriptionAllocations(current, quota)
	if !ok {
		return errors.New("订阅扣费快照不足，无法精确返还额度")
	}
	if err := model.ReturnUserQuotaByBucketWithAllocations(relayInfo.UserId, quota, relayInfo.QuotaBucket, relayInfo.UsingGroupId, 0, refund); err != nil {
		return err
	}
	setRelaySubscriptionAllocations(relayInfo, remaining)
	return nil
}

func adjustOutstandingPreConsumed(current int, delta int, field string) int {
	next := current + delta
	if next < 0 {
		common.SysLog(fmt.Sprintf("unexpected negative %s after post consume reconcile: current=%d delta=%d", field, current, delta))
		return 0
	}
	return next
}

func reconcileRelayPreConsumedState(relayInfo *relaycommon.RelayInfo, userDelta int, tokenDelta int, requestCountBucket bool) {
	if relayInfo == nil {
		return
	}
	if requestCountBucket {
		relayInfo.FinalPreConsumedQuota = adjustOutstandingPreConsumed(relayInfo.FinalPreConsumedQuota, tokenDelta, "final_pre_consumed_quota")
		return
	}
	switch relayInfo.QuotaBucket {
	case model.UserQuotaBucketTokens, model.UserQuotaBucketPayToken:
		relayInfo.FinalPreConsumedTokens = adjustOutstandingPreConsumed(relayInfo.FinalPreConsumedTokens, userDelta, "final_pre_consumed_tokens")
		relayInfo.FinalPreConsumedQuota = adjustOutstandingPreConsumed(relayInfo.FinalPreConsumedQuota, tokenDelta, "final_pre_consumed_quota")
	default:
		relayInfo.FinalPreConsumedQuota = adjustOutstandingPreConsumed(relayInfo.FinalPreConsumedQuota, userDelta, "final_pre_consumed_quota")
	}
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, userQuotaDelta int, tokenQuotaDelta int, preConsumedTokenQuota int, sendEmail bool) (err error) {
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		return nil
	}
	requestCountBucket := relayInfo != nil &&
		(relayInfo.QuotaBucket == model.UserQuotaBucketRequest || relayInfo.QuotaBucket == model.UserQuotaBucketPayRequest)

	getBucketProductID := func() int {
		if relayInfo == nil {
			return 0
		}
		switch relayInfo.QuotaBucket {
		case model.UserQuotaBucketPayToken:
			return relayInfo.PayTokenProductId
		default:
			return 0
		}
	}
	setBucketProductID := func(productID int) {
		if relayInfo == nil || productID == 0 {
			return
		}
		switch relayInfo.QuotaBucket {
		case model.UserQuotaBucketPayToken:
			relayInfo.PayTokenProductId = productID
		}
	}

	userDelta := userQuotaDelta
	if requestCountBucket {
		userDelta = 0
	}
	if userDelta > 0 {
		if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
			err = consumeRelayPaygQuota(relayInfo, userDelta)
		} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription || relayInfo.QuotaBucket == model.UserQuotaBucketTokens {
			err = consumeRelaySubscriptionQuota(relayInfo, userDelta)
		} else if relayInfo.QuotaBucket != "" {
			selectedPaygProductId, decErr := model.DecreaseUserQuotaByBucket(
				relayInfo.UserId,
				userDelta,
				relayInfo.QuotaBucket,
				relayInfo.UsingGroupId,
				getBucketProductID(),
			)
			setBucketProductID(selectedPaygProductId)
			err = decErr
		} else {
			err = model.DecreaseUserQuota(relayInfo.UserId, userDelta)
		}
		if errors.Is(err, model.ErrUserDailyQuotaExceeded) {
			var totalRemaining, dailyCapacity int
			var totalUnlimited, dailyUnlimited bool
			var capErr error
			if relayInfo.QuotaBucket == model.UserQuotaBucketTokens {
				totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, capErr = model.GetUserTokenSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
			} else {
				totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, capErr = model.GetUserSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
			}
			if capErr != nil {
				err = wrapQuotaDetailError(err, buildUserDailyQuotaExceededMessage(relayInfo, userDelta, -1, -1))
			} else {
				if totalUnlimited {
					totalRemaining = -2
				}
				if dailyUnlimited {
					dailyCapacity = -2
				}
				err = wrapQuotaDetailError(err, buildUserDailyQuotaExceededMessage(relayInfo, userDelta, totalRemaining, dailyCapacity))
			}
			return err
		}
	} else if userDelta < 0 {
		if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
			err = returnRelayPaygQuota(relayInfo, -userDelta)
		} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription || relayInfo.QuotaBucket == model.UserQuotaBucketTokens {
			err = returnRelaySubscriptionQuota(relayInfo, -userDelta)
		} else if relayInfo.QuotaBucket != "" {
			err = model.ReturnUserQuotaByBucket(
				relayInfo.UserId,
				-userDelta,
				relayInfo.QuotaBucket,
				relayInfo.UsingGroupId,
				getBucketProductID(),
			)
		} else {
			err = model.ReturnUserQuota(relayInfo.UserId, -userDelta)
		}
	}
	if err != nil {
		return err
	}

	if !relayInfo.IsPlayground {
		tokenDelta := tokenQuotaDelta
		if tokenDelta > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, tokenDelta)
		} else if tokenDelta < 0 {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -tokenDelta)
		}
		if err != nil {
			if errors.Is(err, model.ErrTokenDailyQuotaExceeded) {
				if token, tokErr := model.GetTokenById(relayInfo.TokenId); tokErr == nil && token != nil {
					today := common.GetTodayDateInt()
					usedToday := token.DailyQuotaUsed
					if token.DailyQuotaResetDate != today {
						usedToday = 0
					}
					remaining := token.DailyQuotaLimit - usedToday
					err = wrapQuotaDetailError(err, buildTokenDailyQuotaExceededMessage(relayInfo, tokenDelta, token.DailyQuotaLimit, usedToday, remaining))
				}
			}

			// rollback user quota
			var rollbackErr error
			if userDelta > 0 {
				if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
					rollbackErr = returnRelayPaygQuota(relayInfo, userDelta)
				} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription || relayInfo.QuotaBucket == model.UserQuotaBucketTokens {
					rollbackErr = returnRelaySubscriptionQuota(relayInfo, userDelta)
				} else if relayInfo.QuotaBucket != "" {
					rollbackErr = model.ReturnUserQuotaByBucket(
						relayInfo.UserId,
						userDelta,
						relayInfo.QuotaBucket,
						relayInfo.UsingGroupId,
						getBucketProductID(),
					)
				} else {
					rollbackErr = model.ReturnUserQuota(relayInfo.UserId, userDelta)
				}
			} else if userDelta < 0 {
				rollbackQuota := -userDelta
				if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
					rollbackErr = consumeRelayPaygQuota(relayInfo, rollbackQuota)
				} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription || relayInfo.QuotaBucket == model.UserQuotaBucketTokens {
					rollbackErr = consumeRelaySubscriptionQuota(relayInfo, rollbackQuota)
				} else if relayInfo.QuotaBucket != "" {
					_, rollbackErr = model.DecreaseUserQuotaByBucket(
						relayInfo.UserId,
						rollbackQuota,
						relayInfo.QuotaBucket,
						relayInfo.UsingGroupId,
						getBucketProductID(),
					)
				} else {
					rollbackErr = model.DecreaseUserQuota(relayInfo.UserId, rollbackQuota)
				}
			}
			return combineBillingRollbackError(err, rollbackErr, "回滚用户额度")
		}
	}

	if sendEmail && !requestCountBucket && relayInfo.QuotaBucket != model.UserQuotaBucketTokens && relayInfo.QuotaBucket != model.UserQuotaBucketPayToken {
		if (tokenQuotaDelta + preConsumedTokenQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, tokenQuotaDelta, preConsumedTokenQuota)
		}
	}

	reconcileRelayPreConsumedState(relayInfo, userDelta, tokenQuotaDelta, requestCountBucket)

	return nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := fmt.Sprintf("%s/topup", system_setting.ServerAddress)

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink, topUpLink}
			}

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
	})
}

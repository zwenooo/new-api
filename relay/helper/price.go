package helper

import (
	"fmt"
	"one-api/common"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// HandleGroupRatio resolves effective group ratio (including user-group special ratio).
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		EffectiveGroupRatio: 1.0, // default ratio
		PublicGroupRatio:    1.0,
		PrivateGroupRatio:   1.0,
		GroupRatio:          1.0, // legacy alias for EffectiveGroupRatio
		GroupSpecialRatio:   -1,
	}
	if relayInfo != nil && relayInfo.UsingGroupId > 0 {
		publicGroupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroupId)
		groupRatioInfo.EffectiveGroupRatio = publicGroupRatio
		groupRatioInfo.PublicGroupRatio = publicGroupRatio
		groupRatioInfo.PrivateGroupRatio = publicGroupRatio
		groupRatioInfo.GroupRatio = publicGroupRatio
	}
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		groupRatioInfo.EffectiveGroupRatio = 0
		groupRatioInfo.PrivateGroupRatio = 0
		groupRatioInfo.GroupRatio = 0
		groupRatioInfo.PublicGroupRatio = 0
		return groupRatioInfo
	}
	if relayInfo == nil {
		return groupRatioInfo
	}
	resolvedInfo, _, err := model.ResolveUserGroupRatioInfoByID(relayInfo.UserId, relayInfo.UsingGroupId)
	if err != nil {
		return groupRatioInfo
	}
	return resolvedInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)
	serviceTier := info.EffectiveServiceTier()
	serviceTierMultiplier := relaycommon.ServiceTierCostMultiplier(serviceTier)

	groupRatioInfo := HandleGroupRatio(c, info)

	var preConsumedQuota int
	var visiblePreConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	if !usePrice {
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, fmt.Errorf("模型 %s 倍率或价格未配置，请联系管理员设置或开始自用模式；Model %s ratio or price not set, please set or start self-use mode", matchName, matchName)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		modelPrice, modelRatio, serviceTierMultiplier = relaycommon.ApplyServiceTierPricing(usePrice, modelPrice, modelRatio, serviceTier)
		cacheCreationRatio, cacheCreationRatio5m, cacheCreationRatio1h = relaycommon.AdjustCacheCreationRatiosForServiceTier(info.OriginModelName, serviceTier, cacheCreationRatio, cacheCreationRatio5m, cacheCreationRatio1h)
		ratio := modelRatio * groupRatioInfo.EffectiveGroupRatio
		visibleRatio := modelRatio * groupRatioInfo.PublicGroupRatio
		inputLongMultiplier, outputLongMultiplier := relaycommon.LongContextInputOutputMultipliers(info.OriginModelName, promptTokens, 0)

		// Pre-consume estimate:
		// - prompt tokens use base ratio
		// - max output tokens (completion) use completion ratio
		promptPart := float64(common.Max(promptTokens, common.PreConsumedQuota)) * ratio * inputLongMultiplier
		completionPart := 0.0
		if meta.MaxTokens > 0 && completionRatio > 0 {
			completionPart = float64(meta.MaxTokens) * ratio * completionRatio * outputLongMultiplier
		}
		preConsumedQuota = int(promptPart + completionPart)

		visiblePromptPart := float64(common.Max(promptTokens, common.PreConsumedQuota)) * visibleRatio * inputLongMultiplier
		visibleCompletionPart := 0.0
		if meta.MaxTokens > 0 && completionRatio > 0 {
			visibleCompletionPart = float64(meta.MaxTokens) * visibleRatio * completionRatio * outputLongMultiplier
		}
		visiblePreConsumedQuota = int(visiblePromptPart + visibleCompletionPart)
	} else {
		modelPrice, modelRatio, serviceTierMultiplier = relaycommon.ApplyServiceTierPricing(usePrice, modelPrice, modelRatio, serviceTier)
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.EffectiveGroupRatio)
		visiblePreConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.PublicGroupRatio)
	}

	priceData := types.PriceData{
		ModelPrice:              modelPrice,
		ModelRatio:              modelRatio,
		CompletionRatio:         completionRatio,
		GroupRatioInfo:          groupRatioInfo,
		UsePrice:                usePrice,
		CacheRatio:              cacheRatio,
		ImageRatio:              imageRatio,
		AudioRatio:              audioRatio,
		AudioCompletionRatio:    audioCompletionRatio,
		CacheCreationRatio:      cacheCreationRatio,
		CacheCreation5mRatio:    cacheCreationRatio5m,
		CacheCreation1hRatio:    cacheCreationRatio1h,
		ShouldPreConsumedQuota:  preConsumedQuota,
		VisiblePreConsumedQuota: visiblePreConsumedQuota,
		ServiceTier:             serviceTier,
		ServiceTierMultiplier:   serviceTierMultiplier,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次计费的 PriceHelper (MJ、Task)
func ComputePerCallQuotaProfile(modelPrice, costModelPrice float64, groupRatioInfo types.GroupRatioInfo) types.QuotaProfile {
	return types.QuotaProfile{
		SettledQuota: int(modelPrice * common.QuotaPerUnit * groupRatioInfo.EffectiveGroupRatio),
		VisibleQuota: int(modelPrice * common.QuotaPerUnit * groupRatioInfo.PublicGroupRatio),
		CostQuota:    int(costModelPrice * common.QuotaPerUnit),
	}
}

func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) types.PerCallPriceData {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	// 如果没有配置价格，则使用默认价格
	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelRatioMap()[info.OriginModelName]
		if !ok {
			modelPrice = 0.1
		} else {
			modelPrice = defaultPrice
		}
	}
	modelPrice, _, _ = relaycommon.ApplyServiceTierPricing(true, modelPrice, 0, info.EffectiveServiceTier())

	costModelName := strings.TrimSpace(info.UpstreamModelName)
	if costModelName == "" {
		costModelName = info.OriginModelName
	}
	costModelPrice, costSuccess := ratio_setting.GetModelPrice(costModelName, true)
	if !costSuccess {
		costModelPrice = modelPrice
	}
	costModelPrice, _, _ = relaycommon.ApplyServiceTierPricing(true, costModelPrice, 0, info.EffectiveServiceTier())

	quotaProfile := ComputePerCallQuotaProfile(modelPrice, costModelPrice, groupRatioInfo)
	priceData := types.PerCallPriceData{
		ModelPrice:     modelPrice,
		Quota:          quotaProfile.SettledQuota,
		VisibleQuota:   quotaProfile.VisibleQuota,
		CostQuota:      quotaProfile.CostQuota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData
}

func ContainPriceOrRatio(modelName string) bool {
	_, ok := ratio_setting.GetModelPrice(modelName, false)
	if ok {
		return true
	}
	_, ok, _ = ratio_setting.GetModelRatio(modelName)
	if ok {
		return true
	}
	return false
}

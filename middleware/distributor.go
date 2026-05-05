package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relayconstant "one-api/relay/constant"
	"one-api/service"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model   string `json:"model"`
	GroupId int    `json:"group_id,omitempty"`
}

const channelConcurrencySlotContextKey = "__channel_concurrency_slot"

type heldChannelConcurrencySlot struct {
	channelID int
}

func normalizeRequestedModelName(model string) string {
	return strings.TrimSpace(model)
}

func shouldDeferFinalRelayChannelSelection(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	return strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/v1beta/")
}

func getHeldChannelConcurrencySlot(c *gin.Context) (*heldChannelConcurrencySlot, bool) {
	if c == nil {
		return nil, false
	}
	raw, ok := c.Get(channelConcurrencySlotContextKey)
	if !ok || raw == nil {
		return nil, false
	}
	slot, ok := raw.(*heldChannelConcurrencySlot)
	if !ok || slot == nil || slot.channelID <= 0 {
		return nil, false
	}
	return slot, true
}

func setHeldChannelConcurrencySlot(c *gin.Context, slot *heldChannelConcurrencySlot) {
	if c == nil {
		return
	}
	if slot == nil {
		if c.Keys != nil {
			delete(c.Keys, channelConcurrencySlotContextKey)
		}
		return
	}
	c.Set(channelConcurrencySlotContextKey, slot)
}

func releaseHeldChannelConcurrencySlot(c *gin.Context) {
	slot, ok := getHeldChannelConcurrencySlot(c)
	if !ok {
		return
	}
	model.ReleaseChannelConcurrency(slot.channelID)
	setHeldChannelConcurrencySlot(c, nil)
}

func ReleaseCurrentChannelConcurrencySlot(c *gin.Context) {
	releaseHeldChannelConcurrencySlot(c)
}

func ensureChannelConcurrencySlot(c *gin.Context, channel *model.Channel) *types.NewAPIError {
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	held, hasHeld := getHeldChannelConcurrencySlot(c)
	if hasHeld && held.channelID == channel.Id {
		return nil
	}

	limit := channel.GetMaxConcurrency()
	if limit > 0 && !model.TryAcquireChannelRequestSlot(channel) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("渠道 %s（#%d）已达到最大并行度 %d", channel.Name, channel.Id, limit),
			types.ErrorCodeChannelConcurrencyExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
		)
	}

	if hasHeld && held.channelID != channel.Id {
		model.ReleaseChannelConcurrency(held.channelID)
	}
	if limit > 0 {
		setHeldChannelConcurrencySlot(c, &heldChannelConcurrencySlot{channelID: channel.Id})
	} else {
		setHeldChannelConcurrencySlot(c, nil)
	}
	return nil
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request, "+err.Error())
			return
		}
		if modelRequest != nil {
			seedOriginalModelContext(c, modelRequest.Model)
		}

		var channel *model.Channel
		var candidateGroupIDs []int
		needSelectChannel := shouldSelectChannel
		if channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
			id, err := strconv.Atoi(channelId.(string))
			if err == nil {
				channel, err = model.GetChannelById(id, true)
			}
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				if !shouldSelectChannel {
					abortWithOpenAiMessage(c, http.StatusBadRequest, "该渠道已被禁用")
					return
				}
				clearSpecificChannelBinding(c)
				channel = nil
			} else {
				needSelectChannel = false
			}
		}

		deferFinalChannelSelection := false
		if needSelectChannel {
			// Select a channel for the user
			// check token model mapping
			modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
			if modelLimitEnable {
				s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
				if !ok {
					// token model limit is empty, all models are not allowed
					abortWithOpenAiMessage(c, http.StatusBadRequest, "该令牌无权访问任何模型")
					return
				}
				var tokenModelLimit map[string]bool
				tokenModelLimit, ok = s.(map[string]bool)
				if !ok {
					tokenModelLimit = map[string]bool{}
				}
				matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
				if _, ok := tokenModelLimit[matchName]; !ok {
					abortWithOpenAiMessage(c, http.StatusBadRequest, "该令牌无权访问模型 "+modelRequest.Model)
					return
				}
			}

			usingGroupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
			if usingGroupID <= 0 {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的分组")
				return
			}

			selectionAuthority, selectionErr := ResolveRuntimeSelectionAuthority(
				c,
				c.Request.URL.Path,
				modelRequest.Model,
				modelRequest.GroupId,
			)
			if selectionErr != nil {
				abortWithOpenAiMessage(c, selectionErr.StatusCode, selectionErr.Error(), string(selectionErr.GetErrorCode()))
				return
			}
			usingGroupID = selectionAuthority.CurrentGroupID
			candidateGroupIDs = append([]int(nil), selectionAuthority.CandidateGroupIDs...)

			if channel == nil && shouldDeferFinalRelayChannelSelection(c) {
				deferFinalChannelSelection = true
			}

			if !deferFinalChannelSelection && len(candidateGroupIDs) > 0 {
				modelLookupErr := func(groupID int, lookupModel string, err error) {
					if strings.TrimSpace(lookupModel) == "" {
						lookupModel = modelRequest.Model
					}
					if model.IsChannelConcurrencyLimitReachedErr(err) {
						abortWithOpenAiMessage(
							c,
							http.StatusTooManyRequests,
							fmt.Sprintf("分组 %s 下模型 %s 的所有可用渠道已达到最大并行度（distributor）", formatRuntimeSelectionGroupLabel(groupID), lookupModel),
							string(types.ErrorCodeChannelConcurrencyExceeded),
						)
						return
					}
					message := fmt.Sprintf("获取分组 %s 下模型 %s 的可用渠道失败（数据库一致性已被破坏，distributor）: %s", formatRuntimeSelectionGroupLabel(groupID), lookupModel, err.Error())
					abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, string(types.ErrorCodeModelNotFound))
				}
				uaAcceptedAny := false
				selectFromCandidates := func() bool {
					selectedChannel, selectedGroupID, accepted, selectErr := selectionAuthority.SelectChannel(c)
					if accepted {
						uaAcceptedAny = true
					}
					if selectErr != nil {
						switch selectErr.Step {
						case "messages_to_responses_compat":
							abortWithOpenAiMessage(c, http.StatusInternalServerError, "渠道级 messages->responses 模型映射配置错误")
						default:
							modelLookupErr(selectErr.GroupID, selectErr.LookupModel, selectErr.Err)
						}
						return false
					}
					if selectedChannel == nil {
						return true
					}
					channel = selectedChannel
					if selectedGroupID > 0 && selectedGroupID != usingGroupID {
						selectionAuthority.SetSelectedGroup(selectedGroupID)
						selectionAuthority.ApplyContext(c)
						usingGroupID = selectionAuthority.CurrentGroupID
					}
					return true
				}
				if !selectFromCandidates() {
					return
				}

				candidateGroupIDs = append([]int(nil), selectionAuthority.CandidateGroupIDs...)

				if channel == nil {
					showGroup := selectionAuthority.DisplayGroupSummary()

					if !uaAcceptedAny {
						abortWithOpenAiMessage(
							c,
							http.StatusBadRequest,
							fmt.Sprintf("UA 不被允许访问分组 %s。UA=%q（distributor）", showGroup, selectionAuthority.UserAgent),
							string(types.ErrorCodeAccessDenied),
						)
						return
					}
					abortWithOpenAiMessage(
						c,
						http.StatusBadRequest,
						fmt.Sprintf("分组 %s 下模型 %s 无可用渠道（distributor）", showGroup, modelRequest.Model),
						string(types.ErrorCodeModelNotFound),
					)
					return
				}
			}
		}

		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		defer ReleaseCurrentChannelConcurrencySlot(c)
		if IsDeferredResponsesWSDistribute(c) {
			if channel != nil {
				if newAPIError := SetupContextForSelectedChannel(c, channel, ""); newAPIError != nil {
					abortWithOpenAiMessage(c, newAPIError.StatusCode, newAPIError.Error(), string(newAPIError.GetErrorCode()))
					return
				}
			}
			c.Next()
			return
		}
		if deferFinalChannelSelection {
			c.Next()
			return
		}
		if channel == nil && len(candidateGroupIDs) == 0 {
			c.Next()
			return
		}
		if newAPIError := SetupContextForSelectedChannel(c, channel, modelRequest.Model); newAPIError != nil {
			abortWithOpenAiMessage(c, newAPIError.StatusCode, newAPIError.Error(), string(newAPIError.GetErrorCode()))
			return
		}
		c.Next()
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

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, err
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, errors.New(mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("无效的请求, 无法解析模型")
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/video/generations") {
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			err = common.UnmarshalBodyReusable(c, &modelRequest)
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
		// Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if strings.EqualFold(c.Request.Method, http.MethodGet) &&
		strings.HasPrefix(c.Request.URL.Path, "/v1/responses") &&
		strings.EqualFold(strings.TrimSpace(c.GetHeader("Upgrade")), "websocket") {
		shouldSelectChannel = false
		MarkDeferredResponsesWSDistribute(c)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		err = common.UnmarshalBodyReusable(c, &modelRequest)
	}
	if err != nil {
		return nil, false, errors.New("无效的请求, " + err.Error())
	}
	modelRequest.Model = normalizeRequestedModelName(modelRequest.Model)
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = normalizeRequestedModelName(c.Query("model"))
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = normalizeRequestedModelName(c.Param("model"))
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			modelRequest.Model = normalizeRequestedModelName(c.PostForm("model"))
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, normalizeRequestedModelName(c.PostForm("model")))
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, normalizeRequestedModelName(c.PostForm("model")))
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	modelRequest.Model = normalizeRequestedModelName(modelRequest.Model)
	if shouldSelectChannel && modelRequest.Model == "" {
		return nil, false, errors.New("未指定模型名称，模型名称不能为空")
	}
	return &modelRequest, shouldSelectChannel, nil
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NewAPIError {
	seedOriginalModelContext(c, modelName)
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	channelSetting := channel.GetSetting()
	key, index, newAPIError := channel.GetNextEnabledKey()
	if newAPIError != nil {
		return newAPIError
	}
	if newAPIError := ensureChannelConcurrencySlot(c, channel); newAPIError != nil {
		return newAPIError
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channelSetting)
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, channel.GetParamOverride())
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, channel.GetHeaderOverride())
	if nil != channel.OpenAIOrganization && *channel.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())
	common.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, channelSetting.MessagesToResponsesCompat)
	if channel.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		// 必须设置为 false，否则在重试到单个 key 的时候会导致日志显示错误
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	// c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	// TODO: api_version统一
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeVertexAi:
		c.Set("region", channel.Other)
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeGemini:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeAli:
		c.Set("plugin", channel.Other)
	case constant.ChannelCloudflare:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channel.Other)
	}
	return nil
}

func seedOriginalModelContext(c *gin.Context, modelName string) {
	if c == nil {
		return
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return
	}
	if strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyOriginalModel)) != "" {
		return
	}
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
}

// extractModelNameFromGeminiPath 从 Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
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

package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/model"
	relayconstant "one-api/relay/constant"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func SelectChannelForModelRequest(c *gin.Context, modelRequest *ModelRequest) *types.NewAPIError {
	if c == nil {
		return types.NewErrorWithStatusCode(errors.New("context is nil"), types.ErrorCodeInvalidRequest, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	if modelRequest == nil {
		return types.NewErrorWithStatusCode(errors.New("model request is nil"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	var channel *model.Channel
	if channelID, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
		id, err := strconv.Atoi(channelID.(string))
		if err == nil {
			channel, err = model.GetChannelById(id, true)
		}
		if err != nil {
			return types.NewErrorWithStatusCode(errors.New("无效的渠道 Id"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if channel.Status != common.ChannelStatusEnabled {
			return types.NewErrorWithStatusCode(errors.New("该渠道已被禁用"), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}

	if channel == nil {
		modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
		if modelLimitEnable {
			s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			if !ok {
				return types.NewErrorWithStatusCode(errors.New("该令牌无权访问任何模型"), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			tokenModelLimit, ok := s.(map[string]bool)
			if !ok {
				tokenModelLimit = map[string]bool{}
			}
			matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model)
			if _, ok := tokenModelLimit[matchName]; !ok {
				return types.NewErrorWithStatusCode(fmt.Errorf("该令牌无权访问模型 %s", modelRequest.Model), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		}

		if strings.TrimSpace(modelRequest.Model) == "" {
			return types.NewErrorWithStatusCode(errors.New("未指定模型名称，模型名称不能为空"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}

		usingGroupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
		if usingGroupID <= 0 {
			return types.NewErrorWithStatusCode(errors.New("无效的分组"), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}

		selectionAuthority, selectionErr := ResolveRuntimeSelectionAuthority(
			c,
			c.Request.URL.Path,
			modelRequest.Model,
			modelRequest.GroupId,
		)
		if selectionErr != nil {
			return selectionErr
		}
		usingGroupID = selectionAuthority.CurrentGroupID
		candidateGroupIDs := append([]int(nil), selectionAuthority.CandidateGroupIDs...)
		if len(candidateGroupIDs) > 0 {
			modelLookupErr := func(groupID int, lookupModel string, err error) *types.NewAPIError {
				if strings.TrimSpace(lookupModel) == "" {
					lookupModel = modelRequest.Model
				}
				if model.IsChannelConcurrencyLimitReachedErr(err) {
					return types.NewErrorWithStatusCode(
						fmt.Errorf("分组 %s 下模型 %s 的所有可用渠道已达到最大并行度（responses-ws）", formatRuntimeSelectionGroupLabel(groupID), lookupModel),
						types.ErrorCodeChannelConcurrencyExceeded,
						http.StatusTooManyRequests,
						types.ErrOptionWithSkipRetry(),
					)
				}
				message := fmt.Sprintf("获取分组 %s 下模型 %s 的可用渠道失败（数据库一致性已被破坏，responses-ws）: %s", formatRuntimeSelectionGroupLabel(groupID), lookupModel, err.Error())
				return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeModelNotFound, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
			}
			uaAcceptedAny := false
			selectFromCandidates := func() *types.NewAPIError {
				selectedChannel, selectedGroupID, accepted, selectErr := selectionAuthority.SelectChannel(c)
				if accepted {
					uaAcceptedAny = true
				}
				if selectErr != nil {
					switch selectErr.Step {
					case "messages_to_responses_compat":
						return types.NewErrorWithStatusCode(errors.New("渠道级 messages->responses 模型映射配置错误"), types.ErrorCodeGetChannelFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
					default:
						return modelLookupErr(selectErr.GroupID, selectErr.LookupModel, selectErr.Err)
					}
				}
				if selectedChannel == nil {
					return nil
				}
				channel = selectedChannel
				if selectedGroupID > 0 && selectedGroupID != usingGroupID {
					selectionAuthority.SetSelectedGroup(selectedGroupID)
					selectionAuthority.ApplyContext(c)
					usingGroupID = selectionAuthority.CurrentGroupID
				}
				return nil
			}
			if err := selectFromCandidates(); err != nil {
				return err
			}

			candidateGroupIDs = append([]int(nil), selectionAuthority.CandidateGroupIDs...)

			if channel == nil {
				showGroup := selectionAuthority.DisplayGroupSummary()
				if !uaAcceptedAny {
					return types.NewErrorWithStatusCode(
						fmt.Errorf("UA 不被允许访问分组 %s。UA=%q（responses-ws）", showGroup, selectionAuthority.UserAgent),
						types.ErrorCodeAccessDenied,
						http.StatusBadRequest,
						types.ErrOptionWithSkipRetry(),
					)
				}
				return types.NewErrorWithStatusCode(
					fmt.Errorf("分组 %s 下模型 %s 无可用渠道（responses-ws）", showGroup, modelRequest.Model),
					types.ErrorCodeModelNotFound,
					http.StatusBadRequest,
					types.ErrOptionWithSkipRetry(),
				)
			}
		}
	}

	if common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime).IsZero() {
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	}
	if channel == nil && common.GetContextKeyInt(c, constant.ContextKeyChannelId) <= 0 {
		return nil
	}
	return SetupContextForSelectedChannel(c, channel, modelRequest.Model)
}

func IsDeferredResponsesWSDistribute(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return common.GetContextKeyBool(c, constant.ContextKeyDeferredResponsesWSDistribute)
}

func MarkDeferredResponsesWSDistribute(c *gin.Context) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyDeferredResponsesWSDistribute, true)
	c.Set("relay_mode", relayconstant.RelayModeResponses)
}

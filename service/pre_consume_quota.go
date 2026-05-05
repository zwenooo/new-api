package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
	"one-api/setting/ratio_setting"
)

func ReturnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo) error {
	if relayInfo == nil {
		return nil
	}
	relayInfoCopy := *relayInfo
	err := returnPreConsumedQuotaContext(c, &relayInfoCopy)
	syncReturnPreConsumedSnapshot(relayInfo, &relayInfoCopy)
	if err != nil {
		common.SysLog("error return pre-consumed quota: " + err.Error())
	}
	return err
}

func returnPreConsumedQuotaContext(ctx context.Context, relayInfo *relaycommon.RelayInfo) error {
	if relayInfo == nil {
		return nil
	}
	var refundErr error
	if relayInfo.FinalPreConsumedRequests != 0 && relayInfo.RequestSubscriptionId != 0 {
		logger.LogRequestInfo(ctx, fmt.Sprintf("用户 %d 请求失败, 返还预扣次数 %d", relayInfo.UserId, relayInfo.FinalPreConsumedRequests))
		if err := model.ReturnUserRequestSubscription(relayInfo.UserId, relayInfo.RequestSubscriptionId, relayInfo.FinalPreConsumedRequests); err != nil {
			refundErr = errors.Join(refundErr, fmt.Errorf("返还预扣请求次数失败: %w", err))
		} else {
			relayInfo.FinalPreConsumedRequests = 0
			relayInfo.RequestSubscriptionId = 0
		}
	}
	if relayInfo.FinalPreConsumedPayRequests != 0 {
		logger.LogRequestInfo(ctx, fmt.Sprintf("用户 %d 请求失败, 返还预扣按次付费次数 %d", relayInfo.UserId, relayInfo.FinalPreConsumedPayRequests))
		if err := model.ReturnUserPayRequestQuotaWithProduct(relayInfo.UserId, relayInfo.PayRequestProductId, relayInfo.FinalPreConsumedPayRequests); err != nil {
			refundErr = errors.Join(refundErr, fmt.Errorf("返还预扣按次付费次数失败: %w", err))
		} else {
			relayInfo.FinalPreConsumedPayRequests = 0
			relayInfo.PayRequestProductId = 0
		}
	}
	if relayInfo.FinalPreConsumedTokens != 0 {
		logger.LogRequestInfo(ctx, fmt.Sprintf("用户 %d 请求失败, 返还预扣 tokens %d", relayInfo.UserId, relayInfo.FinalPreConsumedTokens))
		bucket := relayInfo.QuotaBucket
		productID := 0
		if bucket == model.UserQuotaBucketPayToken {
			productID = relayInfo.PayTokenProductId
		}
		if err := model.ReturnUserQuotaByBucketWithAllocations(
			relayInfo.UserId,
			relayInfo.FinalPreConsumedTokens,
			bucket,
			relayInfo.UsingGroupId,
			productID,
			relayInfo.SubscriptionAllocations,
		); err != nil {
			refundErr = errors.Join(refundErr, fmt.Errorf("返还预扣 tokens 失败: %w", err))
		} else {
			relayInfo.FinalPreConsumedTokens = 0
			relayInfo.SubscriptionAllocations = nil
			if bucket == model.UserQuotaBucketPayToken {
				relayInfo.PayTokenProductId = 0
			}
		}
	}
	if relayInfo.FinalPreConsumedQuota != 0 {
		logger.LogRequestInfo(ctx, fmt.Sprintf("用户 %d 请求失败, 返还预扣费额度 %s", relayInfo.UserId, logger.FormatQuota(relayInfo.FinalPreConsumedQuota)))
		// Token-based buckets only pre-consume token quota (API token), so refund token quota directly
		// to avoid mixing currency quota into user's token balance.
		if relayInfo.QuotaBucket == model.UserQuotaBucketTokens ||
			relayInfo.QuotaBucket == model.UserQuotaBucketPayToken ||
			relayInfo.QuotaBucket == model.UserQuotaBucketRequest ||
			relayInfo.QuotaBucket == model.UserQuotaBucketPayRequest {
			if relayInfo.IsPlayground {
				return refundErr
			}
			if relayInfo.TokenId == 0 || relayInfo.TokenKey == "" {
				return errors.Join(refundErr, errors.New("缺少 token 信息，无法返还预扣 token 额度"))
			}
			if err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, relayInfo.FinalPreConsumedQuota); err != nil {
				refundErr = errors.Join(refundErr, fmt.Errorf("返还预扣 token 额度失败: %w", err))
			} else {
				relayInfo.FinalPreConsumedQuota = 0
			}
			return refundErr
		}
		if err := PostConsumeQuota(relayInfo, -relayInfo.FinalPreConsumedQuota, -relayInfo.FinalPreConsumedQuota, 0, false); err != nil {
			refundErr = errors.Join(refundErr, fmt.Errorf("返还预扣费额度失败: %w", err))
		} else {
			relayInfo.FinalPreConsumedQuota = 0
			switch relayInfo.QuotaBucket {
			case model.UserQuotaBucketPayg:
				relayInfo.PaygProductId = 0
				relayInfo.PaygProductAllocations = nil
			case model.UserQuotaBucketSubscription:
				relayInfo.SubscriptionAllocations = nil
			}
		}
	}
	return refundErr
}

func syncReturnPreConsumedSnapshot(target *relaycommon.RelayInfo, source *relaycommon.RelayInfo) {
	if target == nil || source == nil {
		return
	}
	target.SubscriptionAllocations = cloneSubscriptionAllocations(source.SubscriptionAllocations)
	target.PaygProductId = source.PaygProductId
	target.PaygProductAllocations = cloneProductQuotaAllocations(source.PaygProductAllocations)
	target.PayTokenProductId = source.PayTokenProductId
	target.RequestSubscriptionId = source.RequestSubscriptionId
	target.PayRequestProductId = source.PayRequestProductId
	target.FinalPreConsumedQuota = source.FinalPreConsumedQuota
	target.FinalPreConsumedTokens = source.FinalPreConsumedTokens
	target.FinalPreConsumedRequests = source.FinalPreConsumedRequests
	target.FinalPreConsumedPayRequests = source.FinalPreConsumedPayRequests
}

// MarkWssRequestRoundCommitted clears the refundable request-count reservation state
// for one completed websocket response round after the terminal event has been
// delivered to the downstream client.
func MarkWssRequestRoundCommitted(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil {
		return
	}
	relayInfo.FinalPreConsumedRequests = 0
	relayInfo.RequestSubscriptionId = 0
	relayInfo.FinalPreConsumedPayRequests = 0
	relayInfo.PayRequestProductId = 0
}

// PreConsumeWssRequestRound ensures the current websocket response round has one
// pending request-count reservation (after group-ratio scaling) when the active
// billing bucket is request based.
// It intentionally does not touch FinalPreConsumedQuota/FinalPreConsumedTokens
// because websocket session-level token/quota settlement is still handled elsewhere.
func PreConsumeWssRequestRound(relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo == nil {
		return nil
	}
	requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
	switch relayInfo.QuotaBucket {
	case model.UserQuotaBucketRequest:
		if relayInfo.FinalPreConsumedRequests != 0 {
			return nil
		}
		if requestUnits <= 0 {
			return nil
		}
		subId, err := model.PreConsumeUserRequestSubscription(relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.FinalPreConsumedRequests = requestUnits
		relayInfo.RequestSubscriptionId = subId
	case model.UserQuotaBucketPayRequest:
		if relayInfo.FinalPreConsumedPayRequests != 0 {
			return nil
		}
		if requestUnits <= 0 {
			return nil
		}
		productId, err := model.PreConsumeUserPayRequestQuotaWithProduct(relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.FinalPreConsumedPayRequests = requestUnits
		relayInfo.PayRequestProductId = productId
	}
	return nil
}

// ReturnWssRequestRoundReservation refunds the current websocket response round's
// pending request-count reservation after the failed terminal event has been
// delivered to the downstream client, then clears the pending reservation state.
func ReturnWssRequestRoundReservation(c *gin.Context, relayInfo *relaycommon.RelayInfo) error {
	if relayInfo == nil {
		return nil
	}
	if relayInfo.FinalPreConsumedRequests != 0 && relayInfo.RequestSubscriptionId != 0 {
		logger.LogRequestInfo(c, fmt.Sprintf("用户 %d websocket 单轮失败, 返还预扣次数 %d", relayInfo.UserId, relayInfo.FinalPreConsumedRequests))
		if err := model.ReturnUserRequestSubscription(relayInfo.UserId, relayInfo.RequestSubscriptionId, relayInfo.FinalPreConsumedRequests); err != nil {
			return err
		}
	}
	if relayInfo.FinalPreConsumedPayRequests != 0 && relayInfo.PayRequestProductId != 0 {
		logger.LogRequestInfo(c, fmt.Sprintf("用户 %d websocket 单轮失败, 返还预扣按次付费次数 %d", relayInfo.UserId, relayInfo.FinalPreConsumedPayRequests))
		if err := model.ReturnUserPayRequestQuotaWithProduct(relayInfo.UserId, relayInfo.PayRequestProductId, relayInfo.FinalPreConsumedPayRequests); err != nil {
			return err
		}
	}
	MarkWssRequestRoundCommitted(relayInfo)
	return nil
}

// PreConsumeQuota checks if the user has enough quota to pre-consume.
// It returns the pre-consumed quota if successful, or an error if not.
func PreConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		relayInfo.FinalPreConsumedQuota = 0
		relayInfo.FinalPreConsumedTokens = 0
		return nil
	}
	convertTokenPreConsumeError := func(err error) *types.NewAPIError {
		if errors.Is(err, model.ErrTokenDailyQuotaExceeded) {
			if token, tokErr := model.GetTokenById(relayInfo.TokenId); tokErr == nil && token != nil {
				today := common.GetTodayDateInt()
				usedToday := token.DailyQuotaUsed
				if token.DailyQuotaResetDate != today {
					usedToday = 0
				}
				remaining := token.DailyQuotaLimit - usedToday
				err = wrapQuotaDetailError(err, buildTokenDailyQuotaExceededMessage(relayInfo, preConsumedQuota, token.DailyQuotaLimit, usedToday, remaining))
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodeTokenDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketRequest {
		requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
		subId := 0
		var err error
		if requestUnits > 0 {
			subId, err = model.PreConsumeUserRequestSubscription(relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
		}
		if preConsumedQuota > 0 {
			if err := PreConsumeTokenQuota(relayInfo, preConsumedQuota); err != nil {
				if requestUnits > 0 {
					if rollErr := model.ReturnUserRequestSubscription(relayInfo.UserId, subId, requestUnits); rollErr != nil {
						relayInfo.FinalPreConsumedQuota = preConsumedQuota
						relayInfo.FinalPreConsumedRequests = requestUnits
						relayInfo.RequestSubscriptionId = subId
						err = combineBillingRollbackError(err, rollErr, "回滚预扣请求次数")
					}
				}
				return convertTokenPreConsumeError(err)
			}
		}
		relayInfo.FinalPreConsumedQuota = preConsumedQuota
		relayInfo.FinalPreConsumedRequests = requestUnits
		relayInfo.RequestSubscriptionId = subId
		return nil
	}
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketPayRequest {
		requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
		productId := 0
		var err error
		if requestUnits > 0 {
			productId, err = model.PreConsumeUserPayRequestQuotaWithProduct(relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
		}
		if preConsumedQuota > 0 {
			if err := PreConsumeTokenQuota(relayInfo, preConsumedQuota); err != nil {
				if requestUnits > 0 {
					if rollErr := model.ReturnUserPayRequestQuotaWithProduct(relayInfo.UserId, productId, requestUnits); rollErr != nil {
						relayInfo.FinalPreConsumedQuota = preConsumedQuota
						relayInfo.FinalPreConsumedPayRequests = requestUnits
						relayInfo.PayRequestProductId = productId
						err = combineBillingRollbackError(err, rollErr, "回滚预扣按次付费次数")
					}
				}
				return convertTokenPreConsumeError(err)
			}
		}
		relayInfo.FinalPreConsumedQuota = preConsumedQuota
		relayInfo.FinalPreConsumedPayRequests = requestUnits
		relayInfo.PayRequestProductId = productId
		return nil
	}

	if relayInfo != nil && (relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken) {
		bucket := relayInfo.QuotaBucket
		switch relayInfo.RelayMode {
		case relayconstant.RelayModeChatCompletions,
			relayconstant.RelayModeCompletions,
			relayconstant.RelayModeEmbeddings,
			relayconstant.RelayModeModerations,
			relayconstant.RelayModeEdits,
			relayconstant.RelayModeRerank,
			relayconstant.RelayModeResponses,
			relayconstant.RelayModeRealtime,
			relayconstant.RelayModeGemini,
			relayconstant.RelayModeAudioTranslation,
			relayconstant.RelayModeAudioTranscription,
			relayconstant.RelayModeSunoSubmit,
			relayconstant.RelayModeVideoSubmit:
			// supported
		default:
			return types.NewErrorWithStatusCode(
				errors.New("tokens 计费暂不支持该接口"),
				types.ErrorCodeAccessDenied,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if relayInfo.UsingGroupId <= 0 {
			msg := "tokens 订阅缺少分组信息"
			if bucket == model.UserQuotaBucketPayToken {
				msg = "按token付费缺少分组信息"
			}
			return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}

		preConsumedTokens := ComputeTokenBucketPreConsumedUsage(relayInfo)
		if relayInfo.RelayMode == relayconstant.RelayModeSunoSubmit || relayInfo.RelayMode == relayconstant.RelayModeVideoSubmit {
			preConsumedTokens = relayInfo.FinalPreConsumedTokens
		}

		relayInfo.FinalPreConsumedQuota = preConsumedQuota
		relayInfo.FinalPreConsumedTokens = preConsumedTokens

		if preConsumedTokens > 0 {
			productID := 0
			if bucket == model.UserQuotaBucketPayToken {
				productID = relayInfo.PayTokenProductId
			}
			selectedProductID, subscriptionAllocations, err := model.DecreaseUserQuotaByBucketWithAllocations(relayInfo.UserId, preConsumedTokens, bucket, relayInfo.UsingGroupId, productID)
			if bucket == model.UserQuotaBucketPayToken && selectedProductID != 0 {
				relayInfo.PayTokenProductId = selectedProductID
			}
			setRelaySubscriptionAllocations(relayInfo, subscriptionAllocations)
			if errors.Is(err, model.ErrUserDailyQuotaExceeded) {
				if bucket == model.UserQuotaBucketTokens {
					totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, capErr := model.GetUserTokenSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
					if capErr != nil {
						err = wrapQuotaDetailError(err, buildUserDailyQuotaExceededMessage(relayInfo, preConsumedTokens, -1, -1))
					} else {
						if totalUnlimited {
							totalRemaining = -2
						}
						if dailyUnlimited {
							dailyCapacity = -2
						}
						err = wrapQuotaDetailError(err, buildUserDailyQuotaExceededMessage(relayInfo, preConsumedTokens, totalRemaining, dailyCapacity))
					}
				}
				relayInfo.FinalPreConsumedQuota = 0
				relayInfo.FinalPreConsumedTokens = 0
				setRelaySubscriptionAllocations(relayInfo, nil)
				return types.NewErrorWithStatusCode(err, types.ErrorCodeUserDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			if err != nil {
				relayInfo.FinalPreConsumedQuota = 0
				relayInfo.FinalPreConsumedTokens = 0
				setRelaySubscriptionAllocations(relayInfo, nil)
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
		}

		if preConsumedQuota > 0 {
			err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
			if err != nil {
				// rollback user tokens to keep token/user consistent
				rollbackProductID := 0
				if bucket == model.UserQuotaBucketPayToken {
					rollbackProductID = relayInfo.PayTokenProductId
				}
				rollbackErr := model.ReturnUserQuotaByBucketWithAllocations(
					relayInfo.UserId,
					preConsumedTokens,
					bucket,
					relayInfo.UsingGroupId,
					rollbackProductID,
					relayInfo.SubscriptionAllocations,
				)
				if rollbackErr == nil {
					relayInfo.FinalPreConsumedQuota = 0
					relayInfo.FinalPreConsumedTokens = 0
					setRelaySubscriptionAllocations(relayInfo, nil)
				} else {
					err = combineBillingRollbackError(err, rollbackErr, "回滚预扣用户tokens")
				}
				if errors.Is(err, model.ErrTokenDailyQuotaExceeded) {
					if token, tokErr := model.GetTokenById(relayInfo.TokenId); tokErr == nil && token != nil {
						today := common.GetTodayDateInt()
						usedToday := token.DailyQuotaUsed
						if token.DailyQuotaResetDate != today {
							usedToday = 0
						}
						remaining := token.DailyQuotaLimit - usedToday
						err = wrapQuotaDetailError(err, buildTokenDailyQuotaExceededMessage(relayInfo, preConsumedQuota, token.DailyQuotaLimit, usedToday, remaining))
					}
					return types.NewErrorWithStatusCode(err, types.ErrorCodeTokenDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
				}
				return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
		}

		logger.LogRequestInfo(c, fmt.Sprintf("用户 %d 预扣倍率后 tokens %d", relayInfo.UserId, preConsumedTokens))
		return nil
	}

	// For bucketed billing (subscription/payg/free), the actual pre-consume operations below already enforce
	// both subscription daily limits and token daily limits (via DecreaseUserQuotaByBucket/DecreaseTokenQuota).
	// Avoid doing an extra DB-heavy pre-check here to reduce load under high QPS.
	if relayInfo != nil && relayInfo.QuotaBucket != "" {
		relayInfo.UserQuota = common.GetContextKeyInt(c, constant.ContextKeyUserQuota)

		// Keep the original behavior: if pricing yields 0, do not pre-consume.
		if preConsumedQuota <= 0 {
			relayInfo.FinalPreConsumedQuota = 0
			return nil
		}

		var decErr error
		if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
			decErr = consumeRelayPaygQuota(relayInfo, preConsumedQuota)
		} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription {
			decErr = consumeRelaySubscriptionQuota(relayInfo, preConsumedQuota)
		} else {
			selectedPaygProductId, innerErr := model.DecreaseUserQuotaByBucket(
				relayInfo.UserId,
				preConsumedQuota,
				relayInfo.QuotaBucket,
				relayInfo.UsingGroupId,
				relayInfo.PaygProductId,
			)
			if selectedPaygProductId != 0 {
				relayInfo.PaygProductId = selectedPaygProductId
			}
			decErr = innerErr
		}
		if decErr != nil {
			relayInfo.FinalPreConsumedQuota = 0
			setRelaySubscriptionAllocations(relayInfo, nil)
			if errors.Is(decErr, model.ErrUserDailyQuotaExceeded) {
				totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, capErr := model.GetUserSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
				if capErr != nil {
					decErr = wrapQuotaDetailError(decErr, buildUserDailyQuotaExceededMessage(relayInfo, preConsumedQuota, -1, -1))
				} else {
					if totalUnlimited {
						totalRemaining = -2
					}
					if dailyUnlimited {
						dailyCapacity = -2
					}
					decErr = wrapQuotaDetailError(decErr, buildUserDailyQuotaExceededMessage(relayInfo, preConsumedQuota, totalRemaining, dailyCapacity))
				}
				return types.NewErrorWithStatusCode(decErr, types.ErrorCodeUserDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			decErr = wrapFreeBucketInsufficientError(relayInfo, preConsumedQuota, decErr)
			return types.NewErrorWithStatusCode(decErr, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.FinalPreConsumedQuota = preConsumedQuota

		decTokenErr := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if decTokenErr != nil {
			// rollback user quota to keep token/user consistent
			var rollbackErr error
			if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
				rollbackErr = returnRelayPaygQuota(relayInfo, preConsumedQuota)
			} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription {
				rollbackErr = returnRelaySubscriptionQuota(relayInfo, preConsumedQuota)
			} else {
				rollbackErr = model.ReturnUserQuotaByBucket(
					relayInfo.UserId,
					preConsumedQuota,
					relayInfo.QuotaBucket,
					relayInfo.UsingGroupId,
					relayInfo.PaygProductId,
				)
			}
			if rollbackErr == nil {
				relayInfo.FinalPreConsumedQuota = 0
				setRelaySubscriptionAllocations(relayInfo, nil)
			} else {
				decTokenErr = combineBillingRollbackError(decTokenErr, rollbackErr, "回滚预扣用户额度")
			}
			if errors.Is(decTokenErr, model.ErrTokenDailyQuotaExceeded) {
				today := common.GetTodayDateInt()
				usedToday := common.GetContextKeyInt(c, constant.ContextKeyTokenDailyQuotaUsed)
				resetDate := common.GetContextKeyInt(c, constant.ContextKeyTokenDailyQuotaResetDate)
				limit := common.GetContextKeyInt(c, constant.ContextKeyTokenDailyQuotaLimit)
				if resetDate != today {
					usedToday = 0
				}
				remaining := limit - usedToday
				decTokenErr = wrapQuotaDetailError(decTokenErr, buildTokenDailyQuotaExceededMessage(relayInfo, preConsumedQuota, limit, usedToday, remaining))
				return types.NewErrorWithStatusCode(decTokenErr, types.ErrorCodeTokenDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			return types.NewErrorWithStatusCode(decTokenErr, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}

		logger.LogRequestInfo(c, fmt.Sprintf("用户 %d 预扣费 %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota)))
		relayInfo.FinalPreConsumedQuota = preConsumedQuota
		return nil
	}

	// Legacy (bucket-less) behavior: keep compatibility for older code paths.
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}

	trustQuota := common.GetTrustQuota()

	relayInfo.UserQuota = userQuota
	var trustedLog string
	if relayInfo.QuotaBucket == "" && userQuota > trustQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > trustQuota {
				// 令牌额度充足，信任令牌（延迟日志，待通过日限校验后再打印）
				preConsumedQuota = 0
				trustedLog = fmt.Sprintf("用户 %d 剩余额度 %s 且令牌 %d 额度 %d 充足, 信任且不需要预扣费", relayInfo.UserId, logger.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota)
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			trustedLog = fmt.Sprintf("用户 %d 额度充足且为无限额度令牌, 信任且不需要预扣费", relayInfo.UserId)
		}
	}

	// 自由额度不设用户级每日用量限制：不再基于 user.DailyQuotaLimit 影响预扣费信任策略

	// 在进行任何实际扣费（包含信任模式为0预扣费）之前，一律做一次“当日订阅日限 + 令牌日限”的可用性校验，
	// 以避免信任模式绕过订阅日限导致请求完成后才失败、但HTTP返回已是200的情况。
	// 为了让校验更精确，提前把当前计划预扣费额度写入 relayInfo.FinalPreConsumedQuota，供估算使用。
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	if err := ensureDailyQuotaAvailability(relayInfo); err != nil {
		relayInfo.FinalPreConsumedQuota = 0
		return types.NewErrorWithStatusCode(err, mapDailyQuotaErrorCode(err), http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	if trustedLog != "" {
		logger.LogRequestInfo(c, trustedLog)
	}

	if preConsumedQuota > 0 {
		if relayInfo.QuotaBucket != "" {
			if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
				err = consumeRelayPaygQuota(relayInfo, preConsumedQuota)
			} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription {
				err = consumeRelaySubscriptionQuota(relayInfo, preConsumedQuota)
			} else {
				selectedPaygProductId, decErr := model.DecreaseUserQuotaByBucket(
					relayInfo.UserId,
					preConsumedQuota,
					relayInfo.QuotaBucket,
					relayInfo.UsingGroupId,
					relayInfo.PaygProductId,
				)
				if selectedPaygProductId != 0 {
					relayInfo.PaygProductId = selectedPaygProductId
				}
				err = decErr
			}
		} else {
			err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota)
		}
		if err != nil {
			relayInfo.FinalPreConsumedQuota = 0
			setRelaySubscriptionAllocations(relayInfo, nil)
			if errors.Is(err, model.ErrUserDailyQuotaExceeded) {
				totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, capErr := model.GetUserSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
				if capErr != nil {
					err = wrapQuotaDetailError(err, buildUserDailyQuotaExceededMessage(relayInfo, preConsumedQuota, -1, -1))
				} else {
					if totalUnlimited {
						totalRemaining = -2
					}
					if dailyUnlimited {
						dailyCapacity = -2
					}
					err = wrapQuotaDetailError(err, buildUserDailyQuotaExceededMessage(relayInfo, preConsumedQuota, totalRemaining, dailyCapacity))
				}
				return types.NewErrorWithStatusCode(err, types.ErrorCodeUserDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}

		err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			// rollback user quota to keep token/user consistent
			var rollbackErr error
			if relayInfo.QuotaBucket != "" {
				if relayInfo.QuotaBucket == model.UserQuotaBucketPayg {
					rollbackErr = returnRelayPaygQuota(relayInfo, preConsumedQuota)
				} else if relayInfo.QuotaBucket == model.UserQuotaBucketSubscription {
					rollbackErr = returnRelaySubscriptionQuota(relayInfo, preConsumedQuota)
				} else {
					rollbackErr = model.ReturnUserQuotaByBucket(
						relayInfo.UserId,
						preConsumedQuota,
						relayInfo.QuotaBucket,
						relayInfo.UsingGroupId,
						relayInfo.PaygProductId,
					)
				}
			} else {
				rollbackErr = model.ReturnUserQuota(relayInfo.UserId, preConsumedQuota)
			}
			if rollbackErr == nil {
				relayInfo.FinalPreConsumedQuota = 0
				setRelaySubscriptionAllocations(relayInfo, nil)
			} else {
				err = combineBillingRollbackError(err, rollbackErr, "回滚预扣用户额度")
			}
			if errors.Is(err, model.ErrTokenDailyQuotaExceeded) {
				if token, tokErr := model.GetTokenById(relayInfo.TokenId); tokErr == nil && token != nil {
					today := common.GetTodayDateInt()
					usedToday := token.DailyQuotaUsed
					if token.DailyQuotaResetDate != today {
						usedToday = 0
					}
					remaining := token.DailyQuotaLimit - usedToday
					err = wrapQuotaDetailError(err, buildTokenDailyQuotaExceededMessage(relayInfo, preConsumedQuota, token.DailyQuotaLimit, usedToday, remaining))
				}
				return types.NewErrorWithStatusCode(err, types.ErrorCodeTokenDailyQuotaExceeded, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		logger.LogRequestInfo(c, fmt.Sprintf("用户 %d 预扣费 %s, 预扣费后剩余额度: %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), logger.FormatQuota(userQuota-preConsumedQuota)))
	}
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	return nil
}

func ensureDailyQuotaAvailability(relayInfo *relaycommon.RelayInfo) error {
	if relayInfo == nil {
		return errors.New("relayInfo 为空")
	}
	if relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		return nil
	}

	today := common.GetTodayDateInt()

	expectedQuotaCurrency := relayInfo.FinalPreConsumedQuota
	if expectedQuotaCurrency <= 0 {
		expectedQuotaCurrency = relayInfo.PriceData.ShouldPreConsumedQuota
	}
	if expectedQuotaCurrency <= 0 {
		expectedQuotaCurrency = 1
	}

	expectedBucket := expectedQuotaCurrency
	if relayInfo.QuotaBucket == model.UserQuotaBucketTokens || relayInfo.QuotaBucket == model.UserQuotaBucketPayToken {
		expectedBucket = relayInfo.FinalPreConsumedTokens
		if expectedBucket <= 0 {
			expectedBucket = ComputeTokenBucketPreConsumedUsage(relayInfo)
		}
	}

	// Strengthen estimate when trusting (no pre-consume) to avoid exceeding daily limit post-stream.
	// Only when request explicitly carries a max tokens hint, include completion tokens into estimate.
	if relayInfo.PriceData.UsePrice == false {
		// base ratio = model * resolved user-aware group ratio
		ratio := relayInfo.PriceData.ModelRatio * relayInfo.PriceData.GroupRatioInfo.EffectiveGroupRatio
		// completion ratio per model (text completion part)
		completionRatio := ratio_setting.GetCompletionRatio(relayInfo.OriginModelName)

		// try to extract max completion tokens from request
		maxCompletionTokens := extractMaxTokens(relayInfo)

		if maxCompletionTokens > 0 && ratio > 0 {
			// conservative add-on estimate: prompt + completion(max)
			inputLongMultiplier, outputLongMultiplier := relaycommon.LongContextInputOutputMultipliers(relayInfo.OriginModelName, relayInfo.PromptTokens, 0)
			promptPart := float64(relayInfo.PromptTokens) * inputLongMultiplier
			completionPart := float64(maxCompletionTokens) * completionRatio * outputLongMultiplier
			conservative := int((promptPart + completionPart) * ratio)
			if conservative > expectedQuotaCurrency {
				expectedQuotaCurrency = conservative
			}
		}
	}

	// 自由额度不设用户级每日用量限制：不在此处对 user.DailyQuotaLimit 做阻断判断

	// 新分桶逻辑：当 relayInfo.QuotaBucket 指定时，按“当前桶 + 当前分组”校验可用性，避免扣 token 后再失败。
	if relayInfo.QuotaBucket != "" {
		switch relayInfo.QuotaBucket {
		case model.UserQuotaBucketRequest:
			requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
			if requestUnits <= 0 {
				return nil
			}
			// Request-count subscriptions already reserve request credits before reaching this check
			// in the normal relay path. Keep a read-only fallback here for other callers.
			if relayInfo.FinalPreConsumedRequests == 0 || relayInfo.RequestSubscriptionId == 0 {
				if relayInfo.UsingGroupId <= 0 {
					return errors.New("次数订阅缺少分组信息")
				}
				groupIDs, has, err := model.GetUserRequestSubscriptionGroupCandidatesByCount(relayInfo.UserId, requestUnits)
				if err != nil {
					return err
				}
				if !has {
					return errors.New("次数订阅不足")
				}
				matched := false
				for _, gid := range groupIDs {
					if gid == relayInfo.UsingGroupId {
						matched = true
						break
					}
				}
				if !matched {
					return errors.New("次数订阅不足")
				}
			}
		case model.UserQuotaBucketPayRequest:
			requestUnits := ComputeRequestBucketUsage(relayInfo, 1)
			if requestUnits <= 0 {
				return nil
			}
			// Pay-request buckets already reserve the current request's count credit before reaching this check
			// in the normal relay path. Keep a read-only fallback here for other callers.
			if relayInfo.FinalPreConsumedPayRequests == 0 || relayInfo.PayRequestProductId == 0 {
				if relayInfo.UsingGroupId <= 0 {
					return errors.New("按次付费缺少分组信息")
				}
				_, ok, err := model.FindUserPayRequestConsumableProductIdTx(nil, relayInfo.UserId, relayInfo.UsingGroupId, requestUnits)
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("按次付费次数不足")
				}
			}
		case model.UserQuotaBucketSubscription:
			totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, err := model.GetUserSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
			if err != nil {
				return err
			}
			displayTotalRemaining := totalRemaining
			displayDailyCapacity := dailyCapacity
			if totalUnlimited {
				displayTotalRemaining = -2
			}
			if dailyUnlimited {
				displayDailyCapacity = -2
			}
			if !dailyUnlimited && dailyCapacity < expectedQuotaCurrency {
				if totalUnlimited || totalRemaining >= expectedQuotaCurrency {
					return wrapQuotaDetailError(model.ErrUserDailyQuotaExceeded, buildUserDailyQuotaExceededMessage(relayInfo, expectedQuotaCurrency, displayTotalRemaining, displayDailyCapacity))
				}
				return wrapQuotaDetailError(errors.New("订阅额度不足"), buildSubscriptionQuotaInsufficientMessage(relayInfo, expectedQuotaCurrency, displayTotalRemaining, displayDailyCapacity))
			}
		case model.UserQuotaBucketTokens:
			if expectedBucket <= 0 {
				return nil
			}
			totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, err := model.GetUserTokenSubscriptionCapacityForGroup(relayInfo.UserId, relayInfo.UsingGroupId)
			if err != nil {
				return err
			}
			displayTotalRemaining := totalRemaining
			displayDailyCapacity := dailyCapacity
			if totalUnlimited {
				displayTotalRemaining = -2
			}
			if dailyUnlimited {
				displayDailyCapacity = -2
			}
			if !dailyUnlimited && dailyCapacity < expectedBucket {
				if totalUnlimited || totalRemaining >= expectedBucket {
					return wrapQuotaDetailError(model.ErrUserDailyQuotaExceeded, buildUserDailyQuotaExceededMessage(relayInfo, expectedBucket, displayTotalRemaining, displayDailyCapacity))
				}
				return wrapQuotaDetailError(errors.New("tokens 额度不足"), buildSubscriptionQuotaInsufficientMessage(relayInfo, expectedBucket, displayTotalRemaining, displayDailyCapacity))
			}
		case model.UserQuotaBucketPayToken:
			if expectedBucket <= 0 {
				return nil
			}
			_, ok, err := model.FindUserPayTokenConsumableProductIdTx(nil, relayInfo.UserId, relayInfo.UsingGroupId, expectedBucket)
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("按token付费余额不足")
			}
		case model.UserQuotaBucketPayg:
			_, ok, err := model.FindUserPaygConsumableProductIdTx(nil, relayInfo.UserId, relayInfo.UsingGroupId, expectedQuotaCurrency)
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("按量付费额度不足")
			}
		case model.UserQuotaBucketFree:
			// 自由额度（不含订阅/按量付费）只做余额校验，不涉及日限
			var user struct {
				Quota       int
				RedeemQuota int
				PaygQuota   int `gorm:"column:payg_quota"`
			}
			if err := model.DB.Model(&model.User{}).
				Select("quota", "redeem_quota", "payg_quota").
				Where("id = ?", relayInfo.UserId).
				Scan(&user).Error; err != nil {
				return err
			}
			freeRemain := user.Quota - user.RedeemQuota - user.PaygQuota
			if freeRemain < 0 {
				freeRemain = 0
			}
			if freeRemain < expectedQuotaCurrency {
				return wrapQuotaDetailError(errors.New("用户额度不足"), buildFreeQuotaInsufficientMessage(relayInfo, expectedQuotaCurrency, freeRemain))
			}
		default:
			return errors.New("bucket 无效")
		}
	} else {
		// 兼容旧逻辑：订阅额度“日限”只在订阅内生效。
		// 若订阅当日已达日限，但用户仍有其他订阅或按量余额可用，应允许继续消费。
		// 校验策略：估算当次可从“所有订阅（按各自日限与剩余额度约束取 min）+ 按量余额”
		// 可用的总容量，若不足以覆盖 expectedQuota，则判定为命中用户订阅日限；否则放行。
		if qb, err := model.GetUserQuotaBreakdown(relayInfo.UserId); err == nil && qb != nil && qb.HasSubscription {
			paygRemain := qb.PaygRemaining
			dailyUnlimited := qb.SubscriptionDailyLimitUnlimited
			subCapacity := 0
			subRemaining := 0
			for _, s := range qb.Subscriptions {
				if s.TotalQuota == 0 {
					// Unlimited-total subscriptions: daily capacity is governed only by daily limits.
					if s.DailyQuotaLimit <= 0 {
						dailyUnlimited = true
						break
					}
					used := s.DailyQuotaUsed
					if used < 0 {
						used = 0
					}
					todayRemain := s.DailyQuotaLimit - used
					if todayRemain < 0 {
						todayRemain = 0
					}
					subCapacity += todayRemain
					continue
				}
				subRemaining += s.RemainingQuota
				used := s.DailyQuotaUsed
				var todayRemain int
				if s.DailyQuotaLimit > 0 {
					if used < 0 {
						used = 0
					}
					todayRemain = s.DailyQuotaLimit - used
					if todayRemain < 0 {
						todayRemain = 0
					}
				} else {
					todayRemain = s.RemainingQuota
				}
				if todayRemain > s.RemainingQuota {
					todayRemain = s.RemainingQuota
				}
				if todayRemain < 0 {
					todayRemain = 0
				}
				subCapacity += todayRemain
			}
			if !dailyUnlimited && paygRemain+subCapacity < expectedQuotaCurrency {
				return wrapQuotaDetailError(model.ErrUserDailyQuotaExceeded, buildUserDailyQuotaExceededMessage(relayInfo, expectedQuotaCurrency, subRemaining, subCapacity))
			}
		}
	}

	// Playground (/pg) and other no-token flows don't have token daily limits.
	if relayInfo.TokenId > 0 {
		token, err := model.GetTokenById(relayInfo.TokenId)
		if err != nil {
			return err
		}
		if token.DailyQuotaLimit > 0 {
			usedToday := token.DailyQuotaUsed
			if token.DailyQuotaResetDate != today {
				usedToday = 0
			}
			remaining := token.DailyQuotaLimit - usedToday
			if remaining <= 0 {
				return wrapQuotaDetailError(model.ErrTokenDailyQuotaExceeded, buildTokenDailyQuotaExceededMessage(relayInfo, expectedQuotaCurrency, token.DailyQuotaLimit, usedToday, remaining))
			}
			// 令牌每日限额仍保持严格（不引入信任超额）
			if expectedQuotaCurrency > remaining {
				return wrapQuotaDetailError(model.ErrTokenDailyQuotaExceeded, buildTokenDailyQuotaExceededMessage(relayInfo, expectedQuotaCurrency, token.DailyQuotaLimit, usedToday, remaining))
			}
		}
	}

	return nil
}

// EnsureDailyQuotaForExpected 对指定的预期消耗额度进行“当日日限”可用性校验。
// 该方法适用于任务类/图片类等未走通用 PreConsume 流程的场景，
// 会在不改变外部行为的前提下，尽早阻断超过用户订阅日限或令牌日限的请求。
func EnsureDailyQuotaForExpected(relayInfo *relaycommon.RelayInfo, expectedQuota int) error {
	if relayInfo != nil && relayInfo.QuotaBucket == model.UserQuotaBucketFree && model.IsGroupNoBilling(relayInfo.UsingGroupId) {
		return nil
	}
	// 不修改原对象的其余字段，只设置一次本次预计消耗额度
	if expectedQuota > 0 {
		relayInfo.FinalPreConsumedQuota = expectedQuota
	}
	return ensureDailyQuotaAvailability(relayInfo)
}

func wrapFreeBucketInsufficientError(relayInfo *relaycommon.RelayInfo, expectedQuota int, err error) error {
	if relayInfo == nil || relayInfo.QuotaBucket != model.UserQuotaBucketFree || err == nil {
		return err
	}
	if strings.TrimSpace(err.Error()) != "用户额度不足" {
		return err
	}

	freeRemaining, queryErr := getUserFreeQuotaRemaining(relayInfo.UserId)
	if queryErr != nil {
		return wrapQuotaDetailError(err, buildFreeQuotaInsufficientMessage(relayInfo, expectedQuota, -1))
	}
	return wrapQuotaDetailError(err, buildFreeQuotaInsufficientMessage(relayInfo, expectedQuota, freeRemaining))
}

func getUserFreeQuotaRemaining(userId int) (int, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	var user struct {
		Quota       int
		RedeemQuota int
		PaygQuota   int `gorm:"column:payg_quota"`
	}
	if err := model.DB.Model(&model.User{}).
		Select("quota", "redeem_quota", "payg_quota").
		Where("id = ?", userId).
		Scan(&user).Error; err != nil {
		return 0, err
	}
	freeRemaining := user.Quota - user.RedeemQuota - user.PaygQuota
	if freeRemaining < 0 {
		freeRemaining = 0
	}
	return freeRemaining, nil
}

// extractMaxTokens tries to read max output tokens from the original request for a conservative daily-limit estimate.
func extractMaxTokens(relayInfo *relaycommon.RelayInfo) int {
	if relayInfo == nil || relayInfo.Request == nil {
		return 0
	}
	switch r := relayInfo.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		return int(r.MaxOutputTokens)
	case *dto.GeneralOpenAIRequest:
		if r.MaxCompletionTokens > r.MaxTokens {
			return int(r.MaxCompletionTokens)
		}
		return int(r.MaxTokens)
	case *dto.ClaudeRequest:
		if r.MaxTokens != nil {
			return int(*r.MaxTokens)
		}
		return 0
	case *dto.GeminiChatRequest:
		return int(r.GenerationConfig.MaxOutputTokens)
	default:
		return 0
	}
}

func mapDailyQuotaErrorCode(err error) types.ErrorCode {
	if errors.Is(err, model.ErrUserDailyQuotaExceeded) {
		return types.ErrorCodeUserDailyQuotaExceeded
	}
	if errors.Is(err, model.ErrTokenDailyQuotaExceeded) {
		return types.ErrorCodeTokenDailyQuotaExceeded
	}
	return types.ErrorCodeInsufficientUserQuota
}

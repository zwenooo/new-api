package service

import (
	"context"
	"fmt"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	legacyTaskBillingSourceWallet       = "wallet"
	legacyTaskBillingSourceSubscription = "subscription"
	legacyTaskRefundLogType             = model.LogTypeSystem
)

func taskHasBillingSnapshot(privateData model.TaskPrivateData) bool {
	return privateData.QuotaBucket != "" ||
		len(privateData.SubscriptionAllocations) > 0 ||
		len(privateData.PaygProductAllocations) > 0 ||
		privateData.PaygProductID != 0 ||
		privateData.PayTokenProductID != 0 ||
		privateData.RequestSubscriptionID != 0 ||
		privateData.PayRequestProductID != 0 ||
		privateData.FinalPreConsumedQuota != 0 ||
		privateData.FinalPreConsumedTokens != 0 ||
		privateData.FinalPreConsumedRequests != 0 ||
		privateData.FinalPreConsumedPayReqs != 0
}

func taskHasLegacyBilling(privateData model.TaskPrivateData) bool {
	switch strings.ToLower(strings.TrimSpace(privateData.BillingSource)) {
	case legacyTaskBillingSourceWallet:
		return true
	case legacyTaskBillingSourceSubscription:
		return privateData.SubscriptionID > 0
	default:
		return false
	}
}

func resolveTaskTokenKey(ctx context.Context, tokenID int, taskID string) string {
	if tokenID <= 0 {
		return ""
	}
	token, err := model.GetTokenById(tokenID)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取任务 token key 失败 (token_id=%d, task=%s): %s", tokenID, taskID, err.Error()))
		return ""
	}
	if token == nil {
		return ""
	}
	return token.Key
}

func taskUsesLegacySubscription(task *model.Task) bool {
	if task == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(task.PrivateData.BillingSource), legacyTaskBillingSourceSubscription) &&
		task.PrivateData.SubscriptionID > 0
}

func taskAdjustLegacyFunding(task *model.Task, delta int) error {
	if task == nil || delta == 0 {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(task.PrivateData.BillingSource)) {
	case legacyTaskBillingSourceSubscription:
		if task.PrivateData.SubscriptionID <= 0 {
			return fmt.Errorf("legacy task missing subscription_id: task=%s", task.TaskID)
		}
		return model.PostConsumeUserSubscriptionDelta(task.PrivateData.SubscriptionID, int64(delta))
	case legacyTaskBillingSourceWallet:
		if delta > 0 {
			return model.DecreaseUserQuota(task.UserId, delta)
		}
		return model.IncreaseUserQuota(task.UserId, -delta, false)
	default:
		return fmt.Errorf("unsupported legacy billing source %q: task=%s", task.PrivateData.BillingSource, task.TaskID)
	}
}

func taskAdjustLegacyTokenQuota(ctx context.Context, task *model.Task, delta int) {
	if task == nil || task.PrivateData.TokenID <= 0 || delta == 0 {
		return
	}
	tokenKey := resolveTaskTokenKey(ctx, task.PrivateData.TokenID, task.TaskID)
	if tokenKey == "" {
		return
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenID, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenID, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整任务 token 额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
	}
}

func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if task == nil {
		return other
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		other["group_ratio"] = bc.GroupRatio
		if len(bc.OtherRatios) > 0 {
			for k, v := range bc.OtherRatios {
				other[k] = v
			}
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskModelName(task *model.Task) string {
	if task == nil {
		return ""
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

func normalizeTaskRelaySnapshot(ctx context.Context, task *model.Task, info *relaycommon.RelayInfo) {
	if task == nil || info == nil || task.Quota <= 0 {
		return
	}
	if info.FinalPreConsumedQuota > task.Quota {
		logger.LogWarn(ctx, fmt.Sprintf(
			"任务 %s 检测到异常额度快照 stored=%d, quota=%d，按任务额度修正",
			task.TaskID,
			info.FinalPreConsumedQuota,
			task.Quota,
		))
		info.FinalPreConsumedQuota = task.Quota
	}
	if (info.QuotaBucket == model.UserQuotaBucketTokens || info.QuotaBucket == model.UserQuotaBucketPayToken) &&
		info.FinalPreConsumedTokens > task.Quota {
		logger.LogWarn(ctx, fmt.Sprintf(
			"任务 %s 检测到异常 token 快照 stored=%d, quota=%d，按任务额度修正",
			task.TaskID,
			info.FinalPreConsumedTokens,
			task.Quota,
		))
		info.FinalPreConsumedTokens = task.Quota
	}
}

func buildTaskRelayInfo(task *model.Task) (*relaycommon.RelayInfo, bool, error) {
	if task == nil {
		return nil, false, fmt.Errorf("task is nil")
	}
	privateData := task.PrivateData
	info := &relaycommon.RelayInfo{
		UserId:                      task.UserId,
		TokenId:                     privateData.TokenID,
		TokenKey:                    privateData.TokenKey,
		QuotaBucket:                 privateData.QuotaBucket,
		UsingGroupId:                privateData.UsingGroupID,
		SubscriptionAllocations:     cloneSubscriptionAllocations(privateData.SubscriptionAllocations),
		PaygProductId:               privateData.PaygProductID,
		PaygProductAllocations:      cloneProductQuotaAllocations(privateData.PaygProductAllocations),
		PayTokenProductId:           privateData.PayTokenProductID,
		RequestSubscriptionId:       privateData.RequestSubscriptionID,
		PayRequestProductId:         privateData.PayRequestProductID,
		FinalPreConsumedQuota:       privateData.FinalPreConsumedQuota,
		FinalPreConsumedTokens:      privateData.FinalPreConsumedTokens,
		FinalPreConsumedRequests:    privateData.FinalPreConsumedRequests,
		FinalPreConsumedPayRequests: privateData.FinalPreConsumedPayReqs,
	}
	hasSnapshot := taskHasBillingSnapshot(privateData)
	if privateData.TokenID > 0 && info.TokenKey == "" {
		info.TokenKey = resolveTaskTokenKey(context.Background(), privateData.TokenID, task.TaskID)
	}
	if len(info.PaygProductAllocations) == 0 && info.PaygProductId != 0 && info.FinalPreConsumedQuota > 0 {
		info.PaygProductAllocations = []relaycommon.ProductQuotaAllocation{
			{ProductId: info.PaygProductId, Quota: info.FinalPreConsumedQuota},
		}
	}
	return info, hasSnapshot, nil
}

func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) {
	if task == nil {
		return
	}
	if task.PrivateData.RefundAppliedAt > 0 {
		return
	}
	if taskHasRefundLog(ctx, task) {
		return
	}
	info, hasSnapshot, err := buildTaskRelayInfo(task)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("构建任务退款上下文失败 task=%s: %s", task.TaskID, err.Error()))
		return
	}
	if hasSnapshot {
		normalizeTaskRelaySnapshot(ctx, task, info)
		if err := returnPreConsumedQuotaContext(ctx, info); err != nil {
			syncTaskOutstandingBillingSnapshot(task, info)
			persistTaskBillingSnapshot(ctx, task, "持久化剩余退款快照失败")
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 退款失败: %s", task.TaskID, err.Error()))
			return
		}
		revertTaskUsageMetrics(task)
		clearTaskRefundSnapshot(task)
		task.PrivateData.RefundAppliedAt = time.Now().Unix()
		persistTaskBillingSnapshot(ctx, task, "清理任务退款快照失败")
		recordTaskRefundLog(task, reason)
		return
	}
	if task.Quota <= 0 {
		return
	}
	if !taskHasLegacyBilling(task.PrivateData) {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 缺少可退款的计费快照，拒绝使用模糊退款路径: %s", task.TaskID, reason))
		return
	}

	logger.LogWarn(ctx, fmt.Sprintf("任务 %s 缺少新计费快照，使用旧退款路径: %s", task.TaskID, reason))
	if err := taskAdjustLegacyFunding(task, -task.Quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("旧退款路径返还资金失败 task=%s: %s", task.TaskID, err.Error()))
		return
	}
	taskAdjustLegacyTokenQuota(ctx, task, -task.Quota)
	revertTaskUsageMetrics(task)
	task.PrivateData.RefundAppliedAt = time.Now().Unix()
	persistTaskBillingSnapshot(ctx, task, "持久化旧退款标记失败")
	recordTaskRefundLog(task, reason)
}

func revertTaskUsageMetrics(task *model.Task) {
	if task == nil || !taskHasRecordedSubmitUsage(task) {
		return
	}
	visibleQuota := task.PrivateData.FinalVisibleQuota
	costQuota := task.PrivateData.FinalCostQuota
	if task.Quota == 0 && visibleQuota == 0 && costQuota == 0 {
		return
	}
	model.UpdateUserUsageQuotas(task.UserId, -task.Quota, -visibleQuota, -costQuota)
	model.UpdateChannelUsageQuotas(task.ChannelId, -task.Quota, -visibleQuota, -costQuota)
}

func taskHasRecordedSubmitUsage(task *model.Task) bool {
	if task == nil {
		return false
	}
	return strings.TrimSpace(task.PrivateData.UpstreamTaskID) != "" || len(task.Data) > 0
}

func syncTaskOutstandingBillingSnapshot(task *model.Task, info *relaycommon.RelayInfo) {
	if task == nil || info == nil {
		return
	}
	task.PrivateData.SubscriptionAllocations = cloneSubscriptionAllocations(info.SubscriptionAllocations)
	task.PrivateData.PaygProductID = info.PaygProductId
	task.PrivateData.PaygProductAllocations = cloneProductQuotaAllocations(info.PaygProductAllocations)
	task.PrivateData.PayTokenProductID = info.PayTokenProductId
	task.PrivateData.RequestSubscriptionID = info.RequestSubscriptionId
	task.PrivateData.PayRequestProductID = info.PayRequestProductId
	task.PrivateData.FinalPreConsumedQuota = info.FinalPreConsumedQuota
	task.PrivateData.FinalPreConsumedTokens = info.FinalPreConsumedTokens
	task.PrivateData.FinalPreConsumedRequests = info.FinalPreConsumedRequests
	task.PrivateData.FinalPreConsumedPayReqs = info.FinalPreConsumedPayRequests
}

func clearTaskRefundSnapshot(task *model.Task) {
	if task == nil {
		return
	}
	task.PrivateData.SubscriptionAllocations = nil
	task.PrivateData.PaygProductID = 0
	task.PrivateData.PaygProductAllocations = nil
	task.PrivateData.PayTokenProductID = 0
	task.PrivateData.RequestSubscriptionID = 0
	task.PrivateData.PayRequestProductID = 0
	task.PrivateData.FinalPreConsumedQuota = 0
	task.PrivateData.FinalVisibleQuota = 0
	task.PrivateData.FinalCostQuota = 0
	task.PrivateData.FinalPreConsumedTokens = 0
	task.PrivateData.FinalPreConsumedRequests = 0
	task.PrivateData.FinalPreConsumedPayReqs = 0
}

func adjustTaskSettlementUsageMetrics(ctx context.Context, task *model.Task, quotaDelta int) {
	if task == nil || quotaDelta == 0 {
		return
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", task.UserId).
		Update("used_quota", gorm.Expr("used_quota + ?", quotaDelta)).Error; err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("修正任务结算后的用户已用额度失败 task=%s: %s", task.TaskID, err.Error()))
	}
	model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
}

func persistTaskBillingSnapshot(ctx context.Context, task *model.Task, message string) {
	if task == nil {
		return
	}
	if _, err := task.UpdateWithStatus(task.Status); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("%s task=%s: %s", message, task.TaskID, err.Error()))
	}
}

func recordTaskRefundLog(task *model.Task, reason string) {
	if task == nil {
		return
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   legacyTaskRefundLogType,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     task.Quota,
		TokenId:   task.PrivateData.TokenID,
		Group:     task.Group,
		Other:     other,
	})
}

func taskHasRefundLog(ctx context.Context, task *model.Task) bool {
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return false
	}
	var count int64
	pattern := fmt.Sprintf("%%\"task_id\":\"%s\"%%", task.TaskID)
	if err := model.LOG_DB.Model(&model.Log{}).
		Where("type = ? AND user_id = ? AND other LIKE ?", legacyTaskRefundLogType, task.UserId, pattern).
		Count(&count).Error; err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("查询任务退款日志失败 task=%s: %s", task.TaskID, err.Error()))
		return false
	}
	return count > 0
}

func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string) {
	if task == nil || actualQuota <= 0 {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota
	if quotaDelta == 0 {
		return
	}

	info, hasSnapshot, err := buildTaskRelayInfo(task)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("构建任务结算上下文失败 task=%s: %s", task.TaskID, err.Error()))
		return
	}
	if hasSnapshot {
		normalizeTaskRelaySnapshot(ctx, task, info)
		if err := PostConsumeQuota(info, quotaDelta, quotaDelta, preConsumedQuota, false); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 差额结算失败: %s", task.TaskID, err.Error()))
			return
		}

		task.Quota = actualQuota
		oldFinalPreConsumedQuota := task.PrivateData.FinalPreConsumedQuota
		oldFinalPreConsumedTokens := task.PrivateData.FinalPreConsumedTokens
		oldSubscriptionAllocations := cloneSubscriptionAllocations(task.PrivateData.SubscriptionAllocations)
		oldPaygProductID := task.PrivateData.PaygProductID
		oldPaygProductAllocations := cloneProductQuotaAllocations(task.PrivateData.PaygProductAllocations)
		task.PrivateData.FinalPreConsumedQuota = actualQuota
		if task.PrivateData.QuotaBucket == model.UserQuotaBucketTokens || task.PrivateData.QuotaBucket == model.UserQuotaBucketPayToken {
			task.PrivateData.FinalPreConsumedTokens = actualQuota
		}
		task.PrivateData.SubscriptionAllocations = cloneSubscriptionAllocations(info.SubscriptionAllocations)
		task.PrivateData.PaygProductID = info.PaygProductId
		task.PrivateData.PaygProductAllocations = cloneProductQuotaAllocations(info.PaygProductAllocations)
		if err := task.Update(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("更新任务结算结果失败 task=%s: %s", task.TaskID, err.Error()))
			if rollbackErr := PostConsumeQuota(info, -quotaDelta, -quotaDelta, 0, false); rollbackErr != nil {
				logger.LogError(ctx, fmt.Sprintf("任务 %s 结算回滚失败: %s", task.TaskID, rollbackErr.Error()))
			}
			task.Quota = preConsumedQuota
			task.PrivateData.FinalPreConsumedQuota = oldFinalPreConsumedQuota
			task.PrivateData.FinalPreConsumedTokens = oldFinalPreConsumedTokens
			task.PrivateData.SubscriptionAllocations = oldSubscriptionAllocations
			task.PrivateData.PaygProductID = oldPaygProductID
			task.PrivateData.PaygProductAllocations = oldPaygProductAllocations
			return
		}
		adjustTaskSettlementUsageMetrics(ctx, task, quotaDelta)
		return
	}
	if !taskHasLegacyBilling(task.PrivateData) {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 缺少可结算的计费快照，拒绝使用模糊差额结算路径: %s", task.TaskID, reason))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 缺少新计费快照，使用旧差额结算路径: %s", task.TaskID, reason))
	if err := taskAdjustLegacyFunding(task, quotaDelta); err != nil {
		logger.LogError(ctx, fmt.Sprintf("旧差额结算资金调整失败 task=%s: %s", task.TaskID, err.Error()))
		return
	}
	taskAdjustLegacyTokenQuota(ctx, task, quotaDelta)

	task.Quota = actualQuota
	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
	} else {
		logType = legacyTaskRefundLogType
		logQuota = -quotaDelta
	}
	adjustTaskSettlementUsageMetrics(ctx, task, quotaDelta)
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenID,
		Group:     task.Group,
		Other:     other,
	})
}

func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if task == nil || totalTokens <= 0 {
		return
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelRatio <= 0 || bc.GroupRatio <= 0 {
		return
	}
	actualQuota := int(float64(totalTokens) * bc.ModelRatio * bc.GroupRatio)
	RecalculateTaskQuota(ctx, task, actualQuota, fmt.Sprintf("token重算: tokens=%d", totalTokens))
}

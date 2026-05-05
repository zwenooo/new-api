package service

import (
	"context"
	"fmt"
	"one-api/logger"
	"one-api/model"
	"strings"
	"time"
)

const taskSubmitRepairTTL = 24 * time.Hour

func RememberTaskSubmitRepair(task *model.Task) error {
	if task == nil || task.ID == 0 {
		return nil
	}
	taskID := normalizeTaskSubmitRepairID(task.TaskID)
	if taskID == "" {
		return nil
	}
	taskSnapshot := *task
	taskSnapshot.PrivateData = cloneTaskSubmitRepairPrivateData(task.PrivateData)
	taskSnapshot.Data = append([]byte(nil), task.Data...)
	if err := model.UpsertTaskSubmitRepair(&taskSnapshot, time.Now().Add(taskSubmitRepairTTL).Unix()); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("持久化异步任务提交修复记录失败 task=%s: %s", task.TaskID, err.Error()))
		return err
	}
	return nil
}

func ClearTaskSubmitRepair(taskID string) {
	taskID = normalizeTaskSubmitRepairID(taskID)
	if taskID == "" {
		return
	}
	if err := model.DeleteTaskSubmitRepair(taskID); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("删除异步任务提交修复记录失败 task=%s: %s", taskID, err.Error()))
	}
}

func TryRepairPendingTaskSubmit(ctx context.Context, task *model.Task) bool {
	if task == nil || task.ID == 0 || strings.TrimSpace(task.PrivateData.UpstreamTaskID) != "" {
		return false
	}
	taskID := normalizeTaskSubmitRepairID(task.TaskID)
	if taskID == "" {
		return false
	}
	repair, exist, err := model.GetTaskSubmitRepair(taskID)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("读取异步任务提交修复记录失败 task=%s: %s", task.TaskID, err.Error()))
		return false
	}
	if !exist || repair == nil {
		return false
	}
	fromStatus := task.Status
	repairedTask := *task
	repairedTask.PrivateData = cloneTaskSubmitRepairPrivateData(repair.PrivateData)
	repairedTask.Data = append([]byte(nil), repair.Data...)
	won, err := repairedTask.UpdateWithStatus(fromStatus)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("修复异步任务提交记录失败 task=%s: %s", task.TaskID, err.Error()))
		return false
	}
	if !won {
		logger.LogWarn(ctx, fmt.Sprintf("修复异步任务提交记录跳过 task=%s: status changed from %s", task.TaskID, fromStatus))
		return false
	}
	*task = repairedTask
	ClearTaskSubmitRepair(taskID)
	logger.LogInfo(ctx, fmt.Sprintf("已修复异步任务提交记录 task=%s upstream=%s", task.TaskID, task.PrivateData.UpstreamTaskID))
	return true
}

func normalizeTaskSubmitRepairID(value string) string {
	return strings.TrimSpace(value)
}

func cloneTaskSubmitRepairPrivateData(privateData model.TaskPrivateData) model.TaskPrivateData {
	clone := privateData
	clone.SubscriptionAllocations = cloneSubscriptionAllocations(privateData.SubscriptionAllocations)
	clone.PaygProductAllocations = cloneProductQuotaAllocations(privateData.PaygProductAllocations)
	if privateData.BillingContext != nil {
		billingContext := *privateData.BillingContext
		if len(privateData.BillingContext.OtherRatios) > 0 {
			billingContext.OtherRatios = make(map[string]float64, len(privateData.BillingContext.OtherRatios))
			for key, value := range privateData.BillingContext.OtherRatios {
				billingContext.OtherRatios[key] = value
			}
		}
		clone.BillingContext = &billingContext
	} else {
		clone.BillingContext = nil
	}
	return clone
}

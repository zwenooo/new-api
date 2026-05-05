package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"
)

type TaskPollingAdaptor interface {
	Init(info *relaycommon.RelayInfo)
	FetchTask(baseURL string, key string, proxy string, body map[string]any) (*http.Response, error)
	ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error)
}

type taskCompletionBillingAdaptor interface {
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
}

var GetTaskAdaptorFunc func(platform constant.TaskPlatform) TaskPollingAdaptor

const taskSubmitPersistGraceSeconds = 60

func sweepTimedOutTasks(ctx context.Context) {
	if constant.TaskTimeoutMinutes <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(constant.TaskTimeoutMinutes)*60
	tasks := model.GetTimedOutUnfinishedTasks(cutoff, 100)
	if len(tasks) == 0 {
		return
	}

	const legacyTaskCutoff int64 = 1740182400
	now := time.Now().Unix()
	reason := fmt.Sprintf("任务超时（%d分钟）", constant.TaskTimeoutMinutes)
	legacyReason := "任务超时（旧系统遗留任务，不进行退款，请联系管理员）"
	timedOutCount := 0

	for _, task := range tasks {
		referenceTime := task.SubmitTime
		if task.PrivateData.SubmitDispatchTime > referenceTime {
			referenceTime = task.PrivateData.SubmitDispatchTime
		}
		if referenceTime >= cutoff {
			continue
		}
		isLegacy := task.SubmitTime > 0 && task.SubmitTime < legacyTaskCutoff
		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		if isLegacy {
			task.FailReason = legacyReason
		} else {
			task.FailReason = reason
		}

		won, err := task.UpdateWithStatus(oldStatus)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务超时 CAS 更新失败 task=%s: %s", task.TaskID, err.Error()))
			continue
		}
		if !won {
			continue
		}
		timedOutCount++
		if !isLegacy {
			RefundTaskQuota(ctx, task, task.FailReason)
		}
	}

	if timedOutCount > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("已处理超时任务 %d 个", timedOutCount))
	}
}

func TaskPollingLoop() {
	for {
		time.Sleep(15 * time.Second)
		common.SysLog("任务进度轮询开始")
		ctx := context.TODO()
		sweepTimedOutTasks(ctx)

		allTasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
		platformTask := make(map[constant.TaskPlatform][]*model.Task)
		for _, task := range allTasks {
			platformTask[task.Platform] = append(platformTask[task.Platform], task)
		}

		for platform, tasks := range platformTask {
			if len(tasks) == 0 {
				continue
			}
			taskChannelM := make(map[int][]string)
			taskM := make(map[string]*model.Task)
			staleTasksWithoutUpstreamID := make([]*model.Task, 0)
			pendingTasksWithoutUpstreamID := 0
			dispatchedTasksWithoutUpstreamID := 0
			legacyFallbackTasks := 0
			now := time.Now().Unix()
			for _, task := range tasks {
				upstreamID := strings.TrimSpace(task.PrivateData.UpstreamTaskID)
				if upstreamID == "" && TryRepairPendingTaskSubmit(ctx, task) {
					upstreamID = strings.TrimSpace(task.PrivateData.UpstreamTaskID)
				}
				if upstreamID == "" {
					upstreamID = legacyTaskPollingUpstreamID(task)
					if upstreamID != "" {
						legacyFallbackTasks++
					}
				}
				if upstreamID == "" {
					if task.PrivateData.SubmitDispatchTime != 0 {
						dispatchedTasksWithoutUpstreamID++
					} else if task.SubmitTime > 0 && now-task.SubmitTime >= taskSubmitPersistGraceSeconds {
						staleTasksWithoutUpstreamID = append(staleTasksWithoutUpstreamID, task)
					} else {
						pendingTasksWithoutUpstreamID++
					}
					continue
				}
				taskKey := buildTaskPollingMapKey(task.ChannelId, task.PrivateData.Key, upstreamID)
				taskM[taskKey] = task
				taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], taskKey)
			}
			if len(staleTasksWithoutUpstreamID) > 0 {
				failPolledTasks(ctx, staleTasksWithoutUpstreamID, "任务缺少上游 task_id，无法继续轮询")
			}
			if pendingTasksWithoutUpstreamID > 0 {
				logger.LogWarn(ctx, fmt.Sprintf("检测到 %d 个近期缺少 upstream task_id 的任务，等待提交链补全", pendingTasksWithoutUpstreamID))
			}
			if dispatchedTasksWithoutUpstreamID > 0 {
				logger.LogWarn(ctx, fmt.Sprintf("检测到 %d 个已发往上游但仍缺少 upstream task_id 的任务，保留等待正常超时兜底", dispatchedTasksWithoutUpstreamID))
			}
			if legacyFallbackTasks > 0 {
				logger.LogWarn(ctx, fmt.Sprintf("检测到 %d 个旧任务缺少 private upstream task_id，已回退使用 task_id 继续轮询", legacyFallbackTasks))
			}
			if len(taskChannelM) == 0 {
				continue
			}

			switch platform {
			case constant.TaskPlatformMidjourney:
				continue
			case constant.TaskPlatformSuno:
				_ = UpdateSunoTasks(ctx, taskChannelM, taskM)
			default:
				if err := UpdateVideoTasks(ctx, platform, taskChannelM, taskM); err != nil {
					common.SysLog(fmt.Sprintf("UpdateVideoTasks fail: %s", err))
				}
			}
		}
		common.SysLog("任务进度轮询完成")
	}
}

func UpdateSunoTasks(ctx context.Context, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskKeys := range taskChannelM {
		if err := updateSunoTasks(ctx, channelId, taskKeys, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新 Suno 任务失败: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateSunoTasks(ctx context.Context, channelId int, taskKeys []string, taskM map[string]*model.Task) error {
	if len(taskKeys) == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(channelId)
	if err != nil {
		failTaskKeys(ctx, taskKeys, taskM, fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId))
		return err
	}
	adaptor := GetTaskAdaptorFunc(constant.TaskPlatformSuno)
	if adaptor == nil {
		return errors.New("suno adaptor not found")
	}

	taskIDsByKey := make(map[string][]string)
	for _, taskKey := range taskKeys {
		task := taskM[taskKey]
		if task == nil {
			continue
		}
		key := strings.TrimSpace(task.PrivateData.Key)
		if key == "" {
			key = ch.Key
		}
		upstreamID := resolvedTaskPollingUpstreamID(task)
		if upstreamID == "" {
			continue
		}
		taskIDsByKey[key] = append(taskIDsByKey[key], upstreamID)
	}

	var batchErr error
	for key, keyTaskIDs := range taskIDsByKey {
		info := &relaycommon.RelayInfo{}
		info.ChannelMeta = &relaycommon.ChannelMeta{
			ChannelBaseUrl: ch.GetBaseURL(),
		}
		info.ApiKey = key
		adaptor.Init(info)

		resp, err := adaptor.FetchTask(ch.GetBaseURL(), key, ch.GetSetting().Proxy, map[string]any{
			"ids": keyTaskIDs,
		})
		if err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("fetch suno tasks failed: %w", err))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			batchErr = errors.Join(batchErr, fmt.Errorf("get task status code: %d", resp.StatusCode))
			continue
		}

		responseBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("read suno task response failed: %w", err))
			continue
		}
		var responseItems dto.TaskResponse[[]dto.SunoDataResponse]
		if err := common.Unmarshal(responseBody, &responseItems); err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("unmarshal suno task response failed: %w", err))
			continue
		}
		if !responseItems.IsSuccess() {
			batchErr = errors.Join(batchErr, fmt.Errorf("upstream suno fetch failed: %s", string(responseBody)))
			continue
		}

		for _, responseItem := range responseItems.Data {
			task := getPolledTask(taskM, channelId, key, responseItem.TaskID)
			if task == nil || !taskNeedsUpdate(task, responseItem) {
				continue
			}

			snap := task.Snapshot()
			shouldRefund := false
			shouldSettle := false

			normalizedStatus := normalizeSunoTaskStatus(responseItem.Status)
			if responseItem.FailReason != "" {
				task.Status = model.TaskStatusFailure
			} else if normalizedStatus != "" {
				task.Status = normalizedStatus
			}
			task.FailReason = lo.If(responseItem.FailReason != "", responseItem.FailReason).Else(task.FailReason)
			task.SubmitTime = lo.If(responseItem.SubmitTime != 0, responseItem.SubmitTime).Else(task.SubmitTime)
			task.StartTime = lo.If(responseItem.StartTime != 0, responseItem.StartTime).Else(task.StartTime)
			task.FinishTime = lo.If(responseItem.FinishTime != 0, responseItem.FinishTime).Else(task.FinishTime)
			task.Data = SanitizeTaskPayload(responseItem.Data, task.TaskID, task.GetUpstreamTaskID())

			if responseItem.FailReason != "" || task.Status == model.TaskStatusFailure {
				task.Progress = "100%"
				shouldRefund = true
			}
			if task.Status == model.TaskStatusSuccess {
				task.Progress = "100%"
				shouldSettle = true
			}

			isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
			if isDone && snap.Status != task.Status {
				won, err := task.UpdateWithStatus(snap.Status)
				if err != nil || !won {
					shouldRefund = false
					shouldSettle = false
					continue
				}
			} else if !snap.Equal(task.Snapshot()) {
				if _, err := task.UpdateWithStatus(snap.Status); err != nil {
					logger.LogError(ctx, fmt.Sprintf("更新 Suno 任务失败 task=%s: %s", task.TaskID, err.Error()))
					continue
				}
			}

			if shouldSettle {
				settleTaskBillingOnComplete(ctx, adaptor, task, &relaycommon.TaskInfo{Status: responseItem.Status})
			}
			if shouldRefund {
				RefundTaskQuota(ctx, task, task.FailReason)
			}
		}
	}
	return batchErr
}

func taskNeedsUpdate(oldTask *model.Task, newTask dto.SunoDataResponse) bool {
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if newStatus := normalizeSunoTaskStatus(newTask.Status); newStatus != "" {
		if oldTask.Status != newStatus {
			return true
		}
	} else if string(oldTask.Status) != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}
	if (oldTask.Status == model.TaskStatusFailure || oldTask.Status == model.TaskStatusSuccess) && oldTask.Progress != "100%" {
		return true
	}

	oldData, _ := common.Marshal(oldTask.Data)
	newData, _ := common.Marshal(newTask.Data)
	sort.Slice(oldData, func(i, j int) bool { return oldData[i] < oldData[j] })
	sort.Slice(newData, func(i, j int) bool { return newData[i] < newData[j] })
	return string(oldData) != string(newData)
}

func normalizeSunoTaskStatus(status string) model.TaskStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return ""
	case "not_start":
		return model.TaskStatusNotStart
	case "submitted":
		return model.TaskStatusSubmitted
	case "queueing", "queued":
		return model.TaskStatusQueued
	case "processing":
		return model.TaskStatusInProgress
	case "success":
		return model.TaskStatusSuccess
	case "failed", "failure":
		return model.TaskStatusFailure
	default:
		return ""
	}
}

func legacyTaskPollingUpstreamID(task *model.Task) string {
	if task == nil || strings.TrimSpace(task.PrivateData.UpstreamTaskID) != "" {
		return ""
	}
	return strings.TrimSpace(task.GetUpstreamTaskID())
}

func resolvedTaskPollingUpstreamID(task *model.Task) string {
	if task == nil {
		return ""
	}
	if upstreamID := strings.TrimSpace(task.PrivateData.UpstreamTaskID); upstreamID != "" {
		return upstreamID
	}
	return legacyTaskPollingUpstreamID(task)
}

func buildTaskPollingMapKey(channelID int, stickyKey string, upstreamID string) string {
	return fmt.Sprintf("%d|%s|%s", channelID, strings.TrimSpace(stickyKey), strings.TrimSpace(upstreamID))
}

func getPolledTask(taskM map[string]*model.Task, channelID int, stickyKey string, upstreamID string) *model.Task {
	task := taskM[buildTaskPollingMapKey(channelID, stickyKey, upstreamID)]
	if task != nil {
		return task
	}
	if strings.TrimSpace(stickyKey) == "" {
		return nil
	}
	return taskM[buildTaskPollingMapKey(channelID, "", upstreamID)]
}

func failTaskKeys(ctx context.Context, taskKeys []string, taskM map[string]*model.Task, reason string) {
	tasks := make([]*model.Task, 0, len(taskKeys))
	for _, taskKey := range taskKeys {
		if task := taskM[taskKey]; task != nil {
			tasks = append(tasks, task)
		}
	}
	failPolledTasks(ctx, tasks, reason)
}

func failPolledTasks(ctx context.Context, tasks []*model.Task, reason string) {
	if len(tasks) == 0 {
		return
	}
	now := time.Now().Unix()
	for _, task := range tasks {
		if task == nil {
			continue
		}
		snap := task.Snapshot()
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		task.FailReason = reason
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("标记任务失败失败 task=%s: %s", task.TaskID, err.Error()))
			continue
		}
		if !won {
			continue
		}
		RefundTaskQuota(ctx, task, reason)
	}
}

func UpdateVideoTasks(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskKeys := range taskChannelM {
		if err := updateVideoTasks(ctx, platform, channelId, taskKeys, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新视频任务失败: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateVideoTasks(ctx context.Context, platform constant.TaskPlatform, channelId int, taskKeys []string, taskM map[string]*model.Task) error {
	if len(taskKeys) == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(channelId)
	if err != nil {
		failTaskKeys(ctx, taskKeys, taskM, fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId))
		return fmt.Errorf("cache get channel failed: %w", err)
	}

	adaptor := GetTaskAdaptorFunc(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: ch.GetBaseURL(),
	}
	info.ApiKey = ch.Key
	adaptor.Init(info)

	for _, taskKey := range taskKeys {
		if err := updateVideoSingleTask(ctx, adaptor, ch, taskKey, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("更新视频任务失败 task_key=%s: %s", taskKey, err.Error()))
		}
		time.Sleep(time.Second)
	}
	return nil
}

func updateVideoSingleTask(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, taskKey string, taskM map[string]*model.Task) error {
	task := taskM[taskKey]
	if task == nil {
		return fmt.Errorf("task %s not found", taskKey)
	}

	key := ch.Key
	if task.PrivateData.Key != "" {
		key = task.PrivateData.Key
	}
	upstreamTaskID := resolvedTaskPollingUpstreamID(task)
	if upstreamTaskID == "" {
		return fmt.Errorf("task %s missing upstream task id", task.TaskID)
	}

	resp, err := adaptor.FetchTask(ch.GetBaseURL(), key, ch.GetSetting().Proxy, map[string]any{
		"task_id": upstreamTaskID,
		"action":  task.Action,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	snap := task.Snapshot()
	taskResult := &relaycommon.TaskInfo{}
	var responseItems dto.TaskResponse[model.Task]
	if err = common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.GetResultURL()
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = SanitizeTaskPayload(t.Data, task.TaskID, task.GetUpstreamTaskID())
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return err
	}

	task.Data = SanitizeTaskPayload(redactVideoResponseBody(responseBody), task.TaskID, task.GetUpstreamTaskID())
	if taskResult.Status == "" {
		var errorResult dto.GeneralErrorResponse
		if err = common.Unmarshal(responseBody, &errorResult); err == nil {
			if openaiError := errorResult.TryToOpenAIError(); openaiError != nil {
				if fmt.Sprintf("%v", openaiError.Code) == "429" {
					return nil
				}
				taskResult = relaycommon.FailTaskInfo("upstream returned error")
			}
		}
		if taskResult.Status == "" {
			taskResult = relaycommon.FailTaskInfo("upstream returned empty status")
		}
	}

	now := time.Now().Unix()
	shouldRefund := false
	shouldSettle := false

	task.Status = model.TaskStatus(taskResult.Status)
	switch task.Status {
	case model.TaskStatusSubmitted:
		task.Progress = "10%"
	case model.TaskStatusQueued:
		task.Progress = "20%"
	case model.TaskStatusInProgress:
		task.Progress = "30%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if !strings.HasPrefix(taskResult.Url, "data:") && taskResult.Url != "" {
			task.PrivateData.ResultURL = taskResult.Url
		}
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		shouldRefund = true
	default:
		return fmt.Errorf("unknown task status %s", taskResult.Status)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}

	isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			return err
		}
		if !won {
			return nil
		}
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			return err
		}
	} else {
		return nil
	}

	if shouldSettle {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
	return nil
}

func settleTaskBillingOnComplete(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo) {
	if task != nil && task.PrivateData.BillingContext != nil && task.PrivateData.BillingContext.PerCallBilling {
		return
	}
	if billingAdaptor, ok := adaptor.(taskCompletionBillingAdaptor); ok {
		if actualQuota := billingAdaptor.AdjustBillingOnComplete(task, taskResult); actualQuota > 0 {
			RecalculateTaskQuota(ctx, task, actualQuota, "adaptor计费调整")
			return
		}
	}
	if taskResult != nil && taskResult.TotalTokens > 0 {
		RecalculateTaskQuotaByTokens(ctx, task, taskResult.TotalTokens)
	}
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	delete(m, "task_id")
	delete(m, "name")
	delete(m, "operationName")
	if data, ok := m["data"].(map[string]any); ok {
		delete(data, "task_id")
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

/*
Task 任务通过平台、Action 区分任务
*/
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	info.InitChannelMeta(c)
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	if strings.TrimSpace(info.TaskRelayInfo.PublicTaskID) == "" {
		info.TaskRelayInfo.PublicTaskID = model.GenerateTaskID()
	}
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}

	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	// get & validate taskRequest 获取并验证文本请求
	taskErr = adaptor.ValidateRequestAndSetAction(c, info)
	if taskErr != nil {
		return
	}

	if !model.GroupAllowsUserAgent(info.UsingGroupId, c.GetHeader("User-Agent")) {
		return service.TaskErrorWrapperLocal(errors.New("ua_not_allowed"), "access_denied", http.StatusForbidden)
	}

	billingRatios := map[string]float64(nil)
	if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		info.Action = constant.TaskActionRemix
	}
	if info.Action == constant.TaskActionRemix {
		videoID := strings.TrimSpace(c.Param("video_id"))
		if videoID == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}
	if info.OriginTaskID != "" {
		originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
		if err != nil {
			taskErr = service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
			return
		}
		if !exist {
			taskErr = service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
			return
		}
		if info.TaskRelayInfo != nil {
			info.TaskRelayInfo.OriginUpstreamTaskID = originTask.GetUpstreamTaskID()
		}
		if info.OriginModelName == "" {
			info.OriginModelName = resolveOriginTaskModelName(originTask)
		}
		originTaskKey := strings.TrimSpace(originTask.PrivateData.Key)
		if originTask.ChannelId != info.ChannelId {
			channel, err := model.GetChannelById(originTask.ChannelId, true)
			if err != nil {
				taskErr = service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				return service.TaskErrorWrapperLocal(errors.New("该任务所属渠道已被禁用"), "task_channel_disable", http.StatusBadRequest)
			}
			key := originTaskKey
			if key == "" {
				var newAPIError *types.NewAPIError
				key, _, newAPIError = channel.GetNextEnabledKey()
				if newAPIError != nil {
					return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
				}
			}
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())
			common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)
			common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
			common.SetContextKey(c, constant.ContextKeyChannelKey, key)
			common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)

			info.ApiKey = key
			info.ChannelBaseUrl = channel.GetBaseURL()
			info.ChannelId = originTask.ChannelId
			info.ChannelType = channel.Type
			info.ChannelSetting = channel.GetSetting()
			adaptor.Init(info)
		} else if originTaskKey != "" {
			common.SetContextKey(c, constant.ContextKeyChannelKey, originTaskKey)
			info.ApiKey = originTaskKey
		}
		if info.Action == constant.TaskActionRemix {
			billingRatios = mergeTaskBillingRatios(billingRatios, extractOriginTaskBillingRatios(originTask))
		}
	}

	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 预扣
	noBilling, err := model.IsUserEligibleForNoBillingGroup(info.UserId, info.UsingGroupId)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "check_no_billing_eligibility_failed", http.StatusInternalServerError)
		return
	}
	if noBilling {
		info.QuotaBucket = model.UserQuotaBucketFree
	}

	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return service.TaskErrorWrapper(err, "channel_model_mapped_error", http.StatusInternalServerError)
	}
	priceData := helper.ModelPriceHelperPerCall(c, info)
	quota := priceData.Quota

	billingRatios = mergeTaskBillingRatios(billingRatios, adaptor.EstimateBilling(c, info))

	// build body
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", common.RequestBodyErrorStatusCode(err))
			return
		}
		taskErr = service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
		return
	}
	task := initPendingTaskSubmitRecord(platform, info, modelName, quota, priceData, billingRatios)
	if err := task.Insert(); err != nil {
		taskErr = service.TaskErrorWrapper(err, "persist_task_failed", http.StatusInternalServerError)
		return
	}
	if info.QuotaBucket == model.UserQuotaBucketTokens || info.QuotaBucket == model.UserQuotaBucketPayToken {
		info.FinalPreConsumedTokens = quota
	}
	if apiErr := service.PreConsumeQuota(c, quota, info); apiErr != nil {
		if taskSubmitHasOutstandingReservation(info) {
			syncTaskSubmitBillingSnapshot(task, info)
			if err := service.RememberTaskSubmitRepair(task); err != nil {
				common.SysError(fmt.Sprintf("persist task submit repair snapshot error: task=%s err=%s", task.TaskID, err.Error()))
			}
			if err := updateTaskSubmitRecordWithRetry(task); err != nil {
				common.SysError(fmt.Sprintf("persist failed pre-consume snapshot error: task=%s err=%s", task.TaskID, err.Error()))
			}
			taskErr = refundTaskSubmitFailure(c, info, task, taskErrorFromPreConsume(apiErr))
			return
		} else {
			discardTaskSubmitRecord(task, apiErr.Error())
		}
		taskErr = taskErrorFromPreConsume(apiErr)
		return
	}
	syncTaskSubmitBillingSnapshot(task, info)
	task.PrivateData.SubmitDispatchTime = time.Now().Unix()
	if err := service.RememberTaskSubmitRepair(task); err != nil {
		common.SysError(fmt.Sprintf("persist task submit dispatch repair error: task=%s err=%s", task.TaskID, err.Error()))
	}
	if err := updateTaskSubmitRecordWithRetry(task); err != nil {
		taskErr = refundTaskSubmitFailure(c, info, task, service.TaskErrorWrapper(err, "persist_task_failed", http.StatusInternalServerError))
		return
	}
	// do request
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		taskErr = refundTaskSubmitFailure(c, info, task, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError))
		return
	}
	// handle response
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		err = errors.New(string(responseBody))
		code := "fail_to_fetch_task"
		if resp.StatusCode == http.StatusRequestEntityTooLarge || common.IsRequestBodyTooLargeError(err) {
			code = "invalid_request"
		}
		taskErr = refundTaskSubmitFailure(c, info, task, service.TaskErrorWrapper(err, code, resp.StatusCode))
		return
	}

	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		taskErr = refundTaskSubmitFailure(c, info, task, taskErr)
		return
	}
	billingRatios = mergeTaskBillingRatios(billingRatios, adaptor.AdjustBillingOnSubmit(info, taskData))

	task.Data = service.SanitizeTaskPayload(taskData, task.TaskID, upstreamTaskID)
	task.PrivateData.UpstreamTaskID = upstreamTaskID
	if task.PrivateData.BillingContext != nil {
		task.PrivateData.BillingContext.OtherRatios = billingRatios
	}
	repairErr := service.RememberTaskSubmitRepair(task)
	err = updateTaskSubmitRecordWithRetry(task)
	if err != nil {
		if repairErr != nil {
			taskErr = service.TaskErrorWrapperLocal(errors.New("任务提交成功，但持久化修复记录失败，请联系管理员"), "update_data_error", http.StatusInternalServerError)
			return
		}
		common.SysError(fmt.Sprintf("update task failed after upstream success: task=%s upstream=%s err=%s", task.TaskID, upstreamTaskID, err.Error()))
	} else {
		service.ClearTaskSubmitRepair(task.TaskID)
	}
	copyTaskSubmitResponseHeaders(c, resp)
	c.Data(http.StatusOK, "application/json", taskData)

	if quota != 0 || noBilling {
		tokenName := c.GetString("token_name")
		logContent := fmt.Sprintf("模型固定价格 %.2f，分组倍率 %.2f，操作 %s", priceData.ModelPrice, priceData.GroupRatioInfo.EffectiveGroupRatio, info.Action)
		other := make(map[string]interface{})
		other["model_price"] = priceData.ModelPrice
		other["group_ratio"] = priceData.GroupRatioInfo.EffectiveGroupRatio
		other["public_group_ratio"] = priceData.GroupRatioInfo.PublicGroupRatio
		other["private_group_ratio"] = priceData.GroupRatioInfo.PrivateGroupRatio
		other["group_ratio_source"] = priceData.GroupRatioInfo.Source
		other["base_multiplier_applied"] = priceData.GroupRatioInfo.BaseMultiplierApplied
		other["settled_quota"] = priceData.Quota
		other["visible_quota"] = priceData.VisibleQuota
		other["cost_quota"] = priceData.CostQuota
		if noBilling {
			other["no_billing"] = true
		}
		if priceData.GroupRatioInfo.HasSpecialRatio {
			other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
		}
		if len(billingRatios) > 0 {
			other["billing_ratios"] = billingRatios
		}
		if info.IsModelMapped {
			other["is_model_mapped"] = true
		}
		if strings.TrimSpace(info.UpstreamModelName) != "" && info.UpstreamModelName != modelName {
			other["upstream_model_name"] = info.UpstreamModelName
		}
		model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
			ChannelId:    info.ChannelId,
			ModelName:    modelName,
			TokenName:    tokenName,
			Quota:        quota,
			VisibleQuota: priceData.VisibleQuota,
			CostQuota:    priceData.CostQuota,
			Content:      logContent,
			TokenId:      info.TokenId,
			Group:        fmt.Sprintf("%d", info.UsingGroupId),
			Other:        other,
		})
		model.UpdateUserUsageAndRequestCount(info.UserId, quota, priceData.VisibleQuota, priceData.CostQuota)
		if quota != 0 || priceData.VisibleQuota != 0 || priceData.CostQuota != 0 {
			model.UpdateChannelUsageQuotas(info.ChannelId, quota, priceData.VisibleQuota, priceData.CostQuota)
		}
	}
	return nil
}

func taskErrorFromPreConsume(apiErr *types.NewAPIError) *dto.TaskError {
	if apiErr == nil {
		return nil
	}
	code := string(apiErr.GetErrorCode())
	if code == "" {
		code = "update_data_error"
	}
	statusCode := apiErr.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	return service.TaskErrorWrapperLocal(apiErr, code, statusCode)
}

func refundTaskSubmitFailure(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task, original *dto.TaskError) *dto.TaskError {
	if original == nil {
		return nil
	}
	if err := service.ReturnPreConsumedQuota(c, info); err != nil {
		if task != nil {
			if taskSubmitHasOutstandingReservation(info) {
				syncTaskSubmitBillingSnapshot(task, info)
				if rememberErr := service.RememberTaskSubmitRepair(task); rememberErr != nil {
					common.SysError(fmt.Sprintf("persist outstanding task submit repair error: task=%s err=%s", task.TaskID, rememberErr.Error()))
				}
				if updateErr := updateTaskSubmitRecordWithRetry(task); updateErr != nil {
					common.SysError(fmt.Sprintf("persist outstanding task submit snapshot error: task=%s err=%s", task.TaskID, updateErr.Error()))
				}
			} else {
				discardTaskSubmitRecord(task, original.Message)
			}
		}
		publicTaskID := ""
		if info != nil && info.TaskRelayInfo != nil {
			publicTaskID = strings.TrimSpace(info.TaskRelayInfo.PublicTaskID)
		}
		userID := 0
		if info != nil {
			userID = info.UserId
		}
		common.SysError(fmt.Sprintf(
			"refund task submit pre-consumed quota failed: user=%d task=%s err=%s",
			userID,
			publicTaskID,
			err.Error(),
		))
		return service.TaskErrorWrapperLocal(errors.New("计费回滚失败，请联系管理员"), "update_data_error", http.StatusInternalServerError)
	}
	discardTaskSubmitRecord(task, original.Message)
	return original
}

func initPendingTaskSubmitRecord(platform constant.TaskPlatform, info *relaycommon.RelayInfo, modelName string, quota int, priceData types.PerCallPriceData, billingRatios map[string]float64) *model.Task {
	task := model.InitTask(platform, info)
	task.Quota = quota
	task.VisibleQuota = priceData.VisibleQuota
	task.CostQuota = priceData.CostQuota
	task.Action = info.Action
	task.PrivateData.FinalPreConsumedQuota = info.FinalPreConsumedQuota
	task.PrivateData.FinalVisibleQuota = priceData.VisibleQuota
	task.PrivateData.FinalCostQuota = priceData.CostQuota
	task.PrivateData.FinalPreConsumedTokens = info.FinalPreConsumedTokens
	task.PrivateData.FinalPreConsumedRequests = info.FinalPreConsumedRequests
	task.PrivateData.FinalPreConsumedPayReqs = info.FinalPreConsumedPayRequests
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      priceData.ModelPrice,
		GroupRatio:      priceData.GroupRatioInfo.EffectiveGroupRatio,
		OriginModelName: modelName,
		OtherRatios:     billingRatios,
		PerCallBilling:  true,
	}
	return task
}

func finalizeFailedTaskSubmitRecord(task *model.Task, reason string) {
	if task == nil {
		return
	}
	service.ClearTaskSubmitRepair(task.TaskID)
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	if strings.TrimSpace(reason) != "" {
		task.FailReason = reason
	}
	clearTaskSubmitBillingSnapshot(task)
	if err := task.Update(); err != nil {
		common.SysError(fmt.Sprintf("finalize failed task submit record error: task=%s err=%s", task.TaskID, err.Error()))
	}
}

func clearTaskSubmitBillingSnapshot(task *model.Task) {
	if task == nil {
		return
	}
	task.PrivateData.UpstreamTaskID = ""
	task.PrivateData.SubmitDispatchTime = 0
	task.PrivateData.SubscriptionAllocations = nil
	task.PrivateData.PaygProductAllocations = nil
	task.PrivateData.PaygProductID = 0
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

func discardTaskSubmitRecord(task *model.Task, reason string) {
	if task == nil || task.ID == 0 {
		return
	}
	service.ClearTaskSubmitRepair(task.TaskID)
	if err := model.DB.Delete(task).Error; err != nil {
		common.SysError(fmt.Sprintf("discard task submit record error: task=%s err=%s", task.TaskID, err.Error()))
		finalizeFailedTaskSubmitRecord(task, reason)
	}
}

func updateTaskSubmitRecordWithRetry(task *model.Task) error {
	if task == nil {
		return nil
	}
	fromStatus := task.Status
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		won, err := task.UpdateWithStatus(fromStatus)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			continue
		}
		if !won {
			return fmt.Errorf("task status changed while persisting submit record: task=%s expected=%s", task.TaskID, fromStatus)
		}
		return nil
	}
	return lastErr
}

func syncTaskSubmitBillingSnapshot(task *model.Task, info *relaycommon.RelayInfo) {
	if task == nil || info == nil {
		return
	}
	task.PrivateData.SubscriptionAllocations = cloneTaskSubmitSubscriptionAllocations(info.SubscriptionAllocations)
	task.PrivateData.PaygProductID = info.PaygProductId
	task.PrivateData.PaygProductAllocations = cloneTaskSubmitProductAllocations(info.PaygProductAllocations)
	task.PrivateData.PayTokenProductID = info.PayTokenProductId
	task.PrivateData.RequestSubscriptionID = info.RequestSubscriptionId
	task.PrivateData.PayRequestProductID = info.PayRequestProductId
	task.PrivateData.FinalPreConsumedQuota = info.FinalPreConsumedQuota
	task.PrivateData.FinalPreConsumedTokens = info.FinalPreConsumedTokens
	task.PrivateData.FinalPreConsumedRequests = info.FinalPreConsumedRequests
	task.PrivateData.FinalPreConsumedPayReqs = info.FinalPreConsumedPayRequests
}

func taskSubmitHasOutstandingReservation(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	return len(info.SubscriptionAllocations) > 0 ||
		len(info.PaygProductAllocations) > 0 ||
		info.PaygProductId != 0 ||
		info.PayTokenProductId != 0 ||
		info.RequestSubscriptionId != 0 ||
		info.PayRequestProductId != 0 ||
		info.FinalPreConsumedQuota != 0 ||
		info.FinalPreConsumedTokens != 0 ||
		info.FinalPreConsumedRequests != 0 ||
		info.FinalPreConsumedPayRequests != 0
}

func cloneTaskSubmitSubscriptionAllocations(allocations []relaycommon.SubscriptionUnitAllocation) []relaycommon.SubscriptionUnitAllocation {
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

func cloneTaskSubmitProductAllocations(allocations []relaycommon.ProductQuotaAllocation) []relaycommon.ProductQuotaAllocation {
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

func copyTaskSubmitResponseHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || c.Writer == nil || resp == nil {
		return
	}
	for key, values := range resp.Header {
		if len(values) == 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "content-length", "content-type", "content-encoding", "transfer-encoding", "connection":
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			taskResp = &dto.TaskError{
				Code:       "invalid_request",
				Message:    err.Error(),
				StatusCode: common.RequestBodyErrorStatusCode(err),
				LocalError: true,
				Error:      err,
			}
			return
		}
		taskResp = service.TaskErrorWrapper(err, "invalid_request", common.RequestBodyErrorStatusCode(err))
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = json.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}
	service.TryRepairPendingTaskSubmit(c.Request.Context(), originTask)

	respBody, err = json.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}
	service.TryRepairPendingTaskSubmit(c.Request.Context(), originTask)

	func() {
		upstreamTaskID := strings.TrimSpace(originTask.PrivateData.UpstreamTaskID)
		if upstreamTaskID == "" {
			candidate := strings.TrimSpace(originTask.TaskID)
			if candidate != "" && !strings.HasPrefix(candidate, "task_") {
				upstreamTaskID = candidate
			}
		}
		if upstreamTaskID == "" {
			return
		}
		channelModel, err2 := model.GetChannelById(originTask.ChannelId, true)
		if err2 != nil {
			return
		}
		if channelModel.Type != constant.ChannelTypeVertexAi {
			return
		}
		baseURL := constant.ChannelBaseURLs[channelModel.Type]
		if channelModel.GetBaseURL() != "" {
			baseURL = channelModel.GetBaseURL()
		}
		adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
		if adaptor == nil {
			return
		}
		proxy := channelModel.GetSetting().Proxy
		resp, err2 := adaptor.FetchTask(baseURL, channelModel.Key, proxy, map[string]any{
			"task_id": upstreamTaskID,
			"action":  originTask.Action,
		})
		if err2 != nil || resp == nil {
			return
		}
		defer resp.Body.Close()
		body, err2 := io.ReadAll(resp.Body)
		if err2 != nil {
			return
		}
		ti, err2 := adaptor.ParseTaskResult(body)
		if err2 == nil && ti != nil {
			updatedTask := *originTask
			oldStatus := updatedTask.Status
			if ti.Status != "" {
				updatedTask.Status = model.TaskStatus(ti.Status)
			}
			if ti.Progress != "" {
				updatedTask.Progress = ti.Progress
			}
			if ti.Reason != "" {
				updatedTask.FailReason = ti.Reason
			}
			if ti.Url != "" {
				updatedTask.PrivateData.ResultURL = ti.Url
			}
			if won, err3 := updatedTask.UpdateWithStatus(oldStatus); err3 == nil && won {
				originTask = &updatedTask
			}
			var raw map[string]any
			_ = json.Unmarshal(body, &raw)
			format := "mp4"
			if respObj, ok := raw["response"].(map[string]any); ok {
				if vids, ok := respObj["videos"].([]any); ok && len(vids) > 0 {
					if v0, ok := vids[0].(map[string]any); ok {
						if mt, ok := v0["mimeType"].(string); ok && mt != "" {
							if strings.Contains(mt, "mp4") {
								format = "mp4"
							} else {
								format = mt
							}
						}
					}
				}
			}
			status := "processing"
			switch originTask.Status {
			case model.TaskStatusSuccess:
				status = "succeeded"
			case model.TaskStatusFailure:
				status = "failed"
			case model.TaskStatusQueued, model.TaskStatusSubmitted:
				status = "queued"
			}
			out := map[string]any{
				"error":    nil,
				"format":   format,
				"metadata": nil,
				"status":   status,
				"task_id":  originTask.TaskID,
				"url":      originTask.GetResultURL(),
			}
			respBody, _ = json.Marshal(dto.TaskResponse[any]{
				Code: "success",
				Data: out,
			})
		}
	}()

	if len(respBody) == 0 && isJimengOfficialFetchRequest(c) {
		respBody, err = buildJimengOfficialFetchRespBody(originTask)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
		}
		return
	}

	if len(respBody) == 0 {
		respBody, err = json.Marshal(dto.TaskResponse[any]{
			Code: "success",
			Data: TaskModel2Dto(originTask),
		})
	}
	return
}

func isJimengOfficialFetchRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(c.Query("Action")), "CVSync2AsyncGetResult")
}

func buildJimengOfficialFetchRespBody(task *model.Task) ([]byte, error) {
	data := map[string]any{
		"binary_data_base64": []any{},
		"image_urls":         []any{},
		"resp_data":          "",
		"status":             jimengTaskStatus(task),
		"video_url":          "",
	}
	resp := map[string]any{
		"code":         10000,
		"message":      "Success",
		"request_id":   "",
		"status":       10000,
		"time_elapsed": "",
		"data":         data,
	}
	if len(task.Data) > 0 {
		var stored map[string]any
		if err := json.Unmarshal(task.Data, &stored); err == nil {
			for key, value := range stored {
				resp[key] = value
			}
			if storedData, ok := stored["data"].(map[string]any); ok {
				for key, value := range storedData {
					data[key] = value
				}
			}
		}
	}
	data["status"] = jimengTaskStatus(task)
	if task.Status == model.TaskStatusSuccess {
		data["video_url"] = task.GetResultURL()
	}
	if task.Status == model.TaskStatusFailure {
		resp["message"] = task.FailReason
		resp["status"] = 50000
	}
	resp["data"] = data
	return json.Marshal(resp)
}

func jimengTaskStatus(task *model.Task) string {
	if task == nil {
		return "in_queue"
	}
	switch task.Status {
	case model.TaskStatusSuccess:
		return "done"
	case model.TaskStatusFailure:
		return "failed"
	default:
		return "in_queue"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:           task.ID,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
		TaskID:       task.TaskID,
		Platform:     string(task.Platform),
		UserId:       task.UserId,
		Group:        task.Group,
		ChannelId:    task.ChannelId,
		Quota:        task.Quota,
		VisibleQuota: task.VisibleQuota,
		CostQuota:    task.CostQuota,
		Action:       task.Action,
		Status:       string(task.Status),
		FailReason:   task.FailReason,
		ResultURL:    task.GetResultURL(),
		SubmitTime:   task.SubmitTime,
		StartTime:    task.StartTime,
		FinishTime:   task.FinishTime,
		Progress:     task.Progress,
		Properties:   task.Properties,
		Username:     task.Username,
		Data:         task.Data,
	}
}

func resolveOriginTaskModelName(originTask *model.Task) string {
	if originTask == nil {
		return ""
	}
	if originTask.Properties.OriginModelName != "" {
		return originTask.Properties.OriginModelName
	}
	if originTask.Properties.UpstreamModelName != "" {
		return originTask.Properties.UpstreamModelName
	}
	var taskData map[string]interface{}
	_ = common.Unmarshal(originTask.Data, &taskData)
	modelName, _ := taskData["model"].(string)
	return strings.TrimSpace(modelName)
}

func extractOriginTaskBillingRatios(originTask *model.Task) map[string]float64 {
	if originTask == nil {
		return nil
	}
	if bc := originTask.PrivateData.BillingContext; bc != nil {
		return mergeTaskBillingRatios(bc.OtherRatios)
	}

	var taskData map[string]interface{}
	_ = common.Unmarshal(originTask.Data, &taskData)
	secondsStr, _ := taskData["seconds"].(string)
	seconds, _ := strconv.Atoi(secondsStr)
	if seconds <= 0 {
		seconds = 4
	}
	sizeStr, _ := taskData["size"].(string)
	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func mergeTaskBillingRatios(ratios ...map[string]float64) map[string]float64 {
	var merged map[string]float64
	for _, ratioMap := range ratios {
		if len(ratioMap) == 0 {
			continue
		}
		if merged == nil {
			merged = make(map[string]float64)
		}
		for key, value := range ratioMap {
			merged[key] = value
		}
	}
	return merged
}

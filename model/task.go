package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/constant"
	commonRelay "one-api/relay/common"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskStatusNotStart   TaskStatus = "NOT_START"
	TaskStatusSubmitted             = "SUBMITTED"
	TaskStatusQueued                = "QUEUED"
	TaskStatusInProgress            = "IN_PROGRESS"
	TaskStatusFailure               = "FAILURE"
	TaskStatusSuccess               = "SUCCESS"
	TaskStatusUnknown               = "UNKNOWN"
)

type Task struct {
	ID           int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt    int64                 `json:"created_at" gorm:"index"`
	UpdatedAt    int64                 `json:"updated_at"`
	TaskID       string                `json:"task_id" gorm:"type:varchar(191);index"`
	Platform     constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index"`
	UserId       int                   `json:"user_id" gorm:"index"`
	Group        string                `json:"group" gorm:"type:varchar(50)"`
	ChannelId    int                   `json:"channel_id" gorm:"index"`
	Quota        int                   `json:"quota"`
	VisibleQuota int                   `json:"visible_quota" gorm:"default:0"`
	CostQuota    int                   `json:"cost_quota" gorm:"default:0"`
	Action       string                `json:"action" gorm:"type:varchar(40);index"`
	Status       TaskStatus            `json:"status" gorm:"type:varchar(20);index"`
	FailReason   string                `json:"fail_reason"`
	SubmitTime   int64                 `json:"submit_time" gorm:"index"`
	StartTime    int64                 `json:"start_time" gorm:"index"`
	FinishTime   int64                 `json:"finish_time" gorm:"index"`
	Progress     string                `json:"progress" gorm:"type:varchar(20);index"`
	Properties   Properties            `json:"properties" gorm:"type:json"`
	Username     string                `json:"username,omitempty" gorm:"-"`
	PrivateData  TaskPrivateData       `json:"-" gorm:"column:private_data;type:json"`
	Data         json.RawMessage       `json:"data" gorm:"type:json"`
}

func (t *Task) SetData(data any) {
	b, _ := common.Marshal(data)
	t.Data = json.RawMessage(b)
}

func (t *Task) GetData(v any) error {
	return common.Unmarshal(t.Data, v)
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

func (m *Properties) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return common.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	return common.Marshal(m)
}

type TaskPrivateData struct {
	Key                      string                                   `json:"key,omitempty"`
	TokenKey                 string                                   `json:"token_key,omitempty"`
	UpstreamTaskID           string                                   `json:"upstream_task_id,omitempty"`
	SubmitDispatchTime       int64                                    `json:"submit_dispatch_time,omitempty"`
	ResultURL                string                                   `json:"result_url,omitempty"`
	BillingSource            string                                   `json:"billing_source,omitempty"`
	SubscriptionID           int                                      `json:"subscription_id,omitempty"`
	QuotaBucket              string                                   `json:"quota_bucket,omitempty"`
	UsingGroupID             int                                      `json:"using_group_id,omitempty"`
	UserGroupID              int                                      `json:"user_group_id,omitempty"`
	TokenID                  int                                      `json:"token_id,omitempty"`
	SubscriptionAllocations  []commonRelay.SubscriptionUnitAllocation `json:"subscription_allocations,omitempty"`
	PaygProductID            int                                      `json:"payg_product_id,omitempty"`
	PaygProductAllocations   []commonRelay.ProductQuotaAllocation     `json:"payg_product_allocations,omitempty"`
	PayTokenProductID        int                                      `json:"pay_token_product_id,omitempty"`
	RequestSubscriptionID    int                                      `json:"request_subscription_id,omitempty"`
	PayRequestProductID      int                                      `json:"pay_request_product_id,omitempty"`
	FinalPreConsumedQuota    int                                      `json:"final_pre_consumed_quota,omitempty"`
	FinalVisibleQuota        int                                      `json:"final_visible_quota,omitempty"`
	FinalCostQuota           int                                      `json:"final_cost_quota,omitempty"`
	FinalPreConsumedTokens   int                                      `json:"final_pre_consumed_tokens,omitempty"`
	FinalPreConsumedRequests int                                      `json:"final_pre_consumed_requests,omitempty"`
	FinalPreConsumedPayReqs  int                                      `json:"final_pre_consumed_pay_requests,omitempty"`
	RefundAppliedAt          int64                                    `json:"refund_applied_at,omitempty"`
	BillingContext           *TaskBillingContext                      `json:"billing_context,omitempty"`
}

type TaskBillingContext struct {
	ModelPrice      float64            `json:"model_price,omitempty"`
	GroupRatio      float64            `json:"group_ratio,omitempty"`
	ModelRatio      float64            `json:"model_ratio,omitempty"`
	OtherRatios     map[string]float64 `json:"other_ratios,omitempty"`
	OriginModelName string             `json:"origin_model_name,omitempty"`
	PerCallBilling  bool               `json:"per_call_billing,omitempty"`
}

func (p *TaskPrivateData) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		*p = TaskPrivateData{}
		return nil
	}
	return common.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	b, err := common.Marshal(p)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(b)
	if bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	return b, nil
}

func (t *Task) GetUpstreamTaskID() string {
	if t == nil {
		return ""
	}
	if upstreamTaskID := strings.TrimSpace(t.PrivateData.UpstreamTaskID); upstreamTaskID != "" {
		return upstreamTaskID
	}
	taskID := strings.TrimSpace(t.TaskID)
	if taskID == "" {
		return ""
	}
	if !strings.HasPrefix(taskID, "task_") || t.AllowLegacyTaskIDFallback() {
		return taskID
	}
	return ""
}

func (t *Task) AllowLegacyTaskIDFallback() bool {
	if t == nil {
		return false
	}
	return t.PrivateData.BillingContext == nil &&
		t.PrivateData.QuotaBucket == "" &&
		t.PrivateData.SubmitDispatchTime == 0
}

func (t *Task) GetResultURL() string {
	if t.PrivateData.ResultURL != "" {
		return t.PrivateData.ResultURL
	}
	return t.FailReason
}

type SyncTaskQueryParams struct {
	Platform       constant.TaskPlatform
	ChannelID      string
	TaskID         string
	UserID         string
	Action         string
	Status         string
	StartTimestamp int64
	EndTimestamp   int64
	UserIDs        []int
}

func GenerateTaskID() string {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil || key == "" {
		return fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	return "task_" + key
}

func InitTask(platform constant.TaskPlatform, relayInfo *commonRelay.RelayInfo) *Task {
	taskID := GenerateTaskID()
	task := &Task{
		TaskID:     taskID,
		SubmitTime: time.Now().Unix(),
		Status:     TaskStatusNotStart,
		Progress:   "0%",
		Platform:   platform,
	}

	if relayInfo == nil {
		return task
	}

	if relayInfo.TaskRelayInfo != nil && relayInfo.TaskRelayInfo.PublicTaskID != "" {
		task.TaskID = relayInfo.TaskRelayInfo.PublicTaskID
	}

	task.UserId = relayInfo.UserId
	task.Group = fmt.Sprintf("%d", relayInfo.UsingGroupId)
	task.ChannelId = relayInfo.ChannelId
	task.Properties.OriginModelName = relayInfo.OriginModelName
	task.Properties.UpstreamModelName = relayInfo.UpstreamModelName
	task.PrivateData.Key = relayInfo.ApiKey
	task.PrivateData.TokenKey = relayInfo.TokenKey
	task.PrivateData.QuotaBucket = relayInfo.QuotaBucket
	task.PrivateData.UsingGroupID = relayInfo.UsingGroupId
	task.PrivateData.UserGroupID = relayInfo.UserGroupId
	task.PrivateData.TokenID = relayInfo.TokenId
	task.PrivateData.SubscriptionAllocations = relayInfo.SubscriptionAllocations
	task.PrivateData.PaygProductID = relayInfo.PaygProductId
	task.PrivateData.PaygProductAllocations = relayInfo.PaygProductAllocations
	task.PrivateData.PayTokenProductID = relayInfo.PayTokenProductId
	task.PrivateData.RequestSubscriptionID = relayInfo.RequestSubscriptionId
	task.PrivateData.PayRequestProductID = relayInfo.PayRequestProductId
	return task
}

func TaskGetAllUserTask(userId int, startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	query := DB.Where("user_id = ?", userId)

	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	if err := query.Omit("channel_id").Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error; err != nil {
		return nil
	}
	return tasks
}

func TaskGetAllTasks(startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	query := DB

	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error; err != nil {
		return nil
	}
	return tasks
}

func GetTimedOutUnfinishedTasks(cutoffUnix int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("progress != ?", "100%").
		Where("status NOT IN ?", []string{string(TaskStatusFailure), string(TaskStatusSuccess)}).
		Where("submit_time < ?", cutoffUnix).
		Order("submit_time").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetAllUnFinishSyncTasks(limit int) []*Task {
	var tasks []*Task
	err := DB.Where("progress != ?", "100%").
		Where("status != ?", TaskStatusFailure).
		Where("status != ?", TaskStatusSuccess).
		Limit(limit).
		Order("id").
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetByOnlyTaskId(taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	err := DB.Where("task_id = ?", taskId).First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskId(userId int, taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	err := DB.Where("user_id = ? and task_id = ?", userId, taskId).First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskIds(userId int, taskIds []any) ([]*Task, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	var tasks []*Task
	err := DB.Where("user_id = ? and task_id in (?)", userId, taskIds).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func TaskUpdateProgress(id int64, progress string) error {
	return DB.Model(&Task{}).Where("id = ?", id).Update("progress", progress).Error
}

func (task *Task) Insert() error {
	return DB.Create(task).Error
}

type taskSnapshot struct {
	Status     TaskStatus
	Progress   string
	StartTime  int64
	FinishTime int64
	FailReason string
	ResultURL  string
	Data       json.RawMessage
}

func (s taskSnapshot) Equal(other taskSnapshot) bool {
	return s.Status == other.Status &&
		s.Progress == other.Progress &&
		s.StartTime == other.StartTime &&
		s.FinishTime == other.FinishTime &&
		s.FailReason == other.FailReason &&
		s.ResultURL == other.ResultURL &&
		bytes.Equal(s.Data, other.Data)
}

func (t *Task) Snapshot() taskSnapshot {
	return taskSnapshot{
		Status:     t.Status,
		Progress:   t.Progress,
		StartTime:  t.StartTime,
		FinishTime: t.FinishTime,
		FailReason: t.FailReason,
		ResultURL:  t.PrivateData.ResultURL,
		Data:       t.Data,
	}
}

func (task *Task) Update() error {
	return DB.Save(task).Error
}

func (t *Task) UpdateWithStatus(fromStatus TaskStatus) (bool, error) {
	result := DB.Model(t).Where("status = ?", fromStatus).Select("*").Updates(t)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func TaskBulkUpdate(taskIDs []string, params map[string]any) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return DB.Model(&Task{}).Where("task_id in (?)", taskIDs).Updates(params).Error
}

func TaskBulkUpdateByTaskIds(taskIDs []int64, params map[string]any) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return DB.Model(&Task{}).Where("id in (?)", taskIDs).Updates(params).Error
}

func TaskBulkUpdateByID(ids []int64, params map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&Task{}).Where("id in (?)", ids).Updates(params).Error
}

type TaskQuotaUsage struct {
	Mode  string  `json:"mode"`
	Count float64 `json:"count"`
}

func SumUsedTaskQuota(queryParams SyncTaskQueryParams) (stat []TaskQuotaUsage, err error) {
	query := DB.Model(Task{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	err = query.Select("mode, sum(quota) as count").Group("mode").Find(&stat).Error
	return stat, err
}

func TaskCountAllTasks(queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

func TaskCountAllUserTask(userId int, queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{}).Where("user_id = ?", userId)
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

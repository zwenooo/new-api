package service

import (
	"context"
	"path/filepath"
	"testing"

	"one-api/billing"
	"one-api/common"
	"one-api/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTaskBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "task-billing.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Group{},
		&model.User{},
		&model.UserSubscription{},
		&model.UserRequestSubscription{},
		&model.UserRequestSubscriptionGroup{},
		&model.Task{},
		&model.Log{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withTaskBillingDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldLogConsumeEnabled := common.LogConsumeEnabled

	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.LogConsumeEnabled = oldLogConsumeEnabled
	})
}

func createTaskBillingUser(t *testing.T, db *gorm.DB, user model.User) model.User {
	t.Helper()
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createTaskBillingGroup(t *testing.T, db *gorm.DB, code string) model.Group {
	t.Helper()
	group := model.Group{
		Code:           code,
		DisplayName:    code,
		Ratio:          1,
		UserSelectable: true,
		Enabled:        true,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group %s: %v", code, err)
	}
	return group
}

func createTaskBillingRequestSubscription(t *testing.T, db *gorm.DB, userID int, groupID int, used int) model.UserRequestSubscription {
	t.Helper()
	sub := model.UserRequestSubscription{
		UserId:                userID,
		DailyRequestLimit:     billing.DisplayIntUnitsToStored(5),
		DailyRequestUsed:      used,
		DailyRequestResetDate: common.GetTodayDateInt(),
		TotalRequestLimit:     billing.DisplayIntUnitsToStored(10),
		TotalRequestUsed:      used,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create request subscription: %v", err)
	}
	if err := db.Create(&model.UserRequestSubscriptionGroup{
		SubscriptionId: sub.Id,
		GroupId:        groupID,
	}).Error; err != nil {
		t.Fatalf("create request subscription group binding: %v", err)
	}
	return sub
}

func createLegacyWalletTask(userID int, quota int) *model.Task {
	return &model.Task{
		TaskID:    "task_legacy_billing",
		UserId:    userID,
		Quota:     quota,
		Status:    model.TaskStatusFailure,
		ChannelId: 0,
		Group:     "test",
		PrivateData: model.TaskPrivateData{
			BillingSource: legacyTaskBillingSourceWallet,
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "test-model",
			},
		},
	}
}

func countTaskBillingLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := model.LOG_DB.Model(&model.Log{}).Count(&count).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	return count
}

func reloadUser(t *testing.T, id int) model.User {
	t.Helper()
	var user model.User
	if err := model.DB.First(&user, id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	return user
}

func reloadTask(t *testing.T, id int64) model.Task {
	t.Helper()
	var task model.Task
	if err := model.DB.First(&task, id).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	return task
}

func lastTaskBillingLog(t *testing.T) model.Log {
	t.Helper()
	var log model.Log
	if err := model.LOG_DB.Order("id DESC").First(&log).Error; err != nil {
		t.Fatalf("reload last log: %v", err)
	}
	return log
}

func TestRefundTaskQuotaLegacyWalletPersistsRefundMarker(t *testing.T) {
	db := newTaskBillingTestDB(t)
	withTaskBillingDB(t, db)

	user := createTaskBillingUser(t, db, model.User{
		Username: "legacy-refund",
		Password: "password123",
		Quota:    1000,
	})

	task := createLegacyWalletTask(user.Id, 300)
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	ctx := context.Background()
	RefundTaskQuota(ctx, task, "task failed")

	reloadedTask := reloadTask(t, task.ID)
	if reloadedTask.PrivateData.RefundAppliedAt <= 0 {
		t.Fatalf("refund_applied_at = %d, want > 0", reloadedTask.PrivateData.RefundAppliedAt)
	}

	RefundTaskQuota(ctx, &reloadedTask, "task failed again")

	storedUser := reloadUser(t, user.Id)
	if storedUser.Quota != 1300 {
		t.Fatalf("user quota = %d, want 1300", storedUser.Quota)
	}
	if got := countTaskBillingLogs(t); got != 1 {
		t.Fatalf("log count = %d, want 1", got)
	}

	log := lastTaskBillingLog(t)
	if log.Type != legacyTaskRefundLogType {
		t.Fatalf("log type = %d, want %d", log.Type, legacyTaskRefundLogType)
	}
}

func TestRecalculateTaskQuotaLegacyPositiveDeltaKeepsRequestCountStable(t *testing.T) {
	db := newTaskBillingTestDB(t)
	withTaskBillingDB(t, db)

	user := createTaskBillingUser(t, db, model.User{
		Username:     "legacy-settle-pos",
		Password:     "password123",
		Quota:        1000,
		UsedQuota:    200,
		RequestCount: 1,
	})

	task := createLegacyWalletTask(user.Id, 200)
	task.Status = model.TaskStatusSuccess

	RecalculateTaskQuota(context.Background(), task, 350, "adaptor adjustment")

	storedUser := reloadUser(t, user.Id)
	if storedUser.Quota != 850 {
		t.Fatalf("user quota = %d, want 850", storedUser.Quota)
	}
	if storedUser.UsedQuota != 350 {
		t.Fatalf("user used_quota = %d, want 350", storedUser.UsedQuota)
	}
	if storedUser.RequestCount != 1 {
		t.Fatalf("user request_count = %d, want 1", storedUser.RequestCount)
	}
	if task.Quota != 350 {
		t.Fatalf("task quota = %d, want 350", task.Quota)
	}

	log := lastTaskBillingLog(t)
	if log.Type != model.LogTypeConsume {
		t.Fatalf("log type = %d, want %d", log.Type, model.LogTypeConsume)
	}
	if log.Quota != 150 {
		t.Fatalf("log quota = %d, want 150", log.Quota)
	}
}

func TestRecalculateTaskQuotaLegacyNegativeDeltaRollsBackUsedQuotaOnly(t *testing.T) {
	db := newTaskBillingTestDB(t)
	withTaskBillingDB(t, db)

	user := createTaskBillingUser(t, db, model.User{
		Username:     "legacy-settle-neg",
		Password:     "password123",
		Quota:        1000,
		UsedQuota:    500,
		RequestCount: 1,
	})

	task := createLegacyWalletTask(user.Id, 500)
	task.Status = model.TaskStatusSuccess

	RecalculateTaskQuota(context.Background(), task, 300, "adaptor adjustment")

	storedUser := reloadUser(t, user.Id)
	if storedUser.Quota != 1200 {
		t.Fatalf("user quota = %d, want 1200", storedUser.Quota)
	}
	if storedUser.UsedQuota != 300 {
		t.Fatalf("user used_quota = %d, want 300", storedUser.UsedQuota)
	}
	if storedUser.RequestCount != 1 {
		t.Fatalf("user request_count = %d, want 1", storedUser.RequestCount)
	}
	if task.Quota != 300 {
		t.Fatalf("task quota = %d, want 300", task.Quota)
	}

	log := lastTaskBillingLog(t)
	if log.Type != legacyTaskRefundLogType {
		t.Fatalf("log type = %d, want %d", log.Type, legacyTaskRefundLogType)
	}
	if log.Quota != 200 {
		t.Fatalf("log quota = %d, want 200", log.Quota)
	}
}

func TestRefundTaskQuotaPersistsOutstandingSnapshotAfterPartialRequestRefundFailure(t *testing.T) {
	db := newTaskBillingTestDB(t)
	withTaskBillingDB(t, db)

	group := createTaskBillingGroup(t, db, "partial-request")
	user := createTaskBillingUser(t, db, model.User{
		Username: "partial-request-refund",
		Password: "password123",
		GroupId:  group.Id,
		Group:    group.Code,
	})
	requestUnits := billing.DisplayIntUnitsToStored(1)
	sub := createTaskBillingRequestSubscription(t, db, user.Id, group.Id, requestUnits)

	task := &model.Task{
		TaskID: "task_partial_request_refund",
		UserId: user.Id,
		Quota:  20,
		Status: model.TaskStatusFailure,
		Group:  group.Code,
		PrivateData: model.TaskPrivateData{
			QuotaBucket:              model.UserQuotaBucketRequest,
			UsingGroupID:             group.Id,
			RequestSubscriptionID:    sub.Id,
			FinalPreConsumedRequests: requestUnits,
			FinalPreConsumedQuota:    20,
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "test-model",
			},
		},
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	RefundTaskQuota(context.Background(), task, "missing token info")

	var storedSub model.UserRequestSubscription
	if err := db.First(&storedSub, sub.Id).Error; err != nil {
		t.Fatalf("reload request subscription: %v", err)
	}
	if storedSub.DailyRequestUsed != 0 {
		t.Fatalf("daily_request_used after partial refund = %d, want 0", storedSub.DailyRequestUsed)
	}
	if storedSub.TotalRequestUsed != 0 {
		t.Fatalf("total_request_used after partial refund = %d, want 0", storedSub.TotalRequestUsed)
	}

	reloadedTask := reloadTask(t, task.ID)
	if reloadedTask.PrivateData.RequestSubscriptionID != 0 {
		t.Fatalf("request_subscription_id after partial refund = %d, want 0", reloadedTask.PrivateData.RequestSubscriptionID)
	}
	if reloadedTask.PrivateData.FinalPreConsumedRequests != 0 {
		t.Fatalf("final_pre_consumed_requests after partial refund = %d, want 0", reloadedTask.PrivateData.FinalPreConsumedRequests)
	}
	if reloadedTask.PrivateData.FinalPreConsumedQuota != 20 {
		t.Fatalf("final_pre_consumed_quota after partial refund = %d, want 20", reloadedTask.PrivateData.FinalPreConsumedQuota)
	}
	if reloadedTask.PrivateData.RefundAppliedAt != 0 {
		t.Fatalf("refund_applied_at after partial refund = %d, want 0", reloadedTask.PrivateData.RefundAppliedAt)
	}
	if got := countTaskBillingLogs(t); got != 0 {
		t.Fatalf("log count after partial refund = %d, want 0", got)
	}
}

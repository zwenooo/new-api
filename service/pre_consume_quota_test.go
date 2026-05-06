package service

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"one-api/billing"
	"one-api/model"
	relaycommon "one-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPreConsumeQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "pre-consume-quota.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Group{},
		&model.User{},
		&model.UserSubscription{},
		&model.UserSubscriptionGroup{},
		&model.Token{},
		&model.UserRequestSubscription{},
		&model.UserRequestSubscriptionGroup{},
		&model.UserRequestSubscriptionPresetRevisionBinding{},
		&model.PayRequestUserBalance{},
		&model.PayRequestProductGroup{},
		&model.Task{},
		&model.Log{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withPreConsumeQuotaDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
}

func createPreConsumeQuotaTestGroup(t *testing.T, db *gorm.DB, code string) model.Group {
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

func createPreConsumeQuotaTestUser(t *testing.T, db *gorm.DB, username string, group model.Group) model.User {
	t.Helper()
	user := model.User{
		Username: username,
		Password: "password123",
		GroupId:  group.Id,
		Group:    group.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return user
}

func createPreConsumeQuotaRequestSubscription(t *testing.T, db *gorm.DB, userID int, groupID int) model.UserRequestSubscription {
	t.Helper()
	sub := model.UserRequestSubscription{
		UserId:                userID,
		DailyRequestLimit:     billing.DisplayIntUnitsToStored(5),
		TotalRequestLimit:     billing.DisplayIntUnitsToStored(10),
		DailyRequestUsed:      0,
		TotalRequestUsed:      0,
		ExpireAt:              0,
		InvalidAt:             0,
		DailyRequestResetDate: 0,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create request subscription: %v", err)
	}
	binding := model.UserRequestSubscriptionGroup{
		SubscriptionId: sub.Id,
		GroupId:        groupID,
	}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatalf("create request subscription group binding: %v", err)
	}
	return sub
}

func createPreConsumeQuotaPayRequestBalance(t *testing.T, db *gorm.DB, userID int, productID int, groupID int, requests int) {
	t.Helper()
	groupIDsJSON, err := model.MarshalGroupIDsJSON([]int{groupID})
	if err != nil {
		t.Fatalf("marshal group ids: %v", err)
	}
	balance := model.PayRequestUserBalance{
		UserId:            userID,
		ProductId:         productID,
		ProductName:       "pay-request",
		SortOrder:         10,
		AllowedGroupIds:   groupIDsJSON,
		RemainingRequests: requests,
		HistoryRequests:   requests,
	}
	if err := db.Create(&balance).Error; err != nil {
		t.Fatalf("create pay-request balance: %v", err)
	}
	if err := db.Create(&model.PayRequestProductGroup{
		ProductId: productID,
		GroupId:   groupID,
	}).Error; err != nil {
		t.Fatalf("create pay-request product group: %v", err)
	}
}

func newPreConsumeQuotaGinContext() *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx
}

func reloadPreConsumeQuotaRequestSubscription(t *testing.T, db *gorm.DB, id int) model.UserRequestSubscription {
	t.Helper()
	var sub model.UserRequestSubscription
	if err := db.First(&sub, id).Error; err != nil {
		t.Fatalf("reload request subscription: %v", err)
	}
	return sub
}

func reloadPreConsumeQuotaPayRequestBalance(t *testing.T, db *gorm.DB, userID int, productID int) model.PayRequestUserBalance {
	t.Helper()
	var balance model.PayRequestUserBalance
	if err := db.Where("user_id = ? AND product_id = ?", userID, productID).First(&balance).Error; err != nil {
		t.Fatalf("reload pay-request balance: %v", err)
	}
	return balance
}

func reloadPreConsumeQuotaUser(t *testing.T, db *gorm.DB, userID int) model.User {
	t.Helper()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	return user
}

func reloadPreConsumeQuotaToken(t *testing.T, db *gorm.DB, tokenID int) model.Token {
	t.Helper()
	var token model.Token
	if err := db.First(&token, tokenID).Error; err != nil {
		t.Fatalf("reload token: %v", err)
	}
	return token
}

func TestPreConsumeQuotaRequestBucketReturnIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newPreConsumeQuotaTestDB(t)
	withPreConsumeQuotaDB(t, db)

	group := createPreConsumeQuotaTestGroup(t, db, "request")
	user := createPreConsumeQuotaTestUser(t, db, "request-bucket-user", group)
	sub := createPreConsumeQuotaRequestSubscription(t, db, user.Id, group.Id)
	requestUnits := billing.DisplayIntUnitsToStored(1)

	ctx := newPreConsumeQuotaGinContext()
	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UsingGroupId: group.Id,
		QuotaBucket:  model.UserQuotaBucketRequest,
	}

	if err := PreConsumeQuota(ctx, 0, relayInfo); err != nil {
		t.Fatalf("PreConsumeQuota() error = %v", err)
	}
	if relayInfo.FinalPreConsumedRequests != requestUnits {
		t.Fatalf("FinalPreConsumedRequests = %d, want %d", relayInfo.FinalPreConsumedRequests, requestUnits)
	}
	if relayInfo.RequestSubscriptionId != sub.Id {
		t.Fatalf("RequestSubscriptionId = %d, want %d", relayInfo.RequestSubscriptionId, sub.Id)
	}

	storedSub := reloadPreConsumeQuotaRequestSubscription(t, db, sub.Id)
	if storedSub.DailyRequestUsed != requestUnits {
		t.Fatalf("daily_request_used = %d, want %d", storedSub.DailyRequestUsed, requestUnits)
	}
	if storedSub.TotalRequestUsed != requestUnits {
		t.Fatalf("total_request_used = %d, want %d", storedSub.TotalRequestUsed, requestUnits)
	}

	if err := ReturnPreConsumedQuota(ctx, relayInfo); err != nil {
		t.Fatalf("ReturnPreConsumedQuota(first) error = %v", err)
	}
	if relayInfo.FinalPreConsumedRequests != 0 {
		t.Fatalf("FinalPreConsumedRequests after refund = %d, want 0", relayInfo.FinalPreConsumedRequests)
	}
	if relayInfo.RequestSubscriptionId != 0 {
		t.Fatalf("RequestSubscriptionId after refund = %d, want 0", relayInfo.RequestSubscriptionId)
	}

	storedSub = reloadPreConsumeQuotaRequestSubscription(t, db, sub.Id)
	if storedSub.DailyRequestUsed != 0 {
		t.Fatalf("daily_request_used after refund = %d, want 0", storedSub.DailyRequestUsed)
	}
	if storedSub.TotalRequestUsed != 0 {
		t.Fatalf("total_request_used after refund = %d, want 0", storedSub.TotalRequestUsed)
	}

	if err := ReturnPreConsumedQuota(ctx, relayInfo); err != nil {
		t.Fatalf("ReturnPreConsumedQuota(second) error = %v", err)
	}

	storedSub = reloadPreConsumeQuotaRequestSubscription(t, db, sub.Id)
	if storedSub.DailyRequestUsed != 0 {
		t.Fatalf("daily_request_used after second refund = %d, want 0", storedSub.DailyRequestUsed)
	}
	if storedSub.TotalRequestUsed != 0 {
		t.Fatalf("total_request_used after second refund = %d, want 0", storedSub.TotalRequestUsed)
	}
}

func TestWssPayRequestRoundReservationCommitAndRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newPreConsumeQuotaTestDB(t)
	withPreConsumeQuotaDB(t, db)

	group := createPreConsumeQuotaTestGroup(t, db, "pay-request")
	user := createPreConsumeQuotaTestUser(t, db, "pay-request-user", group)
	totalRequests := billing.DisplayIntUnitsToStored(5)
	requestUnits := billing.DisplayIntUnitsToStored(1)
	createPreConsumeQuotaPayRequestBalance(t, db, user.Id, 99101, group.Id, totalRequests)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UsingGroupId: group.Id,
		QuotaBucket:  model.UserQuotaBucketPayRequest,
	}

	if err := PreConsumeWssRequestRound(relayInfo); err != nil {
		t.Fatalf("PreConsumeWssRequestRound(first) error = %v", err)
	}
	if relayInfo.FinalPreConsumedPayRequests != requestUnits {
		t.Fatalf("FinalPreConsumedPayRequests = %d, want %d", relayInfo.FinalPreConsumedPayRequests, requestUnits)
	}
	if relayInfo.PayRequestProductId != 99101 {
		t.Fatalf("PayRequestProductId = %d, want 99101", relayInfo.PayRequestProductId)
	}
	if len(relayInfo.PayRequestProductAllocations) != 1 ||
		relayInfo.PayRequestProductAllocations[0].ProductId != 99101 ||
		relayInfo.PayRequestProductAllocations[0].Quota != requestUnits {
		t.Fatalf("PayRequestProductAllocations = %#v, want one allocation for 99101/%d", relayInfo.PayRequestProductAllocations, requestUnits)
	}

	storedBalance := reloadPreConsumeQuotaPayRequestBalance(t, db, user.Id, 99101)
	if storedBalance.RemainingRequests != totalRequests-requestUnits {
		t.Fatalf("remaining_requests after first round = %d, want %d", storedBalance.RemainingRequests, totalRequests-requestUnits)
	}
	storedUser := reloadPreConsumeQuotaUser(t, db, user.Id)
	if storedUser.PayRequestQuota != totalRequests-requestUnits {
		t.Fatalf("user pay_request_quota after first round = %d, want %d", storedUser.PayRequestQuota, totalRequests-requestUnits)
	}

	MarkWssRequestRoundCommitted(relayInfo)
	if relayInfo.FinalPreConsumedPayRequests != 0 {
		t.Fatalf("FinalPreConsumedPayRequests after commit = %d, want 0", relayInfo.FinalPreConsumedPayRequests)
	}
	if relayInfo.PayRequestProductId != 0 {
		t.Fatalf("PayRequestProductId after commit = %d, want 0", relayInfo.PayRequestProductId)
	}
	if len(relayInfo.PayRequestProductAllocations) != 0 {
		t.Fatalf("PayRequestProductAllocations after commit = %#v, want empty", relayInfo.PayRequestProductAllocations)
	}

	storedBalance = reloadPreConsumeQuotaPayRequestBalance(t, db, user.Id, 99101)
	if storedBalance.RemainingRequests != totalRequests-requestUnits {
		t.Fatalf("remaining_requests after commit = %d, want %d", storedBalance.RemainingRequests, totalRequests-requestUnits)
	}

	if err := PreConsumeWssRequestRound(relayInfo); err != nil {
		t.Fatalf("PreConsumeWssRequestRound(second) error = %v", err)
	}

	ctx := newPreConsumeQuotaGinContext()
	if err := ReturnWssRequestRoundReservation(ctx, relayInfo); err != nil {
		t.Fatalf("ReturnWssRequestRoundReservation(first) error = %v", err)
	}
	if relayInfo.FinalPreConsumedPayRequests != 0 {
		t.Fatalf("FinalPreConsumedPayRequests after refund = %d, want 0", relayInfo.FinalPreConsumedPayRequests)
	}
	if relayInfo.PayRequestProductId != 0 {
		t.Fatalf("PayRequestProductId after refund = %d, want 0", relayInfo.PayRequestProductId)
	}
	if len(relayInfo.PayRequestProductAllocations) != 0 {
		t.Fatalf("PayRequestProductAllocations after refund = %#v, want empty", relayInfo.PayRequestProductAllocations)
	}

	storedBalance = reloadPreConsumeQuotaPayRequestBalance(t, db, user.Id, 99101)
	if storedBalance.RemainingRequests != totalRequests-requestUnits {
		t.Fatalf("remaining_requests after refund = %d, want %d", storedBalance.RemainingRequests, totalRequests-requestUnits)
	}
	storedUser = reloadPreConsumeQuotaUser(t, db, user.Id)
	if storedUser.PayRequestQuota != totalRequests-requestUnits {
		t.Fatalf("user pay_request_quota after refund = %d, want %d", storedUser.PayRequestQuota, totalRequests-requestUnits)
	}

	if err := ReturnWssRequestRoundReservation(ctx, relayInfo); err != nil {
		t.Fatalf("ReturnWssRequestRoundReservation(second) error = %v", err)
	}

	storedBalance = reloadPreConsumeQuotaPayRequestBalance(t, db, user.Id, 99101)
	if storedBalance.RemainingRequests != totalRequests-requestUnits {
		t.Fatalf("remaining_requests after second refund = %d, want %d", storedBalance.RemainingRequests, totalRequests-requestUnits)
	}
}

func TestRefundTaskQuotaPersistsRecoverablePartialRefundSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newPreConsumeQuotaTestDB(t)
	withTaskBillingDB(t, db)

	group := createPreConsumeQuotaTestGroup(t, db, "task-partial")
	user := createTaskBillingUser(t, db, model.User{
		Username: "task-partial-user",
		Password: "password123",
		GroupId:  group.Id,
		Group:    group.Code,
		Quota:    1000,
	})
	sub := createPreConsumeQuotaRequestSubscription(t, db, user.Id, group.Id)
	if err := db.Model(&model.UserRequestSubscription{}).Where("id = ?", sub.Id).Updates(map[string]interface{}{
		"daily_request_used": billing.DisplayIntUnitsToStored(1),
		"total_request_used": billing.DisplayIntUnitsToStored(1),
	}).Error; err != nil {
		t.Fatalf("seed request subscription usage: %v", err)
	}

	token := model.Token{
		Id:          77,
		UserId:      user.Id,
		Key:         "sk-task-partial",
		Name:        "task-partial-token",
		Group:       group.Code,
		RemainQuota: 70,
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	task := &model.Task{
		TaskID: "task_partial_refund_snapshot",
		UserId: user.Id,
		Quota:  30,
		Status: model.TaskStatusFailure,
		Group:  group.Code,
		PrivateData: model.TaskPrivateData{
			QuotaBucket:              model.UserQuotaBucketRequest,
			UsingGroupID:             group.Id,
			RequestSubscriptionID:    sub.Id,
			FinalPreConsumedQuota:    30,
			FinalPreConsumedRequests: billing.DisplayIntUnitsToStored(1),
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "test-model",
			},
		},
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	RefundTaskQuota(context.Background(), task, "first-attempt")

	reloadedTask := reloadTask(t, task.ID)
	if reloadedTask.PrivateData.RequestSubscriptionID != 0 {
		t.Fatalf("RequestSubscriptionID after partial refund = %d, want 0", reloadedTask.PrivateData.RequestSubscriptionID)
	}
	if reloadedTask.PrivateData.FinalPreConsumedRequests != 0 {
		t.Fatalf("FinalPreConsumedRequests after partial refund = %d, want 0", reloadedTask.PrivateData.FinalPreConsumedRequests)
	}
	if reloadedTask.PrivateData.FinalPreConsumedQuota != 30 {
		t.Fatalf("FinalPreConsumedQuota after partial refund = %d, want 30", reloadedTask.PrivateData.FinalPreConsumedQuota)
	}
	if reloadedTask.PrivateData.RefundAppliedAt != 0 {
		t.Fatalf("RefundAppliedAt after partial refund = %d, want 0", reloadedTask.PrivateData.RefundAppliedAt)
	}
	refundedSub := reloadPreConsumeQuotaRequestSubscription(t, db, sub.Id)
	if refundedSub.DailyRequestUsed != 0 || refundedSub.TotalRequestUsed != 0 {
		t.Fatalf("request subscription not restored by partial refund leg: daily=%d total=%d", refundedSub.DailyRequestUsed, refundedSub.TotalRequestUsed)
	}
	if got := countTaskBillingLogs(t); got != 0 {
		t.Fatalf("refund log count after partial refund = %d, want 0", got)
	}

	reloadedTask.PrivateData.TokenID = token.Id
	reloadedTask.PrivateData.TokenKey = token.Key
	if _, err := reloadedTask.UpdateWithStatus(reloadedTask.Status); err != nil {
		t.Fatalf("persist recovered token snapshot: %v", err)
	}

	RefundTaskQuota(context.Background(), &reloadedTask, "retry-after-token-restore")

	reloadedTask = reloadTask(t, task.ID)
	if reloadedTask.PrivateData.FinalPreConsumedQuota != 0 {
		t.Fatalf("FinalPreConsumedQuota after recovery = %d, want 0", reloadedTask.PrivateData.FinalPreConsumedQuota)
	}
	if reloadedTask.PrivateData.RefundAppliedAt <= 0 {
		t.Fatalf("RefundAppliedAt after recovery = %d, want > 0", reloadedTask.PrivateData.RefundAppliedAt)
	}
	refundedToken := reloadPreConsumeQuotaToken(t, db, token.Id)
	if refundedToken.RemainQuota != 100 {
		t.Fatalf("token remain_quota after recovery refund = %d, want 100", refundedToken.RemainQuota)
	}
	if got := countTaskBillingLogs(t); got != 1 {
		t.Fatalf("refund log count after recovery = %d, want 1", got)
	}
}

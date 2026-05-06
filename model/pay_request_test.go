package model

import (
	"path/filepath"
	"reflect"
	"testing"

	relaycommon "one-api/relay/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPayRequestTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "pay-request.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Group{}, &User{}, &PayRequestUserBalance{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createPayRequestBalance(t *testing.T, db *gorm.DB, userID int, productID int, sortOrder int, remainingRequests int, groupIDs []int) {
	t.Helper()

	groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
	if err != nil {
		t.Fatalf("marshal group ids: %v", err)
	}
	balance := PayRequestUserBalance{
		UserId:            userID,
		ProductId:         productID,
		ProductName:       "p",
		SortOrder:         sortOrder,
		AllowedGroupIds:   groupIDsJSON,
		RemainingRequests: remainingRequests,
		HistoryRequests:   remainingRequests,
	}
	if err := db.Create(&balance).Error; err != nil {
		t.Fatalf("create pay-request balance %d: %v", productID, err)
	}
}

func TestPreConsumeUserPayRequestQuotaWithProductReturnsSelectedProductID(t *testing.T) {
	db := newPayRequestTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")

	user := User{
		Username: "pay-request-consume",
		Password: "password123",
		GroupId:  groupX.Id,
		Group:    groupX.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPayRequestBalance(t, db, user.Id, 99101, 30, 5, []int{groupX.Id})
	createPayRequestBalance(t, db, user.Id, 99102, 20, 7, []int{groupX.Id})

	productID, err := PreConsumeUserPayRequestQuotaWithProduct(user.Id, groupX.Id, 4)
	if err != nil {
		t.Fatalf("PreConsumeUserPayRequestQuotaWithProduct() error = %v", err)
	}
	if productID != 99101 {
		t.Fatalf("PreConsumeUserPayRequestQuotaWithProduct() productID = %d, want 99101", productID)
	}

	var consumed101 PayRequestUserBalance
	if err := db.Where("user_id = ? AND product_id = ?", user.Id, 99101).First(&consumed101).Error; err != nil {
		t.Fatalf("reload consumed balance: %v", err)
	}
	if consumed101.RemainingRequests != 1 {
		t.Fatalf("consumed balance remaining_requests = %d, want 1", consumed101.RemainingRequests)
	}

	var untouched102 PayRequestUserBalance
	if err := db.Where("user_id = ? AND product_id = ?", user.Id, 99102).First(&untouched102).Error; err != nil {
		t.Fatalf("reload untouched balance: %v", err)
	}
	if untouched102.RemainingRequests != 7 {
		t.Fatalf("untouched balance remaining_requests = %d, want 7", untouched102.RemainingRequests)
	}

	var storedUser User
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after consume: %v", err)
	}
	if storedUser.PayRequestQuota != 8 {
		t.Fatalf("user pay_request_quota after consume = %d, want 8", storedUser.PayRequestQuota)
	}

	if err := ReturnUserPayRequestQuotaWithProduct(user.Id, productID, 4); err != nil {
		t.Fatalf("ReturnUserPayRequestQuotaWithProduct() error = %v", err)
	}

	if err := db.Where("user_id = ? AND product_id = ?", user.Id, 99101).First(&consumed101).Error; err != nil {
		t.Fatalf("reload restored balance: %v", err)
	}
	if consumed101.RemainingRequests != 5 {
		t.Fatalf("restored balance remaining_requests = %d, want 5", consumed101.RemainingRequests)
	}
	if err := db.Where("user_id = ? AND product_id = ?", user.Id, 99102).First(&untouched102).Error; err != nil {
		t.Fatalf("reload untouched balance after restore: %v", err)
	}
	if untouched102.RemainingRequests != 7 {
		t.Fatalf("untouched balance remaining_requests after restore = %d, want 7", untouched102.RemainingRequests)
	}

	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after restore: %v", err)
	}
	if storedUser.PayRequestQuota != 12 {
		t.Fatalf("user pay_request_quota after restore = %d, want 12", storedUser.PayRequestQuota)
	}
}

func TestConsumeAndRestoreUserPayRequestQuotaWithAllocations(t *testing.T) {
	db := newPayRequestTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	user := User{
		Username: "pay-request-allocations",
		Password: "password123",
		GroupId:  groupX.Id,
		Group:    groupX.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPayRequestBalance(t, db, user.Id, 99101, 30, 3, []int{groupX.Id})
	createPayRequestBalance(t, db, user.Id, 99102, 20, 4, []int{groupX.Id})
	createPayRequestBalance(t, db, user.Id, 99103, 10, 9, []int{groupY.Id})

	allocations, err := DecreaseUserPayRequestQuotaWithAllocations(user.Id, groupX.Id, 5)
	if err != nil {
		t.Fatalf("DecreaseUserPayRequestQuotaWithAllocations() error = %v", err)
	}
	wantAllocations := []relaycommon.ProductQuotaAllocation{
		{ProductId: 99101, Quota: 3},
		{ProductId: 99102, Quota: 2},
	}
	if !reflect.DeepEqual(allocations, wantAllocations) {
		t.Fatalf("DecreaseUserPayRequestQuotaWithAllocations() allocations = %#v, want %#v", allocations, wantAllocations)
	}

	var balances []PayRequestUserBalance
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances: %v", err)
	}
	gotRemaining := map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingRequests
	}
	wantRemaining := map[int]int{99101: 0, 99102: 2, 99103: 9}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining requests = %#v, want %#v", gotRemaining, wantRemaining)
	}

	var storedUser User
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after consume: %v", err)
	}
	if storedUser.PayRequestQuota != 11 {
		t.Fatalf("user pay_request_quota after consume = %d, want 11", storedUser.PayRequestQuota)
	}

	if err := ReturnUserPayRequestQuotaWithAllocations(user.Id, allocations); err != nil {
		t.Fatalf("ReturnUserPayRequestQuotaWithAllocations() error = %v", err)
	}

	balances = nil
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances after restore: %v", err)
	}
	gotRemaining = map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingRequests
	}
	wantRemaining = map[int]int{99101: 3, 99102: 4, 99103: 9}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining requests after restore = %#v, want %#v", gotRemaining, wantRemaining)
	}
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after restore: %v", err)
	}
	if storedUser.PayRequestQuota != 16 {
		t.Fatalf("user pay_request_quota after restore = %d, want 16", storedUser.PayRequestQuota)
	}
}

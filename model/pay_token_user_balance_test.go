package model

import (
	"path/filepath"
	"reflect"
	"testing"

	relaycommon "one-api/relay/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPayTokenUserBalanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "pay-token-user-balance.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Group{}, &User{}, &PayTokenUserBalance{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createPayTokenUserBalance(t *testing.T, db *gorm.DB, userID int, productID int, sortOrder int, remainingTokens int, groupIDs []int) {
	t.Helper()

	groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
	if err != nil {
		t.Fatalf("marshal group ids: %v", err)
	}
	balance := PayTokenUserBalance{
		UserId:          userID,
		ProductId:       productID,
		ProductName:     "p",
		SortOrder:       sortOrder,
		AllowedGroupIds: groupIDsJSON,
		RemainingTokens: remainingTokens,
		HistoryTokens:   remainingTokens,
	}
	if err := db.Create(&balance).Error; err != nil {
		t.Fatalf("create pay-token balance %d: %v", productID, err)
	}
}

func TestConsumeAndRestoreUserPayTokenQuotaWithAllocations(t *testing.T) {
	db := newPayTokenUserBalanceTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	user := User{
		Username: "pay-token-allocations",
		Password: "password123",
		GroupId:  groupX.Id,
		Group:    groupX.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPayTokenUserBalance(t, db, user.Id, 99201, 30, 300, []int{groupX.Id})
	createPayTokenUserBalance(t, db, user.Id, 99202, 20, 400, []int{groupX.Id})
	createPayTokenUserBalance(t, db, user.Id, 99203, 10, 900, []int{groupY.Id})

	allocations, err := DecreaseUserPayTokenQuotaWithAllocations(user.Id, groupX.Id, 500)
	if err != nil {
		t.Fatalf("DecreaseUserPayTokenQuotaWithAllocations() error = %v", err)
	}
	wantAllocations := []relaycommon.ProductQuotaAllocation{
		{ProductId: 99201, Quota: 300},
		{ProductId: 99202, Quota: 200},
	}
	if !reflect.DeepEqual(allocations, wantAllocations) {
		t.Fatalf("DecreaseUserPayTokenQuotaWithAllocations() allocations = %#v, want %#v", allocations, wantAllocations)
	}

	var balances []PayTokenUserBalance
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances: %v", err)
	}
	gotRemaining := map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingTokens
	}
	wantRemaining := map[int]int{99201: 0, 99202: 200, 99203: 900}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining tokens = %#v, want %#v", gotRemaining, wantRemaining)
	}

	var storedUser User
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after consume: %v", err)
	}
	if storedUser.PayTokenQuota != 1100 {
		t.Fatalf("user pay_token_quota after consume = %d, want 1100", storedUser.PayTokenQuota)
	}

	if err := ReturnUserPayTokenQuotaWithAllocations(user.Id, allocations); err != nil {
		t.Fatalf("ReturnUserPayTokenQuotaWithAllocations() error = %v", err)
	}

	balances = nil
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances after restore: %v", err)
	}
	gotRemaining = map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingTokens
	}
	wantRemaining = map[int]int{99201: 300, 99202: 400, 99203: 900}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining tokens after restore = %#v, want %#v", gotRemaining, wantRemaining)
	}
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after restore: %v", err)
	}
	if storedUser.PayTokenQuota != 1600 {
		t.Fatalf("user pay_token_quota after restore = %d, want 1600", storedUser.PayTokenQuota)
	}
}

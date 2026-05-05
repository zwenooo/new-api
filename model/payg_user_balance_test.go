package model

import (
	"path/filepath"
	"reflect"
	"testing"

	relaycommon "one-api/relay/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPaygUserBalanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "payg-user-balance.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Group{}, &User{}, &PaygUserBalance{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withModelDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
	})
}

func createTestGroup(t *testing.T, db *gorm.DB, code string) Group {
	t.Helper()
	group := Group{
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

func createPaygBalance(t *testing.T, db *gorm.DB, userID int, productID int, sortOrder int, remainingQuota int, groupIDs []int) {
	t.Helper()
	groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
	if err != nil {
		t.Fatalf("marshal group ids: %v", err)
	}
	balance := PaygUserBalance{
		UserId:                  userID,
		ProductId:               productID,
		ProductName:             "p",
		SortOrder:               sortOrder,
		AllowedGroupIds:         groupIDsJSON,
		OverrideAllowedGroupIds: true,
		RemainingQuota:          remainingQuota,
		HistoryQuota:            remainingQuota,
	}
	if err := db.Create(&balance).Error; err != nil {
		t.Fatalf("create payg balance %d: %v", productID, err)
	}
}

func TestFindUserPaygConsumableProductIdTxAggregatesAcrossMatchingBalances(t *testing.T) {
	db := newPaygUserBalanceTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	user := User{
		Username:               "payg-find",
		Password:               "password123",
		Quota:                  1900,
		PayAsYouGoQuota:        1900,
		PayAsYouGoHistoryQuota: 1900,
		GroupId:                groupX.Id,
		Group:                  groupX.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPaygBalance(t, db, user.Id, 101, 30, 600, []int{groupX.Id})
	createPaygBalance(t, db, user.Id, 102, 20, 800, []int{groupY.Id})
	createPaygBalance(t, db, user.Id, 103, 10, 500, []int{groupX.Id})

	productID, ok, err := FindUserPaygConsumableProductIdTx(db, user.Id, groupX.Id, 1000)
	if err != nil {
		t.Fatalf("FindUserPaygConsumableProductIdTx() error = %v", err)
	}
	if !ok {
		t.Fatal("FindUserPaygConsumableProductIdTx() ok = false, want true")
	}
	if productID != 101 {
		t.Fatalf("FindUserPaygConsumableProductIdTx() productID = %d, want 101", productID)
	}
}

func TestConsumeAndRestoreUserPaygQuotaWithAllocations(t *testing.T) {
	db := newPaygUserBalanceTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	user := User{
		Username:               "payg-consume",
		Password:               "password123",
		Quota:                  2300,
		PayAsYouGoQuota:        2300,
		PayAsYouGoHistoryQuota: 2300,
		GroupId:                groupX.Id,
		Group:                  groupX.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPaygBalance(t, db, user.Id, 101, 30, 600, []int{groupX.Id})
	createPaygBalance(t, db, user.Id, 102, 20, 800, []int{groupY.Id})
	createPaygBalance(t, db, user.Id, 103, 10, 900, []int{groupX.Id})

	allocations, userDelta, err := consumeUserPaygQuotaWithAllocations(user.Id, groupX.Id, 1000)
	if err != nil {
		t.Fatalf("consumeUserPaygQuotaWithAllocations() error = %v", err)
	}
	if userDelta != 1000 {
		t.Fatalf("consumeUserPaygQuotaWithAllocations() userDelta = %d, want 1000", userDelta)
	}
	wantAllocations := []relaycommon.ProductQuotaAllocation{
		{ProductId: 101, Quota: 600},
		{ProductId: 103, Quota: 400},
	}
	if !reflect.DeepEqual(allocations, wantAllocations) {
		t.Fatalf("consumeUserPaygQuotaWithAllocations() allocations = %#v, want %#v", allocations, wantAllocations)
	}

	var balances []PaygUserBalance
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances: %v", err)
	}
	gotRemaining := map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingQuota
	}
	wantRemaining := map[int]int{101: 0, 102: 800, 103: 500}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining quotas = %#v, want %#v", gotRemaining, wantRemaining)
	}

	var storedUser User
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if storedUser.PayAsYouGoQuota != 1300 {
		t.Fatalf("user payg_quota = %d, want 1300", storedUser.PayAsYouGoQuota)
	}
	if storedUser.Quota != 1300 {
		t.Fatalf("user quota = %d, want 1300", storedUser.Quota)
	}

	restored, err := restoreUserPaygQuotaWithAllocations(user.Id, allocations)
	if err != nil {
		t.Fatalf("restoreUserPaygQuotaWithAllocations() error = %v", err)
	}
	if restored != 1000 {
		t.Fatalf("restoreUserPaygQuotaWithAllocations() restored = %d, want 1000", restored)
	}

	balances = nil
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances after restore: %v", err)
	}
	gotRemaining = map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingQuota
	}
	wantRemaining = map[int]int{101: 600, 102: 800, 103: 900}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining quotas after restore = %#v, want %#v", gotRemaining, wantRemaining)
	}

	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after restore: %v", err)
	}
	if storedUser.PayAsYouGoQuota != 2300 {
		t.Fatalf("user payg_quota after restore = %d, want 2300", storedUser.PayAsYouGoQuota)
	}
	if storedUser.Quota != 2300 {
		t.Fatalf("user quota after restore = %d, want 2300", storedUser.Quota)
	}
}

func TestConsumeAndRestoreUserPaygQuotaWithAllocationsTracksDustDelta(t *testing.T) {
	db := newPaygUserBalanceTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	user := User{
		Username:               "payg-dust",
		Password:               "password123",
		Quota:                  1900,
		PayAsYouGoQuota:        1900,
		PayAsYouGoHistoryQuota: 1900,
		GroupId:                groupX.Id,
		Group:                  groupX.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPaygBalance(t, db, user.Id, 101, 30, 600, []int{groupX.Id})
	createPaygBalance(t, db, user.Id, 102, 20, 800, []int{groupY.Id})
	createPaygBalance(t, db, user.Id, 103, 10, 500, []int{groupX.Id})

	allocations, userDelta, err := consumeUserPaygQuotaWithAllocations(user.Id, groupX.Id, 1000)
	if err != nil {
		t.Fatalf("consumeUserPaygQuotaWithAllocations() error = %v", err)
	}
	if userDelta != 1100 {
		t.Fatalf("consumeUserPaygQuotaWithAllocations() userDelta = %d, want 1100", userDelta)
	}
	wantAllocations := []relaycommon.ProductQuotaAllocation{
		{ProductId: 101, Quota: 600},
		{ProductId: 103, Quota: 400},
	}
	if !reflect.DeepEqual(allocations, wantAllocations) {
		t.Fatalf("consumeUserPaygQuotaWithAllocations() allocations = %#v, want %#v", allocations, wantAllocations)
	}

	var balances []PaygUserBalance
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances: %v", err)
	}
	gotRemaining := map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingQuota
	}
	wantRemaining := map[int]int{101: 0, 102: 800, 103: 0}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining quotas = %#v, want %#v", gotRemaining, wantRemaining)
	}

	var storedUser User
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if storedUser.PayAsYouGoQuota != 800 {
		t.Fatalf("user payg_quota = %d, want 800", storedUser.PayAsYouGoQuota)
	}
	if storedUser.Quota != 800 {
		t.Fatalf("user quota = %d, want 800", storedUser.Quota)
	}

	restored, err := restoreUserPaygQuotaWithAllocations(user.Id, allocations)
	if err != nil {
		t.Fatalf("restoreUserPaygQuotaWithAllocations() error = %v", err)
	}
	if restored != 600 {
		t.Fatalf("restoreUserPaygQuotaWithAllocations() restored = %d, want 600", restored)
	}

	balances = nil
	if err := db.Order("product_id ASC").Find(&balances).Error; err != nil {
		t.Fatalf("reload balances after restore: %v", err)
	}
	gotRemaining = map[int]int{}
	for _, balance := range balances {
		gotRemaining[balance.ProductId] = balance.RemainingQuota
	}
	wantRemaining = map[int]int{101: 600, 102: 800, 103: 0}
	if !reflect.DeepEqual(gotRemaining, wantRemaining) {
		t.Fatalf("remaining quotas after restore = %#v, want %#v", gotRemaining, wantRemaining)
	}

	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after restore: %v", err)
	}
	if storedUser.PayAsYouGoQuota != 1400 {
		t.Fatalf("user payg_quota after restore = %d, want 1400", storedUser.PayAsYouGoQuota)
	}
	if storedUser.Quota != 1400 {
		t.Fatalf("user quota after restore = %d, want 1400", storedUser.Quota)
	}
}

func TestUpsertPaygUserBalanceTxRevivesZeroedProductBalanceWithCurrentGroups(t *testing.T) {
	db := newPaygUserBalanceTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "a")
	groupB := createTestGroup(t, db, "b")

	user := User{
		Username: "payg-revive",
		Password: "password123",
		Quota:    0,
		GroupId:  groupA.Id,
		Group:    groupA.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	oldGroupJSON, err := MarshalGroupIDsJSON([]int{groupA.Id})
	if err != nil {
		t.Fatalf("marshal old groups: %v", err)
	}
	existing := PaygUserBalance{
		UserId:                  user.Id,
		ProductId:               2001,
		ProductName:             "legacy-product",
		SortOrder:               1,
		AllowedGroupIds:         oldGroupJSON,
		OverrideAllowedGroupIds: true,
		RemainingQuota:          0,
		HistoryQuota:            100,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create balance: %v", err)
	}

	if err := UpsertPaygUserBalanceTx(db, user.Id, 2001, "current-product", 9, []int{groupB.Id}, 300); err != nil {
		t.Fatalf("UpsertPaygUserBalanceTx() error = %v", err)
	}

	var reloaded PaygUserBalance
	if err := db.Where("id = ?", existing.Id).First(&reloaded).Error; err != nil {
		t.Fatalf("reload balance: %v", err)
	}
	gotGroups, err := ParseGroupIDsJSON(reloaded.AllowedGroupIds)
	if err != nil {
		t.Fatalf("parse groups: %v", err)
	}
	if !reflect.DeepEqual(gotGroups, []int{groupB.Id}) {
		t.Fatalf("allowed groups = %#v, want %#v", gotGroups, []int{groupB.Id})
	}
	if reloaded.OverrideAllowedGroupIds {
		t.Fatal("override_allowed_group_ids = true, want false")
	}
	if reloaded.RemainingQuota != 300 {
		t.Fatalf("remaining_quota = %d, want 300", reloaded.RemainingQuota)
	}
	if reloaded.HistoryQuota != 400 {
		t.Fatalf("history_quota = %d, want 400", reloaded.HistoryQuota)
	}
}

func TestResetProductBackedPaygBalanceGroupsToProductTx(t *testing.T) {
	db := newPaygUserBalanceTestDB(t)
	withModelDB(t, db)

	if err := db.AutoMigrate(&PaygProductGroup{}); err != nil {
		t.Fatalf("auto migrate payg product groups: %v", err)
	}

	groupA := createTestGroup(t, db, "a")
	groupB := createTestGroup(t, db, "b")

	user := User{
		Username: "payg-reset",
		Password: "password123",
		Quota:    0,
		GroupId:  groupA.Id,
		Group:    groupA.Code,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&PaygProductGroup{ProductId: 3001, GroupId: groupB.Id}).Error; err != nil {
		t.Fatalf("create payg product group: %v", err)
	}

	oldGroupJSON, err := MarshalGroupIDsJSON([]int{groupA.Id})
	if err != nil {
		t.Fatalf("marshal old groups: %v", err)
	}
	balance := PaygUserBalance{
		UserId:                  user.Id,
		ProductId:               3001,
		ProductName:             "product",
		SortOrder:               1,
		AllowedGroupIds:         oldGroupJSON,
		OverrideAllowedGroupIds: true,
		RemainingQuota:          0,
		HistoryQuota:            100,
	}
	if err := db.Create(&balance).Error; err != nil {
		t.Fatalf("create balance: %v", err)
	}

	if err := ResetProductBackedPaygBalanceGroupsToProductTx(db, balance.Id, balance.ProductId); err != nil {
		t.Fatalf("ResetProductBackedPaygBalanceGroupsToProductTx() error = %v", err)
	}

	var reloaded PaygUserBalance
	if err := db.Where("id = ?", balance.Id).First(&reloaded).Error; err != nil {
		t.Fatalf("reload balance: %v", err)
	}
	gotGroups, err := ParseGroupIDsJSON(reloaded.AllowedGroupIds)
	if err != nil {
		t.Fatalf("parse groups: %v", err)
	}
	if !reflect.DeepEqual(gotGroups, []int{groupB.Id}) {
		t.Fatalf("allowed groups = %#v, want %#v", gotGroups, []int{groupB.Id})
	}
	if reloaded.OverrideAllowedGroupIds {
		t.Fatal("override_allowed_group_ids = true, want false")
	}
}

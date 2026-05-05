package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPayBalanceUserSnapshotTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "pay-balance-user-snapshot.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Group{},
		&User{},
		&PayRequestUserBalance{},
		&PayRequestProductGroup{},
		&PayTokenUserBalance{},
		&PayTokenProductGroup{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db
}

func createPayTokenBalance(t *testing.T, db *gorm.DB, userID int, productID int, sortOrder int, remainingTokens int, groupIDs []int) {
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

func TestBackfillUsersPayRequestSnapshotFromBalancesRebuildsUserSnapshot(t *testing.T) {
	db := newPayBalanceUserSnapshotTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	oldGroups, err := MarshalGroupIDsJSON([]int{groupX.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON(old) error = %v", err)
	}
	user := User{
		Username:                "pay-request-snapshot-user",
		Password:                "password123",
		GroupId:                 groupX.Id,
		Group:                   groupX.Code,
		PayRequestQuota:         1,
		PayRequestHistoryQuota:  1,
		PayRequestAllowedGroups: oldGroups,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPayRequestBalance(t, db, user.Id, 1101, 20, 3, []int{groupX.Id})
	createPayRequestBalance(t, db, user.Id, 1102, 10, 5, []int{groupY.Id})

	if err := BackfillUsersPayRequestSnapshotFromBalances(db); err != nil {
		t.Fatalf("BackfillUsersPayRequestSnapshotFromBalances() error = %v", err)
	}

	var reloaded User
	if err := db.Where("id = ?", user.Id).First(&reloaded).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	expectedGroups, err := MarshalGroupIDsJSON([]int{groupX.Id, groupY.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON(expected) error = %v", err)
	}
	if reloaded.PayRequestQuota != 8 {
		t.Fatalf("PayRequestQuota = %d, want 8", reloaded.PayRequestQuota)
	}
	if reloaded.PayRequestHistoryQuota != 8 {
		t.Fatalf("PayRequestHistoryQuota = %d, want 8", reloaded.PayRequestHistoryQuota)
	}
	if string(reloaded.PayRequestAllowedGroups) != string(expectedGroups) {
		t.Fatalf("PayRequestAllowedGroups = %s, want %s", reloaded.PayRequestAllowedGroups, expectedGroups)
	}
}

func TestBackfillUsersPayTokenSnapshotFromBalancesRebuildsUserSnapshot(t *testing.T) {
	db := newPayBalanceUserSnapshotTestDB(t)
	withModelDB(t, db)

	groupX := createTestGroup(t, db, "x")
	groupY := createTestGroup(t, db, "y")

	oldGroups, err := MarshalGroupIDsJSON([]int{groupX.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON(old) error = %v", err)
	}
	user := User{
		Username:              "pay-token-snapshot-user",
		Password:              "password123",
		GroupId:               groupX.Id,
		Group:                 groupX.Code,
		PayTokenQuota:         2,
		PayTokenHistoryQuota:  2,
		PayTokenAllowedGroups: oldGroups,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createPayTokenBalance(t, db, user.Id, 2101, 20, 3000, []int{groupX.Id})
	createPayTokenBalance(t, db, user.Id, 2102, 10, 5000, []int{groupY.Id})

	if err := BackfillUsersPayTokenSnapshotFromBalances(db); err != nil {
		t.Fatalf("BackfillUsersPayTokenSnapshotFromBalances() error = %v", err)
	}

	var reloaded User
	if err := db.Where("id = ?", user.Id).First(&reloaded).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	expectedGroups, err := MarshalGroupIDsJSON([]int{groupX.Id, groupY.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON(expected) error = %v", err)
	}
	if reloaded.PayTokenQuota != 8000 {
		t.Fatalf("PayTokenQuota = %d, want 8000", reloaded.PayTokenQuota)
	}
	if reloaded.PayTokenHistoryQuota != 8000 {
		t.Fatalf("PayTokenHistoryQuota = %d, want 8000", reloaded.PayTokenHistoryQuota)
	}
	if string(reloaded.PayTokenAllowedGroups) != string(expectedGroups) {
		t.Fatalf("PayTokenAllowedGroups = %s, want %s", reloaded.PayTokenAllowedGroups, expectedGroups)
	}
}

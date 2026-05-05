package model

import (
	"path/filepath"
	"testing"

	"one-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newUserGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "user-group.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Group{}, &UserGroup{}, &User{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestBackfillUserGroupsFromModelGroupsTxAssignsLegacyUsers(t *testing.T) {
	db := newUserGroupTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "group-a")
	groupB := createTestGroup(t, db, "group-b")

	users := []User{
		{
			Username: "legacy-user-a",
			Password: "password123",
			GroupId:  groupA.Id,
			Group:    groupA.Code,
			AffCode:  "uga1",
			Status:   common.UserStatusEnabled,
		},
		{
			Username: "legacy-user-b",
			Password: "password123",
			GroupId:  groupB.Id,
			Group:    groupB.Code,
			AffCode:  "ugb1",
			Status:   common.UserStatusEnabled,
		},
	}
	for _, user := range users {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create legacy user %s: %v", user.Username, err)
		}
	}

	if err := BackfillUserGroupsFromModelGroupsTx(db); err != nil {
		t.Fatalf("BackfillUserGroupsFromModelGroupsTx() error = %v", err)
	}

	var userGroups []UserGroup
	if err := db.Order("id ASC").Find(&userGroups).Error; err != nil {
		t.Fatalf("load user groups: %v", err)
	}
	if len(userGroups) != 2 {
		t.Fatalf("len(userGroups) = %d, want 2", len(userGroups))
	}

	var loadedUsers []User
	if err := db.Order("id ASC").Find(&loadedUsers).Error; err != nil {
		t.Fatalf("load users: %v", err)
	}
	for _, user := range loadedUsers {
		if user.UserGroupId <= 0 {
			t.Fatalf("user %s user_group_id = %d, want > 0", user.Username, user.UserGroupId)
		}
	}
}

func TestUserInsertInfersUserGroupFromModelGroup(t *testing.T) {
	db := newUserGroupTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "group-a")

	user := &User{
		Username:     "new-user-group",
		Password:     "password123",
		DisplayName:  "new-user-group",
		GroupId:      groupA.Id,
		Group:        groupA.Code,
		CustomerType: CustomerTypeReseller,
		Status:       common.UserStatusEnabled,
	}
	if err := user.Insert(0); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	loaded, err := GetUserById(user.Id, true)
	if err != nil {
		t.Fatalf("GetUserById() error = %v", err)
	}
	if loaded.UserGroupId <= 0 {
		t.Fatalf("loaded.UserGroupId = %d, want > 0", loaded.UserGroupId)
	}

	userGroup, err := GetUserGroupByID(nil, loaded.UserGroupId)
	if err != nil {
		t.Fatalf("GetUserGroupByID() error = %v", err)
	}
	if userGroup.SourceModelGroupId != groupA.Id {
		t.Fatalf("userGroup.SourceModelGroupId = %d, want %d", userGroup.SourceModelGroupId, groupA.Id)
	}
}

func TestUserInsertWithoutModelGroupUsesDefaultUserGroup(t *testing.T) {
	db := newUserGroupTestDB(t)
	withModelDB(t, db)

	user := &User{
		Username:    "new-user-default-user-group",
		Password:    "password123",
		DisplayName: "new-user-default-user-group",
		Status:      common.UserStatusEnabled,
	}
	if err := user.Insert(0); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	loaded, err := GetUserById(user.Id, true)
	if err != nil {
		t.Fatalf("GetUserById() error = %v", err)
	}
	if loaded.GroupId != 0 {
		t.Fatalf("loaded.GroupId = %d, want 0", loaded.GroupId)
	}
	if loaded.UserGroupId <= 0 {
		t.Fatalf("loaded.UserGroupId = %d, want > 0", loaded.UserGroupId)
	}

	userGroup, err := GetUserGroupByID(nil, loaded.UserGroupId)
	if err != nil {
		t.Fatalf("GetUserGroupByID() error = %v", err)
	}
	if userGroup.SourceModelGroupId != 0 {
		t.Fatalf("default user group SourceModelGroupId = %d, want 0", userGroup.SourceModelGroupId)
	}
	if userGroup.Code != defaultUserGroupCode {
		t.Fatalf("default user group code = %q, want %q", userGroup.Code, defaultUserGroupCode)
	}
}

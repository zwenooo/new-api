package model

import (
	"path/filepath"
	"strings"
	"testing"

	"one-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newGroupChannelBindingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "group-channel-binding.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Group{},
		&Channel{},
		&ChannelGroup{},
		&ChannelBackupGroup{},
		&Ability{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createGroupBindingTestChannel(t *testing.T, db *gorm.DB, id int, name string, models string, groupIDs []int, backupGroupIDs []int) *Channel {
	t.Helper()

	priority := int64(100)
	channel := &Channel{
		Id:       id,
		Name:     name,
		Key:      "sk-test-" + name,
		Status:   common.ChannelStatusEnabled,
		Models:   models,
		Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel %d: %v", id, err)
	}
	if err := upsertChannelGroupsTx(db, channel.Id, groupIDs); err != nil {
		t.Fatalf("bind channel %d groups: %v", id, err)
	}
	if err := upsertChannelBackupGroupsTx(db, channel.Id, backupGroupIDs); err != nil {
		t.Fatalf("bind channel %d backup groups: %v", id, err)
	}
	if err := channel.UpdateAbilities(db); err != nil {
		t.Fatalf("update channel %d abilities: %v", id, err)
	}
	return channel
}

func countGroupChannelAbility(t *testing.T, db *gorm.DB, channelID int, groupID int, modelName string) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&Ability{}).
		Where("channel_id = ? AND group_id = ? AND model = ?", channelID, groupID, modelName).
		Count(&count).Error; err != nil {
		t.Fatalf("count abilities: %v", err)
	}
	return count
}

func TestSyncGroupChannelBindingsRejectsRemovingLastPrimaryGroup(t *testing.T) {
	db := newGroupChannelBindingTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "group-a")
	groupB := createTestGroup(t, db, "group-b")

	channelOnlyA := createGroupBindingTestChannel(t, db, 101, "only-a", "gpt-4o", []int{groupA.Id}, nil)
	channelAB := createGroupBindingTestChannel(t, db, 102, "a-b", "gpt-4.1", []int{groupA.Id, groupB.Id}, nil)

	if _, err := SyncGroupChannelBindings(nil, groupA.Id, []int{channelAB.Id}); err == nil {
		t.Fatal("SyncGroupChannelBindings() error = nil, want blocked removal error")
	} else if !strings.Contains(err.Error(), "失去所有主分组") {
		t.Fatalf("SyncGroupChannelBindings() error = %v, want last-primary-group message", err)
	}

	gotOnlyA, err := GetChannelGroupIDs(channelOnlyA.Id)
	if err != nil {
		t.Fatalf("GetChannelGroupIDs(channelOnlyA) error = %v", err)
	}
	if !equalSortedIDs(gotOnlyA, []int{groupA.Id}) {
		t.Fatalf("GetChannelGroupIDs(channelOnlyA) = %v, want [%d]", gotOnlyA, groupA.Id)
	}

	gotAB, err := GetChannelGroupIDs(channelAB.Id)
	if err != nil {
		t.Fatalf("GetChannelGroupIDs(channelAB) error = %v", err)
	}
	if !equalSortedIDs(gotAB, []int{groupA.Id, groupB.Id}) {
		t.Fatalf("GetChannelGroupIDs(channelAB) = %v, want [%d %d]", gotAB, groupA.Id, groupB.Id)
	}
}

func TestSyncGroupChannelBindingsPreservesOtherGroupsAndAbilities(t *testing.T) {
	db := newGroupChannelBindingTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "group-a")
	groupB := createTestGroup(t, db, "group-b")
	groupC := createTestGroup(t, db, "group-c")

	channelAB := createGroupBindingTestChannel(t, db, 201, "channel-ab", "gpt-4o", []int{groupA.Id, groupB.Id}, []int{groupC.Id})
	channelB := createGroupBindingTestChannel(t, db, 202, "channel-b", "gpt-4.1", []int{groupB.Id}, []int{groupA.Id})

	affected, err := SyncGroupChannelBindings(nil, groupA.Id, []int{channelB.Id})
	if err != nil {
		t.Fatalf("SyncGroupChannelBindings() error = %v", err)
	}
	if affected != 2 {
		t.Fatalf("SyncGroupChannelBindings() affected = %d, want 2", affected)
	}

	gotAB, err := GetChannelGroupIDs(channelAB.Id)
	if err != nil {
		t.Fatalf("GetChannelGroupIDs(channelAB) error = %v", err)
	}
	if !equalSortedIDs(gotAB, []int{groupB.Id}) {
		t.Fatalf("GetChannelGroupIDs(channelAB) = %v, want [%d]", gotAB, groupB.Id)
	}

	gotABBackup, err := GetChannelBackupGroupIDs(channelAB.Id)
	if err != nil {
		t.Fatalf("GetChannelBackupGroupIDs(channelAB) error = %v", err)
	}
	if !equalSortedIDs(gotABBackup, []int{groupC.Id}) {
		t.Fatalf("GetChannelBackupGroupIDs(channelAB) = %v, want [%d]", gotABBackup, groupC.Id)
	}

	gotB, err := GetChannelGroupIDs(channelB.Id)
	if err != nil {
		t.Fatalf("GetChannelGroupIDs(channelB) error = %v", err)
	}
	if !equalSortedIDs(gotB, []int{groupA.Id, groupB.Id}) {
		t.Fatalf("GetChannelGroupIDs(channelB) = %v, want [%d %d]", gotB, groupA.Id, groupB.Id)
	}

	gotBBackup, err := GetChannelBackupGroupIDs(channelB.Id)
	if err != nil {
		t.Fatalf("GetChannelBackupGroupIDs(channelB) error = %v", err)
	}
	if len(gotBBackup) != 0 {
		t.Fatalf("GetChannelBackupGroupIDs(channelB) = %v, want empty after overlap cleanup", gotBBackup)
	}

	if count := countGroupChannelAbility(t, db, channelAB.Id, groupA.Id, "gpt-4o"); count != 0 {
		t.Fatalf("ability count for channelAB/groupA = %d, want 0", count)
	}
	if count := countGroupChannelAbility(t, db, channelAB.Id, groupB.Id, "gpt-4o"); count != 1 {
		t.Fatalf("ability count for channelAB/groupB = %d, want 1", count)
	}
	if count := countGroupChannelAbility(t, db, channelB.Id, groupA.Id, "gpt-4.1"); count != 1 {
		t.Fatalf("ability count for channelB/groupA = %d, want 1", count)
	}

	items, err := ListGroupChannelBindings(nil, groupA.Id)
	if err != nil {
		t.Fatalf("ListGroupChannelBindings() error = %v", err)
	}
	selectedByChannel := make(map[int]bool, len(items))
	for _, item := range items {
		selectedByChannel[item.Id] = item.Selected
	}
	if selectedByChannel[channelAB.Id] {
		t.Fatal("ListGroupChannelBindings() channelAB selected = true, want false")
	}
	if !selectedByChannel[channelB.Id] {
		t.Fatal("ListGroupChannelBindings() channelB selected = false, want true")
	}
}

package model

import (
	"path/filepath"
	"reflect"
	"testing"

	"one-api/common"
	"one-api/dto"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newChannelCacheRevisionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "channel-cache-revision.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Option{},
		&Channel{},
		&ChannelGroup{},
		&ChannelBackupGroup{},
		&ChannelUserBinding{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withChannelCacheSnapshot(t *testing.T) {
	t.Helper()

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroupID2Model2Channels := groupID2model2channels
	oldChannelsIDM := channelsIDM
	oldChannelID2GroupIDs := channelID2groupIDs
	oldChannelID2BackupGroupIDs := channelID2backupGroupIDs
	oldBoundUsers := channelBoundUsersCache
	oldUserBoundChannels := userBoundChannelsCache
	oldBindingReady := channelBindingCacheReady
	oldBindingErr := channelBindingCacheInitErr

	channelCacheRevisionMu.Lock()
	oldLastSyncedRevision := channelCacheLastSyncedRevision
	oldLastSyncedRevisionT := channelCacheLastSyncedRevisionT
	channelCacheRevisionMu.Unlock()

	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled

		channelSyncLock.Lock()
		groupID2model2channels = oldGroupID2Model2Channels
		channelsIDM = oldChannelsIDM
		channelID2groupIDs = oldChannelID2GroupIDs
		channelID2backupGroupIDs = oldChannelID2BackupGroupIDs
		channelSyncLock.Unlock()

		channelBindingLock.Lock()
		channelBoundUsersCache = oldBoundUsers
		userBoundChannelsCache = oldUserBoundChannels
		channelBindingCacheReady = oldBindingReady
		channelBindingCacheInitErr = oldBindingErr
		channelBindingLock.Unlock()

		channelCacheRevisionMu.Lock()
		channelCacheLastSyncedRevision = oldLastSyncedRevision
		channelCacheLastSyncedRevisionT = oldLastSyncedRevisionT
		channelCacheRevisionMu.Unlock()
	})
}

func seedChannelCacheRevisionOptionMap() {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()

	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[channelCacheRevisionOptionKey] = ""
}

func mustCreateChannelCacheTestChannel(t *testing.T, db *gorm.DB, id int, status int, models string) {
	t.Helper()

	priority := int64(100)
	channel := Channel{
		Id:       id,
		Name:     "channel-cache-test",
		Key:      "sk-test",
		Status:   status,
		Models:   models,
		Priority: &priority,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel %d: %v", id, err)
	}
}

func mustCreateCompatChannelCacheTestChannel(t *testing.T, db *gorm.DB, id int, status int, models string) {
	t.Helper()

	priority := int64(100)
	channel := Channel{
		Id:       id,
		Name:     "channel-cache-compat-test",
		Key:      "sk-compat-test",
		Status:   status,
		Models:   models,
		Priority: &priority,
	}
	channel.SetSetting(dto.ChannelSettings{MessagesToResponsesCompat: true})
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create compat channel %d: %v", id, err)
	}
}

func mustBindChannelToGroup(t *testing.T, db *gorm.DB, channelID int, groupID int) {
	t.Helper()

	if err := db.Create(&ChannelGroup{ChannelId: channelID, GroupId: groupID}).Error; err != nil {
		t.Fatalf("bind channel %d to group %d: %v", channelID, groupID, err)
	}
}

func mustReadOptionValue(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()

	var option Option
	if err := db.Where("key = ?", key).First(&option).Error; err != nil {
		t.Fatalf("read option %s: %v", key, err)
	}
	return option.Value
}

func TestBumpChannelCacheRevisionMarksSyncedRevisionAfterInit(t *testing.T) {
	db := newChannelCacheRevisionTestDB(t)
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	withChannelCacheSnapshot(t)

	common.MemoryCacheEnabled = true
	seedChannelCacheRevisionOptionMap()

	mustCreateChannelCacheTestChannel(t, db, 101, common.ChannelStatusEnabled, "gpt-4o")
	mustBindChannelToGroup(t, db, 101, 7)

	BumpChannelCacheRevision()
	revision := readChannelCacheRevisionFromOptionMap()
	if revision == "" {
		t.Fatal("readChannelCacheRevisionFromOptionMap() = empty, want persisted revision")
	}
	if stored := mustReadOptionValue(t, db, channelCacheRevisionOptionKey); stored != revision {
		t.Fatalf("stored revision = %q, want %q", stored, revision)
	}

	InitChannelCache()

	if got := getChannelCacheLastSyncedRevision(); got != revision {
		t.Fatalf("getChannelCacheLastSyncedRevision() = %q, want %q", got, revision)
	}
	if got, err := getEnabledChannelIDsByGroup(7); err != nil {
		t.Fatalf("getEnabledChannelIDsByGroup() error = %v", err)
	} else if !reflect.DeepEqual(got, []int{101}) {
		t.Fatalf("getEnabledChannelIDsByGroup() = %v, want [101]", got)
	}
}

func TestInitChannelCacheRebuildRefreshesGroupBindingsAfterRevisionBump(t *testing.T) {
	db := newChannelCacheRevisionTestDB(t)
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	withChannelCacheSnapshot(t)

	common.MemoryCacheEnabled = true
	seedChannelCacheRevisionOptionMap()

	mustCreateChannelCacheTestChannel(t, db, 201, common.ChannelStatusEnabled, "claude-sonnet")
	mustBindChannelToGroup(t, db, 201, 11)

	BumpChannelCacheRevision()
	firstRevision := readChannelCacheRevisionFromOptionMap()
	InitChannelCache()

	if got, err := getEnabledChannelIDsByGroup(11); err != nil {
		t.Fatalf("getEnabledChannelIDsByGroup(11) error = %v", err)
	} else if !reflect.DeepEqual(got, []int{201}) {
		t.Fatalf("getEnabledChannelIDsByGroup(11) = %v, want [201]", got)
	}

	if err := db.Where("channel_id = ?", 201).Delete(&ChannelGroup{}).Error; err != nil {
		t.Fatalf("delete old channel group binding: %v", err)
	}
	mustBindChannelToGroup(t, db, 201, 12)

	BumpChannelCacheRevision()
	secondRevision := readChannelCacheRevisionFromOptionMap()
	if secondRevision == "" || secondRevision == firstRevision {
		t.Fatalf("second revision = %q, want non-empty and different from %q", secondRevision, firstRevision)
	}

	InitChannelCache()

	if got := getChannelCacheLastSyncedRevision(); got != secondRevision {
		t.Fatalf("getChannelCacheLastSyncedRevision() after rebuild = %q, want %q", got, secondRevision)
	}
	if got, err := getEnabledChannelIDsByGroup(11); err != nil {
		t.Fatalf("getEnabledChannelIDsByGroup(11) after rebuild error = %v", err)
	} else if len(got) != 0 {
		t.Fatalf("getEnabledChannelIDsByGroup(11) after rebuild = %v, want empty", got)
	}
	if got, err := getEnabledChannelIDsByGroup(12); err != nil {
		t.Fatalf("getEnabledChannelIDsByGroup(12) after rebuild error = %v", err)
	} else if !reflect.DeepEqual(got, []int{201}) {
		t.Fatalf("getEnabledChannelIDsByGroup(12) after rebuild = %v, want [201]", got)
	}
}

func TestInitChannelCacheRebuildRefreshesCompatChannelBindingsAfterRevisionBump(t *testing.T) {
	db := newChannelCacheRevisionTestDB(t)
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	withChannelCacheSnapshot(t)

	common.MemoryCacheEnabled = true
	seedChannelCacheRevisionOptionMap()

	mustCreateCompatChannelCacheTestChannel(t, db, 301, common.ChannelStatusEnabled, "claude-3-5-sonnet")
	mustBindChannelToGroup(t, db, 301, 21)

	BumpChannelCacheRevision()
	InitChannelCache()

	ok, err := GroupHasMessagesToResponsesCompatChannel(21, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("GroupHasMessagesToResponsesCompatChannel(21) error = %v", err)
	}
	if !ok {
		t.Fatal("GroupHasMessagesToResponsesCompatChannel(21) = false, want true")
	}

	if err := db.Where("channel_id = ?", 301).Delete(&ChannelGroup{}).Error; err != nil {
		t.Fatalf("delete compat channel group binding: %v", err)
	}
	mustBindChannelToGroup(t, db, 301, 22)

	BumpChannelCacheRevision()
	InitChannelCache()

	ok, err = GroupHasMessagesToResponsesCompatChannel(21, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("GroupHasMessagesToResponsesCompatChannel(21) after rebuild error = %v", err)
	}
	if ok {
		t.Fatal("GroupHasMessagesToResponsesCompatChannel(21) = true, want false after revision refresh")
	}
	ok, err = GroupHasMessagesToResponsesCompatChannel(22, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("GroupHasMessagesToResponsesCompatChannel(22) after rebuild error = %v", err)
	}
	if !ok {
		t.Fatal("GroupHasMessagesToResponsesCompatChannel(22) = false, want true after revision refresh")
	}
}

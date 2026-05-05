package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"one-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newGroupCleanupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "group-cleanup.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Option{},
		&Group{},
		&Channel{},
		&ChannelGroup{},
		&ChannelBackupGroup{},
		&Ability{},
		&Token{},
		&TokenAllowedGroup{},
		&User{},
		&PaygProduct{},
		&PaygProductGroup{},
		&PaygUserBalance{},
		&RedemptionPreset{},
		&SubscriptionProductGroup{},
		&SubscriptionProductGroupDailyLimit{},
		&UserSubscription{},
		&UserSubscriptionGroup{},
		&UserSubscriptionGroupDailyLimit{},
		&PricingProfileGroupFactor{},
		&UserGroupPriceOverride{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withGroupCleanupEnv(t *testing.T, db *gorm.DB) {
	t.Helper()
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	withChannelCacheSnapshot(t)
	initCol()

	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})
}

func createTestTokenWithGroups(t *testing.T, db *gorm.DB, id int, userID int, key string, groupIDs []int) {
	t.Helper()

	token := &Token{
		Id:           id,
		UserId:       userID,
		Key:          key,
		Name:         key,
		Status:       common.TokenStatusEnabled,
		CreatedTime:  common.GetTimestamp(),
		AccessedTime: common.GetTimestamp(),
		ExpiredTime:  -1,
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("create token %d: %v", id, err)
	}
	if err := upsertTokenAllowedGroupsTx(db, id, groupIDs); err != nil {
		t.Fatalf("upsert token groups %d: %v", id, err)
	}
}

func mustSetOption(t *testing.T, db *gorm.DB, key string, value string) {
	t.Helper()
	if err := db.Save(&Option{Key: key, Value: value}).Error; err != nil {
		t.Fatalf("save option %s: %v", key, err)
	}
}

func mustReadOption(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()
	var option Option
	if err := db.Where("key = ?", key).First(&option).Error; err != nil {
		t.Fatalf("read option %s: %v", key, err)
	}
	return option.Value
}

func TestSoftDeleteGroupByIDCleansCoreDependencies(t *testing.T) {
	db := newGroupCleanupTestDB(t)
	withGroupCleanupEnv(t, db)

	groupA := createTestGroup(t, db, legacySubscriptionDefaultGroup)
	groupB := createTestGroup(t, db, "group-b")

	createGroupBindingTestChannel(t, db, 101, "only-a", "gpt-4o", []int{groupA.Id}, nil)
	createGroupBindingTestChannel(t, db, 102, "a-b", "gpt-4.1", []int{groupA.Id, groupB.Id}, nil)

	user := User{
		Username:               "cleanup-user",
		Password:               "password123",
		Role:                   common.RoleCommonUser,
		Status:                 common.UserStatusEnabled,
		GroupId:                groupA.Id,
		Group:                  groupA.Code,
		Quota:                  50,
		PayAsYouGoQuota:        50,
		PayAsYouGoHistoryQuota: 50,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createTestTokenWithGroups(t, db, 201, user.Id, "sk-cleanup-a", []int{groupA.Id})
	createTestTokenWithGroups(t, db, 202, user.Id, "sk-cleanup-ab", []int{groupA.Id, groupB.Id})

	preset := &RedemptionPreset{
		Id:          301,
		Name:        "preset-a",
		Description: "preset",
		Mode:        "subscription",
		Enabled:     true,
		Archived:    false,
		CreatedTime: common.GetTimestamp(),
		UpdatedTime: common.GetTimestamp(),
	}
	if err := db.Create(preset).Error; err != nil {
		t.Fatalf("create preset: %v", err)
	}
	if err := upsertSubscriptionProductGroupsTx(db, preset.Id, []int{groupA.Id}); err != nil {
		t.Fatalf("upsert preset groups: %v", err)
	}

	groupIDsJSON, err := MarshalGroupIDsJSON([]int{groupA.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON() error = %v", err)
	}
	if err := db.Create(&PaygUserBalance{
		UserId:                  user.Id,
		ProductId:               -1,
		ProductName:             "legacy-payg",
		AllowedGroupIds:         groupIDsJSON,
		OverrideAllowedGroupIds: true,
		RemainingQuota:          50,
		HistoryQuota:            50,
	}).Error; err != nil {
		t.Fatalf("create payg balance: %v", err)
	}

	subscription := &UserSubscription{
		Id:             401,
		UserId:         user.Id,
		RemainingQuota: 20,
		TotalQuota:     20,
		Credited:       true,
	}
	if err := db.Create(subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if err := upsertUserSubscriptionGroupsTx(db, subscription.Id, []int{groupA.Id}); err != nil {
		t.Fatalf("upsert user subscription groups: %v", err)
	}

	if err := db.Create(&PricingProfileGroupFactor{
		ProfileId: 1,
		GroupId:   groupA.Id,
		Factor:    1.2,
	}).Error; err != nil {
		t.Fatalf("create pricing profile group factor: %v", err)
	}
	if err := db.Create(&UserGroupPriceOverride{
		UserId:  user.Id,
		GroupId: groupA.Id,
		Factor:  1.5,
	}).Error; err != nil {
		t.Fatalf("create user group override: %v", err)
	}

	autoGroupsJSON, err := MarshalGroupIDsJSONKeepOrder([]int{groupA.Id, groupB.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSONKeepOrder(AutoGroups) error = %v", err)
	}
	mustSetOption(t, db, "AutoGroups", string(autoGroupsJSON))
	mustSetOption(t, db, "ModelRequestRateLimitGroup", fmt.Sprintf(`{"%d":[10,5],"%d":[20,10]}`, groupA.Id, groupB.Id))
	mustSetOption(t, db, "cx_pool.cx_group_ids", string(autoGroupsJSON))

	summary, err := SoftDeleteGroupByID(nil, groupA.Id)
	if err != nil {
		t.Fatalf("SoftDeleteGroupByID() error = %v", err)
	}
	if summary.ArchivedGroupId != groupA.Id {
		t.Fatalf("ArchivedGroupId = %d, want %d", summary.ArchivedGroupId, groupA.Id)
	}

	groups, err := ListGroups(nil)
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Id != groupB.Id {
		t.Fatalf("ListGroups() = %#v, want only groupB", groups)
	}
	if err := ValidateGroupIDsExist(nil, []int{groupA.Id}); err == nil {
		t.Fatal("ValidateGroupIDsExist(groupA) error = nil, want missing group")
	}

	var archived Group
	if err := db.Unscoped().Where("id = ?", groupA.Id).First(&archived).Error; err != nil {
		t.Fatalf("read archived group: %v", err)
	}
	if !archived.Archived || archived.Enabled || archived.UserSelectable {
		t.Fatalf("archived group flags = %#v, want archived=true enabled=false user_selectable=false", archived)
	}

	channelA, err := GetChannelById(101, false)
	if err != nil {
		t.Fatalf("GetChannelById(101) error = %v", err)
	}
	if channelA.Status != common.ChannelStatusManuallyDisabled {
		t.Fatalf("channel 101 status = %d, want manually disabled", channelA.Status)
	}
	groupIDs, err := GetChannelGroupIDs(101)
	if err != nil {
		t.Fatalf("GetChannelGroupIDs(101) error = %v", err)
	}
	if len(groupIDs) != 0 {
		t.Fatalf("GetChannelGroupIDs(101) = %v, want empty", groupIDs)
	}
	groupIDs, err = GetChannelGroupIDs(102)
	if err != nil {
		t.Fatalf("GetChannelGroupIDs(102) error = %v", err)
	}
	if !equalSortedIDs(groupIDs, []int{groupB.Id}) {
		t.Fatalf("GetChannelGroupIDs(102) = %v, want [%d]", groupIDs, groupB.Id)
	}

	if err := db.Where("id = ?", 201).First(&Token{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("token 201 should be soft deleted, got err=%v", err)
	}
	tokenAB, err := GetTokenById(202)
	if err != nil {
		t.Fatalf("GetTokenById(202) error = %v", err)
	}
	tokenGroupIDs, err := ParseGroupIDsJSONKeepOrder(tokenAB.AllowedGroupIds)
	if err != nil {
		t.Fatalf("ParseGroupIDsJSONKeepOrder(tokenAB) error = %v", err)
	}
	if !equalOrderedPositiveIDs(tokenGroupIDs, []int{groupB.Id}) {
		t.Fatalf("tokenAB groups = %v, want [%d]", tokenGroupIDs, groupB.Id)
	}

	var updatedUser User
	if err := db.Where("id = ?", user.Id).First(&updatedUser).Error; err != nil {
		t.Fatalf("read user after cleanup: %v", err)
	}
	if updatedUser.GroupId != groupB.Id {
		t.Fatalf("user.GroupId = %d, want %d", updatedUser.GroupId, groupB.Id)
	}
	if updatedUser.PayAsYouGoQuota != 0 {
		t.Fatalf("user.PayAsYouGoQuota = %d, want 0", updatedUser.PayAsYouGoQuota)
	}

	var updatedBalance PaygUserBalance
	if err := db.Where("user_id = ?", user.Id).First(&updatedBalance).Error; err != nil {
		t.Fatalf("read payg balance after cleanup: %v", err)
	}
	if updatedBalance.RemainingQuota != 0 {
		t.Fatalf("payg balance remaining_quota = %d, want 0", updatedBalance.RemainingQuota)
	}

	var updatedPreset RedemptionPreset
	if err := db.Where("id = ?", preset.Id).First(&updatedPreset).Error; err != nil {
		t.Fatalf("read preset after cleanup: %v", err)
	}
	if !updatedPreset.Archived || updatedPreset.Enabled {
		t.Fatalf("preset flags = %#v, want archived=true enabled=false", updatedPreset)
	}

	var updatedSub UserSubscription
	if err := db.Where("id = ?", subscription.Id).First(&updatedSub).Error; err != nil {
		t.Fatalf("read subscription after cleanup: %v", err)
	}
	if updatedSub.InvalidAt == 0 {
		t.Fatal("subscription.InvalidAt = 0, want invalidated")
	}

	var pricingCount int64
	if err := db.Model(&PricingProfileGroupFactor{}).Where("group_id = ?", groupA.Id).Count(&pricingCount).Error; err != nil {
		t.Fatalf("count pricing profile group factors: %v", err)
	}
	if pricingCount != 0 {
		t.Fatalf("pricing profile group factors count = %d, want 0", pricingCount)
	}

	autoGroupsRaw := mustReadOption(t, db, "AutoGroups")
	wantAutoGroups, err := MarshalGroupIDsJSONKeepOrder([]int{groupB.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSONKeepOrder(autoGroups) error = %v", err)
	}
	if autoGroupsRaw != string(wantAutoGroups) {
		t.Fatalf("AutoGroups = %s, want %s", autoGroupsRaw, string(wantAutoGroups))
	}
	rateLimitRaw := mustReadOption(t, db, "ModelRequestRateLimitGroup")
	if strings.Contains(rateLimitRaw, fmt.Sprintf(`"%d"`, groupA.Id)) {
		t.Fatalf("ModelRequestRateLimitGroup still contains deleted group: %s", rateLimitRaw)
	}
	cxPoolGroupsRaw := mustReadOption(t, db, "cx_pool.cx_group_ids")
	wantCxPool, err := MarshalGroupIDsJSONKeepOrder([]int{groupB.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSONKeepOrder(cx_pool) error = %v", err)
	}
	if cxPoolGroupsRaw != string(wantCxPool) {
		t.Fatalf("cx_pool.cx_group_ids = %s, want %s", cxPoolGroupsRaw, string(wantCxPool))
	}
}

func TestBulkRemapTokenAllowedGroupsPreservesOrderAndDedupes(t *testing.T) {
	db := newGroupCleanupTestDB(t)
	withGroupCleanupEnv(t, db)

	groupA := createTestGroup(t, db, "group-a")
	groupB := createTestGroup(t, db, "group-b")
	groupC := createTestGroup(t, db, "group-c")

	user := User{
		Username: "token-remap-user",
		Password: "password123",
		GroupId:  groupA.Id,
		Group:    groupA.Code,
		Status:   common.UserStatusEnabled,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	createTestTokenWithGroups(t, db, 501, user.Id, "sk-remap-a", []int{groupA.Id})
	createTestTokenWithGroups(t, db, 502, user.Id, "sk-remap-cab", []int{groupC.Id, groupA.Id, groupB.Id})
	createTestTokenWithGroups(t, db, 503, user.Id, "sk-remap-ba", []int{groupB.Id, groupA.Id})

	summary, err := BulkRemapTokenAllowedGroups(nil, groupA.Id, groupB.Id)
	if err != nil {
		t.Fatalf("BulkRemapTokenAllowedGroups() error = %v", err)
	}
	if summary.UpdatedTokens != 3 {
		t.Fatalf("UpdatedTokens = %d, want 3", summary.UpdatedTokens)
	}

	assertTokenGroups := func(tokenID int, want []int) {
		t.Helper()
		token, err := GetTokenById(tokenID)
		if err != nil {
			t.Fatalf("GetTokenById(%d) error = %v", tokenID, err)
		}
		groupIDs, err := ParseGroupIDsJSONKeepOrder(token.AllowedGroupIds)
		if err != nil {
			t.Fatalf("ParseGroupIDsJSONKeepOrder(token#%d) error = %v", tokenID, err)
		}
		if !equalOrderedPositiveIDs(groupIDs, want) {
			t.Fatalf("token#%d groups = %v, want %v", tokenID, groupIDs, want)
		}
	}

	assertTokenGroups(501, []int{groupB.Id})
	assertTokenGroups(502, []int{groupC.Id, groupB.Id})
	assertTokenGroups(503, []int{groupB.Id})
}

func TestSoftDeletePaygGroupAllowed(t *testing.T) {
	db := newGroupCleanupTestDB(t)
	withGroupCleanupEnv(t, db)

	paygGroup := createTestGroup(t, db, "payg")
	defaultGroup := createTestGroup(t, db, "default")

	summary, err := SoftDeleteGroupByID(nil, paygGroup.Id)
	if err != nil {
		t.Fatalf("SoftDeleteGroupByID(payg) error = %v", err)
	}
	if summary.ArchivedGroupId != paygGroup.Id {
		t.Fatalf("ArchivedGroupId = %d, want %d", summary.ArchivedGroupId, paygGroup.Id)
	}

	groups, err := ListGroups(nil)
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Id != defaultGroup.Id {
		t.Fatalf("ListGroups() after payg delete = %#v, want only default group", groups)
	}

	paygFallbackCode, err := PaygFallbackGroupCode(nil)
	if err != nil {
		t.Fatalf("PaygFallbackGroupCode() error = %v", err)
	}
	if paygFallbackCode != "default" {
		t.Fatalf("PaygFallbackGroupCode() = %q, want default", paygFallbackCode)
	}
}

func TestSoftDeleteDefaultGroupRejected(t *testing.T) {
	db := newGroupCleanupTestDB(t)
	withGroupCleanupEnv(t, db)

	defaultGroup := createTestGroup(t, db, "default")
	createTestGroup(t, db, "group-a")

	if _, err := SoftDeleteGroupByID(nil, defaultGroup.Id); err == nil {
		t.Fatal("SoftDeleteGroupByID(default) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "不可删除") {
		t.Fatalf("SoftDeleteGroupByID(default) error = %v, want protected-group rejection", err)
	}
}

func TestBackfillCxPoolGroupIDsOptionDropsArchivedGroupIDs(t *testing.T) {
	db := newGroupCleanupTestDB(t)
	withGroupCleanupEnv(t, db)

	groupA := createTestGroup(t, db, "group-a")
	groupB := createTestGroup(t, db, "group-b")
	groupArchived := createTestGroup(t, db, "group-c")
	if err := db.Model(&Group{}).Where("id = ?", groupArchived.Id).Updates(map[string]interface{}{
		"enabled":  false,
		"archived": true,
		"code":     archiveGroupCode(groupArchived.Code, groupArchived.Id),
		"name":     archiveGroupCode(groupArchived.DisplayName, groupArchived.Id),
	}).Error; err != nil {
		t.Fatalf("archive group-c: %v", err)
	}

	mustSetOption(t, db, "cx_pool.cx_group_ids", "[1,2,3]")

	if err := backfillCxPoolGroupIDsOption(db); err != nil {
		t.Fatalf("backfillCxPoolGroupIDsOption() error = %v", err)
	}

	got := mustReadOption(t, db, "cx_pool.cx_group_ids")
	want := "[" + strings.Join([]string{
		strconv.Itoa(groupA.Id),
		strconv.Itoa(groupB.Id),
	}, ",") + "]"
	if got != want {
		t.Fatalf("cx_pool.cx_group_ids = %s, want %s", got, want)
	}
}

package service

import (
	"one-api/billing"
	"one-api/model"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/setting/ratio_setting"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestComputeTokenBucketUsageAppliesResponsesServiceTierMultiplier(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ServiceTier: relaycommon.ServiceTierPriority,
	}

	got := ComputeTokenBucketUsage(info, 10)
	if got != 20000 {
		t.Fatalf("ComputeTokenBucketUsage(priority) = %d, want %d", got, 20000)
	}

	info.ServiceTier = relaycommon.ServiceTierFlex
	got = ComputeTokenBucketUsage(info, 10)
	if got != 5000 {
		t.Fatalf("ComputeTokenBucketUsage(flex) = %d, want %d", got, 5000)
	}
}

func TestComputeTokenBucketUsageDoesNotApplyRawBaseMultiplierTwice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		BaseMultiplier: 0.5,
	}

	if got := ComputeTokenBucketUsage(info, 1); got != 1000 {
		t.Fatalf("ComputeTokenBucketUsage() = %d, want %d", got, 1000)
	}
	if got := ComputeRequestBucketUsage(info, 1); got != 1000 {
		t.Fatalf("ComputeRequestBucketUsage() = %d, want %d", got, 1000)
	}
}

func TestGroupRatioResolversFallbackToPublicGroupRatioWhenUserLookupFails(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:group-ratio-fallback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	oldGroupRatioJSON := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatioJSON := ratio_setting.GroupGroupRatio2JSONString()
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"992":2.5}`); err != nil {
		t.Fatalf("UpdateGroupRatioByJSONString() error = %v", err)
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`); err != nil {
		t.Fatalf("UpdateGroupGroupRatioByJSONString() error = %v", err)
	}
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatioJSON)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatioJSON)
	})

	info := &relaycommon.RelayInfo{
		UserId:       123,
		UsingGroupId: 992,
	}

	if got := ResolveEffectiveGroupRatio(info); got != 2.5 {
		t.Fatalf("ResolveEffectiveGroupRatio() = %v, want 2.5", got)
	}
	if got := ResolvePublicGroupRatio(info); got != 2.5 {
		t.Fatalf("ResolvePublicGroupRatio() = %v, want 2.5", got)
	}
	if got, want := ComputeTokenBucketUsage(info, 1), billing.ScaleTokensByGroupRatio(1, 2.5); got != want {
		t.Fatalf("ComputeTokenBucketUsage() = %d, want %d", got, want)
	}
	if got, want := ComputeRequestBucketUsage(info, 1), billing.ScaleRequestsByGroupRatio(1, 2.5); got != want {
		t.Fatalf("ComputeRequestBucketUsage() = %d, want %d", got, want)
	}
}

func newGroupRatioUsageTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "group-ratio-usage.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Group{},
		&model.User{},
		&model.UserSubscription{},
		&model.UserSubscriptionGroup{},
		&model.PricingProfile{},
		&model.PricingProfileGroupFactor{},
		&model.UserGroupPriceOverride{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createGroupRatioUsageTestGroup(t *testing.T, db *gorm.DB, code string) model.Group {
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

func TestResolveEffectiveGroupRatioLegacySpecialRatioOverridesBaseMultiplier(t *testing.T) {
	oldDB := model.DB
	db := newGroupRatioUsageTestDB(t)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	resellerGroup := createGroupRatioUsageTestGroup(t, db, "reseller")
	premiumGroup := createGroupRatioUsageTestGroup(t, db, "premium")

	oldGroupRatioJSON := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatioJSON := ratio_setting.GroupGroupRatio2JSONString()
	if err := ratio_setting.UpdateGroupRatioByJSONString(
		func() string {
			return `{"` + strconv.Itoa(resellerGroup.Id) + `":1,"` + strconv.Itoa(premiumGroup.Id) + `":2}`
		}(),
	); err != nil {
		t.Fatalf("UpdateGroupRatioByJSONString() error = %v", err)
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(
		func() string {
			return `{"` + strconv.Itoa(resellerGroup.Id) + `":{"` + strconv.Itoa(premiumGroup.Id) + `":0.4}}`
		}(),
	); err != nil {
		t.Fatalf("UpdateGroupGroupRatioByJSONString() error = %v", err)
	}
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatioJSON)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatioJSON)
	})

	user := model.User{
		Username:       "legacy-base-user",
		Password:       "password123",
		AffCode:        "legacy-base-user-aff",
		GroupId:        resellerGroup.Id,
		Group:          resellerGroup.Code,
		BaseMultiplier: 1.5,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	info := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UsingGroupId: premiumGroup.Id,
	}
	if got := ResolveEffectiveGroupRatio(info); got != 0.4 {
		t.Fatalf("ResolveEffectiveGroupRatio() = %v, want 0.4", got)
	}
	if got, want := ComputeTokenBucketUsage(info, 1), billing.ScaleTokensByGroupRatio(1, 0.4); got != want {
		t.Fatalf("ComputeTokenBucketUsage() = %d, want %d", got, want)
	}
	if got, want := ComputeRequestBucketUsage(info, 1), billing.ScaleRequestsByGroupRatio(1, 0.4); got != want {
		t.Fatalf("ComputeRequestBucketUsage() = %d, want %d", got, want)
	}
	groupRatioInfo, _, err := model.ResolveUserGroupRatioInfoByID(user.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveUserGroupRatioInfoByID() error = %v", err)
	}
	if groupRatioInfo.Source != "legacy" {
		t.Fatalf("Source = %q, want legacy", groupRatioInfo.Source)
	}
	if groupRatioInfo.BaseMultiplierApplied {
		t.Fatalf("BaseMultiplierApplied = true, want false")
	}
}

package model

import (
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"one-api/billing"
	"one-api/common"
	"one-api/setting/ratio_setting"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPricingProfileDescriptionSchemaAvoidsTextDefault(t *testing.T) {
	field, ok := reflect.TypeOf(PricingProfile{}).FieldByName("Description")
	if !ok {
		t.Fatal("PricingProfile.Description field not found")
	}
	gormTag := field.Tag.Get("gorm")
	if !strings.Contains(gormTag, "type:text") {
		t.Fatalf("PricingProfile.Description gorm tag = %q, want contains %q", gormTag, "type:text")
	}
	if strings.Contains(gormTag, "default:") {
		t.Fatalf("PricingProfile.Description gorm tag = %q, text columns must not declare a default value", gormTag)
	}
}

func newPricingProfileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "pricing-profile.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Group{},
		&User{},
		&UserSubscription{},
		&UserSubscriptionGroup{},
		&PricingProfile{},
		&PricingProfileGroupFactor{},
		&UserGroupPriceOverride{},
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

func withPricingRatioSettings(t *testing.T, groupRatios map[int]float64, legacy map[int]map[int]float64) {
	t.Helper()

	oldGroupRatios := ratio_setting.GetGroupRatioCopy()
	oldLegacyJSON := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		groupRatiosJSON, _ := json.Marshal(oldGroupRatios)
		_ = ratio_setting.UpdateGroupRatioByJSONString(string(groupRatiosJSON))
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldLegacyJSON)
	})

	groupRatiosJSON, err := json.Marshal(groupRatios)
	if err != nil {
		t.Fatalf("marshal group ratios: %v", err)
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(string(groupRatiosJSON)); err != nil {
		t.Fatalf("UpdateGroupRatioByJSONString() error = %v", err)
	}

	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy group ratios: %v", err)
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(string(legacyJSON)); err != nil {
		t.Fatalf("UpdateGroupGroupRatioByJSONString() error = %v", err)
	}
}

func almostEqualFloat64(got float64, want float64) bool {
	return math.Abs(got-want) <= 1e-9
}

func TestResolveUserGroupRatioInfoPrefersNewPricingRulesBeforeLegacyFallback(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	retailGroup := createTestGroup(t, db, "retail")
	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			retailGroup.Id:   1,
			resellerGroup.Id: 1.3,
			premiumGroup.Id:  3,
		},
		map[int]map[int]float64{
			resellerGroup.Id: {
				premiumGroup.Id: 0.4,
			},
		},
	)

	resellerProfile, err := CreatePricingProfile(nil, SavePricingProfileParams{
		Code:          "reseller-std",
		Name:          "Reseller Standard",
		Audience:      CustomerTypeReseller,
		DefaultFactor: 0.7,
		Enabled:       true,
		GroupFactors: []PriceGroupFactor{
			{
				GroupId: premiumGroup.Id,
				Factor:  0.5,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePricingProfile() error = %v", err)
	}

	profileUser := User{
		Username:         "profile-user",
		Password:         "password123",
		AffCode:          "profile-user-aff",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.8,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: resellerProfile.Id,
	}
	if err := db.Create(&profileUser).Error; err != nil {
		t.Fatalf("create profile user: %v", err)
	}

	overrideUser := User{
		Username:         "override-user",
		Password:         "password123",
		AffCode:          "override-user-aff",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.9,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: resellerProfile.Id,
	}
	if err := db.Create(&overrideUser).Error; err != nil {
		t.Fatalf("create override user: %v", err)
	}
	if err := db.Create(&UserGroupPriceOverride{
		UserId:  overrideUser.Id,
		GroupId: premiumGroup.Id,
		Factor:  0.25,
	}).Error; err != nil {
		t.Fatalf("create user override: %v", err)
	}

	baseOnlyUser := User{
		Username:       "base-only-user",
		Password:       "password123",
		AffCode:        "base-only-user-aff",
		GroupId:        resellerGroup.Id,
		Group:          resellerGroup.Code,
		BaseMultiplier: 1.5,
		CustomerType:   CustomerTypeReseller,
	}
	if err := db.Create(&baseOnlyUser).Error; err != nil {
		t.Fatalf("create base-only user: %v", err)
	}

	legacyUser := User{
		Username:     "legacy-user",
		Password:     "password123",
		AffCode:      "legacy-user-aff",
		GroupId:      resellerGroup.Id,
		Group:        resellerGroup.Code,
		CustomerType: CustomerTypeReseller,
	}
	if err := db.Create(&legacyUser).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	if err := RefreshPricingRuleCache(); err != nil {
		t.Fatalf("RefreshPricingRuleCache() error = %v", err)
	}

	anonymousInfo := ResolveUserGroupRatioInfo(nil, premiumGroup.Id)
	if !almostEqualFloat64(anonymousInfo.EffectiveGroupRatio, 3) {
		t.Fatalf("anonymous EffectiveGroupRatio = %v, want 3", anonymousInfo.EffectiveGroupRatio)
	}

	profileInfo := ResolveUserGroupRatioInfo(profileUser.ToBaseUser(), premiumGroup.Id)
	if !almostEqualFloat64(profileInfo.EffectiveGroupRatio, 0.5) {
		t.Fatalf("profile EffectiveGroupRatio = %v, want 0.5", profileInfo.EffectiveGroupRatio)
	}
	if !profileInfo.HasSpecialRatio {
		t.Fatal("profile HasSpecialRatio = false, want true")
	}
	if profileInfo.Source != "profile" {
		t.Fatalf("profile Source = %q, want profile", profileInfo.Source)
	}
	if profileInfo.BaseMultiplierApplied {
		t.Fatal("profile BaseMultiplierApplied = true, want false for per-group override")
	}

	overrideInfo := ResolveUserGroupRatioInfo(overrideUser.ToBaseUser(), premiumGroup.Id)
	if !almostEqualFloat64(overrideInfo.EffectiveGroupRatio, 0.25) {
		t.Fatalf("override EffectiveGroupRatio = %v, want 0.25", overrideInfo.EffectiveGroupRatio)
	}
	if overrideInfo.Source != "override" {
		t.Fatalf("override Source = %q, want override", overrideInfo.Source)
	}
	if overrideInfo.BaseMultiplierApplied {
		t.Fatal("override BaseMultiplierApplied = true, want false")
	}

	baseOnlyInfo := ResolveUserGroupRatioInfo(baseOnlyUser.ToBaseUser(), premiumGroup.Id)
	if !almostEqualFloat64(baseOnlyInfo.EffectiveGroupRatio, 0.4) {
		t.Fatalf("base-only EffectiveGroupRatio = %v, want 0.4", baseOnlyInfo.EffectiveGroupRatio)
	}
	if baseOnlyInfo.Source != "legacy" {
		t.Fatalf("base-only Source = %q, want legacy", baseOnlyInfo.Source)
	}
	if baseOnlyInfo.BaseMultiplierApplied {
		t.Fatal("base-only BaseMultiplierApplied = true, want false when legacy ratio overrides base multiplier")
	}

	legacyInfo := ResolveUserGroupRatioInfo(legacyUser.ToBaseUser(), premiumGroup.Id)
	if !almostEqualFloat64(legacyInfo.EffectiveGroupRatio, 0.4) {
		t.Fatalf("legacy EffectiveGroupRatio = %v, want 0.4", legacyInfo.EffectiveGroupRatio)
	}
	if legacyInfo.Source != "legacy" {
		t.Fatalf("legacy Source = %q, want legacy", legacyInfo.Source)
	}
	if legacyInfo.BaseMultiplierApplied {
		t.Fatal("legacy BaseMultiplierApplied = true, want false")
	}
}

func TestSyncUserGroupPriceOverridesByGroupReplacesEntries(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "group-a")
	groupB := createTestGroup(t, db, "group-b")

	userA := User{
		Username: "price-user-a",
		Password: "password123",
		GroupId:  groupA.Id,
		Group:    groupA.Code,
		Status:   common.UserStatusEnabled,
		AffCode:  "pga1",
	}
	userB := User{
		Username: "price-user-b",
		Password: "password123",
		GroupId:  groupB.Id,
		Group:    groupB.Code,
		Status:   common.UserStatusEnabled,
		AffCode:  "pgb1",
	}
	if err := db.Create(&userA).Error; err != nil {
		t.Fatalf("create userA: %v", err)
	}
	if err := db.Create(&userB).Error; err != nil {
		t.Fatalf("create userB: %v", err)
	}

	if err := db.Create(&UserGroupPriceOverride{
		UserId:  userA.Id,
		GroupId: groupA.Id,
		Factor:  1.2,
	}).Error; err != nil {
		t.Fatalf("seed userA override: %v", err)
	}

	count, affectedUserIDs, err := SyncUserGroupPriceOverridesByGroup(nil, groupA.Id, []SaveGroupUserPriceOverrideEntry{
		{UserId: userB.Id, Factor: 0.8},
	})
	if err != nil {
		t.Fatalf("SyncUserGroupPriceOverridesByGroup() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if !reflect.DeepEqual(affectedUserIDs, []int{userA.Id, userB.Id}) {
		t.Fatalf("affectedUserIDs = %v, want [%d %d]", affectedUserIDs, userA.Id, userB.Id)
	}

	items, err := ListUserGroupPriceOverridesByGroup(nil, groupA.Id)
	if err != nil {
		t.Fatalf("ListUserGroupPriceOverridesByGroup() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].UserId != userB.Id || !almostEqualFloat64(items[0].Factor, 0.8) {
		t.Fatalf("items[0] = %#v, want userB/0.8", items[0])
	}

	userAFactors, err := ListUserGroupPriceOverrides(nil, userA.Id)
	if err != nil {
		t.Fatalf("ListUserGroupPriceOverrides(userA) error = %v", err)
	}
	if len(userAFactors) != 0 {
		t.Fatalf("userA overrides = %v, want empty after replacement", userAFactors)
	}
}

func TestResolveDisplayedGroupRatioAndDiscreteUsageLegacyRatioOverridesBaseMultiplier(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			resellerGroup.Id: 1,
			premiumGroup.Id:  2,
		},
		map[int]map[int]float64{
			resellerGroup.Id: {
				premiumGroup.Id: 0.4,
			},
		},
	)

	user := User{
		Username:       "legacy-base-priced-user",
		Password:       "password123",
		AffCode:        "legacy-base-priced-user-aff",
		GroupId:        resellerGroup.Id,
		Group:          resellerGroup.Code,
		BaseMultiplier: 1.5,
		CustomerType:   CustomerTypeReseller,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := RefreshPricingRuleCache(); err != nil {
		t.Fatalf("RefreshPricingRuleCache() error = %v", err)
	}

	displayRatio, err := ResolveDisplayedGroupRatioByUserID(user.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID() error = %v", err)
	}
	if !almostEqualFloat64(displayRatio, 0.4) {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID() = %v, want 0.4", displayRatio)
	}

	tokenUsage, err := ComputeUserTokenUsage(user.Id, premiumGroup.Id, 1)
	if err != nil {
		t.Fatalf("ComputeUserTokenUsage() error = %v", err)
	}
	if want := billing.ScaleTokensByGroupRatio(1, 0.4); tokenUsage != want {
		t.Fatalf("ComputeUserTokenUsage() = %d, want %d", tokenUsage, want)
	}

	requestUsage, err := ComputeUserRequestUsage(user.Id, premiumGroup.Id, 1)
	if err != nil {
		t.Fatalf("ComputeUserRequestUsage() error = %v", err)
	}
	if want := billing.ScaleRequestsByGroupRatio(1, 0.4); requestUsage != want {
		t.Fatalf("ComputeUserRequestUsage() = %d, want %d", requestUsage, want)
	}
}

func TestResolveDisplayedGroupRatioAndDiscreteUsageFollowResolvedPricingRules(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			resellerGroup.Id: 1,
			premiumGroup.Id:  2,
		},
		map[int]map[int]float64{},
	)

	profile, err := CreatePricingProfile(nil, SavePricingProfileParams{
		Code:          "reseller-priced",
		Name:          "Reseller Priced",
		Audience:      CustomerTypeReseller,
		DefaultFactor: 0.5,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreatePricingProfile() error = %v", err)
	}

	user := User{
		Username:         "priced-user",
		Password:         "password123",
		AffCode:          "priced-user-aff",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.8,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: profile.Id,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	overrideUser := User{
		Username:         "override-priced-user",
		Password:         "password123",
		AffCode:          "override-priced-user-aff",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.6,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: profile.Id,
	}
	if err := db.Create(&overrideUser).Error; err != nil {
		t.Fatalf("create override user: %v", err)
	}
	if err := db.Create(&UserGroupPriceOverride{
		UserId:  overrideUser.Id,
		GroupId: premiumGroup.Id,
		Factor:  0.25,
	}).Error; err != nil {
		t.Fatalf("create override factor: %v", err)
	}

	if err := RefreshPricingRuleCache(); err != nil {
		t.Fatalf("RefreshPricingRuleCache() error = %v", err)
	}

	displayRatio, err := ResolveDisplayedGroupRatioByUserID(user.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID() error = %v", err)
	}
	if !almostEqualFloat64(displayRatio, 0.8) {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID() = %v, want 0.8", displayRatio)
	}

	tokenUsage, err := ComputeUserTokenUsage(user.Id, premiumGroup.Id, 1)
	if err != nil {
		t.Fatalf("ComputeUserTokenUsage() error = %v", err)
	}
	expectedTokenUsage := billing.ScaleTokensByGroupRatio(1, 0.8)
	if tokenUsage != expectedTokenUsage {
		t.Fatalf("ComputeUserTokenUsage() = %d, want %d", tokenUsage, expectedTokenUsage)
	}

	requestUsage, err := ComputeUserRequestUsage(user.Id, premiumGroup.Id, 1)
	if err != nil {
		t.Fatalf("ComputeUserRequestUsage() error = %v", err)
	}
	expectedRequestUsage := billing.ScaleRequestsByGroupRatio(1, 0.8)
	if requestUsage != expectedRequestUsage {
		t.Fatalf("ComputeUserRequestUsage() = %d, want %d", requestUsage, expectedRequestUsage)
	}
	info := ResolveUserGroupRatioInfo(user.ToBaseUser(), premiumGroup.Id)
	if !info.BaseMultiplierApplied {
		t.Fatal("profile default BaseMultiplierApplied = false, want true")
	}
	if info.Source != "profile" {
		t.Fatalf("profile default Source = %q, want profile", info.Source)
	}

	overrideDisplayRatio, err := ResolveDisplayedGroupRatioByUserID(overrideUser.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID(override) error = %v", err)
	}
	if !almostEqualFloat64(overrideDisplayRatio, 0.25) {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID(override) = %v, want 0.25", overrideDisplayRatio)
	}

	overrideTokenUsage, err := ComputeUserTokenUsage(overrideUser.Id, premiumGroup.Id, 1)
	if err != nil {
		t.Fatalf("ComputeUserTokenUsage(override) error = %v", err)
	}
	if want := billing.ScaleTokensByGroupRatio(1, 0.25); overrideTokenUsage != want {
		t.Fatalf("ComputeUserTokenUsage(override) = %d, want %d", overrideTokenUsage, want)
	}
}

func TestBuildUserGroupPricingPreviewReflectsActualPricingSource(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			resellerGroup.Id: 1,
			premiumGroup.Id:  2,
		},
		map[int]map[int]float64{
			resellerGroup.Id: {
				premiumGroup.Id: 0.4,
			},
		},
	)

	profile, err := CreatePricingProfile(nil, SavePricingProfileParams{
		Code:          "preview-profile",
		Name:          "Preview Profile",
		Audience:      CustomerTypeReseller,
		DefaultFactor: 0.7,
		Enabled:       true,
		GroupFactors: []PriceGroupFactor{
			{
				GroupId: premiumGroup.Id,
				Factor:  0.5,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePricingProfile() error = %v", err)
	}

	profileUser := User{
		Username:         "preview-profile-user",
		Password:         "password123",
		AffCode:          "preview-profile-user-aff",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.8,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: profile.Id,
	}
	if err := db.Create(&profileUser).Error; err != nil {
		t.Fatalf("create profile user: %v", err)
	}

	overrideUser := User{
		Username:         "preview-override-user",
		Password:         "password123",
		AffCode:          "preview-override-user-aff",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.8,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: profile.Id,
	}
	if err := db.Create(&overrideUser).Error; err != nil {
		t.Fatalf("create override user: %v", err)
	}
	if err := db.Create(&UserGroupPriceOverride{
		UserId:  overrideUser.Id,
		GroupId: premiumGroup.Id,
		Factor:  0.25,
	}).Error; err != nil {
		t.Fatalf("create user override: %v", err)
	}

	baseOnlyUser := User{
		Username:       "preview-base-only-user",
		Password:       "password123",
		AffCode:        "preview-base-only-user-aff",
		GroupId:        resellerGroup.Id,
		Group:          resellerGroup.Code,
		BaseMultiplier: 1.5,
		CustomerType:   CustomerTypeReseller,
	}
	if err := db.Create(&baseOnlyUser).Error; err != nil {
		t.Fatalf("create base-only user: %v", err)
	}

	legacyUser := User{
		Username:     "preview-legacy-user",
		Password:     "password123",
		AffCode:      "preview-legacy-user-aff",
		GroupId:      resellerGroup.Id,
		Group:        resellerGroup.Code,
		CustomerType: CustomerTypeReseller,
	}
	if err := db.Create(&legacyUser).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	if err := RefreshPricingRuleCache(); err != nil {
		t.Fatalf("RefreshPricingRuleCache() error = %v", err)
	}

	findPreview := func(items []UserGroupPricingPreview, groupID int) UserGroupPricingPreview {
		t.Helper()
		for _, item := range items {
			if item.GroupId == groupID {
				return item
			}
		}
		t.Fatalf("preview for group %d not found", groupID)
		return UserGroupPricingPreview{}
	}

	profilePreview := findPreview(BuildUserGroupPricingPreview(profileUser.ToBaseUser()), premiumGroup.Id)
	if profilePreview.Source != "profile" {
		t.Fatalf("profile preview source = %q, want profile", profilePreview.Source)
	}
	if !almostEqualFloat64(profilePreview.EffectiveRatio, 0.5) {
		t.Fatalf("profile preview effective ratio = %v, want 0.5", profilePreview.EffectiveRatio)
	}
	if !almostEqualFloat64(profilePreview.AppliedFactor, 0.5) {
		t.Fatalf("profile preview applied factor = %v, want 0.5", profilePreview.AppliedFactor)
	}

	overridePreview := findPreview(BuildUserGroupPricingPreview(overrideUser.ToBaseUser()), premiumGroup.Id)
	if overridePreview.Source != "override" {
		t.Fatalf("override preview source = %q, want override", overridePreview.Source)
	}
	if !almostEqualFloat64(overridePreview.EffectiveRatio, 0.25) {
		t.Fatalf("override preview effective ratio = %v, want 0.25", overridePreview.EffectiveRatio)
	}
	if !almostEqualFloat64(overridePreview.AppliedFactor, 0.25) {
		t.Fatalf("override preview applied factor = %v, want 0.25", overridePreview.AppliedFactor)
	}

	baseOnlyPreview := findPreview(BuildUserGroupPricingPreview(baseOnlyUser.ToBaseUser()), premiumGroup.Id)
	if baseOnlyPreview.Source != "legacy" {
		t.Fatalf("base-only preview source = %q, want legacy", baseOnlyPreview.Source)
	}
	if !almostEqualFloat64(baseOnlyPreview.EffectiveRatio, 0.4) {
		t.Fatalf("base-only preview effective ratio = %v, want 0.4", baseOnlyPreview.EffectiveRatio)
	}
	if !almostEqualFloat64(baseOnlyPreview.AppliedFactor, 0.2) {
		t.Fatalf("base-only preview applied factor = %v, want 0.2", baseOnlyPreview.AppliedFactor)
	}

	legacyPreview := findPreview(BuildUserGroupPricingPreview(legacyUser.ToBaseUser()), premiumGroup.Id)
	if legacyPreview.Source != "legacy" {
		t.Fatalf("legacy preview source = %q, want legacy", legacyPreview.Source)
	}
	if !almostEqualFloat64(legacyPreview.EffectiveRatio, 0.4) {
		t.Fatalf("legacy preview effective ratio = %v, want 0.4", legacyPreview.EffectiveRatio)
	}
	if !almostEqualFloat64(legacyPreview.AppliedFactor, 0.2) {
		t.Fatalf("legacy preview applied factor = %v, want 0.2", legacyPreview.AppliedFactor)
	}
}

func TestListLegacyPricingUsersIncludesUsersStillUsingLegacyWithBaseMultiplier(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	retailGroup := createTestGroup(t, db, "retail")
	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			resellerGroup.Id: 1,
			premiumGroup.Id:  2,
		},
		map[int]map[int]float64{
			resellerGroup.Id: {
				premiumGroup.Id: 0.4,
			},
		},
	)

	baseOnlyUser := User{
		Username:       "base-only-user",
		Password:       "password123",
		AffCode:        "base-only-user-aff",
		GroupId:        resellerGroup.Id,
		Group:          resellerGroup.Code,
		BaseMultiplier: 0.8,
		CustomerType:   CustomerTypeReseller,
	}
	if err := db.Create(&baseOnlyUser).Error; err != nil {
		t.Fatalf("create base-only user: %v", err)
	}

	legacyUser := User{
		Username:     "legacy-user",
		Password:     "password123",
		AffCode:      "legacy-user-aff",
		GroupId:      resellerGroup.Id,
		Group:        resellerGroup.Code,
		CustomerType: CustomerTypeReseller,
	}
	if err := db.Create(&legacyUser).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	baseMultiplierOnlyUser := User{
		Username:       "base-multiplier-only-user",
		Password:       "password123",
		AffCode:        "base-multiplier-only-user-aff",
		GroupId:        retailGroup.Id,
		Group:          retailGroup.Code,
		BaseMultiplier: 1.2,
		CustomerType:   CustomerTypeRetail,
	}
	if err := db.Create(&baseMultiplierOnlyUser).Error; err != nil {
		t.Fatalf("create baseMultiplierOnlyUser: %v", err)
	}

	items, err := ListLegacyPricingUsers(db)
	if err != nil {
		t.Fatalf("ListLegacyPricingUsers() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("ListLegacyPricingUsers() len = %d, want 3", len(items))
	}
	if items[0].Id != baseOnlyUser.Id {
		t.Fatalf("ListLegacyPricingUsers()[0].Id = %d, want %d", items[0].Id, baseOnlyUser.Id)
	}
	if items[0].EffectiveSource != "legacy" {
		t.Fatalf("ListLegacyPricingUsers()[0].EffectiveSource = %q, want legacy", items[0].EffectiveSource)
	}
	if items[0].BaseMultiplierApplied {
		t.Fatal("ListLegacyPricingUsers()[0].BaseMultiplierApplied = true, want false")
	}
	if items[1].Id != legacyUser.Id {
		t.Fatalf("ListLegacyPricingUsers()[1].Id = %d, want %d", items[1].Id, legacyUser.Id)
	}
	if items[1].EffectiveSource != "legacy" {
		t.Fatalf("ListLegacyPricingUsers()[1].EffectiveSource = %q, want legacy", items[1].EffectiveSource)
	}
	if items[1].BaseMultiplierApplied {
		t.Fatal("ListLegacyPricingUsers()[1].BaseMultiplierApplied = true, want false")
	}
	if items[2].Id != baseMultiplierOnlyUser.Id {
		t.Fatalf("ListLegacyPricingUsers()[2].Id = %d, want %d", items[2].Id, baseMultiplierOnlyUser.Id)
	}
	if items[2].EffectiveSource != "base_multiplier" {
		t.Fatalf("ListLegacyPricingUsers()[2].EffectiveSource = %q, want base_multiplier", items[2].EffectiveSource)
	}
	if !items[2].BaseMultiplierApplied {
		t.Fatal("ListLegacyPricingUsers()[2].BaseMultiplierApplied = false, want true")
	}
}

func TestUserInsertPersistsPricingProfileAndResolvedRatio(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			resellerGroup.Id: 1,
			premiumGroup.Id:  2,
		},
		map[int]map[int]float64{
			resellerGroup.Id: {
				premiumGroup.Id: 0.4,
			},
		},
	)

	profile, err := CreatePricingProfile(nil, SavePricingProfileParams{
		Code:          "reseller-insert",
		Name:          "Reseller Insert",
		Audience:      CustomerTypeReseller,
		DefaultFactor: 0.5,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreatePricingProfile() error = %v", err)
	}

	user := User{
		Username:         "insert-priced-user",
		Password:         "password123",
		DisplayName:      "Insert Priced User",
		GroupId:          resellerGroup.Id,
		Group:            resellerGroup.Code,
		BaseMultiplier:   0.8,
		CustomerType:     CustomerTypeReseller,
		PricingProfileId: profile.Id,
	}
	if err := user.Insert(0); err != nil {
		t.Fatalf("user.Insert() error = %v", err)
	}

	persisted, err := GetUserById(user.Id, false)
	if err != nil {
		t.Fatalf("GetUserById() error = %v", err)
	}
	if persisted.CustomerType != CustomerTypeReseller {
		t.Fatalf("persisted.CustomerType = %q, want %q", persisted.CustomerType, CustomerTypeReseller)
	}
	if persisted.PricingProfileId != profile.Id {
		t.Fatalf("persisted.PricingProfileId = %d, want %d", persisted.PricingProfileId, profile.Id)
	}
	if !almostEqualFloat64(persisted.BaseMultiplier, 0.8) {
		t.Fatalf("persisted.BaseMultiplier = %v, want 0.8", persisted.BaseMultiplier)
	}

	cached, err := GetUserCache(user.Id)
	if err != nil {
		t.Fatalf("GetUserCache() error = %v", err)
	}
	if cached.CustomerType != CustomerTypeReseller {
		t.Fatalf("cached.CustomerType = %q, want %q", cached.CustomerType, CustomerTypeReseller)
	}
	if cached.PricingProfileId != profile.Id {
		t.Fatalf("cached.PricingProfileId = %d, want %d", cached.PricingProfileId, profile.Id)
	}

	displayRatio, err := ResolveDisplayedGroupRatioByUserID(user.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID() error = %v", err)
	}
	if !almostEqualFloat64(displayRatio, 0.8) {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID() = %v, want 0.8", displayRatio)
	}
}

func TestUserEditSwitchesPricingToNewProfileRule(t *testing.T) {
	db := newPricingProfileTestDB(t)
	withModelDB(t, db)

	resellerGroup := createTestGroup(t, db, "reseller")
	premiumGroup := createTestGroup(t, db, "premium")

	withPricingRatioSettings(t,
		map[int]float64{
			resellerGroup.Id: 1,
			premiumGroup.Id:  2,
		},
		map[int]map[int]float64{
			resellerGroup.Id: {
				premiumGroup.Id: 0.4,
			},
		},
	)

	profile, err := CreatePricingProfile(nil, SavePricingProfileParams{
		Code:          "reseller-edit",
		Name:          "Reseller Edit",
		Audience:      CustomerTypeReseller,
		DefaultFactor: 0.6,
		Enabled:       true,
		GroupFactors: []PriceGroupFactor{
			{
				GroupId: premiumGroup.Id,
				Factor:  0.5,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePricingProfile() error = %v", err)
	}

	user := User{
		Username:     "edit-priced-user",
		Password:     "password123",
		DisplayName:  "Edit Priced User",
		GroupId:      resellerGroup.Id,
		Group:        resellerGroup.Code,
		CustomerType: CustomerTypeReseller,
	}
	if err := user.Insert(0); err != nil {
		t.Fatalf("user.Insert() error = %v", err)
	}

	beforeRatio, err := ResolveDisplayedGroupRatioByUserID(user.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID(before) error = %v", err)
	}
	if !almostEqualFloat64(beforeRatio, 0.4) {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID(before) = %v, want 0.4", beforeRatio)
	}

	updated, err := GetUserById(user.Id, false)
	if err != nil {
		t.Fatalf("GetUserById() error = %v", err)
	}
	updated.BaseMultiplier = 0.8
	updated.CustomerType = CustomerTypeReseller
	updated.PricingProfileId = profile.Id
	if err := updated.Edit(false); err != nil {
		t.Fatalf("updated.Edit(false) error = %v", err)
	}

	persisted, err := GetUserById(user.Id, false)
	if err != nil {
		t.Fatalf("GetUserById(after edit) error = %v", err)
	}
	if persisted.CustomerType != CustomerTypeReseller {
		t.Fatalf("persisted.CustomerType = %q, want %q", persisted.CustomerType, CustomerTypeReseller)
	}
	if persisted.PricingProfileId != profile.Id {
		t.Fatalf("persisted.PricingProfileId = %d, want %d", persisted.PricingProfileId, profile.Id)
	}
	if !almostEqualFloat64(persisted.BaseMultiplier, 0.8) {
		t.Fatalf("persisted.BaseMultiplier = %v, want 0.8", persisted.BaseMultiplier)
	}

	cached, err := GetUserCache(user.Id)
	if err != nil {
		t.Fatalf("GetUserCache(after edit) error = %v", err)
	}
	if cached.PricingProfileId != profile.Id {
		t.Fatalf("cached.PricingProfileId = %d, want %d", cached.PricingProfileId, profile.Id)
	}
	if !almostEqualFloat64(cached.BaseMultiplier, 0.8) {
		t.Fatalf("cached.BaseMultiplier = %v, want 0.8", cached.BaseMultiplier)
	}

	afterRatio, err := ResolveDisplayedGroupRatioByUserID(user.Id, premiumGroup.Id)
	if err != nil {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID(after) error = %v", err)
	}
	if !almostEqualFloat64(afterRatio, 0.5) {
		t.Fatalf("ResolveDisplayedGroupRatioByUserID(after) = %v, want 0.5", afterRatio)
	}
}

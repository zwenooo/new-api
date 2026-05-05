package helper

import (
	"net/http/httptest"
	"one-api/model"
	"one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestModelPriceHelperAppliesResponsesServiceTierMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ratio_setting.InitRatioSettings()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	meta := &types.TokenCountMeta{MaxTokens: 2000}

	baseInfo := &common.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "gpt-5.4",
		BaseMultiplier:  1,
	}
	base, err := ModelPriceHelper(ctx, baseInfo, 10000, meta)
	if err != nil {
		t.Fatalf("baseline ModelPriceHelper error: %v", err)
	}

	priorityInfo := &common.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "gpt-5.4",
		BaseMultiplier:  1,
		ServiceTier:     "fast",
	}
	priority, err := ModelPriceHelper(ctx, priorityInfo, 10000, meta)
	if err != nil {
		t.Fatalf("priority ModelPriceHelper error: %v", err)
	}

	if priority.ServiceTier != common.ServiceTierPriority {
		t.Fatalf("priority.ServiceTier = %q, want %q", priority.ServiceTier, common.ServiceTierPriority)
	}
	if priority.ServiceTierMultiplier != 2 {
		t.Fatalf("priority.ServiceTierMultiplier = %v, want 2", priority.ServiceTierMultiplier)
	}
	if priority.ModelRatio != base.ModelRatio*2 {
		t.Fatalf("priority.ModelRatio = %v, want %v", priority.ModelRatio, base.ModelRatio*2)
	}
	if priority.ShouldPreConsumedQuota != base.ShouldPreConsumedQuota*2 {
		t.Fatalf("priority.ShouldPreConsumedQuota = %d, want %d", priority.ShouldPreConsumedQuota, base.ShouldPreConsumedQuota*2)
	}

	flexInfo := &common.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "gpt-5.4",
		BaseMultiplier:  1,
		ServiceTier:     "flex",
	}
	flex, err := ModelPriceHelper(ctx, flexInfo, 10000, meta)
	if err != nil {
		t.Fatalf("flex ModelPriceHelper error: %v", err)
	}

	if flex.ServiceTier != common.ServiceTierFlex {
		t.Fatalf("flex.ServiceTier = %q, want %q", flex.ServiceTier, common.ServiceTierFlex)
	}
	if flex.ServiceTierMultiplier != 0.5 {
		t.Fatalf("flex.ServiceTierMultiplier = %v, want 0.5", flex.ServiceTierMultiplier)
	}
	if flex.ModelRatio != base.ModelRatio*0.5 {
		t.Fatalf("flex.ModelRatio = %v, want %v", flex.ModelRatio, base.ModelRatio*0.5)
	}
	if flex.ShouldPreConsumedQuota != base.ShouldPreConsumedQuota/2 {
		t.Fatalf("flex.ShouldPreConsumedQuota = %d, want %d", flex.ShouldPreConsumedQuota, base.ShouldPreConsumedQuota/2)
	}
}

func TestHandleGroupRatioFallsBackToPublicGroupRatioWhenUserLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:price-helper-fallback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	oldGroupRatioJSON := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatioJSON := ratio_setting.GroupGroupRatio2JSONString()
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"991":2.5}`); err != nil {
		t.Fatalf("UpdateGroupRatioByJSONString() error = %v", err)
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`); err != nil {
		t.Fatalf("UpdateGroupGroupRatioByJSONString() error = %v", err)
	}
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatioJSON)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatioJSON)
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := HandleGroupRatio(ctx, &common.RelayInfo{
		UserId:       123,
		UsingGroupId: 991,
	})

	if info.EffectiveGroupRatio != 2.5 {
		t.Fatalf("EffectiveGroupRatio = %v, want 2.5", info.EffectiveGroupRatio)
	}
	if info.PublicGroupRatio != 2.5 {
		t.Fatalf("PublicGroupRatio = %v, want 2.5", info.PublicGroupRatio)
	}
	if info.PrivateGroupRatio != 2.5 {
		t.Fatalf("PrivateGroupRatio = %v, want 2.5", info.PrivateGroupRatio)
	}
	if info.GroupRatio != 2.5 {
		t.Fatalf("GroupRatio = %v, want 2.5", info.GroupRatio)
	}
	if info.HasSpecialRatio {
		t.Fatalf("HasSpecialRatio = true, want false")
	}
}

func newHandleGroupRatioTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "handle-group-ratio.db")
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

func createHandleGroupRatioTestGroup(t *testing.T, db *gorm.DB, code string) model.Group {
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

func TestHandleGroupRatioLegacySpecialRatioOverridesBaseMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldDB := model.DB
	db := newHandleGroupRatioTestDB(t)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	resellerGroup := createHandleGroupRatioTestGroup(t, db, "reseller")
	premiumGroup := createHandleGroupRatioTestGroup(t, db, "premium")

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
		Username:       "helper-legacy-base-user",
		Password:       "password123",
		AffCode:        "helper-legacy-base-user-aff",
		GroupId:        resellerGroup.Id,
		Group:          resellerGroup.Code,
		BaseMultiplier: 1.5,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := HandleGroupRatio(ctx, &common.RelayInfo{
		UserId:       user.Id,
		UsingGroupId: premiumGroup.Id,
	})

	if info.EffectiveGroupRatio != 0.4 {
		t.Fatalf("EffectiveGroupRatio = %v, want 0.4", info.EffectiveGroupRatio)
	}
	if info.PublicGroupRatio != 0.4 {
		t.Fatalf("PublicGroupRatio = %v, want 0.4", info.PublicGroupRatio)
	}
	if info.PrivateGroupRatio != 0.4 {
		t.Fatalf("PrivateGroupRatio = %v, want 0.4", info.PrivateGroupRatio)
	}
	if info.GroupRatio != 0.4 {
		t.Fatalf("GroupRatio = %v, want 0.4", info.GroupRatio)
	}
	if info.GroupSpecialRatio != 0.4 {
		t.Fatalf("GroupSpecialRatio = %v, want 0.4", info.GroupSpecialRatio)
	}
	if !info.HasSpecialRatio {
		t.Fatalf("HasSpecialRatio = false, want true")
	}
	if info.Source != "legacy" {
		t.Fatalf("Source = %q, want legacy", info.Source)
	}
	if info.BaseMultiplierApplied {
		t.Fatalf("BaseMultiplierApplied = true, want false")
	}
}

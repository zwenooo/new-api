package model

import (
	"path/filepath"
	"reflect"
	"testing"

	"one-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRedemptionPresetHydrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "redemption-preset-hydration.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Group{},
		&RedemptionPreset{},
		&SubscriptionProductGroup{},
		&SubscriptionProductGroupDailyLimit{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestListRedemptionPresetsHydratesFromValidLegacyAllowedGroupIds(t *testing.T) {
	db := newRedemptionPresetHydrationTestDB(t)
	withModelDB(t, db)

	group := createTestGroup(t, db, "legacy-valid")
	legacyGroups, err := common.Marshal([]int{group.Id})
	if err != nil {
		t.Fatalf("marshal legacy group ids: %v", err)
	}
	if err := db.Create(&SubscriptionProductGroup{ProductId: 10, GroupId: 9999}).Error; err != nil {
		t.Fatalf("create stale product group: %v", err)
	}
	if err := db.Create(&RedemptionPreset{
		Id:              10,
		Name:            "legacy-valid-product",
		Mode:            "subscription",
		Enabled:         true,
		AllowedGroupIds: JSONValue(legacyGroups),
	}).Error; err != nil {
		t.Fatalf("create preset: %v", err)
	}

	presets, err := ListRedemptionPresets()
	if err != nil {
		t.Fatalf("ListRedemptionPresets() error = %v", err)
	}
	if len(presets) != 1 {
		t.Fatalf("len(presets) = %d, want 1", len(presets))
	}

	got, err := ParseGroupIDsJSON(presets[0].AllowedGroupIds)
	if err != nil {
		t.Fatalf("ParseGroupIDsJSON() error = %v", err)
	}
	want := []int{group.Id}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allowed_group_ids = %v, want %v", got, want)
	}
}

func TestListSubscriptionRedemptionPresetsIncludesFreeEnabledProducts(t *testing.T) {
	db := newRedemptionPresetHydrationTestDB(t)
	withModelDB(t, db)

	group := createTestGroup(t, db, "free-product")
	if err := db.Create(&RedemptionPreset{
		Id:       20,
		Name:     "free-product",
		Mode:     "subscription",
		Enabled:  true,
		PriceFen: 0,
		Quota:    100,
	}).Error; err != nil {
		t.Fatalf("create preset: %v", err)
	}
	if err := db.Create(&SubscriptionProductGroup{ProductId: 20, GroupId: group.Id}).Error; err != nil {
		t.Fatalf("create product group: %v", err)
	}

	presets, err := ListSubscriptionRedemptionPresets()
	if err != nil {
		t.Fatalf("ListSubscriptionRedemptionPresets() error = %v", err)
	}
	if len(presets) != 1 {
		t.Fatalf("len(presets) = %d, want 1", len(presets))
	}
	if presets[0].Id != 20 {
		t.Fatalf("preset id = %d, want 20", presets[0].Id)
	}
}

package model

import (
	"path/filepath"
	"testing"

	"one-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newOptionLegacyCleanupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "option-legacy-cleanup.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withOptionMapSnapshot(t *testing.T) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	previous := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		previous[key] = value
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func countOptionRows(t *testing.T, db *gorm.DB, key string) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&Option{}).Where("key = ?", key).Count(&count).Error; err != nil {
		t.Fatalf("count option %s: %v", key, err)
	}
	return count
}

func TestLoadOptionsFromDatabaseKeepsLegacyProductManagementOptionByDefault(t *testing.T) {
	db := newOptionLegacyCleanupTestDB(t)
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	t.Setenv("STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED", "")

	if err := db.Create(&Option{
		Key:   "ProductManagementHideDisabledEnabled",
		Value: "false",
	}).Error; err != nil {
		t.Fatalf("create legacy option: %v", err)
	}

	InitOptionMap()

	if got := countOptionRows(t, db, "ProductManagementHideDisabledEnabled"); got != 1 {
		t.Fatalf("legacy ProductManagementHideDisabledEnabled rows = %d, want 1", got)
	}
	if got := countOptionRows(t, db, "ProductManagementHideArchivedEnabled"); got != 1 {
		t.Fatalf("current ProductManagementHideArchivedEnabled rows = %d, want 1", got)
	}
	if got := common.OptionMap["ProductManagementHideArchivedEnabled"]; got != "false" {
		t.Fatalf("OptionMap[ProductManagementHideArchivedEnabled] = %q, want false", got)
	}
}

func TestLoadOptionsFromDatabaseKeepsLegacyCx2ccAliasesByDefault(t *testing.T) {
	db := newOptionLegacyCleanupTestDB(t)
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	t.Setenv("STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED", "")

	if err := db.Create(&Option{
		Key:   "cx_pool.cx2cc.reasoning_effort",
		Value: "medium",
	}).Error; err != nil {
		t.Fatalf("create legacy cx2cc option: %v", err)
	}

	InitOptionMap()

	if got := countOptionRows(t, db, "cx_pool.cx2cc.reasoning_effort"); got != 1 {
		t.Fatalf("legacy cx_pool.cx2cc.reasoning_effort rows = %d, want 1", got)
	}
	if got := countOptionRows(t, db, "cx2cc.reasoning_effort"); got != 1 {
		t.Fatalf("current cx2cc.reasoning_effort rows = %d, want 1", got)
	}
	if got := common.OptionMap["cx2cc.reasoning_effort"]; got != "medium" {
		t.Fatalf("OptionMap[cx2cc.reasoning_effort] = %q, want medium", got)
	}
}

func TestLoadOptionsFromDatabaseCanFenceCleanupBehindExplicitFlag(t *testing.T) {
	db := newOptionLegacyCleanupTestDB(t)
	withModelDB(t, db)
	withOptionMapSnapshot(t)
	t.Setenv("STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED", "true")

	if err := db.Create([]Option{
		{Key: legacyStartupOptionCleanupKey, Value: "true"},
		{Key: "ProductManagementHideDisabledEnabled", Value: "false"},
		{Key: "cx_pool.cx2cc.reasoning_effort", Value: "low"},
	}).Error; err != nil {
		t.Fatalf("create fenced cleanup options: %v", err)
	}

	InitOptionMap()

	if got := countOptionRows(t, db, "ProductManagementHideDisabledEnabled"); got != 0 {
		t.Fatalf("legacy ProductManagementHideDisabledEnabled rows = %d, want 0", got)
	}
	if got := countOptionRows(t, db, "cx_pool.cx2cc.reasoning_effort"); got != 0 {
		t.Fatalf("legacy cx_pool.cx2cc.reasoning_effort rows = %d, want 0", got)
	}
}

func TestShouldCleanupLegacyStartupOptionsHonorsEnvOverride(t *testing.T) {
	t.Setenv("STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED", "true")
	if !shouldCleanupLegacyStartupOptions(nil) {
		t.Fatal("shouldCleanupLegacyStartupOptions(nil) = false, want true when env override enabled")
	}

	t.Setenv("STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED", "")
	if shouldCleanupLegacyStartupOptions([]*Option{{Key: legacyStartupOptionCleanupKey, Value: "false"}}) {
		t.Fatal("shouldCleanupLegacyStartupOptions() = true, want false")
	}
	if !shouldCleanupLegacyStartupOptions([]*Option{{Key: legacyStartupOptionCleanupKey, Value: "true"}}) {
		t.Fatal("shouldCleanupLegacyStartupOptions() = false, want true")
	}
}

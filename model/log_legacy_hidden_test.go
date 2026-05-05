package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newLegacyHiddenLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "legacy-hidden-log.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Log{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestHasLegacyHiddenUserConsumeLogsRecognizesExplicitMarkers(t *testing.T) {
	db := newLegacyHiddenLogTestDB(t)
	oldLogDB := LOG_DB
	LOG_DB = db
	t.Cleanup(func() {
		LOG_DB = oldLogDB
	})

	rows := []Log{
		{
			UserId:       7,
			CreatedAt:    100,
			Type:         LogTypeConsume,
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 0,
			CostQuota:    0,
			Group:        "1",
			Other:        `{"group_ratio_source":"legacy","base_multiplier_applied":false}`,
		},
		{
			UserId:       8,
			CreatedAt:    100,
			Type:         LogTypeConsume,
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 0,
			CostQuota:    0,
			Group:        "1",
			Other:        `{"group_ratio_source":"public","base_multiplier_applied":false}`,
		},
		{
			UserId:       10,
			CreatedAt:    100,
			Type:         LogTypeConsume,
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 0,
			CostQuota:    0,
			Group:        "1",
			Other:        `{"group_ratio_source":"profile","user_group_ratio":0.5,"base_multiplier_applied":true}`,
		},
		{
			UserId:       11,
			CreatedAt:    100,
			Type:         LogTypeConsume,
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 0,
			CostQuota:    0,
			Group:        "1",
			Other:        `{"user_group_ratio":0.4}`,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	ok, err := HasLegacyHiddenUserConsumeLogs(7, 0, 0, "", "", 0, "")
	if err != nil {
		t.Fatalf("HasLegacyHiddenUserConsumeLogs() error = %v", err)
	}
	if !ok {
		t.Fatal("HasLegacyHiddenUserConsumeLogs() = false, want true")
	}

	ok, err = HasLegacyHiddenUserConsumeLogs(8, 0, 0, "", "", 0, "")
	if err != nil {
		t.Fatalf("HasLegacyHiddenUserConsumeLogs() error = %v", err)
	}
	if ok {
		t.Fatal("HasLegacyHiddenUserConsumeLogs() = true, want false")
	}

	ok, err = HasLegacyHiddenUserConsumeLogs(10, 0, 0, "", "", 0, "")
	if err != nil {
		t.Fatalf("HasLegacyHiddenUserConsumeLogs() error = %v", err)
	}
	if ok {
		t.Fatal("HasLegacyHiddenUserConsumeLogs() = true for profile source, want false")
	}

	ok, err = HasLegacyHiddenUserConsumeLogs(11, 0, 0, "", "", 0, "")
	if err != nil {
		t.Fatalf("HasLegacyHiddenUserConsumeLogs() error = %v", err)
	}
	if !ok {
		t.Fatal("HasLegacyHiddenUserConsumeLogs() = false for old marker-only row, want true")
	}
}

func TestGetLegacyHiddenUserConsumeQuotaBucketsRecognizesBaseMultiplierApplied(t *testing.T) {
	db := newLegacyHiddenLogTestDB(t)
	oldLogDB := LOG_DB
	LOG_DB = db
	t.Cleanup(func() {
		LOG_DB = oldLogDB
	})

	rows := []Log{
		{
			UserId:       9,
			CreatedAt:    3605,
			Type:         LogTypeConsume,
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 0,
			CostQuota:    0,
			Group:        "1",
			Other:        `{"group_ratio_source":"base_multiplier","base_multiplier_applied":true}`,
		},
		{
			UserId:       9,
			CreatedAt:    7205,
			Type:         LogTypeConsume,
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 0,
			CostQuota:    0,
			Group:        "1",
			Other:        `{"group_ratio_source":"public","base_multiplier_applied":false}`,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	buckets, err := GetLegacyHiddenUserConsumeQuotaBuckets(9, 0, 0)
	if err != nil {
		t.Fatalf("GetLegacyHiddenUserConsumeQuotaBuckets() error = %v", err)
	}
	if len(buckets) != 1 {
		t.Fatalf("len(buckets) = %d, want 1", len(buckets))
	}
	if _, ok := buckets[3600]; !ok {
		t.Fatalf("expected hour bucket 3600 in result, got %+v", buckets)
	}
}

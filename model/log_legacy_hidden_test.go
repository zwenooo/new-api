package model

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newLegacyHiddenLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	initCol()

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

func TestFormatUserLogsKeepsBillingFormulaFields(t *testing.T) {
	logs := []*Log{
		{
			Id:    2049,
			Quota: 12,
			Other: `{"admin_info":{"request_headers":{"Authorization":["secret"]}},"model_ratio":2.5,"model_price":1.25,"group_ratio":0.8,"request_ua":"codex_cli_rs","is_model_mapped":true,"upstream_model_name":"upstream-model"}`,
		},
	}

	formatUserLogs(logs)

	if strings.Contains(logs[0].Other, "admin_info") {
		t.Fatalf("formatUserLogs kept admin_info: %s", logs[0].Other)
	}
	if strings.Contains(logs[0].Other, "upstream_model_name") || strings.Contains(logs[0].Other, "is_model_mapped") {
		t.Fatalf("formatUserLogs kept model mapping fields: %s", logs[0].Other)
	}
	if !strings.Contains(logs[0].Other, "model_ratio") ||
		!strings.Contains(logs[0].Other, "model_price") ||
		!strings.Contains(logs[0].Other, "group_ratio") {
		t.Fatalf("formatUserLogs removed billing formula fields: %s", logs[0].Other)
	}
}

func TestSumCacheStatByUAUsesMonitorKeywords(t *testing.T) {
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
			PromptTokens: 100,
			Group:        "1",
			Other:        `{"request_ua":"Codex Desktop/1.0","cache_tokens":25}`,
		},
		{
			UserId:       7,
			CreatedAt:    101,
			Type:         LogTypeConsume,
			PromptTokens: 100,
			Group:        "1",
			Other:        `{"request_ua":"claude-cli","claude":true,"cache_tokens":30,"cache_creation_tokens":10}`,
		},
		{
			UserId:       7,
			CreatedAt:    102,
			Type:         LogTypeConsume,
			PromptTokens: 100,
			Group:        "2",
			Other:        `{"request_ua":"browser","cache_tokens":50}`,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	stats, err := SumCacheStatByUA(1, 200, "", "", "", 0, "", []string{"codex desktop", "claude-cli"})
	if err != nil {
		t.Fatalf("SumCacheStatByUA() error = %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}
	if stats[0].UA != "codex desktop" || stats[0].CacheHitTokens != 25 || stats[0].PromptTokensTotal != 100 {
		t.Fatalf("codex desktop stat = %+v, want hit=25 total=100", stats[0])
	}
	if stats[0].Group != "1" {
		t.Fatalf("codex desktop group = %q, want 1", stats[0].Group)
	}
	if stats[1].UA != "claude-cli" || stats[1].CacheHitTokens != 30 || stats[1].PromptTokensTotal != 140 {
		t.Fatalf("claude-cli stat = %+v, want hit=30 total=140", stats[1])
	}
	if stats[1].Group != "1" {
		t.Fatalf("claude-cli group = %q, want 1", stats[1].Group)
	}
}

func TestSumTokenQuotaStatAggregatesByTokenName(t *testing.T) {
	db := newLegacyHiddenLogTestDB(t)
	oldLogDB := LOG_DB
	LOG_DB = db
	t.Cleanup(func() {
		LOG_DB = oldLogDB
	})

	rows := []Log{
		{
			UserId:       7,
			Username:     "alice",
			CreatedAt:    100,
			Type:         LogTypeConsume,
			TokenName:    "shared-token",
			ModelName:    "gpt-4.1-mini",
			Quota:        10,
			VisibleQuota: 8,
			CostQuota:    12,
		},
		{
			UserId:       7,
			Username:     "alice",
			CreatedAt:    120,
			Type:         LogTypeConsume,
			TokenName:    "shared-token",
			ModelName:    "gpt-4.1-mini",
			Quota:        15,
			VisibleQuota: 12,
			CostQuota:    18,
		},
		{
			UserId:       8,
			Username:     "bob",
			CreatedAt:    130,
			Type:         LogTypeConsume,
			TokenName:    "",
			ModelName:    "gpt-4.1-mini",
			Quota:        4,
			VisibleQuota: 4,
			CostQuota:    4,
		},
		{
			UserId:    7,
			Username:  "alice",
			CreatedAt: 140,
			Type:      LogTypeError,
			TokenName: "shared-token",
			Quota:     99,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create logs: %v", err)
	}

	stats, err := SumTokenQuotaStat(90, 135, "gpt-4.1-mini", "", "", 0, "")
	if err != nil {
		t.Fatalf("SumTokenQuotaStat() error = %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}
	if stats[0].TokenName != "shared-token" || stats[0].Quota != 25 || stats[0].VisibleQuota != 20 || stats[0].CostQuota != 30 || stats[0].Count != 2 {
		t.Fatalf("shared-token stat = %+v, want quota=25 visible=20 cost=30 count=2", stats[0])
	}
	if stats[1].TokenName != "" || stats[1].Quota != 4 || stats[1].Count != 1 {
		t.Fatalf("empty token stat = %+v, want quota=4 count=1", stats[1])
	}

	selfStats, err := SumTokenQuotaStatByUserId(7, 90, 135, "gpt-4.1-mini", "", 0, "")
	if err != nil {
		t.Fatalf("SumTokenQuotaStatByUserId() error = %v", err)
	}
	if len(selfStats) != 1 {
		t.Fatalf("len(selfStats) = %d, want 1", len(selfStats))
	}
	if selfStats[0].TokenName != "shared-token" || selfStats[0].Quota != 25 || selfStats[0].Count != 2 {
		t.Fatalf("self shared-token stat = %+v, want quota=25 count=2", selfStats[0])
	}
}

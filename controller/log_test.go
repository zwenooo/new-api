package controller

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"one-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newControllerLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "controller-log.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Log{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withControllerLogDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
}

func decodeStatResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, body=%s", string(body))
	}
	return payload.Data
}

func TestGetLogsSelfStatMarksQuotaLegacyForLegacySource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newControllerLogTestDB(t)
	withControllerLogDB(t, db)

	if err := db.Create(&model.Log{
		UserId:       7,
		CreatedAt:    3605,
		Type:         model.LogTypeConsume,
		ModelName:    "gpt-4.1-mini",
		Quota:        10,
		VisibleQuota: 0,
		CostQuota:    0,
		Group:        "1",
		Other:        `{"group_ratio_source":"legacy","base_multiplier_applied":false}`,
	}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 7)
	c.Request = httptest.NewRequest("GET", "/api/log/self/stat?type=2&start_timestamp=0&end_timestamp=7200&model_name=gpt-4.1-mini", nil)

	GetLogsSelfStat(c)

	data := decodeStatResponse(t, recorder.Body.Bytes())
	if data["quota_legacy"] != true {
		t.Fatalf("quota_legacy = %v, want true", data["quota_legacy"])
	}
}

func TestGetLogsSelfStatDoesNotMarkQuotaLegacyForPublicSource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newControllerLogTestDB(t)
	withControllerLogDB(t, db)

	if err := db.Create(&model.Log{
		UserId:       8,
		CreatedAt:    3605,
		Type:         model.LogTypeConsume,
		ModelName:    "gpt-4.1-mini",
		Quota:        10,
		VisibleQuota: 0,
		CostQuota:    0,
		Group:        "1",
		Other:        `{"group_ratio_source":"public","base_multiplier_applied":false}`,
	}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 8)
	c.Request = httptest.NewRequest("GET", "/api/log/self/stat?type=2&start_timestamp=0&end_timestamp=7200&model_name=gpt-4.1-mini", nil)

	GetLogsSelfStat(c)

	data := decodeStatResponse(t, recorder.Body.Bytes())
	if data["quota_legacy"] != false {
		t.Fatalf("quota_legacy = %v, want false", data["quota_legacy"])
	}
}

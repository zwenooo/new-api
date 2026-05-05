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

func newUseDataTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "usedata.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.QuotaData{}, &model.Log{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withLogAndMainDB(t *testing.T, db *gorm.DB) {
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

func decodeJSONDataArray(t *testing.T, body []byte) []map[string]any {
	t.Helper()

	var payload struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, body=%s", string(body))
	}
	return payload.Data
}

func TestGetUserQuotaDatesMarksQuotaLegacyUsingHiddenBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUseDataTestDB(t)
	withLogAndMainDB(t, db)

	user := model.User{
		Username: "quota-user",
		Password: "password123",
		AffCode:  "quota-user-aff",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&model.QuotaData{
		UserID:       user.Id,
		Username:     user.Username,
		ModelName:    "gpt-4.1-mini",
		CreatedAt:    3605,
		Count:        1,
		Quota:        10,
		VisibleQuota: 0,
		CostQuota:    0,
	}).Error; err != nil {
		t.Fatalf("create quota data: %v", err)
	}

	if err := db.Create(&model.Log{
		UserId:       user.Id,
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
	c.Set("id", user.Id)
	req := httptest.NewRequest("GET", "/api/data/self?start_timestamp=0&end_timestamp=7200", nil)
	c.Request = req

	GetUserQuotaDates(c)

	rows := decodeJSONDataArray(t, recorder.Body.Bytes())
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0]["quota_legacy"] != true {
		t.Fatalf("quota_legacy = %v, want true", rows[0]["quota_legacy"])
	}
}

func TestGetAllQuotaDatesByUsernameMarksQuotaLegacyUsingHiddenBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUseDataTestDB(t)
	withLogAndMainDB(t, db)

	user := model.User{
		Username: "quota-admin-user",
		Password: "password123",
		AffCode:  "quota-admin-user-aff",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&model.QuotaData{
		UserID:       user.Id,
		Username:     user.Username,
		ModelName:    "gpt-4.1-mini",
		CreatedAt:    3605,
		Count:        1,
		Quota:        10,
		VisibleQuota: 0,
		CostQuota:    0,
	}).Error; err != nil {
		t.Fatalf("create quota data: %v", err)
	}

	if err := db.Create(&model.Log{
		UserId:       user.Id,
		CreatedAt:    3605,
		Type:         model.LogTypeConsume,
		ModelName:    "gpt-4.1-mini",
		Quota:        10,
		VisibleQuota: 0,
		CostQuota:    0,
		Group:        "1",
		Other:        `{"group_ratio_source":"base_multiplier","base_multiplier_applied":true}`,
	}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/api/data?username=quota-admin-user&start_timestamp=0&end_timestamp=7200", nil)
	c.Request = req

	GetAllQuotaDates(c)

	rows := decodeJSONDataArray(t, recorder.Body.Bytes())
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0]["quota_legacy"] != true {
		t.Fatalf("quota_legacy = %v, want true", rows[0]["quota_legacy"])
	}
}

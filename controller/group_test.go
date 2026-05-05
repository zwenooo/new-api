package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"one-api/common"
	"one-api/model"
	"one-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "group.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Option{},
		&model.Group{},
		&model.User{},
		&model.UserGroupPriceOverride{},
		&model.UserSubscription{},
		&model.UserRequestSubscription{},
		&model.PaygUserBalance{},
		&model.PayRequestUserBalance{},
		&model.PayTokenUserBalance{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withGroupControllerDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldOptionMap := common.OptionMap
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.OptionMap = oldOptionMap
	})
}

func TestGetUserGroupsUsesEffectiveRatioForCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newGroupTestDB(t)
	withGroupControllerDB(t, db)

	group := &model.Group{
		Code:           "codex-paygoooo",
		Name:           "codex-paygoooo",
		DisplayName:    "codex-paygoooo",
		Description:    "paygo group",
		Ratio:          0.5,
		UserSelectable: true,
		Enabled:        true,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := model.RefreshGroupSettings(); err != nil {
		t.Fatalf("refresh group settings: %v", err)
	}

	user := &model.User{
		Username:       "11111111",
		Password:       "password123",
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		DisplayName:    "11111111",
		GroupId:        group.Id,
		Group:          group.Code,
		BaseMultiplier: 1,
		CustomerType:   "retail",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserGroupPriceOverride{
		UserId:  user.Id,
		GroupId: group.Id,
		Factor:  1.5,
	}).Error; err != nil {
		t.Fatalf("create override: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", user.Id)
	c.Set("role", user.Role)
	c.Request = httptest.NewRequest("GET", "/api/user/self/groups", nil)

	GetUserGroups(c)

	var payload struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, body=%s", recorder.Body.String())
	}
	if len(payload.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1, body=%s", len(payload.Data), recorder.Body.String())
	}
	gotRatio, ok := payload.Data[0]["ratio"].(float64)
	if !ok {
		t.Fatalf("ratio missing or invalid: %#v", payload.Data[0]["ratio"])
	}
	if gotRatio != 1.5 {
		t.Fatalf("ratio = %v, want 1.5", gotRatio)
	}
	gotBaseRatio, ok := payload.Data[0]["base_ratio"].(float64)
	if !ok {
		t.Fatalf("base_ratio missing or invalid: %#v", payload.Data[0]["base_ratio"])
	}
	if gotBaseRatio != 0.5 {
		t.Fatalf("base_ratio = %v, want 0.5", gotBaseRatio)
	}
}

func TestUpdateUserBackfillsMissingLegacyGroupID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newGroupTestDB(t)
	withGroupControllerDB(t, db)

	user := &model.User{
		Username:       "legacy-zero",
		Password:       "password123",
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		DisplayName:    "legacy-zero",
		GroupId:        0,
		BaseMultiplier: 1,
		CustomerType:   "retail",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	defaultGroupID, err := model.ResolveLegacyDefaultModelGroupID(nil)
	if err != nil {
		t.Fatalf("resolve default group: %v", err)
	}
	enabledGroupsJSON, err := json.Marshal(map[int]bool{defaultGroupID: true})
	if err != nil {
		t.Fatalf("marshal enabled groups: %v", err)
	}
	if err := setting.UpdateEnabledGroupsByJSONString(string(enabledGroupsJSON)); err != nil {
		t.Fatalf("update enabled groups: %v", err)
	}

	body, err := json.Marshal(map[string]interface{}{
		"id":                 user.Id,
		"username":           "legacy-zero",
		"display_name":       "legacy-edited",
		"password":           "",
		"role":               common.RoleCommonUser,
		"status":             common.UserStatusEnabled,
		"quota":              0,
		"daily_quota_limit":  0,
		"base_multiplier":    1,
		"customer_type":      "retail",
		"pricing_profile_id": 0,
		"user_group_id":      0,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))

	UpdateUser(c)

	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, message=%q body=%s", payload.Message, recorder.Body.String())
	}

	var updated model.User
	if err := db.First(&updated, user.Id).Error; err != nil {
		t.Fatalf("load updated user: %v", err)
	}
	if updated.GroupId <= 0 {
		t.Fatalf("updated.GroupId = %d, want > 0", updated.GroupId)
	}
	if updated.DisplayName != "legacy-edited" {
		t.Fatalf("updated.DisplayName = %q, want edited value", updated.DisplayName)
	}
}

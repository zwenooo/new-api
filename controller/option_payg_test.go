package controller

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"one-api/common"
	"one-api/model"
	"one-api/setting/payg_setting"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newOptionPaygTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "option-payg.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?_pragma=foreign_keys(ON)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Option{},
		&model.User{},
		&model.Group{},
		&model.PaygProduct{},
		&model.PaygProductGroup{},
		&model.PaygProductRevision{},
		&model.PaygProductRevisionGroup{},
		&model.PaygUserBalance{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func withOptionPaygTestState(t *testing.T, db *gorm.DB) {
	t.Helper()

	oldDB := model.DB
	model.DB = db

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		"ClawBoxProductModeEnabled": "false",
		"ClawBoxProductId":          "0",
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		model.DB = oldDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})
}

func TestSyncPaygProductsToDBArchivesMissingProductInsteadOfDeleting(t *testing.T) {
	db := newOptionPaygTestDB(t)
	withOptionPaygTestState(t, db)

	if err := db.Create(&model.Group{
		Id:          1,
		Code:        "codex",
		DisplayName: "Codex",
		Ratio:       1,
		Enabled:     true,
	}).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&model.User{
		Id:       10,
		Username: "payg-user",
		Password: "password123",
		Status:   1,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	products := []payg_setting.PaygProduct{
		{
			Id:              1,
			Name:            "old",
			Enabled:         true,
			AllowedGroupIds: []int{1},
		},
		{
			Id:              2,
			Name:            "new",
			Enabled:         true,
			AllowedGroupIds: []int{1},
		},
	}
	if err := syncPaygProductsToDB(products); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if err := db.Create(&model.PaygUserBalance{
		UserId:          10,
		ProductId:       2,
		ProductName:     "new",
		AllowedGroupIds: model.JSONValue(`[1]`),
		RemainingQuota:  100,
	}).Error; err != nil {
		t.Fatalf("create balance: %v", err)
	}

	if err := syncPaygProductsToDB(products[:1]); err != nil {
		t.Fatalf("sync without product 2: %v", err)
	}

	var product model.PaygProduct
	if err := db.Where("id = ?", 2).First(&product).Error; err != nil {
		t.Fatalf("load archived product: %v", err)
	}
	if !product.Archived || product.Enabled {
		t.Fatalf("product archived/enabled = %v/%v, want true/false", product.Archived, product.Enabled)
	}

	var balance model.PaygUserBalance
	if err := db.Where("user_id = ? AND product_id = ?", 10, 2).First(&balance).Error; err != nil {
		t.Fatalf("load balance: %v", err)
	}

	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["payg.products"]
	common.OptionMapRWMutex.RUnlock()
	var optionProducts []payg_setting.PaygProduct
	if err := json.Unmarshal([]byte(raw), &optionProducts); err != nil {
		t.Fatalf("decode payg.products option: %v; raw=%q", err, raw)
	}
	if len(optionProducts) != 2 {
		t.Fatalf("option products length = %d, want 2; raw=%s", len(optionProducts), raw)
	}
	foundArchived := false
	for _, p := range optionProducts {
		if p.Id == 2 {
			foundArchived = p.Archived && !p.Enabled
		}
	}
	if !foundArchived {
		t.Fatalf("archived product 2 was not written back to payg.products: %s", raw)
	}
}

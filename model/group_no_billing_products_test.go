package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newGroupNoBillingReconcileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "group-no-billing.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Group{}, &RedemptionPreset{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestReconcileGroupNoBillingProductKeysTxRemovesMissingSubscriptionRefsAndDisablesNoBilling(t *testing.T) {
	db := newGroupNoBillingReconcileTestDB(t)

	group := Group{
		Code:                 "codex",
		DisplayName:          "codex",
		Ratio:                1,
		NoBilling:            true,
		NoBillingProductKeys: JSONValue(`["subscription:22","subscription:33"]`),
		UserSelectable:       true,
		Enabled:              true,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	updated, err := ReconcileGroupNoBillingProductKeysTx(db)
	if err != nil {
		t.Fatalf("reconcile refs: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var stored Group
	if err := db.First(&stored, group.Id).Error; err != nil {
		t.Fatalf("reload group: %v", err)
	}
	if stored.NoBilling {
		t.Fatalf("expected no_billing to be disabled when all product refs are gone")
	}
	if jsonValueHasElements(stored.NoBillingProductKeys) {
		t.Fatalf("expected no_billing_product_keys to be cleared, got %s", string(stored.NoBillingProductKeys))
	}
}

func TestReconcileGroupNoBillingProductKeysTxRewritesPresetKindWhenModeChanged(t *testing.T) {
	db := newGroupNoBillingReconcileTestDB(t)

	preset := RedemptionPreset{
		Id:      22,
		Name:    "tokens-22",
		Mode:    GroupNoBillingProductKindTokens,
		Enabled: true,
	}
	if err := db.Create(&preset).Error; err != nil {
		t.Fatalf("create preset: %v", err)
	}

	group := Group{
		Code:                 "codex",
		DisplayName:          "codex",
		Ratio:                1,
		NoBilling:            true,
		NoBillingProductKeys: JSONValue(`["subscription:22"]`),
		UserSelectable:       true,
		Enabled:              true,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	updated, err := ReconcileGroupNoBillingProductKeysTx(db)
	if err != nil {
		t.Fatalf("reconcile refs: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var stored Group
	if err := db.First(&stored, group.Id).Error; err != nil {
		t.Fatalf("reload group: %v", err)
	}
	if !stored.NoBilling {
		t.Fatalf("expected no_billing to remain enabled after kind remap")
	}
	refs, err := ParseGroupNoBillingProductKeysJSON(stored.NoBillingProductKeys)
	if err != nil {
		t.Fatalf("parse stored refs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs len = %d, want 1", len(refs))
	}
	if refs[0].Kind != GroupNoBillingProductKindTokens || refs[0].ProductId != 22 {
		t.Fatalf("unexpected ref after reconcile: %+v", refs[0])
	}
}

package model

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRelayDiscreteBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "relay-discrete-billing.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Group{}, &User{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestGetUserRelayDiscreteBillingStateFallsBackToLegacyGroupCodes(t *testing.T) {
	db := newRelayDiscreteBillingTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "legacy-a")
	groupB := createTestGroup(t, db, "legacy-b")

	paygGroups, err := MarshalGroupIDsJSON([]int{groupA.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON() error = %v", err)
	}
	payRequestGroups, err := MarshalGroupNamesJSON([]string{groupB.Code, groupA.Code})
	if err != nil {
		t.Fatalf("MarshalGroupNamesJSON() error = %v", err)
	}
	payTokenGroups, err := MarshalGroupIDsJSON([]int{groupB.Id})
	if err != nil {
		t.Fatalf("MarshalGroupIDsJSON() error = %v", err)
	}

	user := User{
		Username:                "relay-discrete",
		Password:                "password123",
		GroupId:                 groupA.Id,
		Group:                   groupA.Code,
		PayAsYouGoQuota:         90,
		PayAsYouGoAllowedGroups: paygGroups,
		PayRequestQuota:         7,
		PayRequestAllowedGroups: payRequestGroups,
		PayTokenQuota:           11,
		PayTokenAllowedGroups:   payTokenGroups,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	state, err := GetUserRelayDiscreteBillingState(user.Id)
	if err != nil {
		t.Fatalf("GetUserRelayDiscreteBillingState() error = %v", err)
	}
	if state == nil {
		t.Fatal("GetUserRelayDiscreteBillingState() = nil, want non-nil")
	}
	if state.PaygQuota != 90 {
		t.Fatalf("state.PaygQuota = %d, want 90", state.PaygQuota)
	}
	if state.PayRequestQuota != 7 {
		t.Fatalf("state.PayRequestQuota = %d, want 7", state.PayRequestQuota)
	}
	if state.PayTokenQuota != 11 {
		t.Fatalf("state.PayTokenQuota = %d, want 11", state.PayTokenQuota)
	}
	if !reflect.DeepEqual(state.PaygGroups, []int{groupA.Id}) {
		t.Fatalf("state.PaygGroups = %#v, want %#v", state.PaygGroups, []int{groupA.Id})
	}
	if !reflect.DeepEqual(state.PayRequestGroups, []int{groupA.Id, groupB.Id}) {
		t.Fatalf("state.PayRequestGroups = %#v, want %#v", state.PayRequestGroups, []int{groupA.Id, groupB.Id})
	}
	if !reflect.DeepEqual(state.PayTokenGroups, []int{groupB.Id}) {
		t.Fatalf("state.PayTokenGroups = %#v, want %#v", state.PayTokenGroups, []int{groupB.Id})
	}
}

package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newUserSubscriptionCompatTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "user-subscription-compat.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &UserSubscription{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestPostConsumeUserSubscriptionDeltaAdjustsRemainingAndUserQuota(t *testing.T) {
	db := newUserSubscriptionCompatTestDB(t)
	withModelDB(t, db)

	user := User{
		Username:    "legacy-subscription",
		Password:    "password123",
		Quota:       1000,
		RedeemQuota: 1000,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	sub := UserSubscription{
		UserId:         user.Id,
		BillingUnit:    UserSubscriptionBillingUnitQuota,
		TotalQuota:     1000,
		RemainingQuota: 1000,
		Credited:       true,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	if err := PostConsumeUserSubscriptionDelta(sub.Id, 200); err != nil {
		t.Fatalf("PostConsumeUserSubscriptionDelta(consume) error = %v", err)
	}

	var storedSub UserSubscription
	if err := db.First(&storedSub, sub.Id).Error; err != nil {
		t.Fatalf("reload subscription after consume: %v", err)
	}
	if storedSub.RemainingQuota != 800 {
		t.Fatalf("remaining_quota after consume = %d, want 800", storedSub.RemainingQuota)
	}

	var storedUser User
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after consume: %v", err)
	}
	if storedUser.Quota != 800 {
		t.Fatalf("user quota after consume = %d, want 800", storedUser.Quota)
	}
	if storedUser.RedeemQuota != 800 {
		t.Fatalf("user redeem_quota after consume = %d, want 800", storedUser.RedeemQuota)
	}

	if err := PostConsumeUserSubscriptionDelta(sub.Id, -150); err != nil {
		t.Fatalf("PostConsumeUserSubscriptionDelta(refund) error = %v", err)
	}

	if err := db.First(&storedSub, sub.Id).Error; err != nil {
		t.Fatalf("reload subscription after refund: %v", err)
	}
	if storedSub.RemainingQuota != 950 {
		t.Fatalf("remaining_quota after refund = %d, want 950", storedSub.RemainingQuota)
	}
	if err := db.First(&storedUser, user.Id).Error; err != nil {
		t.Fatalf("reload user after refund: %v", err)
	}
	if storedUser.Quota != 950 {
		t.Fatalf("user quota after refund = %d, want 950", storedUser.Quota)
	}
	if storedUser.RedeemQuota != 950 {
		t.Fatalf("user redeem_quota after refund = %d, want 950", storedUser.RedeemQuota)
	}
}

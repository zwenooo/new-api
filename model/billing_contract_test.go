package model

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newBillingContractTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "billing-contract.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Group{},
		&RedemptionPreset{},
		&SubscriptionProductGroup{},
		&RedemptionPresetRevision{},
		&RedemptionPresetRevisionGroup{},
		&RedemptionPresetRevisionGroupDailyLimit{},
		&UserSubscription{},
		&UserSubscriptionGroup{},
		&UserSubscriptionPresetRevisionBinding{},
		&SubscriptionProductGroupDailyLimit{},
		&RedemptionGroupDailyLimit{},
		&UserSubscriptionGroupDailyLimit{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func createBillingTestPreset(t *testing.T, db *gorm.DB, id int) {
	t.Helper()

	preset := RedemptionPreset{
		Id:   id,
		Name: fmt.Sprintf("preset-%d", id),
		Mode: "subscription",
	}
	if err := db.Create(&preset).Error; err != nil {
		t.Fatalf("create preset %d: %v", id, err)
	}
}

func createBillingTestRevision(t *testing.T, db *gorm.DB, id int, presetID int) {
	t.Helper()

	revision := RedemptionPresetRevision{
		Id:           id,
		PresetId:     presetID,
		RevisionNo:   1,
		SnapshotTime: 1,
		Name:         fmt.Sprintf("preset-%d-r1", presetID),
		Mode:         "subscription",
	}
	if err := db.Create(&revision).Error; err != nil {
		t.Fatalf("create revision %d: %v", id, err)
	}
}

func TestResolveEffectiveAllowedGroupsTxPrefersCurrentProductThenFallsBack(t *testing.T) {
	db := newBillingContractTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "a")
	groupB := createTestGroup(t, db, "b")
	groupC := createTestGroup(t, db, "c")

	createBillingTestPreset(t, db, 101)
	if err := db.Create(&SubscriptionProductGroup{ProductId: 101, GroupId: groupA.Id}).Error; err != nil {
		t.Fatalf("create subscription product group: %v", err)
	}

	createBillingTestRevision(t, db, 201, 999)
	if err := db.Create(&RedemptionPresetRevisionGroup{RevisionId: 201, GroupId: groupB.Id}).Error; err != nil {
		t.Fatalf("create revision group: %v", err)
	}

	got, err := resolveEffectiveAllowedGroupsTx(db, []effectiveAllowedGroupTarget{
		{OwnerID: 1, ProductID: 101, RevisionID: 201, SnapshotGroupIDs: []int{groupC.Id}},
		{OwnerID: 2, ProductID: 999, RevisionID: 201, SnapshotGroupIDs: []int{groupC.Id}},
		{OwnerID: 3, ProductID: 998, SnapshotGroupIDs: []int{groupC.Id}},
		{OwnerID: 4, RevisionID: 201},
	}, effectiveAllowedGroupResolverOptions{
		OwnerLabel:    "订阅",
		SnapshotLabel: "订阅快照",
	})
	if err != nil {
		t.Fatalf("resolveEffectiveAllowedGroupsTx() error = %v", err)
	}

	want := map[int][]int{
		1: {groupA.Id},
		2: {groupB.Id},
		3: {groupC.Id},
		4: {groupB.Id},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveEffectiveAllowedGroupsTx() = %#v, want %#v", got, want)
	}
}

func TestResolveEffectiveAllowedGroupsTxRejectsConfiguredProductWithoutGroups(t *testing.T) {
	db := newBillingContractTestDB(t)
	withModelDB(t, db)

	groupB := createTestGroup(t, db, "b")
	groupC := createTestGroup(t, db, "c")

	createBillingTestPreset(t, db, 101)
	createBillingTestRevision(t, db, 201, 101)
	if err := db.Create(&RedemptionPresetRevisionGroup{RevisionId: 201, GroupId: groupB.Id}).Error; err != nil {
		t.Fatalf("create revision group: %v", err)
	}

	_, err := resolveEffectiveAllowedGroupsTx(db, []effectiveAllowedGroupTarget{
		{OwnerID: 1, ProductID: 101, RevisionID: 201, SnapshotGroupIDs: []int{groupC.Id}},
	}, effectiveAllowedGroupResolverOptions{
		OwnerLabel:    "订阅",
		SnapshotLabel: "订阅快照",
	})
	if err == nil {
		t.Fatal("resolveEffectiveAllowedGroupsTx() error = nil, want product-group config error")
	}
	if !strings.Contains(err.Error(), "未配置可用分组") {
		t.Fatalf("resolveEffectiveAllowedGroupsTx() error = %q, want contains %q", err.Error(), "未配置可用分组")
	}
}

func TestLoadEffectiveUserSubscriptionGroupDailyLimitsTxPrecedenceAndScaling(t *testing.T) {
	db := newBillingContractTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "a")

	createBillingTestPreset(t, db, 101)
	createBillingTestRevision(t, db, 201, 999)

	subs := []UserSubscription{
		{Id: 1, SourcePresetId: 101, SourceRedemptionId: 501, BillingUnit: UserSubscriptionBillingUnitQuota},
		{Id: 2, SourcePresetId: 101, BillingUnit: UserSubscriptionBillingUnitQuota},
		{Id: 3, SourcePresetId: 999, SourceRedemptionId: 502, BillingUnit: UserSubscriptionBillingUnitQuota},
		{Id: 4, BillingUnit: UserSubscriptionBillingUnitQuota},
		{Id: 5, SourceRedemptionId: 502, BillingUnit: UserSubscriptionBillingUnitQuota},
		{Id: 6, SourcePresetId: 999, BillingUnit: UserSubscriptionBillingUnitTokens},
	}
	if err := db.Create(&subs).Error; err != nil {
		t.Fatalf("create subscriptions: %v", err)
	}

	rows := []interface{}{
		&SubscriptionProductGroupDailyLimit{ProductId: 101, GroupId: groupA.Id, DailyLimitQuota: 7},
		&RedemptionGroupDailyLimit{RedemptionId: 501, GroupId: groupA.Id, DailyLimitQuota: 3},
		&RedemptionGroupDailyLimit{RedemptionId: 502, GroupId: groupA.Id, DailyLimitQuota: 13},
		&UserSubscriptionGroupDailyLimit{SubscriptionId: 3, GroupId: groupA.Id, DailyLimitQuota: 5},
		&UserSubscriptionGroupDailyLimit{SubscriptionId: 4, GroupId: groupA.Id, DailyLimitQuota: 5},
		&RedemptionPresetRevisionGroupDailyLimit{RevisionId: 201, GroupId: groupA.Id, DailyLimitQuota: 11},
		&UserSubscriptionPresetRevisionBinding{SubscriptionId: 6, PresetId: 999, RevisionId: 201},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed billing rows: %v", err)
		}
	}

	gotLimits, gotSources, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(db, subs)
	if err != nil {
		t.Fatalf("loadEffectiveUserSubscriptionGroupDailyLimitsTx() error = %v", err)
	}

	wantLimits := map[int]map[int]int{
		1: {groupA.Id: 3},
		2: {groupA.Id: 7},
		3: {groupA.Id: 13},
		4: {groupA.Id: 5},
		5: {groupA.Id: 13},
		6: {groupA.Id: discreteUnitsFromDisplayInt(11)},
	}
	if !reflect.DeepEqual(gotLimits, wantLimits) {
		t.Fatalf("effective daily limits = %#v, want %#v", gotLimits, wantLimits)
	}

	wantSources := map[int]userSubscriptionEffectiveGroupDailyLimitSource{
		1: {Kind: "redemption", SourceId: 501},
		2: {Kind: "preset", SourceId: 101},
		3: {Kind: "redemption", SourceId: 502},
		4: {Kind: "subscription", SourceId: 4},
		5: {Kind: "redemption", SourceId: 502},
		6: {Kind: "preset_revision", SourceId: 999, RevisionId: 201},
	}
	if !reflect.DeepEqual(gotSources, wantSources) {
		t.Fatalf("effective daily limit sources = %#v, want %#v", gotSources, wantSources)
	}
}

func TestLoadEffectiveUserSubscriptionGroupDailyLimitsTxConfiguredPresetWithoutLimitsDoesNotFallback(t *testing.T) {
	db := newBillingContractTestDB(t)
	withModelDB(t, db)

	groupA := createTestGroup(t, db, "a")

	createBillingTestPreset(t, db, 101)
	createBillingTestRevision(t, db, 201, 101)

	sub := UserSubscription{Id: 1, SourcePresetId: 101, BillingUnit: UserSubscriptionBillingUnitQuota}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	rows := []interface{}{
		&UserSubscriptionGroupDailyLimit{SubscriptionId: 1, GroupId: groupA.Id, DailyLimitQuota: 5},
		&RedemptionPresetRevisionGroupDailyLimit{RevisionId: 201, GroupId: groupA.Id, DailyLimitQuota: 11},
		&UserSubscriptionPresetRevisionBinding{SubscriptionId: 1, PresetId: 101, RevisionId: 201},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed billing rows: %v", err)
		}
	}

	gotLimits, gotSources, err := loadEffectiveUserSubscriptionGroupDailyLimitsTx(db, []UserSubscription{sub})
	if err != nil {
		t.Fatalf("loadEffectiveUserSubscriptionGroupDailyLimitsTx() error = %v", err)
	}
	if _, ok := gotLimits[sub.Id]; ok {
		t.Fatalf("effective daily limits unexpectedly fell back: %#v", gotLimits[sub.Id])
	}
	if _, ok := gotSources[sub.Id]; ok {
		t.Fatalf("effective daily limit source unexpectedly set: %#v", gotSources[sub.Id])
	}
}

func TestResolveUserSubscriptionSummaryDailyLimitContracts(t *testing.T) {
	t.Run("group_daily_limit_mode_wins", func(t *testing.T) {
		sub := UserSubscription{
			Id:                  1,
			DailyQuotaLimit:     99,
			DailyQuotaUsed:      88,
			DailyQuotaResetDate: 20260413,
		}

		used, dailyLimit, groupDailyLimits := resolveUserSubscriptionSummaryDailyLimit(
			sub,
			20260414,
			map[int]map[int]int{1: {2: 40, 1: 20}},
			map[int]int{1: 60},
			map[int]int{1: 9},
		)

		if used != 9 {
			t.Fatalf("used = %d, want 9", used)
		}
		if dailyLimit != 60 {
			t.Fatalf("dailyLimit = %d, want 60", dailyLimit)
		}
		wantGroupDailyLimits := []GroupDailyQuotaLimit{
			{GroupId: 1, DailyQuotaLimit: 20},
			{GroupId: 2, DailyQuotaLimit: 40},
		}
		if !reflect.DeepEqual(groupDailyLimits, wantGroupDailyLimits) {
			t.Fatalf("groupDailyLimits = %#v, want %#v", groupDailyLimits, wantGroupDailyLimits)
		}
	})

	t.Run("legacy_daily_limit_resets_on_new_day", func(t *testing.T) {
		sub := UserSubscription{
			Id:                  2,
			DailyQuotaLimit:     20,
			DailyQuotaUsed:      8,
			DailyQuotaResetDate: 20260413,
		}

		used, dailyLimit, groupDailyLimits := resolveUserSubscriptionSummaryDailyLimit(
			sub,
			20260414,
			nil,
			nil,
			nil,
		)

		if used != 0 {
			t.Fatalf("used = %d, want 0", used)
		}
		if dailyLimit != 20 {
			t.Fatalf("dailyLimit = %d, want 20", dailyLimit)
		}
		if groupDailyLimits != nil {
			t.Fatalf("groupDailyLimits = %#v, want nil", groupDailyLimits)
		}
	})
}

func TestBuildUserSubscriptionBreakdownSummaryContracts(t *testing.T) {
	createdAt := time.Unix(1710000000, 0)
	sub := UserSubscription{
		Id:             7,
		Source:         "order",
		TotalQuota:     100,
		RemainingQuota: 60,
		CreatedAt:      createdAt,
	}

	var missingGroups []int
	summary, err := buildUserSubscriptionBreakdownSummary(
		sub,
		[]int{2, 1},
		30,
		70,
		[]GroupDailyQuotaLimit{
			{GroupId: 2, DailyQuotaLimit: 40},
			{GroupId: 1, DailyQuotaLimit: 20},
		},
		map[int]int{1: 5},
		func(_ UserSubscription, groupID int) {
			missingGroups = append(missingGroups, groupID)
		},
		subscriptionBreakdownOrderInfo{TradeNo: "T-1", Quantity: 2},
		"额度订阅",
		"兑换码",
	)
	if err != nil {
		t.Fatalf("buildUserSubscriptionBreakdownSummary() error = %v", err)
	}

	if summary.SourceOrderTradeNo != "T-1" {
		t.Fatalf("SourceOrderTradeNo = %q, want %q", summary.SourceOrderTradeNo, "T-1")
	}
	if summary.SourceOrderQuantity != 2 {
		t.Fatalf("SourceOrderQuantity = %d, want 2", summary.SourceOrderQuantity)
	}
	if summary.SourcePresetName != "额度订阅" {
		t.Fatalf("SourcePresetName = %q, want %q", summary.SourcePresetName, "额度订阅")
	}
	if summary.SourceRedemptionName != "兑换码" {
		t.Fatalf("SourceRedemptionName = %q, want %q", summary.SourceRedemptionName, "兑换码")
	}
	if summary.ConsumedQuota != 40 {
		t.Fatalf("ConsumedQuota = %d, want 40", summary.ConsumedQuota)
	}
	if summary.RemainingQuota != 60 {
		t.Fatalf("RemainingQuota = %d, want 60", summary.RemainingQuota)
	}
	if summary.StartAt != createdAt.Unix() {
		t.Fatalf("StartAt = %d, want %d", summary.StartAt, createdAt.Unix())
	}
	if !reflect.DeepEqual(summary.AllowedGroupIds, []int{2, 1}) {
		t.Fatalf("AllowedGroupIds = %#v, want %#v", summary.AllowedGroupIds, []int{2, 1})
	}
	wantBreakdown := []SubscriptionGroupQuotaBreakdown{
		{GroupId: 1, DailyQuotaUsed: 5, DailyQuotaAvailable: 15, DailyQuotaLimit: 20},
		{GroupId: 2, DailyQuotaUsed: 0, DailyQuotaAvailable: 40, DailyQuotaLimit: 40},
	}
	if !reflect.DeepEqual(summary.GroupQuotaBreakdown, wantBreakdown) {
		t.Fatalf("GroupQuotaBreakdown = %#v, want %#v", summary.GroupQuotaBreakdown, wantBreakdown)
	}
	if len(missingGroups) != 0 {
		t.Fatalf("missingGroups = %#v, want empty", missingGroups)
	}
}

func TestBuildUserSubscriptionBreakdownSummaryReportsMissingGroupConfig(t *testing.T) {
	sub := UserSubscription{
		Id:             8,
		TotalQuota:     100,
		RemainingQuota: 60,
	}

	var missingGroups []int
	summary, err := buildUserSubscriptionBreakdownSummary(
		sub,
		[]int{2, 1},
		30,
		70,
		[]GroupDailyQuotaLimit{
			{GroupId: 2, DailyQuotaLimit: 40},
		},
		nil,
		func(_ UserSubscription, groupID int) {
			missingGroups = append(missingGroups, groupID)
		},
		subscriptionBreakdownOrderInfo{},
		"",
		"",
	)
	if err != nil {
		t.Fatalf("buildUserSubscriptionBreakdownSummary() error = %v", err)
	}

	wantBreakdown := []SubscriptionGroupQuotaBreakdown{
		{GroupId: 2, DailyQuotaUsed: 0, DailyQuotaAvailable: 40, DailyQuotaLimit: 40},
	}
	if !reflect.DeepEqual(summary.GroupQuotaBreakdown, wantBreakdown) {
		t.Fatalf("GroupQuotaBreakdown = %#v, want %#v", summary.GroupQuotaBreakdown, wantBreakdown)
	}
	if !reflect.DeepEqual(missingGroups, []int{1}) {
		t.Fatalf("missingGroups = %#v, want %#v", missingGroups, []int{1})
	}
}

func TestEntitlementSemanticsKinds(t *testing.T) {
	tests := []struct {
		name      string
		mode      EntitlementMode
		unit      EntitlementUnit
		kind      EntitlementKind
		label     string
		buildKind func() EntitlementKind
	}{
		{
			name:  "subscription_credit",
			mode:  EntitlementModeSubscription,
			unit:  EntitlementUnitCredit,
			kind:  EntitlementKindSubscriptionCredit,
			label: "订阅额度",
			buildKind: func() EntitlementKind {
				return UserSubscription{BillingUnit: UserSubscriptionBillingUnitQuota}.EntitlementKind()
			},
		},
		{
			name:  "subscription_request",
			mode:  EntitlementModeSubscription,
			unit:  EntitlementUnitRequest,
			kind:  EntitlementKindSubscriptionRequest,
			label: "订阅次数",
			buildKind: func() EntitlementKind {
				return UserRequestSubscription{}.EntitlementKind()
			},
		},
		{
			name:  "subscription_token",
			mode:  EntitlementModeSubscription,
			unit:  EntitlementUnitToken,
			kind:  EntitlementKindSubscriptionToken,
			label: "订阅Token",
			buildKind: func() EntitlementKind {
				return UserSubscription{BillingUnit: UserSubscriptionBillingUnitTokens}.EntitlementKind()
			},
		},
		{
			name:  "prepaid_credit",
			mode:  EntitlementModePrepaid,
			unit:  EntitlementUnitCredit,
			kind:  EntitlementKindPrepaidCredit,
			label: "预付费额度",
			buildKind: func() EntitlementKind {
				return PaygUserBalance{}.EntitlementKind()
			},
		},
		{
			name:  "prepaid_request",
			mode:  EntitlementModePrepaid,
			unit:  EntitlementUnitRequest,
			kind:  EntitlementKindPrepaidRequest,
			label: "预付费次数",
			buildKind: func() EntitlementKind {
				return PayRequestUserBalance{}.EntitlementKind()
			},
		},
		{
			name:  "prepaid_token",
			mode:  EntitlementModePrepaid,
			unit:  EntitlementUnitToken,
			kind:  EntitlementKindPrepaidToken,
			label: "预付费Token",
			buildKind: func() EntitlementKind {
				return PayTokenUserBalance{}.EntitlementKind()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.buildKind()
			if got != tt.kind {
				t.Fatalf("kind = %q, want %q", got, tt.kind)
			}
			if got.Mode() != tt.mode {
				t.Fatalf("mode = %q, want %q", got.Mode(), tt.mode)
			}
			if got.Unit() != tt.unit {
				t.Fatalf("unit = %q, want %q", got.Unit(), tt.unit)
			}
			if got.Label() != tt.label {
				t.Fatalf("label = %q, want %q", got.Label(), tt.label)
			}
		})
	}

	if got := (UserSubscription{BillingUnit: "unknown"}).EntitlementKind(); got != "" {
		t.Fatalf("unknown billing unit kind = %q, want empty", got)
	}
}

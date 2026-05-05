package service

import (
	"testing"

	"one-api/types"
)

func TestGenerateMjOtherInfoIncludesGroupRatioSourceAndBaseMultiplierApplied(t *testing.T) {
	other := GenerateMjOtherInfo(types.PerCallPriceData{
		ModelPrice:   1.2,
		Quota:        10,
		VisibleQuota: 10,
		CostQuota:    10,
		GroupRatioInfo: types.GroupRatioInfo{
			EffectiveGroupRatio:   0.4,
			PublicGroupRatio:      2.0,
			PrivateGroupRatio:     0.4,
			GroupSpecialRatio:     0.4,
			HasSpecialRatio:       true,
			BaseMultiplierApplied: false,
			Source:                "legacy",
		},
	})

	if got := other["group_ratio_source"]; got != "legacy" {
		t.Fatalf("group_ratio_source = %v, want legacy", got)
	}
	if got := other["base_multiplier_applied"]; got != false {
		t.Fatalf("base_multiplier_applied = %v, want false", got)
	}
}

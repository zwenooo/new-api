package controller

import "testing"

func TestBuildPricingGroupRatioKeepSetIncludesBillableNonSelectableGroups(t *testing.T) {
	usableGroup := map[int]string{
		1: "public",
		3: "retail",
	}
	extraGroupIDs := []int{2, 3, -1, 0}

	keepSet := buildPricingGroupRatioKeepSet(usableGroup, extraGroupIDs)

	if len(keepSet) != 3 {
		t.Fatalf("len(keepSet) = %d, want 3", len(keepSet))
	}
	for _, groupID := range []int{1, 2, 3} {
		if _, ok := keepSet[groupID]; !ok {
			t.Fatalf("keepSet missing group %d", groupID)
		}
	}
}

func TestFilterPricingGroupRatiosKeepsOnlyAllowedGroups(t *testing.T) {
	groupRatio := map[int]float64{
		1: 1,
		2: 0.75,
		4: 1.2,
	}
	keepSet := map[int]struct{}{
		1: {},
		2: {},
	}

	filtered := filterPricingGroupRatios(groupRatio, keepSet)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[1] != 1 {
		t.Fatalf("filtered[1] = %v, want 1", filtered[1])
	}
	if filtered[2] != 0.75 {
		t.Fatalf("filtered[2] = %v, want 0.75", filtered[2])
	}
	if _, ok := filtered[4]; ok {
		t.Fatal("filtered unexpectedly contains group 4")
	}
}

package middleware

import "testing"

func TestChooseTokenGroupByModelAccess(t *testing.T) {
	tests := []struct {
		name             string
		currentGroupID   int
		allowedGroupIDs  []int
		supportedGroupID map[int]bool
		billableGroupID  map[int]bool
		expectedGroupID  int
	}{
		{
			name:             "keep current group when it supports model",
			currentGroupID:   2,
			allowedGroupIDs:  []int{2, 3},
			supportedGroupID: map[int]bool{2: true, 3: true},
			billableGroupID:  map[int]bool{2: true, 3: true},
			expectedGroupID:  2,
		},
		{
			name:             "switch to later group when current group does not support model",
			currentGroupID:   2,
			allowedGroupIDs:  []int{2, 3},
			supportedGroupID: map[int]bool{3: true},
			billableGroupID:  map[int]bool{3: true},
			expectedGroupID:  3,
		},
		{
			name:             "drop duplicates and invalid ids",
			currentGroupID:   1,
			allowedGroupIDs:  []int{0, 1, 1, -1, 5},
			supportedGroupID: map[int]bool{5: true},
			billableGroupID:  map[int]bool{5: true},
			expectedGroupID:  5,
		},
		{
			name:             "prefer later group when current group is not billable",
			currentGroupID:   1,
			allowedGroupIDs:  []int{1, 2},
			supportedGroupID: map[int]bool{1: true, 2: true},
			billableGroupID:  map[int]bool{2: true},
			expectedGroupID:  2,
		},
		{
			name:             "keep current group when no billable group is available",
			currentGroupID:   7,
			allowedGroupIDs:  []int{7, 8},
			supportedGroupID: map[int]bool{7: true, 8: true},
			billableGroupID:  map[int]bool{},
			expectedGroupID:  7,
		},
		{
			name:             "keep current group when allowed groups empty",
			currentGroupID:   4,
			allowedGroupIDs:  nil,
			supportedGroupID: map[int]bool{4: true},
			billableGroupID:  map[int]bool{4: true},
			expectedGroupID:  4,
		},
		{
			name:             "keep current group when no allowed group supports model",
			currentGroupID:   4,
			allowedGroupIDs:  []int{4, 5},
			supportedGroupID: map[int]bool{},
			billableGroupID:  map[int]bool{5: true},
			expectedGroupID:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseTokenGroupByModelAccess(
				tt.currentGroupID,
				tt.allowedGroupIDs,
				func(groupID int) bool {
					return tt.supportedGroupID[groupID]
				},
				func(groupID int) bool {
					return tt.billableGroupID[groupID]
				},
			)
			if got != tt.expectedGroupID {
				t.Fatalf("chooseTokenGroupByModelAccess() = %d, want %d", got, tt.expectedGroupID)
			}
		})
	}
}

func TestChooseTokenGroupCandidatesByModelAccess(t *testing.T) {
	tests := []struct {
		name             string
		currentGroupID   int
		allowedGroupIDs  []int
		supportedGroupID map[int]bool
		billableGroupID  map[int]bool
		expectedGroupIDs []int
	}{
		{
			name:             "keep token order for billable supported groups",
			currentGroupID:   1,
			allowedGroupIDs:  []int{1, 2, 3},
			supportedGroupID: map[int]bool{1: true, 2: true, 3: true},
			billableGroupID:  map[int]bool{1: true, 3: true},
			expectedGroupIDs: []int{1, 3},
		},
		{
			name:             "skip unsupported groups even if billable",
			currentGroupID:   2,
			allowedGroupIDs:  []int{2, 4, 5},
			supportedGroupID: map[int]bool{4: true, 5: true},
			billableGroupID:  map[int]bool{2: true, 5: true},
			expectedGroupIDs: []int{5},
		},
		{
			name:             "return empty candidates when none are billable",
			currentGroupID:   7,
			allowedGroupIDs:  []int{7, 8, 9},
			supportedGroupID: map[int]bool{7: true, 9: true},
			billableGroupID:  map[int]bool{},
			expectedGroupIDs: nil,
		},
		{
			name:             "fallback to current group when nothing supports",
			currentGroupID:   6,
			allowedGroupIDs:  []int{1, 2},
			supportedGroupID: map[int]bool{},
			billableGroupID:  map[int]bool{1: true, 2: true},
			expectedGroupIDs: []int{6},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseTokenGroupCandidatesByModelAccess(
				tt.currentGroupID,
				tt.allowedGroupIDs,
				func(groupID int) bool {
					return tt.supportedGroupID[groupID]
				},
				func(groupID int) bool {
					return tt.billableGroupID[groupID]
				},
			)
			if len(got) != len(tt.expectedGroupIDs) {
				t.Fatalf("chooseTokenGroupCandidatesByModelAccess() len = %d, want %d, got=%v", len(got), len(tt.expectedGroupIDs), got)
			}
			for idx := range got {
				if got[idx] != tt.expectedGroupIDs[idx] {
					t.Fatalf("chooseTokenGroupCandidatesByModelAccess() = %v, want %v", got, tt.expectedGroupIDs)
				}
			}
		})
	}
}

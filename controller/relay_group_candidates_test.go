package controller

import (
	"errors"
	"reflect"
	"testing"

	"one-api/middleware"
	"one-api/model"
	"one-api/types"
)

func TestBuildTokenGroupCandidates(t *testing.T) {
	tests := []struct {
		name              string
		usingGroupID      int
		resolvedGroupIDs  []int
		allowedGroupIDs   []int
		expectedCandidate []int
	}{
		{
			name:              "prefer resolved candidates and keep current first",
			usingGroupID:      2,
			resolvedGroupIDs:  []int{2, 3, 4},
			allowedGroupIDs:   []int{1, 2, 3},
			expectedCandidate: []int{2, 3, 4},
		},
		{
			name:              "drop already tried earlier resolved candidates",
			usingGroupID:      3,
			resolvedGroupIDs:  []int{2, 3, 4},
			allowedGroupIDs:   []int{2, 3, 4},
			expectedCandidate: []int{3, 4},
		},
		{
			name:              "fallback_to_allowed_groups_when_no_resolved_candidates",
			usingGroupID:      9,
			allowedGroupIDs:   []int{1, 2, 3},
			expectedCandidate: []int{1, 2, 3},
		},
		{
			name:              "drop_non_positive_using_group",
			usingGroupID:      2,
			allowedGroupIDs:   []int{0, 2, 2, -1, 1},
			expectedCandidate: []int{2, 1},
		},
		{
			name:              "no_using_group",
			usingGroupID:      0,
			allowedGroupIDs:   []int{3, 1},
			expectedCandidate: []int{3, 1},
		},
		{
			name:              "empty_returns_nil",
			usingGroupID:      0,
			allowedGroupIDs:   nil,
			expectedCandidate: nil,
		},
		{
			name:              "fallback_to_using_group_when_allowed_groups_empty",
			usingGroupID:      7,
			allowedGroupIDs:   nil,
			expectedCandidate: []int{7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTokenGroupCandidates(tt.usingGroupID, tt.resolvedGroupIDs, tt.allowedGroupIDs)
			if !reflect.DeepEqual(got, tt.expectedCandidate) {
				t.Fatalf("buildTokenGroupCandidates() = %#v, want %#v", got, tt.expectedCandidate)
			}
		})
	}
}

func TestBuildTokenGroupCandidatesFromRuntimeSelectionPrefersAuthorityCandidates(t *testing.T) {
	authority := &middleware.RuntimeSelectionAuthority{
		CurrentGroupID:    3,
		CandidateGroupIDs: []int{3, 5, 7},
	}

	got := buildTokenGroupCandidatesFromRuntimeSelection(authority, 3, []int{1, 2, 3})
	want := []int{3, 5, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildTokenGroupCandidatesFromRuntimeSelection() = %#v, want %#v", got, want)
	}
}

func TestBuildBillingCandidatesUsesGroupPriorityAcrossBuckets(t *testing.T) {
	groupCandidates := []int{2, 1}

	candidates := buildBillingCandidates(groupCandidates, []billingCandidateSpec{
		{Enabled: true, Bucket: "subscription", AllowedGroups: map[int]struct{}{1: {}, 2: {}}},
		{Enabled: true, Bucket: "payg", AllowedGroups: map[int]struct{}{1: {}, 2: {}}},
	})

	want := []billingCandidate{
		{Bucket: "subscription", GroupID: 2},
		{Bucket: "payg", GroupID: 2},
		{Bucket: "subscription", GroupID: 1},
		{Bucket: "payg", GroupID: 1},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("buildBillingCandidates() = %#v, want %#v", candidates, want)
	}
}

func TestBuildBillingCandidatesSkipsDisabledBuckets(t *testing.T) {
	groupCandidates := []int{3, 2, 1}

	candidates := buildBillingCandidates(groupCandidates, []billingCandidateSpec{
		{Enabled: false, Bucket: "subscription", AllowedGroups: map[int]struct{}{1: {}, 2: {}, 3: {}}},
		{Enabled: true, Bucket: "payg", AllowedGroups: map[int]struct{}{2: {}, 3: {}}},
	})

	want := []billingCandidate{
		{Bucket: "payg", GroupID: 3},
		{Bucket: "payg", GroupID: 2},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("buildBillingCandidates() = %#v, want %#v", candidates, want)
	}
}

func TestBuildBillingCandidatesKeepsNoBillingAheadOfGenericFreeWithoutDuplicateGroupFreeRetry(t *testing.T) {
	groupCandidates := []int{1, 2}

	candidates := buildBillingCandidates(groupCandidates, []billingCandidateSpec{
		{Enabled: true, Bucket: model.UserQuotaBucketFree, AllowedGroups: map[int]struct{}{1: {}}},
		{Enabled: true, Bucket: model.UserQuotaBucketSubscription, AllowedGroups: map[int]struct{}{1: {}, 2: {}}},
		{Enabled: true, Bucket: model.UserQuotaBucketFree, AllowedGroups: map[int]struct{}{2: {}}},
	})

	want := []billingCandidate{
		{Bucket: model.UserQuotaBucketFree, GroupID: 1},
		{Bucket: model.UserQuotaBucketSubscription, GroupID: 1},
		{Bucket: model.UserQuotaBucketSubscription, GroupID: 2},
		{Bucket: model.UserQuotaBucketFree, GroupID: 2},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("buildBillingCandidates() = %#v, want %#v", candidates, want)
	}
}

func TestBuildBillingCandidatesPreservesCurrentRelayBucketPriority(t *testing.T) {
	groupCandidates := []int{9, 3}

	allGroups := map[int]struct{}{3: {}, 9: {}}
	candidates := buildBillingCandidates(groupCandidates, []billingCandidateSpec{
		{Enabled: true, Bucket: model.UserQuotaBucketFree, AllowedGroups: allGroups},
		{Enabled: true, Bucket: model.UserQuotaBucketRequest, AllowedGroups: allGroups},
		{Enabled: true, Bucket: model.UserQuotaBucketPayRequest, AllowedGroups: allGroups},
		{Enabled: true, Bucket: model.UserQuotaBucketTokens, AllowedGroups: allGroups},
		{Enabled: true, Bucket: model.UserQuotaBucketPayToken, AllowedGroups: allGroups},
		{Enabled: true, Bucket: model.UserQuotaBucketSubscription, AllowedGroups: allGroups},
		{Enabled: true, Bucket: model.UserQuotaBucketPayg, AllowedGroups: allGroups},
	})

	want := []billingCandidate{
		{Bucket: model.UserQuotaBucketFree, GroupID: 9},
		{Bucket: model.UserQuotaBucketRequest, GroupID: 9},
		{Bucket: model.UserQuotaBucketPayRequest, GroupID: 9},
		{Bucket: model.UserQuotaBucketTokens, GroupID: 9},
		{Bucket: model.UserQuotaBucketPayToken, GroupID: 9},
		{Bucket: model.UserQuotaBucketSubscription, GroupID: 9},
		{Bucket: model.UserQuotaBucketPayg, GroupID: 9},
		{Bucket: model.UserQuotaBucketFree, GroupID: 3},
		{Bucket: model.UserQuotaBucketRequest, GroupID: 3},
		{Bucket: model.UserQuotaBucketPayRequest, GroupID: 3},
		{Bucket: model.UserQuotaBucketTokens, GroupID: 3},
		{Bucket: model.UserQuotaBucketPayToken, GroupID: 3},
		{Bucket: model.UserQuotaBucketSubscription, GroupID: 3},
		{Bucket: model.UserQuotaBucketPayg, GroupID: 3},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("buildBillingCandidates() = %#v, want %#v", candidates, want)
	}
}

func TestPreferBillingAttemptErrorKeepsMoreInformativeErrorOverGenericFree(t *testing.T) {
	current := types.NewErrorWithStatusCode(errors.New("订阅额度不足"), types.ErrorCodeInsufficientUserQuota, 403)
	next := types.NewErrorWithStatusCode(errors.New("用户额度不足"), types.ErrorCodeInsufficientUserQuota, 403)

	got, bucket := preferBillingAttemptError(current, model.UserQuotaBucketSubscription, next, model.UserQuotaBucketFree)
	if got != current {
		t.Fatalf("preferBillingAttemptError() returned unexpected error: got=%v want=%v", got, current)
	}
	if bucket != model.UserQuotaBucketSubscription {
		t.Fatalf("preferBillingAttemptError() bucket = %q, want %q", bucket, model.UserQuotaBucketSubscription)
	}
}

func TestPreferBillingAttemptErrorReplacesGenericFreeErrorWithSpecificError(t *testing.T) {
	current := types.NewErrorWithStatusCode(errors.New("用户额度不足"), types.ErrorCodeInsufficientUserQuota, 403)
	next := types.NewErrorWithStatusCode(errors.New("按量付费额度不足"), types.ErrorCodeInsufficientUserQuota, 403)

	got, bucket := preferBillingAttemptError(current, model.UserQuotaBucketFree, next, model.UserQuotaBucketPayg)
	if got != next {
		t.Fatalf("preferBillingAttemptError() returned unexpected error: got=%v want=%v", got, next)
	}
	if bucket != model.UserQuotaBucketPayg {
		t.Fatalf("preferBillingAttemptError() bucket = %q, want %q", bucket, model.UserQuotaBucketPayg)
	}
}

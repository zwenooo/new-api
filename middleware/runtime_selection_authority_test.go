package middleware

import (
	"net/http/httptest"
	"testing"

	"one-api/common"
	"one-api/constant"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

func TestRuntimeSelectionAuthoritySelectChannelPrefersCompatFallbackInEarlierGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	callOrder := make([]string, 0, 4)
	authority := newRuntimeSelectionAuthority("/v1/messages", "claude-sonnet", 1, []int{1, 3}, "ua")
	authority.allowUserAgentFn = func(int, string) bool { return true }
	authority.groupAllowsModelFn = func(int, string) bool { return true }
	authority.directChannelLookupFn = func(_ *gin.Context, groupID int, requestedModel string) (*model.Channel, error) {
		callOrder = append(callOrder, "direct")
		if groupID == 3 {
			t.Fatal("later group should not be consulted before compat fallback succeeds")
		}
		return nil, nil
	}
	authority.compatChannelLookupFn = func(_ *gin.Context, groupID int, requestedModel string) (*model.Channel, error) {
		callOrder = append(callOrder, "compat")
		return &model.Channel{Id: 101}, nil
	}

	channel, groupID, uaAcceptedAny, err := authority.SelectChannel(ctx)
	if err != nil {
		t.Fatalf("SelectChannel() unexpected error: %+v", err)
	}
	if !uaAcceptedAny {
		t.Fatal("SelectChannel() should mark ua as accepted")
	}
	if groupID != 1 {
		t.Fatalf("SelectChannel() groupID = %d, want 1", groupID)
	}
	if channel == nil || channel.Id != 101 {
		t.Fatalf("SelectChannel() channel = %+v, want id 101", channel)
	}
	if got, want := len(callOrder), 2; got != want {
		t.Fatalf("call count = %d, want %d", got, want)
	}
}

func TestRuntimeSelectionAuthorityLookupChannelForGroupDefersDirectConcurrencyToCompat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	authority := newRuntimeSelectionAuthority("/v1/messages", "claude-sonnet", 2, []int{2}, "ua")
	authority.allowUserAgentFn = func(int, string) bool { return true }
	authority.groupAllowsModelFn = func(int, string) bool { return true }
	authority.directChannelLookupFn = func(_ *gin.Context, groupID int, requestedModel string) (*model.Channel, error) {
		return nil, model.ErrChannelConcurrencyLimitReached
	}
	authority.compatChannelLookupFn = func(_ *gin.Context, groupID int, requestedModel string) (*model.Channel, error) {
		return &model.Channel{Id: 202}, nil
	}

	channel, lookupModel, stepKind, err := authority.LookupChannelForGroup(ctx, 2)
	if err != nil {
		t.Fatalf("LookupChannelForGroup() unexpected error: %v", err)
	}
	if stepKind != "messages_to_responses_compat" {
		t.Fatalf("stepKind = %q, want messages_to_responses_compat", stepKind)
	}
	if lookupModel != "claude-sonnet" {
		t.Fatalf("lookupModel = %q, want claude-sonnet", lookupModel)
	}
	if channel == nil || channel.Id != 202 {
		t.Fatalf("LookupChannelForGroup() channel = %+v, want id 202", channel)
	}
}

func TestRuntimeSelectionAuthoritySetSelectedGroupRebasesCandidates(t *testing.T) {
	authority := newRuntimeSelectionAuthority("/v1/messages", "claude-sonnet", 2, []int{2, 5, 9}, "ua")
	authority.SetSelectedGroup(5)

	if authority.CurrentGroupID != 5 {
		t.Fatalf("CurrentGroupID = %d, want 5", authority.CurrentGroupID)
	}
	if got, want := authority.CandidateGroupIDs, []int{5, 9}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("CandidateGroupIDs = %v, want %v", got, want)
	}
}

func TestResolveRuntimeSelectionAuthorityReusesMatchingContextAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	authority := newRuntimeSelectionAuthority("/v1/messages", "claude-sonnet", 2, []int{2, 5}, "ua")
	authority.ApplyContext(ctx)

	if usingGroupID := common.GetContextKeyInt(ctx, constant.ContextKeyUsingGroupId); usingGroupID != 2 {
		t.Fatalf("usingGroupID = %d, want 2", usingGroupID)
	}
	if candidates, _ := common.GetContextKeyType[[]int](ctx, constant.ContextKeyResolvedGroupCandidateIds); len(candidates) != 2 || candidates[0] != 2 || candidates[1] != 5 {
		t.Fatalf("resolved group candidates = %v, want [2 5]", candidates)
	}

	resolved, apiErr := ResolveRuntimeSelectionAuthority(ctx, "/v1/messages", "claude-sonnet", 0)
	if apiErr != nil {
		t.Fatalf("ResolveRuntimeSelectionAuthority() error = %v", apiErr)
	}
	if resolved != authority {
		t.Fatal("ResolveRuntimeSelectionAuthority() did not reuse the matching context authority")
	}
}

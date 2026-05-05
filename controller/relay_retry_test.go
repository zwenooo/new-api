package controller

import (
	"errors"
	"net/http/httptest"
	"one-api/common"
	"one-api/constant"
	"one-api/middleware"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"one-api/types"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestShouldRetryUsesAutomaticSwitchStatusCodeBudget(t *testing.T) {
	restoreRetrySettings(t)

	if err := operation_setting.AutomaticSwitchStatusCodeWhitelistFromString("200"); err != nil {
		t.Fatalf("AutomaticSwitchStatusCodeWhitelistFromString error: %v", err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewErrorWithStatusCode(errors.New("upstream returned bad status"), types.ErrorCodeBadResponseStatusCode, 400)

	if mode := getRelayRetryMode(c, err); mode != relayRetryModeSwitchPreferred {
		t.Fatalf("getRelayRetryMode() = %v, want %v", mode, relayRetryModeSwitchPreferred)
	}
}

func TestShouldRetryReturnsWhitelistedStatusImmediately(t *testing.T) {
	restoreRetrySettings(t)

	if err := operation_setting.AutomaticSwitchStatusCodeWhitelistFromString("200\n429"); err != nil {
		t.Fatalf("AutomaticSwitchStatusCodeWhitelistFromString error: %v", err)
	}
	common.RetryTimes = 3

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewErrorWithStatusCode(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, 429)

	if mode := getRelayRetryMode(c, err); mode != relayRetryModeStop {
		t.Fatalf("getRelayRetryMode() = %v, want %v", mode, relayRetryModeStop)
	}
}

func TestShouldRetryFallsBackToStandardRetryWhenWhitelistDisabled(t *testing.T) {
	state := relayRetryState{singleChannelBudget: 2, switchBudget: 1}

	if action := decideRelayRetryAction(relayRetryModeSwitchPreferred, state, true); action != relayRetryActionSwitchChannel {
		t.Fatalf("decideRelayRetryAction() = %v, want %v", action, relayRetryActionSwitchChannel)
	}
	if action := decideRelayRetryAction(relayRetryModeSwitchPreferred, state, false); action != relayRetryActionSameChannel {
		t.Fatalf("decideRelayRetryAction() = %v, want %v", action, relayRetryActionSameChannel)
	}
	if action := decideRelayRetryAction(relayRetryModeSwitchRequired, state, false); action != relayRetryActionStop {
		t.Fatalf("decideRelayRetryAction() = %v, want %v", action, relayRetryActionStop)
	}
}

func TestSpecificChannelSupportsBillingGroupPrefersActualChannelGroups(t *testing.T) {
	actualGroups := map[int]struct{}{2: {}, 5: {}}

	if !specificChannelSupportsBillingGroup(actualGroups, 1, 5) {
		t.Fatal("specificChannelSupportsBillingGroup() = false, want true for actual bound group")
	}
	if specificChannelSupportsBillingGroup(actualGroups, 1, 3) {
		t.Fatal("specificChannelSupportsBillingGroup() = true, want false for non-member group")
	}
}

func TestSpecificChannelSupportsBillingGroupFallsBackToContextSelectedGroup(t *testing.T) {
	if !specificChannelSupportsBillingGroup(nil, 7, 7) {
		t.Fatal("specificChannelSupportsBillingGroup() = false, want true when candidate matches context group")
	}
	if specificChannelSupportsBillingGroup(nil, 7, 8) {
		t.Fatal("specificChannelSupportsBillingGroup() = true, want false when candidate differs from context group")
	}
}

func TestSpecificChannelSupportsBillingGroupAllowsUnknownGroupSetWhenContextMissing(t *testing.T) {
	if !specificChannelSupportsBillingGroup(nil, 0, 9) {
		t.Fatal("specificChannelSupportsBillingGroup() = false, want true when only candidate group is known")
	}
}

func TestTakeContextSelectedChannelForBillingCandidateKeepsSpecificChannelAcrossFallback(t *testing.T) {
	bound := &model.Channel{Id: 123}
	groupSet := map[int]struct{}{2: {}, 5: {}}

	channel, next := takeContextSelectedChannelForBillingCandidate(bound, true, false, 2, groupSet, 2)
	if channel != bound || next != bound {
		t.Fatalf("first take = (%p, %p), want (%p, %p)", channel, next, bound, bound)
	}

	channel, next = takeContextSelectedChannelForBillingCandidate(next, true, false, 2, groupSet, 5)
	if channel != bound || next != bound {
		t.Fatalf("fallback take = (%p, %p), want (%p, %p)", channel, next, bound, bound)
	}
}

func TestTakeContextSelectedChannelForBillingCandidateKeepsSpecificChannelWhenCandidateRejected(t *testing.T) {
	bound := &model.Channel{Id: 456}
	groupSet := map[int]struct{}{5: {}}

	channel, next := takeContextSelectedChannelForBillingCandidate(bound, true, false, 2, groupSet, 3)
	if channel != nil {
		t.Fatalf("channel = %p, want nil", channel)
	}
	if next != bound {
		t.Fatalf("next = %p, want %p", next, bound)
	}
}

func TestTakeContextSelectedChannelForBillingCandidateConsumesNonSpecificCurrentGroupOnce(t *testing.T) {
	bound := &model.Channel{Id: 789}

	channel, next := takeContextSelectedChannelForBillingCandidate(bound, false, false, 7, nil, 7)
	if channel != bound {
		t.Fatalf("channel = %p, want %p", channel, bound)
	}
	if next != nil {
		t.Fatalf("next = %p, want nil", next)
	}
}

func TestPrepareRelayBillingCandidateAttemptDoesNotMutateContextGroup(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUsingGroupId, 2)

	info := &relaycommon.RelayInfo{UsingGroupId: 2, QuotaBucket: model.UserQuotaBucketRequest}
	cand := billingCandidate{Bucket: model.UserQuotaBucketPayRequest, GroupID: 5}

	prepareRelayBillingCandidateAttempt(info, cand)

	if info.UsingGroupId != 5 {
		t.Fatalf("relayInfo.UsingGroupId = %d, want 5", info.UsingGroupId)
	}
	if info.QuotaBucket != model.UserQuotaBucketPayRequest {
		t.Fatalf("relayInfo.QuotaBucket = %q, want %q", info.QuotaBucket, model.UserQuotaBucketPayRequest)
	}
	if got := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId); got != 2 {
		t.Fatalf("context using_group_id = %d, want 2", got)
	}
}

func TestCommitRelayBillingCandidateSelectionUpdatesAuthorityContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	authority := &middleware.RuntimeSelectionAuthority{
		CurrentGroupID:    2,
		CandidateGroupIDs: []int{2, 5, 7},
	}
	info := &relaycommon.RelayInfo{}
	cand := billingCandidate{Bucket: model.UserQuotaBucketSubscription, GroupID: 5}

	commitRelayBillingCandidateSelection(c, info, authority, cand)

	if info.UsingGroupId != 5 {
		t.Fatalf("relayInfo.UsingGroupId = %d, want 5", info.UsingGroupId)
	}
	if info.QuotaBucket != model.UserQuotaBucketSubscription {
		t.Fatalf("relayInfo.QuotaBucket = %q, want %q", info.QuotaBucket, model.UserQuotaBucketSubscription)
	}
	if got := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId); got != 5 {
		t.Fatalf("context using_group_id = %d, want 5", got)
	}
	candidates, _ := common.GetContextKeyType[[]int](c, constant.ContextKeyResolvedGroupCandidateIds)
	if len(candidates) != 2 || candidates[0] != 5 || candidates[1] != 7 {
		t.Fatalf("resolved candidates = %v, want [5 7]", candidates)
	}
}

func TestNewRelayBillingRollbackErrorMasksAsUpdateFailure(t *testing.T) {
	err := newRelayBillingRollbackError(errors.New("inner"))
	if err == nil {
		t.Fatal("newRelayBillingRollbackError() = nil, want non-nil")
	}
	if err.GetErrorCode() != types.ErrorCodeUpdateDataError {
		t.Fatalf("error code = %v, want %v", err.GetErrorCode(), types.ErrorCodeUpdateDataError)
	}
	if err.StatusCode != 500 {
		t.Fatalf("status code = %d, want 500", err.StatusCode)
	}
}

func restoreRetrySettings(t *testing.T) {
	t.Helper()

	backupStandardRetry := common.RetryTimes
	backupWhitelist := append([]int(nil), operation_setting.AutomaticSwitchStatusCodeWhitelist...)
	backupSet := make(map[int]struct{}, len(operation_setting.AutomaticSwitchStatusCodeWhitelistSet))
	for statusCode := range operation_setting.AutomaticSwitchStatusCodeWhitelistSet {
		backupSet[statusCode] = struct{}{}
	}
	backupRetries := operation_setting.AutomaticSwitchMaxRetries

	common.RetryTimes = 0
	operation_setting.AutomaticSwitchStatusCodeWhitelist = nil
	operation_setting.AutomaticSwitchStatusCodeWhitelistSet = map[int]struct{}{}
	operation_setting.AutomaticSwitchMaxRetries = 5

	t.Cleanup(func() {
		common.RetryTimes = backupStandardRetry
		operation_setting.AutomaticSwitchStatusCodeWhitelist = backupWhitelist
		operation_setting.AutomaticSwitchStatusCodeWhitelistSet = backupSet
		operation_setting.AutomaticSwitchMaxRetries = backupRetries
	})
}

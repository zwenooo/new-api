package controller

import (
	"one-api/middleware"
	"one-api/model"
	relaycommon "one-api/relay/common"
)

type relayBillingCandidateSources struct {
	TokenGroupCandidates    []int
	RequestGroups           []int
	SubscriptionGroups      []int
	TokenSubscriptionGroups []int
	PaygQuota               int
	PaygGroups              []int
	PayRequestGroups        []int
	PayTokenGroups          []int
	NoBillingGroupSet       map[int]struct{}
}

func (s *relayBillingCandidateSources) billableGroupIDs() []int {
	if s == nil {
		return nil
	}
	groupIDs := make([]int, 0, 16)
	groupIDs = append(groupIDs, s.RequestGroups...)
	groupIDs = append(groupIDs, s.PayRequestGroups...)
	groupIDs = append(groupIDs, s.TokenSubscriptionGroups...)
	groupIDs = append(groupIDs, s.PayTokenGroups...)
	groupIDs = append(groupIDs, s.SubscriptionGroups...)
	if s.PaygQuota > 0 {
		groupIDs = append(groupIDs, s.PaygGroups...)
	}
	for gid := range s.NoBillingGroupSet {
		groupIDs = append(groupIDs, gid)
	}
	return normalizePositiveGroupCandidatesKeepOrder(groupIDs)
}

func (s *relayBillingCandidateSources) buildCandidates() []billingCandidate {
	if s == nil {
		return nil
	}
	return buildBillingCandidates(s.TokenGroupCandidates, []billingCandidateSpec{
		{Enabled: len(s.NoBillingGroupSet) > 0, Bucket: model.UserQuotaBucketFree, AllowedGroups: s.NoBillingGroupSet},
		{Enabled: len(s.RequestGroups) > 0, Bucket: model.UserQuotaBucketRequest, AllowedGroups: buildGroupIDSet(s.RequestGroups)},
		{Enabled: len(s.PayRequestGroups) > 0, Bucket: model.UserQuotaBucketPayRequest, AllowedGroups: buildGroupIDSet(s.PayRequestGroups)},
		{Enabled: len(s.TokenSubscriptionGroups) > 0, Bucket: model.UserQuotaBucketTokens, AllowedGroups: buildGroupIDSet(s.TokenSubscriptionGroups)},
		{Enabled: len(s.PayTokenGroups) > 0, Bucket: model.UserQuotaBucketPayToken, AllowedGroups: buildGroupIDSet(s.PayTokenGroups)},
		{Enabled: len(s.SubscriptionGroups) > 0, Bucket: model.UserQuotaBucketSubscription, AllowedGroups: buildGroupIDSet(s.SubscriptionGroups)},
		{Enabled: s.PaygQuota > 0, Bucket: model.UserQuotaBucketPayg, AllowedGroups: buildGroupIDSet(s.PaygGroups)},
	})
}

func loadRelayBillingCandidateSources(
	relayInfo *relaycommon.RelayInfo,
	tokenAllowedGroupIDs []int,
	selectionAuthority *middleware.RuntimeSelectionAuthority,
	rawPreConsumedTokens int,
) (*relayBillingCandidateSources, error) {
	usingGroupID := 0
	if relayInfo != nil {
		usingGroupID = relayInfo.UsingGroupId
	}
	sources := &relayBillingCandidateSources{
		TokenGroupCandidates: buildTokenGroupCandidatesFromRuntimeSelection(selectionAuthority, usingGroupID, tokenAllowedGroupIDs),
		NoBillingGroupSet:    make(map[int]struct{}),
	}
	if relayInfo == nil {
		return sources, nil
	}

	requestGroups, hasRequestSubscription, err := model.GetUserRequestSubscriptionGroupCandidates(relayInfo.UserId)
	if err != nil {
		return nil, err
	}
	if hasRequestSubscription && len(requestGroups) > 0 {
		sources.RequestGroups, err = filterRequestSubscriptionGroupsByUsage(
			relayInfo.UserId,
			relayInfo.UserGroupId,
			requestGroups,
			1,
		)
		if err != nil {
			return nil, err
		}
	}

	subscriptionGroups, hasSubscription, tokenSubscriptionGroups, hasTokenSubscription, err := model.GetUserRelaySubscriptionCandidateGroups(relayInfo.UserId)
	if err != nil {
		return nil, err
	}
	if hasSubscription && len(subscriptionGroups) > 0 {
		sources.SubscriptionGroups = append([]int(nil), subscriptionGroups...)
	}
	if hasTokenSubscription && len(tokenSubscriptionGroups) > 0 {
		sources.TokenSubscriptionGroups = append([]int(nil), tokenSubscriptionGroups...)
	}
	if len(sources.TokenSubscriptionGroups) > 0 {
		sources.TokenSubscriptionGroups, err = filterTokenSubscriptionGroupsByUsage(
			relayInfo.UserId,
			relayInfo.UserGroupId,
			sources.TokenSubscriptionGroups,
			rawPreConsumedTokens,
		)
		if err != nil {
			return nil, err
		}
	}

	discreteState, err := model.GetUserRelayDiscreteBillingState(relayInfo.UserId)
	if err != nil {
		return nil, err
	}
	if discreteState != nil {
		sources.PaygQuota = discreteState.PaygQuota
		sources.PaygGroups = append([]int(nil), discreteState.PaygGroups...)
		if discreteState.PayRequestQuota > 0 && len(discreteState.PayRequestGroups) > 0 {
			sources.PayRequestGroups, err = filterPayRequestGroupsByUsage(
				relayInfo.UserId,
				relayInfo.UserGroupId,
				discreteState.PayRequestGroups,
				1,
			)
			if err != nil {
				return nil, err
			}
		}
		if discreteState.PayTokenQuota > 0 && len(discreteState.PayTokenGroups) > 0 {
			sources.PayTokenGroups, err = filterPayTokenGroupsByUsage(
				relayInfo.UserId,
				relayInfo.UserGroupId,
				discreteState.PayTokenGroups,
				rawPreConsumedTokens,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	noBillingGroups, err := model.GetUserEligibleNoBillingGroupIDs(relayInfo.UserId)
	if err != nil {
		return nil, err
	}
	for _, gid := range noBillingGroups {
		if gid <= 0 {
			continue
		}
		sources.NoBillingGroupSet[gid] = struct{}{}
	}

	return sources, nil
}

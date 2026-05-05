package model

import "errors"

func buildGroupIDSet(ids []int) map[int]struct{} {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		set[id] = struct{}{}
	}
	return set
}

func filterRequestBillableGroupIDs(userId int, _ int, candidateGroupIDs []int) ([]int, error) {
	if len(candidateGroupIDs) == 0 {
		return nil, nil
	}
	allowedByRequired := make(map[int]map[int]struct{}, 4)
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := ComputeUserRequestUsage(userId, gid, 1)
		if err != nil {
			return nil, err
		}
		if required <= 0 {
			continue
		}
		allowedSet, ok := allowedByRequired[required]
		if !ok {
			ids, has, err := GetUserRequestSubscriptionGroupCandidatesByCount(userId, required)
			if err != nil {
				return nil, err
			}
			if has {
				allowedSet = buildGroupIDSet(ids)
			} else {
				allowedSet = map[int]struct{}{}
			}
			allowedByRequired[required] = allowedSet
		}
		if _, ok := allowedSet[gid]; ok {
			filtered = append(filtered, gid)
		}
	}
	return NormalizeUniqueSortedIDs(filtered), nil
}

func filterTokenBillableGroupIDs(userId int, _ int, candidateGroupIDs []int) ([]int, error) {
	if len(candidateGroupIDs) == 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := ComputeUserTokenUsage(userId, gid, 1)
		if err != nil {
			return nil, err
		}
		ok, err := CanUserTokenSubscriptionConsumeGroup(userId, gid, required)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, gid)
		}
	}
	return NormalizeUniqueSortedIDs(filtered), nil
}

func filterPayRequestBillableGroupIDs(userId int, _ int, candidateGroupIDs []int) ([]int, error) {
	if len(candidateGroupIDs) == 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := ComputeUserRequestUsage(userId, gid, 1)
		if err != nil {
			return nil, err
		}
		ok := false
		_, ok, err = FindUserPayRequestConsumableProductIdTx(nil, userId, gid, required)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, gid)
		}
	}
	return NormalizeUniqueSortedIDs(filtered), nil
}

func filterPayTokenBillableGroupIDs(userId int, _ int, candidateGroupIDs []int) ([]int, error) {
	if len(candidateGroupIDs) == 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := ComputeUserTokenUsage(userId, gid, 1)
		if err != nil {
			return nil, err
		}
		ok := false
		_, ok, err = FindUserPayTokenConsumableProductIdTx(nil, userId, gid, required)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, gid)
		}
	}
	return NormalizeUniqueSortedIDs(filtered), nil
}

// GetUserBillableGroupIDs returns the union of group IDs that the user can currently bill from.
//
// It includes:
// - active quota subscriptions (billing_unit=quota)
// - active token subscriptions (billing_unit=tokens)
// - active request subscriptions
// - positive pay-as-you-go balances (quota / pay_request / pay_token)
//
// Notes:
//   - Discrete buckets are checked against the user's current-group minimum charge
//     (1 request / 1 token after group-ratio scaling). Runtime relay selection still
//     performs per-request exact checks with the real token estimate.
func GetUserBillableGroupIDs(userId int) ([]int, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	userGroupID, err := GetUserAudienceGroupID(userId, false)
	if err != nil {
		return nil, err
	}

	groupIDs := make([]int, 0, 32)

	if ids, has, err := GetUserRequestSubscriptionGroupCandidates(userId); err != nil {
		return nil, err
	} else if has && len(ids) > 0 {
		filtered, err := filterRequestBillableGroupIDs(userId, userGroupID, ids)
		if err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, filtered...)
	}

	if ids, has, err := GetUserSubscriptionGroupCandidates(userId); err != nil {
		return nil, err
	} else if has && len(ids) > 0 {
		groupIDs = append(groupIDs, ids...)
	}

	if ids, has, err := GetUserTokenSubscriptionGroupCandidates(userId); err != nil {
		return nil, err
	} else if has && len(ids) > 0 {
		filtered, err := filterTokenBillableGroupIDs(userId, userGroupID, ids)
		if err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, filtered...)
	}

	if ids, err := GetUserEligibleNoBillingGroupIDs(userId); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		groupIDs = append(groupIDs, ids...)
	}

	if quota, ids, err := GetUserPayRequestBalanceInfoTx(nil, userId); err != nil {
		return nil, err
	} else if quota > 0 && len(ids) > 0 {
		filtered, err := filterPayRequestBillableGroupIDs(userId, userGroupID, ids)
		if err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, filtered...)
	}

	if quota, ids, err := GetUserPayTokenBalanceInfoTx(nil, userId); err != nil {
		return nil, err
	} else if quota > 0 && len(ids) > 0 {
		filtered, err := filterPayTokenBillableGroupIDs(userId, userGroupID, ids)
		if err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, filtered...)
	}

	if quota, ids, err := GetUserPaygBalanceInfoTx(nil, userId); err != nil {
		return nil, err
	} else if quota > 0 && len(ids) > 0 {
		groupIDs = append(groupIDs, ids...)
	}

	return NormalizeUniqueSortedIDs(groupIDs), nil
}

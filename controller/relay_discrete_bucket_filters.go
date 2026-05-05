package controller

import "one-api/model"

func rawPreConsumedTokenUnits(promptTokens int, maxTokens int) int {
	total := promptTokens
	if maxTokens > 0 {
		total += maxTokens
	}
	if total <= 0 {
		return 1
	}
	return total
}

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

func filterRequestSubscriptionGroupsByUsage(userID int, _ int, candidateGroupIDs []int, requests int) ([]int, error) {
	if len(candidateGroupIDs) == 0 || requests <= 0 {
		return nil, nil
	}
	allowedByRequired := make(map[int]map[int]struct{}, 4)
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range model.NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := model.ComputeUserRequestUsage(userID, gid, requests)
		if err != nil {
			return nil, err
		}
		if required <= 0 {
			continue
		}
		allowedSet, ok := allowedByRequired[required]
		if !ok {
			ids, has, err := model.GetUserRequestSubscriptionGroupCandidatesByCount(userID, required)
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
	return model.NormalizeUniqueSortedIDs(filtered), nil
}

func filterTokenSubscriptionGroupsByUsage(userID int, _ int, candidateGroupIDs []int, rawTokens int) ([]int, error) {
	if len(candidateGroupIDs) == 0 || rawTokens <= 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range model.NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := model.ComputeUserTokenUsage(userID, gid, rawTokens)
		if err != nil {
			return nil, err
		}
		ok, err := model.CanUserTokenSubscriptionConsumeGroup(userID, gid, required)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, gid)
		}
	}
	return model.NormalizeUniqueSortedIDs(filtered), nil
}

func filterPayRequestGroupsByUsage(userID int, _ int, candidateGroupIDs []int, requests int) ([]int, error) {
	if len(candidateGroupIDs) == 0 || requests <= 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range model.NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := model.ComputeUserRequestUsage(userID, gid, requests)
		if err != nil {
			return nil, err
		}
		ok := false
		_, ok, err = model.FindUserPayRequestConsumableProductIdTx(nil, userID, gid, required)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, gid)
		}
	}
	return model.NormalizeUniqueSortedIDs(filtered), nil
}

func filterPayTokenGroupsByUsage(userID int, _ int, candidateGroupIDs []int, rawTokens int) ([]int, error) {
	if len(candidateGroupIDs) == 0 || rawTokens <= 0 {
		return nil, nil
	}
	filtered := make([]int, 0, len(candidateGroupIDs))
	for _, gid := range model.NormalizeUniqueSortedIDs(candidateGroupIDs) {
		required, err := model.ComputeUserTokenUsage(userID, gid, rawTokens)
		if err != nil {
			return nil, err
		}
		ok := false
		_, ok, err = model.FindUserPayTokenConsumableProductIdTx(nil, userID, gid, required)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, gid)
		}
	}
	return model.NormalizeUniqueSortedIDs(filtered), nil
}

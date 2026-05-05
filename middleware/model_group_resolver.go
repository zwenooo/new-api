package middleware

import (
	"strings"

	"one-api/model"
)

func normalizePositiveGroupIDsKeepOrder(groupIDs []int) []int {
	if len(groupIDs) == 0 {
		return nil
	}

	out := make([]int, 0, len(groupIDs))
	seen := make(map[int]struct{}, len(groupIDs))
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}
		out = append(out, gid)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func rebaseGroupCandidatesFromCurrent(candidateGroupIDs []int, currentGroupID int) []int {
	normalized := normalizePositiveGroupIDsKeepOrder(candidateGroupIDs)
	if len(normalized) == 0 {
		return nil
	}
	if currentGroupID <= 0 {
		return normalized
	}
	for idx, gid := range normalized {
		if gid == currentGroupID {
			return append([]int(nil), normalized[idx:]...)
		}
	}
	return normalized
}

func chooseTokenGroupCandidatesByModelAccess(currentGroupID int, tokenAllowedGroupIDs []int, supports func(int) bool, billable func(int) bool) []int {
	normalized := normalizePositiveGroupIDsKeepOrder(tokenAllowedGroupIDs)
	if len(normalized) == 0 {
		if currentGroupID > 0 {
			return []int{currentGroupID}
		}
		return nil
	}

	billableSupported := make([]int, 0, len(normalized))
	supported := make([]int, 0, len(normalized))
	for _, gid := range normalized {
		if !supports(gid) {
			continue
		}
		supported = append(supported, gid)
		if billable == nil || billable(gid) {
			billableSupported = append(billableSupported, gid)
		}
	}

	if len(billableSupported) > 0 {
		return billableSupported
	}
	if billable != nil {
		if len(supported) == 0 && currentGroupID > 0 {
			return []int{currentGroupID}
		}
		return nil
	}
	if len(supported) > 0 {
		return supported
	}
	if currentGroupID > 0 {
		return []int{currentGroupID}
	}
	return nil
}

func chooseTokenGroupByModelAccess(currentGroupID int, tokenAllowedGroupIDs []int, supports func(int) bool, billable func(int) bool) int {
	candidates := chooseTokenGroupCandidatesByModelAccess(currentGroupID, tokenAllowedGroupIDs, supports, billable)
	if len(candidates) == 0 {
		return currentGroupID
	}
	return candidates[0]
}

func groupStaticallySupportsModelRequest(path string, groupID int, requestedModel string) (bool, error) {
	requestedModel = strings.TrimSpace(requestedModel)
	if groupID <= 0 || requestedModel == "" {
		return false, nil
	}

	if !model.GroupAllowsModel(groupID, requestedModel) {
		return false, nil
	}

	if strings.HasPrefix(path, "/v1/messages") {
		if model.GroupHasEnabledModel(groupID, requestedModel) {
			return true, nil
		}
	} else if model.GroupHasEnabledModel(groupID, requestedModel) {
		return true, nil
	}

	if !strings.HasPrefix(path, "/v1/messages") {
		return false, nil
	}

	hasCompat, err := model.GroupHasMessagesToResponsesCompatChannel(groupID, requestedModel)
	if err != nil {
		return false, err
	}
	if hasCompat {
		return true, nil
	}
	return false, nil
}

func resolveUsingGroupForModelRequest(path string, currentGroupID int, tokenAllowedGroupIDs []int, userID int, requestedModel string) (int, error) {
	candidates, err := resolveUsingGroupCandidatesForModelRequest(path, currentGroupID, tokenAllowedGroupIDs, userID, requestedModel)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return currentGroupID, nil
	}
	return candidates[0], nil
}

func resolveUsingGroupCandidatesForModelRequest(path string, currentGroupID int, tokenAllowedGroupIDs []int, userID int, requestedModel string) ([]int, error) {
	supportCache := make(map[int]bool, len(tokenAllowedGroupIDs)+1)
	var supportErr error
	_ = userID

	// Keep middleware resolution cheap: relay/controller already performs the exact
	// bucket-level billing lookup before pre-consume. Repeating the same billable-group
	// DB scans here only adds latency and duplicate database pressure on every request.
	resolvedGroupIDs := chooseTokenGroupCandidatesByModelAccess(
		currentGroupID,
		tokenAllowedGroupIDs,
		func(groupID int) bool {
			if supportErr != nil {
				return false
			}
			if ok, exists := supportCache[groupID]; exists {
				return ok
			}
			ok, err := groupStaticallySupportsModelRequest(path, groupID, requestedModel)
			if err != nil {
				supportErr = err
				return false
			}
			supportCache[groupID] = ok
			return ok
		},
		nil,
	)
	if supportErr != nil {
		return nil, supportErr
	}
	return rebaseGroupCandidatesFromCurrent(resolvedGroupIDs, currentGroupID), nil
}

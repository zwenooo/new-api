package controller

import (
	"strings"

	"one-api/middleware"
	"one-api/model"
	"one-api/types"
)

type billingCandidate struct {
	Bucket  string
	GroupID int
}

type billingCandidateSpec struct {
	Enabled       bool
	Bucket        string
	AllowedGroups map[int]struct{}
}

func normalizePositiveGroupCandidatesKeepOrder(groupIDs []int) []int {
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

func rebaseBillingGroupCandidates(groupCandidates []int, currentGroupID int) []int {
	normalized := normalizePositiveGroupCandidatesKeepOrder(groupCandidates)
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

// buildTokenGroupCandidates builds ordered token group candidates for billing.
//
// Rule:
//  1. Prefer the already-resolved request group candidates from middleware.
//  2. When a current group has already been selected, keep only that group and the
//     remaining later candidates in token order.
//  3. When middleware resolved nothing, fall back to full token allowed groups so
//     relay can still perform exact billing-based selection.
//  4. Fall back to current using_group_id only when token allowed groups are empty.
func buildTokenGroupCandidates(usingGroupID int, resolvedGroupIDs []int, allowedGroupIDs []int) []int {
	candidates := rebaseBillingGroupCandidates(resolvedGroupIDs, usingGroupID)
	if len(candidates) > 0 {
		return candidates
	}
	allowed := normalizePositiveGroupCandidatesKeepOrder(allowedGroupIDs)
	if len(allowed) > 0 {
		return allowed
	}
	if usingGroupID > 0 {
		return []int{usingGroupID}
	}
	return nil
}

func buildTokenGroupCandidatesFromRuntimeSelection(selectionAuthority *middleware.RuntimeSelectionAuthority, usingGroupID int, allowedGroupIDs []int) []int {
	var resolvedGroupIDs []int
	if selectionAuthority != nil {
		resolvedGroupIDs = selectionAuthority.CandidateGroupIDs
	}
	return buildTokenGroupCandidates(usingGroupID, resolvedGroupIDs, allowedGroupIDs)
}

func intersectOrderedGroupCandidates(groupCandidates []int, allowedGroups map[int]struct{}) []int {
	if len(groupCandidates) == 0 || len(allowedGroups) == 0 {
		return nil
	}
	out := make([]int, 0, len(groupCandidates))
	seen := make(map[int]struct{}, len(groupCandidates))
	for _, gid := range groupCandidates {
		if gid <= 0 {
			continue
		}
		if _, ok := allowedGroups[gid]; !ok {
			continue
		}
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}
		out = append(out, gid)
	}
	return out
}

func buildBillingCandidates(groupCandidates []int, specs []billingCandidateSpec) []billingCandidate {
	if len(groupCandidates) == 0 || len(specs) == 0 {
		return nil
	}

	candidates := make([]billingCandidate, 0, len(groupCandidates)*len(specs))
	seenGroups := make(map[int]struct{}, len(groupCandidates))

	for _, gid := range groupCandidates {
		if gid <= 0 {
			continue
		}
		if _, ok := seenGroups[gid]; ok {
			continue
		}
		seenGroups[gid] = struct{}{}

		for _, spec := range specs {
			if !spec.Enabled || len(spec.AllowedGroups) == 0 {
				continue
			}
			if _, ok := spec.AllowedGroups[gid]; !ok {
				continue
			}
			candidates = append(candidates, billingCandidate{Bucket: spec.Bucket, GroupID: gid})
		}
	}

	if len(candidates) == 0 {
		return nil
	}
	return candidates
}

func preferBillingAttemptError(current *types.NewAPIError, currentBucket string, next *types.NewAPIError, nextBucket string) (*types.NewAPIError, string) {
	if next == nil {
		return current, currentBucket
	}
	if current == nil {
		return next, nextBucket
	}
	if isGenericFreeBucketQuotaError(next, nextBucket) {
		return current, currentBucket
	}
	if isGenericFreeBucketQuotaError(current, currentBucket) {
		return next, nextBucket
	}
	return next, nextBucket
}

func isGenericFreeBucketQuotaError(err *types.NewAPIError, bucket string) bool {
	if err == nil {
		return false
	}
	if bucket != model.UserQuotaBucketFree {
		return false
	}
	if err.GetErrorCode() != types.ErrorCodeInsufficientUserQuota {
		return false
	}
	return strings.TrimSpace(err.Error()) == "用户额度不足"
}

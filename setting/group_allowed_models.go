package setting

import (
	"strings"
	"sync"
)

var groupAllowedModels = map[int]map[string]struct{}{}
var groupAllowedModelsMutex sync.RWMutex

// ReplaceGroupAllowedModels replaces the in-memory group->allowed_models mapping.
//
// NOTE: The provided map (and its nested maps) must be treated as immutable after calling.
// This function does not deep-copy for performance.
func ReplaceGroupAllowedModels(next map[int]map[string]struct{}) {
	groupAllowedModelsMutex.Lock()
	groupAllowedModels = next
	groupAllowedModelsMutex.Unlock()
}

// GroupAllowsModel returns whether the given model can be consumed under the specified using_group_id.
//
// Rule:
// - When a group has no configured allowlist, all models are allowed (legacy behavior).
// - When allowlist exists (non-empty), only exact matches are allowed.
func GroupAllowsModel(groupID int, model string) bool {
	if groupID <= 0 {
		return true
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return true
	}

	groupAllowedModelsMutex.RLock()
	set := groupAllowedModels[groupID]
	groupAllowedModelsMutex.RUnlock()

	if len(set) == 0 {
		return true
	}
	_, ok := set[model]
	return ok
}


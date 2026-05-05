package setting

import (
	"strings"
	"sync"
)

var groupAllowedUserAgents = map[int][]string{}
var groupAllowedUserAgentsMutex sync.RWMutex

// ReplaceGroupAllowedUserAgents replaces the in-memory group->allowed_user_agents mapping.
//
// NOTE: The provided map (and its slices) must be treated as immutable after calling.
// This function does not deep-copy for performance.
func ReplaceGroupAllowedUserAgents(next map[int][]string) {
	groupAllowedUserAgentsMutex.Lock()
	groupAllowedUserAgents = next
	groupAllowedUserAgentsMutex.Unlock()
}

// GroupAllowsUserAgent returns whether the given UA can consume under the specified group.
//
// Rule:
// - When a group has no configured allowlist, all UAs are allowed (legacy behavior).
// - When allowlist exists (non-empty), UA must contain any keyword (case-insensitive).
// - When allowlist exists and UA is empty, deny.
func GroupAllowsUserAgent(groupID int, ua string) bool {
	if groupID <= 0 {
		return true
	}
	ua = strings.ToLower(strings.TrimSpace(ua))

	groupAllowedUserAgentsMutex.RLock()
	keywords := groupAllowedUserAgents[groupID]
	groupAllowedUserAgentsMutex.RUnlock()

	if len(keywords) == 0 {
		return true
	}
	if ua == "" {
		return false
	}
	for _, kw := range keywords {
		if strings.Contains(ua, kw) {
			return true
		}
	}
	return false
}

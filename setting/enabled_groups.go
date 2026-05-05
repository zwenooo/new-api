package setting

import (
	"encoding/json"
	"one-api/common"
	"sync"
)

// enabledGroups stores which groups are enabled (not disabled).
//
// NOTE: This is intentionally separated from userUsableGroups:
// - enabledGroups: whether the group can be used at all (controlled by "启用")
// - userUsableGroups: whether the group is user-selectable / shown to users (controlled by "用户可选")
var enabledGroups = map[int]bool{}

var enabledGroupsMutex sync.RWMutex

func GetEnabledGroupsCopy() map[int]bool {
	enabledGroupsMutex.RLock()
	defer enabledGroupsMutex.RUnlock()

	out := make(map[int]bool, len(enabledGroups))
	for k, v := range enabledGroups {
		out[k] = v
	}
	return out
}

func GroupInEnabledGroups(groupID int) bool {
	enabledGroupsMutex.RLock()
	defer enabledGroupsMutex.RUnlock()

	enabled, ok := enabledGroups[groupID]
	return ok && enabled
}

func EnabledGroups2JSONString() string {
	enabledGroupsMutex.RLock()
	defer enabledGroupsMutex.RUnlock()

	jsonBytes, err := json.Marshal(enabledGroups)
	if err != nil {
		common.SysLog("error marshalling enabled groups: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateEnabledGroupsByJSONString(jsonStr string) error {
	enabledGroupsMutex.Lock()
	defer enabledGroupsMutex.Unlock()

	enabledGroups = make(map[int]bool)
	return json.Unmarshal([]byte(jsonStr), &enabledGroups)
}

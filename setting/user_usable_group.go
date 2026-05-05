package setting

import (
	"encoding/json"
	"one-api/common"
	"sync"
)

var userUsableGroups = map[int]string{}
var userUsableGroupsMutex sync.RWMutex

func GetUserUsableGroupsCopy() map[int]string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	copyUserUsableGroups := make(map[int]string)
	for k, v := range userUsableGroups {
		copyUserUsableGroups[k] = v
	}
	return copyUserUsableGroups
}

func UserUsableGroups2JSONString() string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	jsonBytes, err := json.Marshal(userUsableGroups)
	if err != nil {
		common.SysLog("error marshalling user groups: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateUserUsableGroupsByJSONString(jsonStr string) error {
	userUsableGroupsMutex.Lock()
	defer userUsableGroupsMutex.Unlock()

	userUsableGroups = make(map[int]string)
	return json.Unmarshal([]byte(jsonStr), &userUsableGroups)
}

func GetUserUsableGroups(userGroupID int) map[int]string {
	// NOTE:
	// - userUsableGroups 表示“普通用户可选”的分组集合（Enabled && UserSelectable）。
	// - userGroupID 是用户自身的默认分组（tier），不等价于“可选分组”。
	//   当分组设置为 user_selectable=false 时，普通用户不应在任何选择器中看到/选择该分组，
	//   因此这里不再把 userGroupID 兜底加入可选集合。
	_ = userGroupID
	return GetUserUsableGroupsCopy()
}

func GroupInUserUsableGroups(groupID int) bool {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	_, ok := userUsableGroups[groupID]
	return ok
}

func GetUsableGroupDescription(groupID int) string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	if desc, ok := userUsableGroups[groupID]; ok {
		return desc
	}
	return ""
}

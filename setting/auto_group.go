package setting

import "encoding/json"

var AutoGroups = []int{}

var DefaultUseAutoGroup = false

func ContainsAutoGroup(groupID int) bool {
	for _, autoGroup := range AutoGroups {
		if autoGroup == groupID {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	AutoGroups = make([]int, 0)
	return json.Unmarshal([]byte(jsonString), &AutoGroups)
}

func AutoGroups2JsonString() string {
	jsonBytes, err := json.Marshal(AutoGroups)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

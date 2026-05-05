package ratio_setting

import (
	"encoding/json"
	"errors"
	"one-api/common"
	"strconv"
	"sync"
)

var groupRatio = map[int]float64{}
var groupRatioMutex sync.RWMutex

var (
	GroupGroupRatio      = map[int]map[int]float64{}
	groupGroupRatioMutex sync.RWMutex
)

func GetGroupRatioCopy() map[int]float64 {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	groupRatioCopy := make(map[int]float64)
	for k, v := range groupRatio {
		groupRatioCopy[k] = v
	}
	return groupRatioCopy
}

func ContainsGroupRatio(groupID int) bool {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	_, ok := groupRatio[groupID]
	return ok
}

func GroupRatio2JSONString() string {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	jsonBytes, err := json.Marshal(groupRatio)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	groupRatioMutex.Lock()
	defer groupRatioMutex.Unlock()

	groupRatio = make(map[int]float64)
	return json.Unmarshal([]byte(jsonStr), &groupRatio)
}

func GetGroupRatio(groupID int) float64 {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	ratio, ok := groupRatio[groupID]
	if !ok {
		common.SysLog("group ratio not found: " + strconv.Itoa(groupID))
		return 1
	}
	return ratio
}

func GetGroupGroupRatioCopy() map[int]map[int]float64 {
	groupGroupRatioMutex.RLock()
	defer groupGroupRatioMutex.RUnlock()

	out := make(map[int]map[int]float64, len(GroupGroupRatio))
	for userGroupID, ratios := range GroupGroupRatio {
		if len(ratios) == 0 {
			out[userGroupID] = map[int]float64{}
			continue
		}
		copied := make(map[int]float64, len(ratios))
		for usingGroupID, ratio := range ratios {
			copied[usingGroupID] = ratio
		}
		out[userGroupID] = copied
	}
	return out
}

func GetGroupGroupRatio(userGroupID, usingGroupID int) (float64, bool) {
	groupGroupRatioMutex.RLock()
	defer groupGroupRatioMutex.RUnlock()

	gp, ok := GroupGroupRatio[userGroupID]
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroupID]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	groupGroupRatioMutex.RLock()
	defer groupGroupRatioMutex.RUnlock()

	jsonBytes, err := json.Marshal(GroupGroupRatio)
	if err != nil {
		common.SysLog("error marshalling group-group ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	groupGroupRatioMutex.Lock()
	defer groupGroupRatioMutex.Unlock()

	GroupGroupRatio = make(map[int]map[int]float64)
	return json.Unmarshal([]byte(jsonStr), &GroupGroupRatio)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[int]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for id, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + strconv.Itoa(id))
		}
	}
	return nil
}

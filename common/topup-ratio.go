package common

import (
	"encoding/json"
	"strconv"
)

var TopupGroupRatio = map[int]float64{}

func TopupGroupRatio2JSONString() string {
	jsonBytes, err := json.Marshal(TopupGroupRatio)
	if err != nil {
		SysError("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateTopupGroupRatioByJSONString(jsonStr string) error {
	TopupGroupRatio = make(map[int]float64)
	return json.Unmarshal([]byte(jsonStr), &TopupGroupRatio)
}

func GetTopupGroupRatio(groupID int) float64 {
	ratio, ok := TopupGroupRatio[groupID]
	if !ok {
		SysError("topup group ratio not found: " + strconv.Itoa(groupID))
		return 1
	}
	return ratio
}

package model

import (
	"strings"

	"one-api/setting"
	"one-api/setting/ratio_setting"
)

func groupAllowsModel(groupID int, model string) bool {
	if groupID <= 0 {
		return true
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return true
	}
	if setting.GroupAllowsModel(groupID, model) {
		return true
	}
	matchName := ratio_setting.FormatMatchingModelName(model)
	if matchName != "" && matchName != model {
		return setting.GroupAllowsModel(groupID, matchName)
	}
	return false
}

// GroupAllowsModel reports whether the given model can be consumed under the specified using_group_id.
//
// It applies the same normalization rules as the internal routing layer (e.g. gpts / thinking-* matching).
func GroupAllowsModel(groupID int, model string) bool {
	return groupAllowsModel(groupID, model)
}

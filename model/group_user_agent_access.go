package model

import (
	"strings"

	"one-api/setting"
)

func GroupAllowsUserAgent(groupID int, ua string) bool {
	if groupID <= 0 {
		return true
	}
	ua = strings.TrimSpace(ua)
	return setting.GroupAllowsUserAgent(groupID, ua)
}

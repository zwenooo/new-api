package service

import (
	"strings"

	"one-api/common"
)

func readOption(key string) string {
	return readOptionAny(key)
}

func readOptionAny(keys ...string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if common.OptionMap == nil {
		return ""
	}
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if value, ok := common.OptionMap[key]; ok {
			return value
		}
	}
	return ""
}

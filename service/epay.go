package service

import (
	"one-api/setting/operation_setting"
	"one-api/setting/system_setting"
	"strings"
)

func GetCallbackAddress() string {
	callbackAddress := strings.TrimSpace(operation_setting.CustomCallbackAddress)
	if callbackAddress == "" {
		callbackAddress = strings.TrimSpace(system_setting.ServerAddress)
	}
	return strings.TrimRight(callbackAddress, "/")
}

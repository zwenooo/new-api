package taskcommon

import (
	"fmt"

	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

type BaseBilling struct{}

func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

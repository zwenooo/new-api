package operation_setting

import (
	"one-api/setting/config"
	"os"
	"strconv"
	"strings"
)

type MonitorSetting struct {
	AutoTestChannelEnabled           bool   `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes           int    `json:"auto_test_channel_minutes"`
	ServiceStatusDefaultRangeDays    int    `json:"service_status_default_range_days"`
	ServiceStatusDefaultRangeMinutes int    `json:"service_status_default_range_minutes"`
	ServiceStatusDefaultBucket       string `json:"service_status_default_bucket"`
	ServiceStatusUAFilterMode        string `json:"service_status_ua_filter_mode"`
	ServiceStatusUAContains          string `json:"service_status_ua_contains"`
}

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:           false,
	AutoTestChannelMinutes:           10,
	ServiceStatusDefaultRangeDays:    30,
	ServiceStatusDefaultRangeMinutes: 180,
	ServiceStatusDefaultBucket:       "minute",
	ServiceStatusUAFilterMode:        "include",
	ServiceStatusUAContains: strings.Join([]string{
		"opencode",
		"codex_vscode",
		"codex_exec",
		"Codex Desktop",
		"claude-cli",
		"codex_cli_rs",
	}, "\n"),
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = frequency
		}
	}
	return &monitorSetting
}

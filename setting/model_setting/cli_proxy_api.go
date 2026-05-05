package model_setting

import "one-api/setting/config"

type CLIProxyAPISettings struct {
	Mode           string `json:"mode"`
}

var defaultCLIProxyAPISettings = CLIProxyAPISettings{
	Mode: "off",
}

var cliProxyAPISettings = defaultCLIProxyAPISettings

func init() {
	config.GlobalConfig.Register("cli_proxy_api", &cliProxyAPISettings)
}

func GetCLIProxyAPISettings() *CLIProxyAPISettings {
	return &cliProxyAPISettings
}

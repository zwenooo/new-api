package operation_setting

import "one-api/setting/config"

type ChannelAllocationSetting struct {
	UserStickyExclusiveEnabled    bool `json:"user_sticky_exclusive_enabled"`
	UserStickyExclusiveTTLSeconds int  `json:"user_sticky_exclusive_ttl_seconds"`
}

var channelAllocationSetting = ChannelAllocationSetting{
	UserStickyExclusiveEnabled:    false,
	UserStickyExclusiveTTLSeconds: 1800,
}

func init() {
	config.GlobalConfig.Register("channel_allocation_setting", &channelAllocationSetting)
}

func GetChannelAllocationSetting() *ChannelAllocationSetting {
	return &channelAllocationSetting
}


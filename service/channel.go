package service

import (
	"fmt"
	"one-api/common"
	"one-api/dto"
	"one-api/model"
	"one-api/setting/operation_setting"
	"one-api/types"
	"strings"
	"time"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string, restoreAfterSeconds int64) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)

		if restoreAfterSeconds > 0 {
			duration := time.Duration(restoreAfterSeconds) * time.Second
			go func(target types.ChannelError, wait time.Duration) {
				timer := time.NewTimer(wait)
				defer timer.Stop()
				<-timer.C

				channel, err := model.GetChannelById(target.ChannelId, true)
				if err != nil {
					common.SysLog(fmt.Sprintf("通道「%s」（#%d）自动恢复失败，查询渠道信息报错：%v", target.ChannelName, target.ChannelId, err))
					return
				}

				if channel.ChannelInfo.IsMultiKey {
					shouldRestore := false
					if target.UsingKey != "" {
						keys := channel.GetKeys()
						keyIndex := -1
						for idx, key := range keys {
							if key == target.UsingKey {
								keyIndex = idx
								break
							}
						}
						if keyIndex >= 0 {
							if statusList := channel.ChannelInfo.MultiKeyStatusList; statusList != nil {
								if _, disabled := statusList[keyIndex]; disabled {
									shouldRestore = true
								}
							}
						}
					}
					if !shouldRestore && channel.Status != common.ChannelStatusAutoDisabled {
						common.SysLog(fmt.Sprintf("通道「%s」（#%d）目标 Key 已启用或未找到，跳过自动恢复", target.ChannelName, target.ChannelId))
						return
					}
				} else {
					if channel.Status != common.ChannelStatusAutoDisabled {
						common.SysLog(fmt.Sprintf("通道「%s」（#%d）当前状态为 %d，跳过自动恢复", target.ChannelName, target.ChannelId, channel.Status))
						return
					}
				}

				EnableChannel(target.ChannelId, target.UsingKey, target.ChannelName)
			}(channelError, duration)
		}
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(channelType int, err *types.NewAPIError) (bool, string, int64) {
	if !common.AutomaticDisableChannelEnabled {
		return false, "", 0
	}
	if err == nil {
		return false, "", 0
	}
	if types.IsSkipRetryError(err) {
		return false, "", 0
	}

	lowerMessage := strings.ToLower(err.Error())
	search, words := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	if !search {
		return false, "", 0
	}

	originalMessage := err.Error()
	dedup := map[string]struct{}{}
	matched := make([]string, 0, len(words))
	var maxRestoreSeconds int64
	for _, w := range words {
		keyword := strings.TrimSpace(w)
		if keyword == "" {
			continue
		}
		if seconds := operation_setting.GetAutomaticDisableKeywordDuration(keyword); seconds > maxRestoreSeconds {
			maxRestoreSeconds = seconds
		}
		idx := strings.Index(lowerMessage, keyword)
		display := keyword
		if idx >= 0 {
			end := idx + len(keyword)
			if end > len(originalMessage) {
				end = len(originalMessage)
			}
			display = originalMessage[idx:end]
		}
		key := strings.ToLower(display)
		if _, exists := dedup[key]; exists {
			continue
		}
		dedup[key] = struct{}{}
		matched = append(matched, display)
	}

	if len(matched) == 0 {
		return true, "自动禁用(关键词)", maxRestoreSeconds
	}

	return true, fmt.Sprintf("自动禁用(%s)", strings.Join(matched, "、")), maxRestoreSeconds
}

// ShouldSwitchChannel returns true when the upstream status code is not in the configured
// whitelist so that the request can be retried on another eligible channel. Unlike
// ShouldDisableChannel, this does not change channel status; it only affects retry decision.
func ShouldSwitchChannel(channelType int, err *types.NewAPIError) (bool, string) {
	_ = channelType
	if err == nil {
		return false, ""
	}
	if !operation_setting.HasAutomaticSwitchStatusCodeWhitelist() {
		return false, ""
	}
	if operation_setting.IsAutomaticSwitchStatusCodeAllowed(err.StatusCode) {
		return false, ""
	}

	if err.StatusCode <= 0 {
		return true, "自动切换(状态码未知且不在白名单)"
	}
	return true, fmt.Sprintf("自动切换(状态码 %d 不在白名单)", err.StatusCode)
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}

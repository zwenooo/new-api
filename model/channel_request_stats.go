package model

import (
	"fmt"
	"one-api/common"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelBillingModeQuota   = "quota"
	ChannelBillingModeRequest = "request"
)

func NormalizeChannelBillingMode(mode string) string {
	switch mode {
	case "", ChannelBillingModeQuota:
		return ChannelBillingModeQuota
	case ChannelBillingModeRequest:
		return ChannelBillingModeRequest
	default:
		return mode
	}
}

// ChannelRequestDailyStat stores per-channel successful consume request counts by day (YYYYMMDD).
// It is derived from logs where type == LogTypeConsume.
type ChannelRequestDailyStat struct {
	Id               int   `json:"id"`
	ChannelId        int   `json:"channel_id" gorm:"index;uniqueIndex:idx_channel_day,priority:1"`
	Day              int   `json:"day" gorm:"index;uniqueIndex:idx_channel_day,priority:2"`
	SuccessCount     int64 `json:"success_count" gorm:"bigint;default:0"`
	UsedQuota        int64 `json:"used_quota" gorm:"bigint;default:0"`
	VisibleUsedQuota int64 `json:"visible_used_quota" gorm:"bigint;default:0"`
	CostUsedQuota    int64 `json:"cost_used_quota" gorm:"bigint;default:0"`
}

func IncrementChannelRequestSuccessStats(channelId int, createdAt int64) {
	if channelId <= 0 || createdAt <= 0 {
		return
	}

	InitStatsBuffer()
	if statsBufferEnabledNow() {
		addChannelDailySuccessDelta(channelId, createdAt, 1)
		return
	}

	day := common.DateToInt(time.Unix(createdAt, 0))

	if err := DB.Model(&Channel{}).
		Where("id = ?", channelId).
		Update("request_success_count", gorm.Expr("request_success_count + ?", 1)).
		Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to increment channel request_success_count: channel_id=%d, err=%v", channelId, err))
	}

	stat := &ChannelRequestDailyStat{
		ChannelId:    channelId,
		Day:          day,
		SuccessCount: 1,
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "day"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"success_count": gorm.Expr("success_count + ?", 1),
		}),
	}).Create(stat).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to upsert channel request daily stat: channel_id=%d, day=%d, err=%v", channelId, day, err))
	}
}

func AddChannelDailyUsedQuota(channelId int, createdAt int64, deltaQuota int64) {
	AddChannelDailyUsageQuotas(channelId, createdAt, deltaQuota, deltaQuota, 0)
}

func AddChannelDailyCostUsedQuota(channelId int, createdAt int64, deltaQuota int64) {
	AddChannelDailyUsageQuotas(channelId, createdAt, 0, 0, deltaQuota)
}

func AddChannelDailyUsageQuotas(channelId int, createdAt int64, deltaUsedQuota int64, deltaVisibleQuota int64, deltaCostQuota int64) {
	if channelId <= 0 || createdAt <= 0 || (deltaUsedQuota == 0 && deltaVisibleQuota == 0 && deltaCostQuota == 0) {
		return
	}

	InitStatsBuffer()
	if statsBufferEnabledNow() {
		addChannelDailyUsageQuotaDelta(channelId, createdAt, deltaUsedQuota, deltaVisibleQuota, deltaCostQuota)
		return
	}

	day := common.DateToInt(time.Unix(createdAt, 0))
	stat := &ChannelRequestDailyStat{
		ChannelId:        channelId,
		Day:              day,
		UsedQuota:        deltaUsedQuota,
		VisibleUsedQuota: deltaVisibleQuota,
		CostUsedQuota:    deltaCostQuota,
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "day"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"used_quota":         gorm.Expr("used_quota + ?", deltaUsedQuota),
			"visible_used_quota": gorm.Expr("visible_used_quota + ?", deltaVisibleQuota),
			"cost_used_quota":    gorm.Expr("cost_used_quota + ?", deltaCostQuota),
		}),
	}).Create(stat).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to upsert channel daily usage quotas: channel_id=%d, day=%d, delta_used=%d, delta_visible=%d, delta_cost=%d, err=%v", channelId, day, deltaUsedQuota, deltaVisibleQuota, deltaCostQuota, err))
	}
}

func ListChannelRequestDailyStats(channelId int, startDay int, endDay int) ([]*ChannelRequestDailyStat, error) {
	if channelId <= 0 {
		return nil, fmt.Errorf("channelId 无效")
	}
	if startDay <= 0 || endDay <= 0 || startDay > endDay {
		return nil, fmt.Errorf("日期范围无效")
	}

	rows := make([]*ChannelRequestDailyStat, 0)
	err := DB.Model(&ChannelRequestDailyStat{}).
		Where("channel_id = ? AND day >= ? AND day <= ?", channelId, startDay, endDay).
		Order("day asc").
		Find(&rows).
		Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

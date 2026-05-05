package model

import (
	"fmt"
	"one-api/common"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserRequestDailyStat stores per-user successful consume request counts and quota usage by day (YYYYMMDD).
// It is derived from logs where type == LogTypeConsume.
type UserRequestDailyStat struct {
	Id               int   `json:"id"`
	UserId           int   `json:"user_id" gorm:"index;uniqueIndex:idx_user_day,priority:1"`
	Day              int   `json:"day" gorm:"index;uniqueIndex:idx_user_day,priority:2;index:idx_user_day_quota,priority:1;index:idx_user_day_success,priority:1"`
	SuccessCount     int64 `json:"success_count" gorm:"bigint;default:0;index:idx_user_day_success,priority:2"`
	UsedQuota        int64 `json:"used_quota" gorm:"bigint;default:0;index:idx_user_day_quota,priority:2"`
	VisibleUsedQuota int64 `json:"visible_used_quota" gorm:"bigint;default:0"`
	CostUsedQuota    int64 `json:"cost_used_quota" gorm:"bigint;default:0"`
	Tokens           int64 `json:"tokens" gorm:"bigint;default:0"`
}

func (UserRequestDailyStat) TableName() string {
	return "user_request_daily_stats"
}

func IncrementUserRequestDailyStats(userId int, createdAt int64, deltaQuota int64, deltaVisibleQuota int64, deltaCostQuota int64, deltaTokens int64) {
	if userId <= 0 || createdAt <= 0 {
		return
	}

	InitStatsBuffer()
	if statsBufferEnabledNow() {
		addUserDailyStatDelta(userId, createdAt, deltaQuota, deltaVisibleQuota, deltaCostQuota, deltaTokens)
		return
	}

	day := common.DateToInt(time.Unix(createdAt, 0))

	stat := &UserRequestDailyStat{
		UserId:           userId,
		Day:              day,
		SuccessCount:     1,
		UsedQuota:        deltaQuota,
		VisibleUsedQuota: deltaVisibleQuota,
		CostUsedQuota:    deltaCostQuota,
		Tokens:           deltaTokens,
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "day"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"success_count":      gorm.Expr("success_count + ?", 1),
			"used_quota":         gorm.Expr("used_quota + ?", deltaQuota),
			"visible_used_quota": gorm.Expr("visible_used_quota + ?", deltaVisibleQuota),
			"cost_used_quota":    gorm.Expr("cost_used_quota + ?", deltaCostQuota),
			"tokens":             gorm.Expr("tokens + ?", deltaTokens),
		}),
	}).Create(stat).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to upsert user request daily stat: user_id=%d, day=%d, err=%v", userId, day, err))
	}
}

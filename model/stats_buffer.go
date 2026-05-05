package model

import (
	"fmt"
	"one-api/common"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type userDailyStatDelta struct {
	SuccessCount     int64
	UsedQuota        int64
	VisibleUsedQuota int64
	CostUsedQuota    int64
	Tokens           int64
}

type channelDailyStatDelta struct {
	SuccessCount     int64
	UsedQuota        int64
	VisibleUsedQuota int64
	CostUsedQuota    int64
}

var (
	statsBufferMu       sync.Mutex
	statsBufferStarted  bool
	statsBufferEnabled  bool
	statsBufferInterval time.Duration

	userDailyStatsMu sync.Mutex
	userDailyStats   map[int64]userDailyStatDelta // key=userId<<32|day

	channelDailyStatsMu sync.Mutex
	channelDailyStats   map[int64]channelDailyStatDelta // key=channelId<<32|day
)

func InitStatsBuffer() {
	statsBufferMu.Lock()
	defer statsBufferMu.Unlock()
	if statsBufferStarted {
		return
	}

	statsBufferEnabled = common.GetEnvOrDefaultBool("STATS_BUFFER_ENABLED", true)
	if !statsBufferEnabled {
		statsBufferStarted = true
		return
	}

	intervalSeconds := common.GetEnvOrDefault("STATS_BUFFER_INTERVAL_SECONDS", 5)
	if intervalSeconds <= 0 {
		intervalSeconds = 5
	}
	statsBufferInterval = time.Duration(intervalSeconds) * time.Second

	userDailyStats = make(map[int64]userDailyStatDelta)
	channelDailyStats = make(map[int64]channelDailyStatDelta)

	statsBufferStarted = true

	gopool.Go(func() {
		ticker := time.NewTicker(statsBufferInterval)
		defer ticker.Stop()
		for range ticker.C {
			flushStatsBuffers()
		}
	})
}

func statsBufferEnabledNow() bool {
	// No lock: enabled is set once on InitStatsBuffer and is immutable afterwards.
	return statsBufferEnabled
}

func makeCompositeKey(id int, day int) int64 {
	return (int64(id) << 32) | int64(uint32(day))
}

func addUserDailyStatDelta(userId int, createdAt int64, deltaQuota int64, deltaVisibleQuota int64, deltaCostQuota int64, deltaTokens int64) {
	if userId <= 0 || createdAt <= 0 {
		return
	}
	day := common.DateToInt(time.Unix(createdAt, 0))
	key := makeCompositeKey(userId, day)
	userDailyStatsMu.Lock()
	defer userDailyStatsMu.Unlock()
	d := userDailyStats[key]
	d.SuccessCount += 1
	d.UsedQuota += deltaQuota
	d.VisibleUsedQuota += deltaVisibleQuota
	d.CostUsedQuota += deltaCostQuota
	d.Tokens += deltaTokens
	userDailyStats[key] = d
}

func addChannelDailySuccessDelta(channelId int, createdAt int64, deltaSuccess int64) {
	if channelId <= 0 || createdAt <= 0 || deltaSuccess == 0 {
		return
	}
	day := common.DateToInt(time.Unix(createdAt, 0))
	key := makeCompositeKey(channelId, day)
	channelDailyStatsMu.Lock()
	defer channelDailyStatsMu.Unlock()
	d := channelDailyStats[key]
	d.SuccessCount += deltaSuccess
	channelDailyStats[key] = d
}

func addChannelDailyUsedQuotaDelta(channelId int, createdAt int64, deltaQuota int64) {
	addChannelDailyUsageQuotaDelta(channelId, createdAt, deltaQuota, deltaQuota, 0)
}

func addChannelDailyVisibleUsedQuotaDelta(channelId int, createdAt int64, deltaVisibleQuota int64) {
	addChannelDailyUsageQuotaDelta(channelId, createdAt, 0, deltaVisibleQuota, 0)
}

func addChannelDailyCostUsedQuotaDelta(channelId int, createdAt int64, deltaQuota int64) {
	addChannelDailyUsageQuotaDelta(channelId, createdAt, 0, 0, deltaQuota)
}

func addChannelDailyUsageQuotaDelta(channelId int, createdAt int64, deltaUsedQuota int64, deltaVisibleQuota int64, deltaCostQuota int64) {
	if channelId <= 0 || createdAt <= 0 || (deltaUsedQuota == 0 && deltaVisibleQuota == 0 && deltaCostQuota == 0) {
		return
	}
	day := common.DateToInt(time.Unix(createdAt, 0))
	key := makeCompositeKey(channelId, day)
	channelDailyStatsMu.Lock()
	defer channelDailyStatsMu.Unlock()
	d := channelDailyStats[key]
	d.UsedQuota += deltaUsedQuota
	d.VisibleUsedQuota += deltaVisibleQuota
	d.CostUsedQuota += deltaCostQuota
	channelDailyStats[key] = d
}

func flushStatsBuffers() {
	if !statsBufferEnabledNow() || DB == nil {
		return
	}

	userDailyStatsMu.Lock()
	userStore := userDailyStats
	userDailyStats = make(map[int64]userDailyStatDelta)
	userDailyStatsMu.Unlock()

	channelDailyStatsMu.Lock()
	channelStore := channelDailyStats
	channelDailyStats = make(map[int64]channelDailyStatDelta)
	channelDailyStatsMu.Unlock()

	if len(userStore) == 0 && len(channelStore) == 0 {
		return
	}

	// user_request_daily_stats
	if len(userStore) > 0 {
		rows := make([]UserRequestDailyStat, 0, len(userStore))
		for key, d := range userStore {
			uid := int(key >> 32)
			day := int(uint32(key))
			if uid <= 0 || day <= 0 {
				continue
			}
			rows = append(rows, UserRequestDailyStat{
				UserId:           uid,
				Day:              day,
				SuccessCount:     d.SuccessCount,
				UsedQuota:        d.UsedQuota,
				VisibleUsedQuota: d.VisibleUsedQuota,
				CostUsedQuota:    d.CostUsedQuota,
				Tokens:           d.Tokens,
			})
		}
		if len(rows) > 0 {
			exprSuccess := "excluded.success_count"
			exprQuota := "excluded.used_quota"
			exprVisibleQuota := "excluded.visible_used_quota"
			exprCostQuota := "excluded.cost_used_quota"
			exprTokens := "excluded.tokens"
			if common.UsingMySQL {
				exprSuccess = "VALUES(success_count)"
				exprQuota = "VALUES(used_quota)"
				exprVisibleQuota = "VALUES(visible_used_quota)"
				exprCostQuota = "VALUES(cost_used_quota)"
				exprTokens = "VALUES(tokens)"
			}
			if err := DB.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "day"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"success_count":      gorm.Expr("success_count + " + exprSuccess),
					"used_quota":         gorm.Expr("used_quota + " + exprQuota),
					"visible_used_quota": gorm.Expr("visible_used_quota + " + exprVisibleQuota),
					"cost_used_quota":    gorm.Expr("cost_used_quota + " + exprCostQuota),
					"tokens":             gorm.Expr("tokens + " + exprTokens),
				}),
			}).Create(&rows).Error; err != nil {
				common.SysLog(fmt.Sprintf("failed to flush user daily stats buffer: %v", err))
			}
		}
	}

	// channel_request_daily_stats + channels.request_success_count
	if len(channelStore) > 0 {
		rows := make([]ChannelRequestDailyStat, 0, len(channelStore))
		channelSuccessDelta := make(map[int]int64)
		for key, d := range channelStore {
			cid := int(key >> 32)
			day := int(uint32(key))
			if cid <= 0 || day <= 0 {
				continue
			}
			rows = append(rows, ChannelRequestDailyStat{
				ChannelId:        cid,
				Day:              day,
				SuccessCount:     d.SuccessCount,
				UsedQuota:        d.UsedQuota,
				VisibleUsedQuota: d.VisibleUsedQuota,
				CostUsedQuota:    d.CostUsedQuota,
			})
			if d.SuccessCount != 0 {
				channelSuccessDelta[cid] += d.SuccessCount
			}
		}
		if len(rows) > 0 {
			exprSuccess := "excluded.success_count"
			exprUsedQuota := "excluded.used_quota"
			exprVisibleQuota := "excluded.visible_used_quota"
			exprCostQuota := "excluded.cost_used_quota"
			if common.UsingMySQL {
				exprSuccess = "VALUES(success_count)"
				exprUsedQuota = "VALUES(used_quota)"
				exprVisibleQuota = "VALUES(visible_used_quota)"
				exprCostQuota = "VALUES(cost_used_quota)"
			}
			if err := DB.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "channel_id"}, {Name: "day"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"success_count":      gorm.Expr("success_count + " + exprSuccess),
					"used_quota":         gorm.Expr("used_quota + " + exprUsedQuota),
					"visible_used_quota": gorm.Expr("visible_used_quota + " + exprVisibleQuota),
					"cost_used_quota":    gorm.Expr("cost_used_quota + " + exprCostQuota),
				}),
			}).Create(&rows).Error; err != nil {
				common.SysLog(fmt.Sprintf("failed to flush channel daily stats buffer: %v", err))
			}
		}
		for channelId, delta := range channelSuccessDelta {
			if channelId <= 0 || delta == 0 {
				continue
			}
			if err := DB.Model(&Channel{}).
				Where("id = ?", channelId).
				Update("request_success_count", gorm.Expr("request_success_count + ?", delta)).
				Error; err != nil {
				common.SysLog(fmt.Sprintf("failed to flush channel request_success_count: channel_id=%d delta=%d err=%v", channelId, delta, err))
			}
		}
	}
}

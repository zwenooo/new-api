package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GetDailyTokenKingRank(c *gin.Context) {
	if common.DisplayTokenStatEnabled == false {
		c.JSON(http.StatusOK, gin.H{
			"success":   true,
			"message":   "",
			"rank_mode": common.StompKingRankMode,
			"data":      []model.TopTokenUser{},
		})
		return
	}
	now := time.Now().In(time.Local)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	dateStr := strings.TrimSpace(c.Query("date"))

	startTimestamp := todayStart.Unix()
	endTimestamp := now.Unix()

	if dateStr != "" {
		selectedDay, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "date 无效，格式应为 YYYY-MM-DD"})
			return
		}
		selectedStart := time.Date(selectedDay.Year(), selectedDay.Month(), selectedDay.Day(), 0, 0, 0, 0, time.Local)

		minAllowedStart := todayStart.AddDate(0, 0, -6)
		if selectedStart.Before(minAllowedStart) || selectedStart.After(todayStart) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "date 超出范围，仅支持近 7 天"})
			return
		}

		startTimestamp = selectedStart.Unix()
		if selectedStart.Equal(todayStart) {
			endTimestamp = now.Unix()
		} else {
			endTimestamp = selectedStart.AddDate(0, 0, 1).Unix() - 1
		}
	}

	rankMode := strings.TrimSpace(c.Query("rank_mode"))
	if rankMode == "" {
		rankMode = common.StompKingRankMode
	}
	if rankMode == common.StompKingRankModeVisibleQuota {
		rankMode = common.StompKingRankModeCostQuota
	}
	if rankMode != common.StompKingRankModeQuota &&
		rankMode != common.StompKingRankModeCostQuota &&
		rankMode != common.StompKingRankModeSuccessCount {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "rank_mode 无效，仅支持 quota、cost_quota 或 success_count"})
		return
	}

	items, err := model.GetDailyTopKingUsers(startTimestamp, endTimestamp, 10, rankMode)
	if items == nil {
		items = []model.TopTokenUser{}
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "",
		"rank_mode": rankMode,
		"data":      items,
	})
}

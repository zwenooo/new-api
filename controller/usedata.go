package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"

	"github.com/gin-gonic/gin"
)

func markQuotaLegacyByHourBuckets(dates []*model.QuotaData, buckets map[int64]struct{}) {
	if len(dates) == 0 || len(buckets) == 0 {
		return
	}
	for i := range dates {
		if dates[i] == nil {
			continue
		}
		bucket := dates[i].CreatedAt - (dates[i].CreatedAt % 3600)
		if _, ok := buckets[bucket]; ok {
			dates[i].QuotaLegacy = true
		}
	}
}

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if username != "" {
		var user model.User
		if err := model.DB.Where("username = ?", username).First(&user).Error; err == nil && user.Id > 0 {
			buckets, bucketErr := model.GetLegacyHiddenUserConsumeQuotaBuckets(user.Id, startTimestamp, endTimestamp)
			if bucketErr != nil {
				common.ApiError(c, bucketErr)
				return
			}
			markQuotaLegacyByHourBuckets(dates, buckets)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	buckets, bucketErr := model.GetLegacyHiddenUserConsumeQuotaBuckets(userId, startTimestamp, endTimestamp)
	if bucketErr != nil {
		common.ApiError(c, bucketErr)
		return
	}
	markQuotaLegacyByHourBuckets(dates, buckets)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

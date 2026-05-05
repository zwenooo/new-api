package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"

	"github.com/gin-gonic/gin"
)

func ListInvitationSubscriptionRecords(c *gin.Context) {
	inviterId := c.GetInt("id")

	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	paidUserCount, err := model.CountInvitedPaidUsersBySubscription(inviterId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	records, total, err := model.ListInvitationSubscriptionRecords(inviterId, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":           records,
			"total":           total,
			"paid_user_count": paidUserCount,
			"page":            page,
			"page_size":       pageSize,
		},
	})
}


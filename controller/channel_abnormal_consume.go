package controller

import (
	"net/http"
	"one-api/model"
	"one-api/service"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type channelAbnormalConsumeConfigRequest struct {
	ChannelID int  `json:"channel_id"`
	Enabled   bool `json:"enabled"`
}

func GetChannelAbnormalConsumeConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"enabled":     service.GetChannelAbnormalConsumeEnabledMapCopy(),
			"max_records": service.ChannelAbnormalConsumeMaxRecords,
		},
	})
}

func UpdateChannelAbnormalConsumeConfig(c *gin.Context) {
	var req channelAbnormalConsumeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if req.ChannelID <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel_id is required",
		})
		return
	}

	_, err := model.GetChannelById(req.ChannelID, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel not found",
		})
		return
	}

	service.SetChannelAbnormalConsumeEnabled(req.ChannelID, req.Enabled)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id": req.ChannelID,
			"enabled":    req.Enabled,
		},
	})
}

func ListChannelAbnormalConsumeRecords(c *gin.Context) {
	channelID, ok := parseChannelIDQuery(c)
	if !ok {
		return
	}

	limit := service.ChannelAbnormalConsumeMaxRecords
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		n, err := strconv.Atoi(rawLimit)
		if err != nil || n <= 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "invalid limit",
			})
			return
		}
		if n < limit {
			limit = n
		}
	}

	items := service.ListChannelAbnormalConsumeRecords(channelID, limit)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id":   channelID,
			"enabled":      service.IsChannelAbnormalConsumeEnabled(channelID),
			"max_records":  service.ChannelAbnormalConsumeMaxRecords,
			"items":        items,
		},
	})
}

func ClearChannelAbnormalConsumeRecords(c *gin.Context) {
	channelID, ok := parseChannelIDQuery(c)
	if !ok {
		return
	}
	service.ClearChannelAbnormalConsumeRecords(channelID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func parseChannelIDQuery(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.Query("channel_id"))
	if raw == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel_id is required",
		})
		return 0, false
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid channel_id",
		})
		return 0, false
	}
	_, err = model.GetChannelById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel not found",
		})
		return 0, false
	}
	return id, true
}

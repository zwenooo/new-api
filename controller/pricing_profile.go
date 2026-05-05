package controller

import (
	"net/http"
	"strconv"

	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

type pricingProfileUpsertRequest struct {
	Code          string                   `json:"code"`
	Name          string                   `json:"name"`
	Audience      string                   `json:"audience"`
	DefaultFactor float64                  `json:"default_factor"`
	Enabled       *bool                    `json:"enabled"`
	Description   string                   `json:"description"`
	GroupFactors  []model.PriceGroupFactor `json:"group_factors"`
}

func ListPricingProfiles(c *gin.Context) {
	items, err := model.ListPricingProfiles(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    items,
	})
}

func ListLegacyPricingUsers(c *gin.Context) {
	items, err := model.ListLegacyPricingUsers(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    items,
	})
}

func CreatePricingProfile(c *gin.Context) {
	var req pricingProfileUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := model.CreatePricingProfile(nil, model.SavePricingProfileParams{
		Code:          req.Code,
		Name:          req.Name,
		Audience:      req.Audience,
		DefaultFactor: req.DefaultFactor,
		Enabled:       enabled,
		Description:   req.Description,
		GroupFactors:  req.GroupFactors,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    item,
	})
}

func UpdatePricingProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "价格模板 id 无效",
		})
		return
	}
	var req pricingProfileUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := model.UpdatePricingProfile(nil, id, model.SavePricingProfileParams{
		Code:          req.Code,
		Name:          req.Name,
		Audience:      req.Audience,
		DefaultFactor: req.DefaultFactor,
		Enabled:       enabled,
		Description:   req.Description,
		GroupFactors:  req.GroupFactors,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    item,
	})
}

func DeletePricingProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "价格模板 id 无效",
		})
		return
	}
	if err := model.DeletePricingProfile(nil, id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

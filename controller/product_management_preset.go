package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

func ListProductManagementPresets(c *gin.Context) {
	ListRedemptionPresets(c)
}

func UpsertProductManagementPreset(c *gin.Context) {
	UpsertRedemptionPreset(c)
}

func ListProductManagementPresetRevisions(c *gin.Context) {
	ListRedemptionPresetRevisions(c)
}

func RestoreProductManagementPresetRevision(c *gin.Context) {
	RestoreRedemptionPresetRevision(c)
}

func DeleteProductManagementPreset(c *gin.Context) {
	DeleteRedemptionPreset(c)
}

func GenerateProductManagementPresetRedemptions(c *gin.Context) {
	GenerateRedemptionByPreset(c)
}

type productManagementReorderItem struct {
	Type      string `json:"type"`
	Id        int    `json:"id"`
	SortOrder int    `json:"sort_order"`
}

type productManagementReorderRequest struct {
	Products []productManagementReorderItem `json:"products"`
}

func ReorderProductManagementProducts(c *gin.Context) {
	var req productManagementReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(req.Products) == 0 {
		common.ApiErrorMsg(c, "products 不能为空")
		return
	}

	products := make([]model.ProductManagementReorderItem, 0, len(req.Products))
	for _, item := range req.Products {
		products = append(products, model.ProductManagementReorderItem{
			Type:      item.Type,
			Id:        item.Id,
			SortOrder: item.SortOrder,
		})
	}

	if err := model.ReorderProductManagementProducts(products); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

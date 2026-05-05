package controller

import "github.com/gin-gonic/gin"

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

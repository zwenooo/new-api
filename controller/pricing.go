package controller

import (
	"one-api/model"
	"one-api/setting"
	"one-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func buildPricingGroupRatioKeepSet(usableGroup map[int]string, extraGroupIDs []int) map[int]struct{} {
	keepSet := make(map[int]struct{}, len(usableGroup)+len(extraGroupIDs))
	for groupID := range usableGroup {
		if groupID <= 0 {
			continue
		}
		keepSet[groupID] = struct{}{}
	}
	for _, groupID := range extraGroupIDs {
		if groupID <= 0 {
			continue
		}
		keepSet[groupID] = struct{}{}
	}
	return keepSet
}

func filterPricingGroupRatios(groupRatio map[int]float64, keepSet map[int]struct{}) map[int]float64 {
	if len(groupRatio) == 0 {
		return groupRatio
	}
	if len(keepSet) == 0 {
		return map[int]float64{}
	}
	filtered := make(map[int]float64, len(groupRatio))
	for groupID, ratio := range groupRatio {
		if _, ok := keepSet[groupID]; !ok {
			continue
		}
		filtered[groupID] = ratio
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	var userGroupID int
	usableGroup := map[int]string{}
	groupRatio := ratio_setting.GetGroupRatioCopy()
	noBillingEligibleSet := map[int]struct{}{}
	var userBase *model.UserBase
	var billableGroupIDs []int
	if exists {
		currentUserID := userId.(int)
		eligible, err := model.GetUserEligibleNoBillingGroupSet(currentUserID)
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		noBillingEligibleSet = eligible
		user, err := model.GetUserCache(currentUserID)
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		userBase = user
		if user.UserGroupId > 0 {
			userGroupID = user.UserGroupId
		} else {
			userGroupID = user.GroupId
		}

		billableGroupIDs, err = model.GetUserBillableGroupIDs(currentUserID)
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		for groupID := range groupRatio {
			if _, ok := noBillingEligibleSet[groupID]; ok {
				groupRatio[groupID] = 0
				continue
			}
			groupRatio[groupID] = model.ResolveDisplayedGroupRatio(user, groupID)
		}
	}

	if userBase == nil {
		for g := range groupRatio {
			if _, ok := noBillingEligibleSet[g]; ok {
				groupRatio[g] = 0
			}
		}
	}

	usableGroup = setting.GetUserUsableGroups(userGroupID)
	groupRatio = filterPricingGroupRatios(
		groupRatio,
		buildPricingGroupRatioKeepSet(usableGroup, billableGroupIDs),
	)

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        setting.AutoGroups,
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}

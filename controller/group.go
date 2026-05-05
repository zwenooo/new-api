package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"one-api/common"
	"one-api/model"
	"one-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groups, err := model.ListGroups(nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groups,
	})
}

type createGroupRequest struct {
	Code                        string          `json:"code"`
	Name                        string          `json:"name"`
	DisplayName                 string          `json:"display_name"`
	Description                 string          `json:"description"`
	AllowedModels               json.RawMessage `json:"allowed_models"`
	AllowedModelPrefillGroupIds json.RawMessage `json:"allowed_model_prefill_group_ids"`
	AllowedUserAgents           json.RawMessage `json:"allowed_user_agents"`
	Ratio                       *float64        `json:"ratio"`
	NoBilling                   *bool           `json:"no_billing"`
	NoBillingProductKeys        json.RawMessage `json:"no_billing_product_keys"`
	UserSelectable              *bool           `json:"user_selectable"`
	Enabled                     *bool           `json:"enabled"`
}

func CreateGroup(c *gin.Context) {
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	ratio := 1.0
	if req.Ratio != nil {
		ratio = *req.Ratio
	}
	noBilling := false
	if req.NoBilling != nil {
		noBilling = *req.NoBilling
	}
	userSelectable := true
	if req.UserSelectable != nil {
		userSelectable = *req.UserSelectable
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	name := req.Code
	if name == "" {
		name = req.Name
	}
	if name == "" {
		name = req.DisplayName
	}

	g := &model.Group{
		Code:                        name,
		Name:                        name,
		DisplayName:                 name,
		Description:                 req.Description,
		AllowedModels:               model.JSONValue(req.AllowedModels),
		AllowedModelPrefillGroupIds: model.JSONValue(req.AllowedModelPrefillGroupIds),
		AllowedUserAgents:           model.JSONValue(req.AllowedUserAgents),
		Ratio:                       ratio,
		NoBilling:                   noBilling,
		NoBillingProductKeys:        model.JSONValue(req.NoBillingProductKeys),
		UserSelectable:              userSelectable,
		Enabled:                     enabled,
	}
	if err := model.CreateGroup(nil, g); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	g.NormalizeForResponse()
	if err := model.RefreshGroupSettings(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    g,
	})
}

type updateGroupRequest struct {
	Id                          int             `json:"id"`
	Code                        string          `json:"code"`
	Name                        *string         `json:"name"`
	DisplayName                 *string         `json:"display_name"`
	Description                 *string         `json:"description"`
	AllowedModels               json.RawMessage `json:"allowed_models"`
	AllowedModelPrefillGroupIds json.RawMessage `json:"allowed_model_prefill_group_ids"`
	AllowedUserAgents           json.RawMessage `json:"allowed_user_agents"`
	Ratio                       *float64        `json:"ratio"`
	NoBilling                   *bool           `json:"no_billing"`
	NoBillingProductKeys        json.RawMessage `json:"no_billing_product_keys"`
	UserSelectable              *bool           `json:"user_selectable"`
	Enabled                     *bool           `json:"enabled"`
}

func UpdateGroup(c *gin.Context) {
	var req updateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if req.Id <= 0 {
		if req.Code != "" {
			group, err := model.GetGroupByCode(nil, req.Code)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
			req.Id = group.Id
		} else {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 必须提供"})
			return
		}
	}

	var codePtr *string
	if req.Name != nil {
		trimmed := *req.Name
		codePtr = &trimmed
	} else if req.Code != "" {
		trimmed := req.Code
		codePtr = &trimmed
	} else if req.DisplayName != nil {
		trimmed := *req.DisplayName
		codePtr = &trimmed
	}

	var allowedModelsPtr *model.JSONValue
	if req.AllowedModels != nil {
		v := model.JSONValue(req.AllowedModels)
		allowedModelsPtr = &v
	}

	var allowedModelPrefillGroupIdsPtr *model.JSONValue
	if req.AllowedModelPrefillGroupIds != nil {
		v := model.JSONValue(req.AllowedModelPrefillGroupIds)
		allowedModelPrefillGroupIdsPtr = &v
	}

	var allowedUserAgentsPtr *model.JSONValue
	if req.AllowedUserAgents != nil {
		v := model.JSONValue(req.AllowedUserAgents)
		allowedUserAgentsPtr = &v
	}

	var noBillingProductKeysPtr *model.JSONValue
	if req.NoBillingProductKeys != nil {
		v := model.JSONValue(req.NoBillingProductKeys)
		noBillingProductKeysPtr = &v
	}

	updated, err := model.UpdateGroupByID(nil, req.Id, model.UpdateGroupParams{
		Code:                        codePtr,
		Description:                 req.Description,
		AllowedModels:               allowedModelsPtr,
		AllowedModelPrefillGroupIds: allowedModelPrefillGroupIdsPtr,
		AllowedUserAgents:           allowedUserAgentsPtr,
		Ratio:                       req.Ratio,
		NoBilling:                   req.NoBilling,
		NoBillingProductKeys:        noBillingProductKeysPtr,
		UserSelectable:              req.UserSelectable,
		Enabled:                     req.Enabled,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.RefreshGroupSettings(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    updated,
	})
}

func DeleteGroup(c *gin.Context) {
	id := common.String2Int(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}
	summary, err := model.DeleteGroupByID(nil, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": summary})
}

func GetGroupNoBillingProductOptions(c *gin.Context) {
	options, err := model.ListGroupNoBillingProductOptions(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

func GetGroupChannels(c *gin.Context) {
	groupID := common.String2Int(c.Param("id"))
	if groupID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}

	items, err := model.ListGroupChannelBindings(nil, groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	selectedCount := 0
	for _, item := range items {
		if item.Selected {
			selectedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":          items,
			"selected_count": selectedCount,
		},
	})
}

type syncGroupChannelsRequest struct {
	ChannelIds []int `json:"channel_ids"`
}

type remapGroupTokensRequest struct {
	TargetGroupId int `json:"target_group_id"`
}

type saveGroupUserPriceOverrideEntry struct {
	UserId int     `json:"user_id"`
	Factor float64 `json:"factor"`
}

type syncGroupUserPriceOverridesRequest struct {
	Entries []saveGroupUserPriceOverrideEntry `json:"entries"`
}

func SyncGroupChannels(c *gin.Context) {
	groupID := common.String2Int(c.Param("id"))
	if groupID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}

	var req syncGroupChannelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}

	affected, err := model.SyncGroupChannelBindings(nil, groupID, req.ChannelIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	model.BumpChannelCacheRevision()
	model.InitChannelCache()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"affected": affected,
		},
	})
}

func RemapGroupTokens(c *gin.Context) {
	sourceGroupID := common.String2Int(c.Param("id"))
	if sourceGroupID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}

	var req remapGroupTokensRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}

	summary, err := model.BulkRemapTokenAllowedGroups(nil, sourceGroupID, req.TargetGroupId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    summary,
	})
}

func GetGroupUserPriceOverrides(c *gin.Context) {
	groupID := common.String2Int(c.Param("id"))
	if groupID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}

	items, err := model.ListUserGroupPriceOverridesByGroup(nil, groupID)
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

func SyncGroupUserPriceOverrides(c *gin.Context) {
	groupID := common.String2Int(c.Param("id"))
	if groupID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}

	var req syncGroupUserPriceOverridesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}

	entries := make([]model.SaveGroupUserPriceOverrideEntry, 0, len(req.Entries))
	for _, entry := range req.Entries {
		entries = append(entries, model.SaveGroupUserPriceOverrideEntry{
			UserId: entry.UserId,
			Factor: entry.Factor,
		})
	}

	count, affectedUserIDs, err := model.SyncUserGroupPriceOverridesByGroup(nil, groupID, entries)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RefreshPricingRuleCache(); err != nil {
		common.ApiError(c, err)
		return
	}
	for _, userID := range affectedUserIDs {
		_ = model.InvalidateUserCache(userID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count":             count,
			"affected_user_ids": affectedUserIDs,
		},
	})
}

func GetUserGroups(c *gin.Context) {
	groups, err := model.ListGroups(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userId := c.GetInt("id")
	userRole := c.GetInt("role")
	isAdmin := userRole >= common.RoleAdminUser
	var userBase *model.UserBase

	// Load current user's consumable groups once for both filtering and UI hints.
	ownedSet := map[int]struct{}{}
	noBillingEligibleSet := map[int]struct{}{}
	if userId > 0 {
		owned, oErr := model.GetUserBillableGroupIDs(userId)
		if oErr != nil {
			common.ApiError(c, oErr)
			return
		}
		ownedSet = make(map[int]struct{}, len(owned))
		for _, gid := range owned {
			if gid <= 0 {
				continue
			}
			ownedSet[gid] = struct{}{}
		}
		eligible, eErr := model.GetUserEligibleNoBillingGroupSet(userId)
		if eErr != nil {
			common.ApiError(c, eErr)
			return
		}
		noBillingEligibleSet = eligible
		userBase, err = model.GetUserCache(userId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	items := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		if g.Id <= 0 {
			continue
		}
			if !g.Enabled {
				continue
			}
			if model.IsInternalDefaultModelGroupCode(g.Code) {
				continue
			}
			if !isAdmin && !g.UserSelectable {
				if _, ok := ownedSet[g.Id]; !ok {
					continue
			}
		}
		g.NormalizeForResponse()
		desc := strings.TrimSpace(g.Description)
		if desc == "" {
			desc = strings.TrimSpace(g.Code)
		}
		baseRatio := ratio_setting.GetGroupRatio(g.Id)
		ratio := model.ResolveDisplayedGroupRatio(userBase, g.Id)
		_, noBillingEligible := noBillingEligibleSet[g.Id]
		if noBillingEligible {
			ratio = 0
		}
		_, billable := ownedSet[g.Id]
		items = append(items, map[string]interface{}{
			"id":           g.Id,
			"code":         g.Code,
			"display_name": g.DisplayName,
			"ratio":        ratio,
			"base_ratio":   baseRatio,
			"desc":         desc,
			"billable":     billable,
			"no_billing":   noBillingEligible,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    items,
	})
}

// ResolveGroups returns basic group metadata for display purposes.
// It is user-authenticated and should not be used for admin CRUD.
//
// Query:
//   - ids: optional comma-separated group ids (e.g. "1,2,3"). When omitted, returns all groups.
func ResolveGroups(c *gin.Context) {
	idsParam := strings.TrimSpace(c.Query("ids"))
	ids := make([]int, 0)
	if idsParam != "" {
		for _, raw := range strings.Split(idsParam, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			id, err := strconv.Atoi(raw)
			if err != nil || id <= 0 {
				continue
			}
			ids = append(ids, id)
		}
		ids = model.NormalizeUniqueSortedIDs(ids)
	}

	groups, err := model.ListGroups(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	filter := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		filter[id] = struct{}{}
	}

	items := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		if len(filter) > 0 {
			if _, ok := filter[g.Id]; !ok {
				continue
			}
		}
		g.NormalizeForResponse()
		items = append(items, map[string]interface{}{
			"id":              g.Id,
			"code":            g.Code,
			"display_name":    g.DisplayName,
			"user_selectable": g.UserSelectable,
		})
	}

	common.ApiSuccess(c, items)
}

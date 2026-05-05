package controller

import (
	"net/http"

	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

func GetUserGroupsAdmin(c *gin.Context) {
	items, err := model.ListUserGroups(nil)
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

type createUserGroupRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
	Enabled     *bool  `json:"enabled"`
}

func CreateUserGroup(c *gin.Context) {
	var req createUserGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	group := &model.UserGroup{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		Enabled:     enabled,
	}
	if err := model.CreateUserGroup(nil, group); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    group,
	})
}

type updateUserGroupRequest struct {
	Id          int     `json:"id"`
	Code        *string `json:"code"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int    `json:"sort_order"`
	Enabled     *bool   `json:"enabled"`
}

func UpdateUserGroup(c *gin.Context) {
	var req updateUserGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	group, err := model.UpdateUserGroupByID(nil, req.Id, model.UpdateUserGroupParams{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		Enabled:     req.Enabled,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    group,
	})
}

func DeleteUserGroup(c *gin.Context) {
	id := common.String2Int(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "id 无效"})
		return
	}
	if err := model.DeleteUserGroupByID(nil, id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

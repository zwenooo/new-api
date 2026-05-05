package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"codex-service-go/pkg/proxyurl"

	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

func validateProxyPrefillGroupItems(raw model.JSONValue) (string, error) {
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", fmt.Errorf("代理组 items 必须为 JSON 数组: %w", err)
	}
	if len(items) != 1 {
		return "", fmt.Errorf("代理组 items 必须且只能包含 1 个代理地址（当前：%d 个）", len(items))
	}
	proxyURL := strings.TrimSpace(items[0])
	if proxyURL == "" {
		return "", fmt.Errorf("代理组 items[0] 不能为空")
	}
	normalized, err := proxyurl.Normalize(proxyURL)
	if err != nil {
		return "", fmt.Errorf("代理地址无效: %w", err)
	}
	return normalized, nil
}

// GetPrefillGroups 获取预填组列表，可通过 ?type=xxx 过滤
func GetPrefillGroups(c *gin.Context) {
	groupType := c.Query("type")
	groups, err := model.GetAllPrefillGroups(groupType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, groups)
}

// CreatePrefillGroup 创建新的预填组
func CreatePrefillGroup(c *gin.Context) {
	var g model.PrefillGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		common.ApiError(c, err)
		return
	}
	g.Name = strings.TrimSpace(g.Name)
	g.Type = strings.TrimSpace(g.Type)
	if g.Name == "" || g.Type == "" {
		common.ApiErrorMsg(c, "组名称和类型不能为空")
		return
	}
	if g.Type == "model" {
		var items []string
		if err := json.Unmarshal(g.Items, &items); err != nil || len(items) == 0 {
			common.ApiErrorMsg(c, "模型组 items 必须为非空 JSON 数组")
			return
		}
		for _, raw := range items {
			if strings.TrimSpace(raw) == "" {
				common.ApiErrorMsg(c, "模型组 items 不能包含空模型名")
				return
			}
		}
	}
	if g.Type == "proxy" {
		normalized, err := validateProxyPrefillGroupItems(g.Items)
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		buf, err := json.Marshal([]string{normalized})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		g.Items = model.JSONValue(buf)
	}
	// 创建前检查名称
	if dup, err := model.IsPrefillGroupNameDuplicated(0, g.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "组名称已存在")
		return
	}

	if err := g.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(g.Type) == "model" {
		if err := model.RefreshGroupSettings(); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, &g)
}

// UpdatePrefillGroup 更新预填组
func UpdatePrefillGroup(c *gin.Context) {
	var g model.PrefillGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		common.ApiError(c, err)
		return
	}
	g.Name = strings.TrimSpace(g.Name)
	g.Type = strings.TrimSpace(g.Type)
	if g.Id == 0 {
		common.ApiErrorMsg(c, "缺少组 ID")
		return
	}
	if g.Name == "" || g.Type == "" {
		common.ApiErrorMsg(c, "组名称和类型不能为空")
		return
	}

	var existing model.PrefillGroup
	if err := model.DB.First(&existing, g.Id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(existing.Type) == "model" && strings.TrimSpace(g.Type) != "model" {
		if err := model.ValidateModelPrefillGroupNotReferenced(nil, g.Id); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
	}
	if g.Type == "model" {
		var items []string
		if err := json.Unmarshal(g.Items, &items); err != nil || len(items) == 0 {
			common.ApiErrorMsg(c, "模型组 items 必须为非空 JSON 数组")
			return
		}
	}
	if g.Type == "proxy" {
		normalized, err := validateProxyPrefillGroupItems(g.Items)
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		buf, err := json.Marshal([]string{normalized})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		g.Items = model.JSONValue(buf)
	}
	// 名称冲突检查
	if dup, err := model.IsPrefillGroupNameDuplicated(g.Id, g.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "组名称已存在")
		return
	}

	if err := g.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(g.Type) == "model" || strings.TrimSpace(existing.Type) == "model" {
		if err := model.RefreshGroupSettings(); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, &g)
}

// DeletePrefillGroup 删除预填组
func DeletePrefillGroup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var g model.PrefillGroup
	if err := model.DB.First(&g, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(g.Type) == "model" {
		if err := model.ValidateModelPrefillGroupNotReferenced(nil, id); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
	}
	if err := model.DeletePrefillGroupByID(id); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(g.Type) == "model" {
		if err := model.RefreshGroupSettings(); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, nil)
}

package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListRedemptionPresets(c *gin.Context) {
	presets, err := model.ListRedemptionPresets()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    presets,
	})
}

func ListRedemptionPresetRevisions(c *gin.Context) {
	presetID, err := strconv.Atoi(c.Param("id"))
	if err != nil || presetID <= 0 {
		common.ApiErrorMsg(c, "id 无效")
		return
	}
	revisions, err := model.ListRedemptionPresetRevisions(presetID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    revisions,
	})
}

func UpsertRedemptionPreset(c *gin.Context) {
	var preset model.RedemptionPreset
	var options model.RedemptionPresetUpsertOptions
	body, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	requestFields := map[string]json.RawMessage{}
	_ = json.Unmarshal(body, &requestFields)
	if err := json.Unmarshal(body, &preset); err != nil {
		common.ApiError(c, err)
		return
	}
	if raw, ok := requestFields["sync_sold_assets"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &options.SyncSoldAssets); err != nil {
			common.ApiErrorMsg(c, "sync_sold_assets 无效")
			return
		}
	}

	// Patch semantics: when `stock` is not explicitly provided, preserve existing stock to avoid
	// silently resetting to "unlimited" for older consoles.
	if _, ok := requestFields["stock"]; !ok {
		if preset.Id > 0 {
			var existing model.RedemptionPreset
			if err := model.DB.Select("id", "stock").Where("id = ?", preset.Id).First(&existing).Error; err != nil {
				common.ApiError(c, err)
				return
			}
			preset.Stock = existing.Stock
		} else {
			name := strings.TrimSpace(preset.Name)
			if name != "" {
				var existing model.RedemptionPreset
				if err := model.DB.Select("id", "stock").Where("name = ?", name).First(&existing).Error; err == nil {
					preset.Stock = existing.Stock
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					common.ApiError(c, err)
					return
				}
			}
		}
	}

	created, err := model.UpsertRedemptionPreset(nil, &preset, options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RefreshGroupSettings(); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    created,
	})
}

type restoreRedemptionPresetRevisionRequest struct {
	RevisionId     int  `json:"revision_id"`
	SyncSoldAssets bool `json:"sync_sold_assets"`
}

func RestoreRedemptionPresetRevision(c *gin.Context) {
	presetID, err := strconv.Atoi(c.Param("id"))
	if err != nil || presetID <= 0 {
		common.ApiErrorMsg(c, "id 无效")
		return
	}
	var req restoreRedemptionPresetRevisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.RevisionId <= 0 {
		common.ApiErrorMsg(c, "revision_id 无效")
		return
	}
	restored, err := model.RestoreRedemptionPresetFromRevision(nil, presetID, req.RevisionId, model.RedemptionPresetUpsertOptions{
		SyncSoldAssets: req.SyncSoldAssets,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RefreshGroupSettings(); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    restored,
	})
}

func DeleteRedemptionPreset(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteRedemptionPresetById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RefreshGroupSettings(); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type generateRedemptionByPresetRequest struct {
	PresetId int    `json:"preset_id"`
	Name     string `json:"name"`
	Count    int    `json:"count"`
}

func GenerateRedemptionByPreset(c *gin.Context) {
	var req generateRedemptionByPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 100 {
		req.Count = 100
	}

	var (
		preset *model.RedemptionPreset
		err    error
	)
	if req.PresetId > 0 {
		preset, err = model.GetRedemptionPresetById(req.PresetId)
	} else {
		if req.Name == "" {
			common.ApiErrorMsg(c, "name/preset_id 不能为空")
			return
		}
		preset, err = model.GetRedemptionPresetByName(req.Name)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	mode := strings.TrimSpace(preset.Mode)
	if mode == "" {
		common.ApiErrorMsg(c, "preset mode 无效")
		return
	}
	if mode == "free" || mode == "xiaotuan" {
		common.ApiErrorMsg(c, "该商品类型已下线，禁止继续生成兑换码")
		return
	}
	preset.Mode = mode

	adminId := c.GetInt("id")
	now := common.GetTimestamp()

	keys := make([]string, 0, req.Count)
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		groupLimitByID := make(map[int]int, 0)
		groupIDs := make([]int, 0)
		if (preset.Mode == "subscription" || preset.Mode == "tokens") && len(preset.GroupDailyLimits) > 0 {
			for _, item := range preset.GroupDailyLimits {
				gid := item.GroupId
				if gid <= 0 {
					return fmt.Errorf("分组 id 无效")
				}
				groupLimitByID[gid] = item.DailyQuotaLimit
				groupIDs = append(groupIDs, gid)
			}
			groupIDs = model.NormalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) > 0 {
				if err := model.ValidateGroupIDsExist(tx, groupIDs); err != nil {
					return err
				}
			}
		}

		redemptionIDsForLimits := make([]int, 0, req.Count)
		for i := 0; i < req.Count; i++ {
			key := common.GetUUID()
			clean := model.Redemption{
				UserId:            adminId,
				Name:              preset.Name,
				Key:               key,
				Status:            common.RedemptionCodeStatusEnabled,
				Mode:              preset.Mode,
				PriceFen:          preset.PriceFen,
				Quota:             preset.Quota,
				DailyQuotaLimit:   preset.DailyQuotaLimit,
				DailyRequestLimit: preset.DailyRequestLimit,
				CreatedTime:       now,
				ExpiredTime:       preset.ExpiredTime,
				QuotaValidDays:    preset.QuotaValidDays,
				PlanValidDays:     preset.PlanValidDays,
				ChannelIds:        preset.ChannelIds,
				AllowedGroups:     nil,
				AllowedGroupIds:   preset.AllowedGroupIds,
			}
			if err := tx.Create(&clean).Error; err != nil {
				return err
			}
			keys = append(keys, key)
			if len(groupIDs) > 0 && clean.Id > 0 {
				redemptionIDsForLimits = append(redemptionIDsForLimits, clean.Id)
			}
		}

		if len(redemptionIDsForLimits) > 0 && len(groupIDs) > 0 {
			sort.Ints(redemptionIDsForLimits)
			rows := make([]model.RedemptionGroupDailyLimit, 0, len(redemptionIDsForLimits)*len(groupIDs))
			for _, rid := range redemptionIDsForLimits {
				for _, gid := range groupIDs {
					rows = append(rows, model.RedemptionGroupDailyLimit{
						RedemptionId:    rid,
						GroupId:         gid,
						DailyLimitQuota: groupLimitByID[gid],
					})
				}
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
}

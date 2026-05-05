package controller

import (
	"net/http"

	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

type bulkUpdateUserSubscriptionDurationRequest struct {
	UserIDs           []int    `json:"user_ids"`
	SubscriptionTypes []string `json:"subscription_types"`
	MinRemainingDays  *int     `json:"min_remaining_days"`
	MaxRemainingDays  *int     `json:"max_remaining_days"`

	// 二选一：统一增加 N 天，或直接设置 Unix 时间戳
	AddDays     *int   `json:"add_days"`
	SetExpireAt *int64 `json:"set_expire_at"`

	DryRun bool `json:"dry_run"`
}

type bulkCompensateSubscriptionsByPresetRequest struct {
	FaultStartAt         *int64   `json:"fault_start_at"`
	FaultEndAt           *int64   `json:"fault_end_at"`
	SourcePresetIDs      []int    `json:"source_preset_ids"`
	ExcludedUserIDs      []int    `json:"excluded_user_ids"`
	ExcludedUsernames    []string `json:"excluded_usernames"`
	CompensationPresetID int      `json:"compensation_preset_id"`
	ApplyMode            string   `json:"apply_mode"`
	Quantity             *int     `json:"quantity"`
	DryRun               bool     `json:"dry_run"`
}

type bulkExtendOriginalSubscriptionsRequest struct {
	FaultStartAt      *int64   `json:"fault_start_at"`
	FaultEndAt        *int64   `json:"fault_end_at"`
	SourcePresetIDs   []int    `json:"source_preset_ids"`
	ExcludedUserIDs   []int    `json:"excluded_user_ids"`
	ExcludedUsernames []string `json:"excluded_usernames"`
	ExtendDays        *int     `json:"extend_days"`
	DryRun            bool     `json:"dry_run"`
}

func BulkUpdateUserSubscriptionDuration(c *gin.Context) {
	var req bulkUpdateUserSubscriptionDurationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if len(req.SubscriptionTypes) == 0 {
		common.ApiErrorMsg(c, "subscription_types is required")
		return
	}
	if req.MinRemainingDays == nil {
		common.ApiErrorMsg(c, "min_remaining_days is required")
		return
	}

	if (req.AddDays == nil) == (req.SetExpireAt == nil) {
		common.ApiErrorMsg(c, "add_days 与 set_expire_at 必须且只能提供一个")
		return
	}

	types := make([]model.SubscriptionType, 0, len(req.SubscriptionTypes))
	for _, raw := range req.SubscriptionTypes {
		switch raw {
		case string(model.SubscriptionTypeQuota):
			types = append(types, model.SubscriptionTypeQuota)
		case string(model.SubscriptionTypeRequest):
			types = append(types, model.SubscriptionTypeRequest)
		default:
			common.ApiErrorMsg(c, "subscription_types 存在无效值")
			return
		}
	}

	res, err := model.BulkUpdateSubscriptionDuration(model.BulkUpdateSubscriptionDurationParams{
		UserIDs:           req.UserIDs,
		SubscriptionTypes: types,
		MinRemainingDays:  *req.MinRemainingDays,
		MaxRemainingDays:  req.MaxRemainingDays,
		AddDays:           req.AddDays,
		SetExpireAt:       req.SetExpireAt,
		DryRun:            req.DryRun,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, res)
}

func BulkCompensateSubscriptionsByPreset(c *gin.Context) {
	var req bulkCompensateSubscriptionsByPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if req.FaultStartAt == nil {
		common.ApiErrorMsg(c, "fault_start_at is required")
		return
	}
	if req.FaultEndAt == nil {
		common.ApiErrorMsg(c, "fault_end_at is required")
		return
	}
	if len(req.SourcePresetIDs) == 0 {
		common.ApiErrorMsg(c, "source_preset_ids is required")
		return
	}
	if req.CompensationPresetID <= 0 {
		common.ApiErrorMsg(c, "compensation_preset_id 无效")
		return
	}

	quantity := 1
	if req.Quantity != nil {
		quantity = *req.Quantity
	}

	res, err := model.BulkCompensateSubscriptionsByPreset(model.BulkCompensateSubscriptionsByPresetParams{
		FaultStartAt:         *req.FaultStartAt,
		FaultEndAt:           *req.FaultEndAt,
		SourcePresetIDs:      req.SourcePresetIDs,
		ExcludedUserIDs:      req.ExcludedUserIDs,
		ExcludedUsernames:    req.ExcludedUsernames,
		CompensationPresetID: req.CompensationPresetID,
		ApplyMode:            req.ApplyMode,
		Quantity:             quantity,
		DryRun:               req.DryRun,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, res)
}

func BulkExtendOriginalSubscriptions(c *gin.Context) {
	var req bulkExtendOriginalSubscriptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if req.FaultStartAt == nil {
		common.ApiErrorMsg(c, "fault_start_at is required")
		return
	}
	if req.FaultEndAt == nil {
		common.ApiErrorMsg(c, "fault_end_at is required")
		return
	}
	if len(req.SourcePresetIDs) == 0 {
		common.ApiErrorMsg(c, "source_preset_ids is required")
		return
	}
	if req.ExtendDays == nil || *req.ExtendDays <= 0 {
		common.ApiErrorMsg(c, "extend_days 无效")
		return
	}

	res, err := model.BulkExtendOriginalSubscriptions(model.BulkExtendOriginalSubscriptionsParams{
		FaultStartAt:      *req.FaultStartAt,
		FaultEndAt:        *req.FaultEndAt,
		SourcePresetIDs:   req.SourcePresetIDs,
		ExcludedUserIDs:   req.ExcludedUserIDs,
		ExcludedUsernames: req.ExcludedUsernames,
		ExtendDays:        *req.ExtendDays,
		DryRun:            req.DryRun,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, res)
}

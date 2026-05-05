package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createRequestSubscriptionRequest struct {
	DailyRequestLimit float64 `json:"daily_request_limit"`
	TotalRequestLimit float64 `json:"total_request_limit"`
	// 可选：订阅开始时间（unix seconds）。若不提供，默认为当前时间。
	StartAt         *int64 `json:"start_at"`
	ExpireAt        *int64 `json:"expire_at"`
	AllowedGroupIds []int  `json:"allowed_group_ids"`
	Source          string `json:"source"`
}

type updateRequestSubscriptionRequest struct {
	DailyRequestLimit *float64 `json:"daily_request_limit"`
	TotalRequestLimit *float64 `json:"total_request_limit"`
	StartAt           *int64   `json:"start_at"`
	ExpireAt          *int64   `json:"expire_at"`
	AllowedGroupIds   *[]int   `json:"allowed_group_ids"`
}

type createRequestSubscriptionByPresetRequest struct {
	PresetId  int    `json:"preset_id" binding:"required"`
	ApplyMode string `json:"apply_mode"`
	Quantity  *int   `json:"quantity"`
}

type reorderUserRequestSubscriptionsRequest struct {
	SubscriptionIds []int `json:"subscription_ids" binding:"required"`
}

func ListUserRequestSubscriptions(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	breakdown, err := model.GetUserRequestSubscriptionBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    breakdown,
	})
}

func CreateUserRequestSubscription(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	var req createRequestSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if req.DailyRequestLimit < 0 {
		common.ApiErrorMsg(c, "每日次数必须大于等于 0")
		return
	}
	if req.TotalRequestLimit < 0 {
		common.ApiErrorMsg(c, "总次数必须大于等于 0")
		return
	}

	expireAt := int64(0)
	if req.ExpireAt != nil {
		expireAt = *req.ExpireAt
	}
	startAt := int64(0)
	if req.StartAt != nil {
		startAt = *req.StartAt
	}

	allowedIDs := model.NormalizeUniqueSortedIDs(req.AllowedGroupIds)
	if len(allowedIDs) == 0 {
		common.ApiErrorMsg(c, "可用分组不能为空")
		return
	}
	if err := model.ValidateGroupIDsExist(nil, allowedIDs); err != nil {
		common.ApiError(c, err)
		return
	}

	sub, err := model.CreateUserRequestSubscriptionTx(nil, userId, startAt, req.DailyRequestLimit, req.TotalRequestLimit, expireAt, allowedIDs, req.Source, model.UserRequestSubscriptionSourceRef{})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	breakdown, err := model.GetUserRequestSubscriptionBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserRequestSubscription(sub),
			"breakdown":    breakdown,
		},
	})
}

func CreateUserRequestSubscriptionByPreset(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	var req createRequestSubscriptionByPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if req.PresetId <= 0 {
		common.ApiErrorMsg(c, "preset_id 无效")
		return
	}
	applyMode := req.ApplyMode
	if applyMode == "" {
		applyMode = model.SubscriptionApplyModeStack
	}
	if applyMode != model.SubscriptionApplyModeStack && applyMode != model.SubscriptionApplyModeDefer {
		common.ApiErrorMsg(c, "apply_mode 无效")
		return
	}
	quantity := 1
	if req.Quantity != nil {
		quantity = *req.Quantity
	}
	if quantity <= 0 {
		common.ApiErrorMsg(c, "quantity 无效")
		return
	}
	if quantity > 100 {
		common.ApiErrorMsg(c, "quantity 过大")
		return
	}

	var createdSub *model.UserRequestSubscription
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		if now <= 0 || now > common.MaxSupportedUnixTimestamp {
			return errors.New("系统时间异常")
		}

		var preset model.RedemptionPreset
		if err := tx.Where("id = ?", req.PresetId).First(&preset).Error; err != nil {
			return err
		}
		revision, err := model.EnsureCurrentRedemptionPresetRevisionTx(tx, preset.Id)
		if err != nil {
			return err
		}
		if revision.Mode != "request" {
			return errors.New("商品类型错误")
		}
		if quantity > 1 && revision.MultiQuantityDeferOnly && applyMode != model.SubscriptionApplyModeDefer {
			return errors.New("生效方式错误")
		}
		if quantity > 1 && !revision.MultiQuantityEnabled {
			return errors.New("该商品不支持多数量")
		}
		if revision.DailyRequestLimit < 0 {
			return errors.New("商品每日次数无效")
		}
		if revision.Quota < 0 {
			return errors.New("商品总次数无效")
		}
		var groupIDs []int
		if len(revision.AllowedGroupIds) > 0 {
			if err := common.Unmarshal([]byte(revision.AllowedGroupIds), &groupIDs); err != nil {
				return err
			}
		}
		groupIDs = model.NormalizeUniqueSortedIDs(groupIDs)
		if len(groupIDs) == 0 {
			return errors.New("商品可用分组为空")
		}
		if err := model.ValidateGroupIDsExist(tx, groupIDs); err != nil {
			return err
		}

		startAt := now
		if applyMode == model.SubscriptionApplyModeDefer {
			maxExpire, err := model.GetUserRequestSubscriptionMaxExpireAt(tx, userId, now)
			if err != nil {
				return err
			}
			if maxExpire >= startAt {
				startAt = maxExpire + 1
			}
		}
		if startAt > common.MaxSupportedUnixTimestamp {
			return errors.New("订阅开始时间过大")
		}
		if revision.QuotaValidDays < 0 {
			return errors.New("商品有效期无效")
		}

		perUnitExtendSeconds := int64(0)
		if revision.QuotaValidDays > 0 {
			days := int64(revision.QuotaValidDays)
			if days > common.MaxSupportedUnixTimestamp/common.SecondsPerDay {
				return errors.New("订阅有效期过大")
			}
			perUnitExtendSeconds = days * common.SecondsPerDay
		}

		nextDeferStartAt := startAt
		for i := 0; i < quantity; i++ {
			unitStartAt := startAt
			if applyMode == model.SubscriptionApplyModeDefer {
				unitStartAt = nextDeferStartAt
			}

			unitExpireAt := int64(0)
			if perUnitExtendSeconds > 0 {
				if unitStartAt > common.MaxSupportedUnixTimestamp {
					return errors.New("订阅开始时间过大")
				}
				if perUnitExtendSeconds > common.MaxSupportedUnixTimestamp-unitStartAt {
					return errors.New("订阅有效期过大")
				}
				unitExpireAt = unitStartAt + perUnitExtendSeconds
			}

			sub, err := model.CreateUserRequestSubscriptionTx(
				tx,
				userId,
				unitStartAt,
				float64(revision.DailyRequestLimit),
				float64(revision.Quota),
				unitExpireAt,
				groupIDs,
				fmt.Sprintf("admin_preset:%d", preset.Id),
				model.UserRequestSubscriptionSourceRef{PresetId: preset.Id, PresetRevisionId: revision.Id},
			)
			if err != nil {
				return err
			}
			createdSub = sub

			if applyMode == model.SubscriptionApplyModeDefer && perUnitExtendSeconds > 0 {
				nextDeferStartAt = unitExpireAt + 1
			}
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)

	breakdown, err := model.GetUserRequestSubscriptionBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserRequestSubscription(createdSub),
			"breakdown":    breakdown,
		},
	})
}

func ReorderUserRequestSubscriptions(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}

	var req reorderUserRequestSubscriptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	if err := model.ReorderUserRequestSubscriptions(userId, req.SubscriptionIds); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)

	breakdown, err := model.GetUserRequestSubscriptionBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    breakdown,
	})
}

func UpdateUserRequestSubscription(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	subId, err := strconv.Atoi(c.Param("subId"))
	if err != nil || subId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid subscription id"})
		return
	}
	var req updateRequestSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	params := model.UpdateUserRequestSubscriptionParams{
		DailyRequestLimit: req.DailyRequestLimit,
		TotalRequestLimit: req.TotalRequestLimit,
		StartAt:           req.StartAt,
		ExpireAt:          req.ExpireAt,
	}
	if req.AllowedGroupIds != nil {
		allowedIDs := model.NormalizeUniqueSortedIDs(*req.AllowedGroupIds)
		if len(allowedIDs) == 0 {
			common.ApiErrorMsg(c, "可用分组不能为空")
			return
		}
		params.AllowedGroupIds = &allowedIDs
	}

	sub, err := model.UpdateUserRequestSubscription(userId, subId, params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	breakdown, err := model.GetUserRequestSubscriptionBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserRequestSubscription(sub),
			"breakdown":    breakdown,
		},
	})
}

func ActivateSelfRequestSubscription(c *gin.Context) {
	userId := c.GetInt("id")
	if userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	subId, err := strconv.Atoi(c.Param("subId"))
	if err != nil || subId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid subscription id"})
		return
	}
	sub, err := model.ActivatePendingUserRequestSubscription(userId, subId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserRequestSubscription(sub),
		},
	})
}

func DeleteUserRequestSubscription(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	subId, err := strconv.Atoi(c.Param("subId"))
	if err != nil || subId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid subscription id"})
		return
	}
	if err := model.DeleteUserRequestSubscription(userId, subId); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	breakdown, err := model.GetUserRequestSubscriptionBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    breakdown,
	})
}

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

type createSubscriptionRequest struct {
	Quota int `json:"quota" binding:"required"`
	// 可选：初始剩余额度（tokens）。若不提供，默认为与 quota 相同
	Remaining  *int `json:"remaining_quota"`
	DailyQuota *int `json:"daily_quota_limit"`
	// 可选：订阅开始时间（unix seconds）。若不提供，默认为当前时间。
	StartAt          *int64                       `json:"start_at"`
	ExpireAt         *int64                       `json:"expire_at"`
	AllowedGroupIds  []int                        `json:"allowed_group_ids"`
	GroupDailyLimits []model.GroupDailyQuotaLimit `json:"group_daily_limits"`
	Source           string                       `json:"source"`
}

type updateSubscriptionRequest struct {
	Quota            *int                         `json:"quota"`
	Remaining        *int                         `json:"remaining_quota"`
	DailyQuota       *int                         `json:"daily_quota_limit"`
	StartAt          *int64                       `json:"start_at"`
	ExpireAt         *int64                       `json:"expire_at"`
	AllowedGroupIds  *[]int                       `json:"allowed_group_ids"`
	GroupDailyLimits []model.GroupDailyQuotaLimit `json:"group_daily_limits"`
}

type createSubscriptionByPresetRequest struct {
	PresetId  int    `json:"preset_id" binding:"required"`
	ApplyMode string `json:"apply_mode"`
	Quantity  *int   `json:"quantity"`
}

type reorderUserSubscriptionsRequest struct {
	SubscriptionIds []int `json:"subscription_ids" binding:"required"`
}

func ListUserSubscriptions(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	breakdown, err := model.GetUserQuotaBreakdown(userId)
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

func CreateUserSubscription(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	var req createSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	dailyLimit := 0
	if req.DailyQuota != nil {
		dailyLimit = *req.DailyQuota
	}
	expireAt := int64(0)
	if req.ExpireAt != nil {
		expireAt = *req.ExpireAt
	}
	startAt := int64(0)
	if req.StartAt != nil {
		startAt = *req.StartAt
	}
	// 规范初始剩余额度：默认等于总额度，且不会超过总额度且不为负
	initRemain := req.Quota
	if req.Remaining != nil {
		initRemain = *req.Remaining
		if initRemain < 0 {
			initRemain = 0
		}
		if initRemain > req.Quota {
			initRemain = req.Quota
		}
	}

	allowedIDs := model.NormalizeUniqueSortedIDs(req.AllowedGroupIds)
	if len(allowedIDs) == 0 {
		common.ApiErrorMsg(c, "请选择可用分组")
		return
	}
	sub, err := model.CreateUserSubscription(userId, startAt, req.Quota, initRemain, dailyLimit, expireAt, allowedIDs, req.GroupDailyLimits, req.Source)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	breakdown, err := model.GetUserQuotaBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserSubscription(sub),
			"breakdown":    breakdown,
		},
	})
}

func CreateUserSubscriptionByPreset(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	var req createSubscriptionByPresetRequest
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

	var createdSub *model.UserSubscription
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
		if revision.Mode != "subscription" && revision.Mode != "tokens" {
			return errors.New("商品类型错误")
		}
		if quantity > 1 && revision.MultiQuantityDeferOnly && applyMode != model.SubscriptionApplyModeDefer {
			return errors.New("生效方式错误")
		}
		if quantity > 1 && !revision.MultiQuantityEnabled {
			return errors.New("该商品不支持多数量")
		}
		if revision.Quota < 0 {
			return errors.New("商品额度无效")
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

		billingUnit := model.UserSubscriptionBillingUnitQuota
		if revision.Mode == "tokens" {
			billingUnit = model.UserSubscriptionBillingUnitTokens
		}

		startAt := now
		if applyMode == model.SubscriptionApplyModeDefer {
			maxExpire, err := model.GetUserSubscriptionMaxExpireAtWithBillingUnit(tx, userId, now, billingUnit)
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

			sub, err := model.CreateUserSubscriptionTxWithBillingUnit(
				tx,
				userId,
				unitStartAt,
				revision.Quota,
				revision.Quota,
				revision.DailyQuotaLimit,
				unitExpireAt,
				groupIDs,
				billingUnit,
				fmt.Sprintf("admin_preset:%d", preset.Id),
				model.UserSubscriptionSourceRef{PresetId: preset.Id, PresetRevisionId: revision.Id},
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

	breakdown, err := model.GetUserQuotaBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserSubscription(createdSub),
			"breakdown":    breakdown,
		},
	})
}

func ReorderUserSubscriptions(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}

	var req reorderUserSubscriptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	if err := model.ReorderUserSubscriptions(userId, req.SubscriptionIds); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)

	breakdown, err := model.GetUserQuotaBreakdown(userId)
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

func UpdateUserSubscription(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	subId, err := strconv.Atoi(c.Param("subId"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid subscription id"})
		return
	}
	var req updateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	params := model.UpdateUserSubscriptionParams{
		TotalQuota:      req.Quota,
		RemainingQuota:  req.Remaining,
		DailyQuotaLimit: req.DailyQuota,
		StartAt:         req.StartAt,
		ExpireAt:        req.ExpireAt,
	}
	if req.AllowedGroupIds != nil {
		allowedIDs := model.NormalizeUniqueSortedIDs(*req.AllowedGroupIds)
		if len(allowedIDs) == 0 {
			common.ApiErrorMsg(c, "请选择可用分组")
			return
		}
		params.AllowedGroupIds = &allowedIDs
	}
	if req.GroupDailyLimits != nil {
		params.GroupDailyLimits = &req.GroupDailyLimits
	}
	sub, err := model.UpdateUserSubscription(userId, subId, params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	breakdown, err := model.GetUserQuotaBreakdown(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserSubscription(sub),
			"breakdown":    breakdown,
		},
	})
}

func ActivateSelfSubscription(c *gin.Context) {
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
	sub, err := model.ActivatePendingUserSubscription(userId, subId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription": model.PresentUserSubscription(sub),
		},
	})
}

func DeleteUserSubscription(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	subId, err := strconv.Atoi(c.Param("subId"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid subscription id"})
		return
	}
	if err := model.DeleteUserSubscription(userId, subId); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateUserCache(userId)
	breakdown, err := model.GetUserQuotaBreakdown(userId)
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

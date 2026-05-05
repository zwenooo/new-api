package controller

import (
	"errors"
	"net/http"
	"one-api/common"
	"one-api/model"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type generatePaygRedemptionsRequest struct {
	// BillingUnit controls which prepaid-balance unit this redemption will credit:
	// - quota/usd: prepaid credit (historical mode: payg)
	// - requests: prepaid request (historical mode: pay_request)
	// - tokens: prepaid token (historical mode: pay_token)
	BillingUnit string `json:"billing_unit"`

	// One of product_id or allowed_group_ids must be provided.
	// When product_id is provided, allowed_group_ids is ignored and will be loaded from product bindings.
	ProductId       int   `json:"product_id"`
	AllowedGroupIds []int `json:"allowed_group_ids"`

	// Quota represents:
	// - quota/usd: internal quota units (QuotaPerUnit * USD), unless quota_usd is provided
	// - requests: request count
	// - tokens: token count
	Quota int `json:"quota"`
	// QuotaUsd is optional for quota/usd unit. When set, it overrides Quota.
	QuotaUsd float64 `json:"quota_usd"`

	Count       int    `json:"count"`
	Name        string `json:"name"`
	ExpiredTime int64  `json:"expired_time"`
}

func GeneratePaygRedemptions(c *gin.Context) {
	var req generatePaygRedemptionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	unit := strings.TrimSpace(req.BillingUnit)
	if unit == "" {
		common.ApiErrorMsg(c, "billing_unit 不能为空")
		return
	}

	mode := ""
	switch unit {
	case "quota", "usd":
		mode = "payg"
	case "tokens":
		mode = "pay_token"
	case "requests", "request", "count":
		mode = "pay_request"
	default:
		common.ApiErrorMsg(c, "billing_unit 无效")
		return
	}

	quota := req.Quota
	if mode == "payg" && req.QuotaUsd > 0 {
		quota = int((common.QuotaPerUnit * req.QuotaUsd) + 0.5)
	}
	if quota <= 0 {
		common.ApiErrorMsg(c, "quota 必须大于 0")
		return
	}

	if err := validateExpiredTime(req.ExpiredTime); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		common.ApiErrorMsg(c, "count 不能大于 100")
		return
	}

	name := strings.TrimSpace(req.Name)

	groupIDs := make([]int, 0)
	if req.ProductId > 0 {
		switch mode {
		case "payg":
			var product model.PaygProduct
			if err := model.DB.Where("id = ?", req.ProductId).First(&product).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					common.ApiErrorMsg(c, "按量付费商品不存在")
					return
				}
				common.ApiError(c, err)
				return
			}
			ids, err := model.GetPaygProductAllowedGroupIDsTx(nil, product.Id)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			groupIDs = ids
			if name == "" {
				name = strings.TrimSpace(product.Name)
			}
		case "pay_request":
			var product model.PayRequestProduct
			if err := model.DB.Where("id = ?", req.ProductId).First(&product).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					common.ApiErrorMsg(c, "按次付费商品不存在")
					return
				}
				common.ApiError(c, err)
				return
			}
			ids, err := model.GetPayRequestProductAllowedGroupIDsTx(nil, product.Id)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			groupIDs = ids
			if name == "" {
				name = strings.TrimSpace(product.Name)
			}
		case "pay_token":
			var product model.PayTokenProduct
			if err := model.DB.Where("id = ?", req.ProductId).First(&product).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					common.ApiErrorMsg(c, "按token付费商品不存在")
					return
				}
				common.ApiError(c, err)
				return
			}
			ids, err := model.GetPayTokenProductAllowedGroupIDsTx(nil, product.Id)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			groupIDs = ids
			if name == "" {
				name = strings.TrimSpace(product.Name)
			}
		default:
			common.ApiErrorMsg(c, "mode 无效")
			return
		}
	} else {
		groupIDs = model.NormalizeUniqueSortedIDs(req.AllowedGroupIds)
	}

	groupIDs = model.NormalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		common.ApiErrorMsg(c, "请选择可用分组")
		return
	}
	if err := model.ValidateGroupIDsExist(nil, groupIDs); err != nil {
		common.ApiError(c, err)
		return
	}

	if name == "" {
		switch mode {
		case "payg":
			name = "按量付费"
		case "pay_request":
			name = "按次付费"
		case "pay_token":
			name = "按token付费"
		}
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 20 {
		runes := []rune(name)
		name = string(runes[:20])
	}
	if utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 20 {
		common.ApiErrorMsg(c, "name 无效")
		return
	}

	allowedGroupIDsJSON, err := common.Marshal(groupIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	adminId := c.GetInt("id")
	now := common.GetTimestamp()

	keys := make([]string, 0, count)
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < count; i++ {
			key := common.GetUUID()
			clean := model.Redemption{
				UserId:            adminId,
				Name:              name,
				Key:               key,
				Status:            common.RedemptionCodeStatusEnabled,
				Mode:              mode,
				PriceFen:          0,
				Quota:             quota,
				DailyQuotaLimit:   0,
				DailyRequestLimit: 0,
				CreatedTime:       now,
				ExpiredTime:       req.ExpiredTime,
				QuotaValidDays:    0,
				PlanValidDays:     0,
				ChannelIds:        nil,
				AllowedGroups:     nil,
				AllowedGroupIds:   model.JSONValue(allowedGroupIDsJSON),
			}
			if err := tx.Create(&clean).Error; err != nil {
				return err
			}
			keys = append(keys, key)
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

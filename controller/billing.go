package controller

import (
	"github.com/gin-gonic/gin"
	"one-api/common"
	"one-api/dto"
	"one-api/model"
)

type publicUsageQuotaRow struct {
	Quota int `gorm:"column:quota"`
}

func getPublicUsageQuotaByUser(userId int) (int, error) {
	user, err := model.GetUserById(userId, true)
	if err != nil {
		return 0, err
	}
	if user.UsedQuota > 0 {
		return user.UsedQuota, nil
	}
	var row publicUsageQuotaRow
	if err := model.LOG_DB.Table("logs").
		Select("COALESCE(sum(quota), 0) AS quota").
		Where("user_id = ? AND type = ?", userId, model.LogTypeConsume).
		Scan(&row).Error; err != nil {
		return 0, err
	}
	return row.Quota, nil
}

func getPublicUsageQuotaByToken(tokenId int) (int, error) {
	var row publicUsageQuotaRow
	if err := model.LOG_DB.Table("logs").
		Select("COALESCE(sum(quota), 0) AS quota").
		Where("token_id = ? AND type = ?", tokenId, model.LogTypeConsume).
		Scan(&row).Error; err != nil {
		return 0, err
	}
	return row.Quota, nil
}

func renderPublicQuotaAmount(quota int) float64 {
	amount := float64(quota)
	if common.DisplayInCurrencyEnabled {
		amount /= common.QuotaPerUnit
	}
	return amount
}

func GetSubscription(c *gin.Context) {
	var err error
	var expiredTime int64
	if common.DisplayTokenStatEnabled {
		tokenId := c.GetInt("token_id")
		token, tokenErr := model.GetTokenById(tokenId)
		if tokenErr != nil {
			err = tokenErr
		} else {
			expiredTime = token.ExpiredTime
		}
	} else {
		userId := c.GetInt("id")
		// 当展示用户维度时，返回兑换额度到期时间（若存在）
		if cache, e := model.GetUserCache(userId); e == nil {
			if cache.RedeemQuotaExpireAt > 0 {
				expiredTime = cache.RedeemQuotaExpireAt
			}
		}
	}
	if expiredTime <= 0 {
		expiredTime = 0
	}
	if err != nil {
		openAIError := dto.OpenAIError{
			Message: err.Error(),
			Type:    "upstream_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	c.JSON(200, gin.H{
		"object":                "billing_subscription",
		"has_payment_method":    true,
		"soft_limit_usd":        nil,
		"hard_limit_usd":        nil,
		"system_hard_limit_usd": nil,
		"access_until":          expiredTime,
		"quota_details_hidden":  true,
	})
	return
}

func GetUsage(c *gin.Context) {
	var quota int
	var err error
	var token *model.Token
	if common.DisplayTokenStatEnabled {
		tokenId := c.GetInt("token_id")
		token, err = model.GetTokenById(tokenId)
		if err == nil {
			quota, err = getPublicUsageQuotaByToken(token.Id)
		}
	} else {
		userId := c.GetInt("id")
		quota, err = getPublicUsageQuotaByUser(userId)
	}
	if err != nil {
		openAIError := dto.OpenAIError{
			Message: err.Error(),
			Type:    "transfer_api_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}
	usage := OpenAIUsageResponse{
		Object:     "list",
		TotalUsage: renderPublicQuotaAmount(quota) * 100,
	}
	c.JSON(200, usage)
	return
}

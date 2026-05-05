package controller

import (
	"errors"
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type redemptionUpsertRequest struct {
	model.Redemption
}

func normalizeWriteRedemptionMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "", errors.New("mode 必须显式指定")
	}
	switch mode {
	case "activation", "subscription", "tokens", "request", "payg", "pay_request", "pay_token":
		return mode, nil
	case "free":
		return "", errors.New("自由额度兑换码已下线")
	case "xiaotuan":
		return "", errors.New("小团订阅兑换码已下线")
	default:
		return "", errors.New("无效的兑换码类型")
	}
}

func GetAllRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchRedemptions(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
	return
}

func AddRedemption(c *gin.Context) {
	var req redemptionUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	redemption := req.Redemption
	if len(redemption.AllowedGroups) > 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "allowed_groups 已废弃，请使用 allowed_group_ids"})
		return
	}

	allowedGroupIDs := make([]int, 0)
	if len(redemption.AllowedGroupIds) > 0 {
		if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &allowedGroupIDs); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "可用分组解析失败"})
			return
		}
		allowedGroupIDs = model.NormalizeUniqueSortedIDs(allowedGroupIDs)
	}

	mode, err := normalizeWriteRedemptionMode(redemption.Mode)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	if redemption.DailyQuotaLimit < 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "每日额度必须大于等于0"})
		return
	}
	if redemption.DailyRequestLimit < 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "每日次数必须大于等于0"})
		return
	}
	if redemption.QuotaValidDays < 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "额度有效期（天）不能小于0"})
		return
	}
	if redemption.PriceFen < 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "价格（分）不能小于0"})
		return
	}
	if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码名称长度必须在1-20之间"})
		return
	}
	if redemption.Count <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "兑换码个数必须大于0"})
		return
	}
	if redemption.Count > 100 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "一次兑换码批量生成的个数不能大于 100"})
		return
	}
	if err := validateExpiredTime(redemption.ExpiredTime); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	allowedGroupIDsJSON := model.JSONValue(nil)
	if mode == "subscription" || mode == "tokens" || mode == "payg" || mode == "request" || mode == "pay_request" || mode == "pay_token" {
		if len(allowedGroupIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "请选择可用分组"})
			return
		}
		if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		b, err := common.Marshal(allowedGroupIDs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "可用分组解析失败"})
			return
		}
		allowedGroupIDsJSON = model.JSONValue(b)
	} else if len(allowedGroupIDs) > 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该兑换码类型不支持可用分组"})
		return
	}

	switch mode {
	case "activation":
		if redemption.Quota != 0 || redemption.DailyQuotaLimit != 0 || redemption.DailyRequestLimit != 0 || redemption.QuotaValidDays != 0 || redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 || len(allowedGroupIDsJSON) != 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "激活码参数错误"})
			return
		}
	case "subscription":
		if redemption.Quota < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "额度必须大于等于0"})
			return
		}
		if redemption.DailyQuotaLimit < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "每日额度必须大于等于0"})
			return
		}
		if redemption.DailyRequestLimit != 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订阅额度兑换码参数错误"})
			return
		}
		if redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "订阅额度兑换码参数错误"})
			return
		}
	case "tokens":
		if redemption.Quota < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "额度必须大于等于0"})
			return
		}
		if redemption.DailyQuotaLimit < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "每日额度必须大于等于0"})
			return
		}
		if redemption.DailyRequestLimit != 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "tokens 兑换码参数错误"})
			return
		}
		if redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 || len(allowedGroupIDsJSON) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "tokens 兑换码参数错误"})
			return
		}
	case "request":
		if redemption.DailyRequestLimit < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "每日次数必须大于等于0"})
			return
		}
		if redemption.Quota < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "总次数必须大于等于0"})
			return
		}
		if redemption.DailyQuotaLimit != 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "次数订阅兑换码参数错误"})
			return
		}
		if redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 || len(allowedGroupIDsJSON) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "次数订阅兑换码参数错误"})
			return
		}
	case "payg":
		if redemption.Quota <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "额度必须大于0"})
			return
		}
		if redemption.DailyQuotaLimit != 0 || redemption.DailyRequestLimit != 0 || redemption.QuotaValidDays != 0 || redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 || len(allowedGroupIDsJSON) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "按量付费兑换码参数错误"})
			return
		}
	case "pay_request":
		if redemption.Quota <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "次数必须大于0"})
			return
		}
		if redemption.DailyQuotaLimit != 0 || redemption.DailyRequestLimit != 0 || redemption.QuotaValidDays != 0 || redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 || len(allowedGroupIDsJSON) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "按次付费兑换码参数错误"})
			return
		}
	case "pay_token":
		if redemption.Quota <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "tokens 必须大于0"})
			return
		}
		if redemption.DailyQuotaLimit != 0 || redemption.DailyRequestLimit != 0 || redemption.QuotaValidDays != 0 || redemption.PlanValidDays != 0 || len(redemption.ChannelIds) != 0 || len(allowedGroupIDsJSON) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "按token付费兑换码参数错误"})
			return
		}
	default:
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的兑换码类型"})
		return
	}

	keys := make([]string, 0, redemption.Count)
	for i := 0; i < redemption.Count; i++ {
		key := common.GetUUID()
		cleanRedemption := model.Redemption{
			UserId:            c.GetInt("id"),
			Name:              redemption.Name,
			Key:               key,
			Status:            common.RedemptionCodeStatusEnabled,
			CreatedTime:       common.GetTimestamp(),
			Mode:              mode,
			PriceFen:          redemption.PriceFen,
			Quota:             redemption.Quota,
			ExpiredTime:       redemption.ExpiredTime,
			DailyQuotaLimit:   redemption.DailyQuotaLimit,
			DailyRequestLimit: redemption.DailyRequestLimit,
			QuotaValidDays:    redemption.QuotaValidDays,
			PlanValidDays:     redemption.PlanValidDays,
			ChannelIds:        redemption.ChannelIds,
			AllowedGroups:     nil,
			AllowedGroupIds:   allowedGroupIDsJSON,
		}
		if err := cleanRedemption.Insert(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
}

func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	req := redemptionUpsertRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption := req.Redemption
	if len(redemption.AllowedGroups) > 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "allowed_groups 已废弃，请使用 allowed_group_ids"})
		return
	}

	allowedGroupIDs := make([]int, 0)
	if len(redemption.AllowedGroupIds) > 0 {
		if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &allowedGroupIDs); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "可用分组解析失败"})
			return
		}
		allowedGroupIDs = model.NormalizeUniqueSortedIDs(allowedGroupIDs)
	}
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		mode, err := normalizeWriteRedemptionMode(redemption.Mode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if err := validateExpiredTime(redemption.ExpiredTime); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}

		allowedGroupIDsJSON := model.JSONValue(nil)
		if mode == "subscription" || mode == "tokens" || mode == "payg" || mode == "request" || mode == "pay_request" || mode == "pay_token" {
			if len(allowedGroupIDs) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "请选择可用分组"})
				return
			}
			if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
			b, err := common.Marshal(allowedGroupIDs)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "可用分组解析失败"})
				return
			}
			allowedGroupIDsJSON = model.JSONValue(b)
		} else if len(allowedGroupIDs) > 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "该兑换码类型不支持可用分组"})
			return
		}

		// If you add more fields, please also update redemption.Update()
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Mode = mode
		if redemption.PriceFen < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "价格（分）不能小于0"})
			return
		}
		cleanRedemption.PriceFen = redemption.PriceFen
		cleanRedemption.Quota = redemption.Quota
		cleanRedemption.ExpiredTime = redemption.ExpiredTime
		cleanRedemption.DailyQuotaLimit = redemption.DailyQuotaLimit
		if redemption.DailyRequestLimit < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "每日次数必须大于等于0"})
			return
		}
		cleanRedemption.DailyRequestLimit = redemption.DailyRequestLimit
		if redemption.QuotaValidDays < 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "额度有效期（天）不能小于0"})
			return
		}
		cleanRedemption.QuotaValidDays = redemption.QuotaValidDays
		cleanRedemption.PlanValidDays = redemption.PlanValidDays
		cleanRedemption.ChannelIds = redemption.ChannelIds
		cleanRedemption.AllowedGroups = nil
		cleanRedemption.AllowedGroupIds = allowedGroupIDsJSON

		if mode == "activation" {
			if cleanRedemption.Quota != 0 || cleanRedemption.DailyQuotaLimit != 0 || cleanRedemption.DailyRequestLimit != 0 || cleanRedemption.QuotaValidDays != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 || len(cleanRedemption.AllowedGroupIds) != 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "激活码参数错误"})
				return
			}
		} else if mode == "subscription" {
			if cleanRedemption.Quota < 0 || cleanRedemption.DailyQuotaLimit < 0 || cleanRedemption.DailyRequestLimit != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "订阅额度兑换码参数错误"})
				return
			}
		} else if mode == "tokens" {
			if cleanRedemption.Quota < 0 || cleanRedemption.DailyQuotaLimit < 0 || cleanRedemption.DailyRequestLimit != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 || len(cleanRedemption.AllowedGroupIds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "tokens 兑换码参数错误"})
				return
			}
		} else if mode == "request" {
			if cleanRedemption.DailyRequestLimit < 0 || cleanRedemption.Quota < 0 || cleanRedemption.DailyQuotaLimit != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 || len(cleanRedemption.AllowedGroupIds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "次数订阅兑换码参数错误"})
				return
			}
		} else if mode == "payg" {
			if cleanRedemption.Quota <= 0 || cleanRedemption.DailyQuotaLimit != 0 || cleanRedemption.DailyRequestLimit != 0 || cleanRedemption.QuotaValidDays != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 || len(cleanRedemption.AllowedGroupIds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "按量付费兑换码参数错误"})
				return
			}
		} else if mode == "pay_request" {
			if cleanRedemption.Quota <= 0 || cleanRedemption.DailyQuotaLimit != 0 || cleanRedemption.DailyRequestLimit != 0 || cleanRedemption.QuotaValidDays != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 || len(cleanRedemption.AllowedGroupIds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "按次付费兑换码参数错误"})
				return
			}
		} else if mode == "pay_token" {
			if cleanRedemption.Quota <= 0 || cleanRedemption.DailyQuotaLimit != 0 || cleanRedemption.DailyRequestLimit != 0 || cleanRedemption.QuotaValidDays != 0 || cleanRedemption.PlanValidDays != 0 || len(cleanRedemption.ChannelIds) != 0 || len(cleanRedemption.AllowedGroupIds) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "按token付费兑换码参数错误"})
				return
			}
		} else {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的兑换码类型"})
			return
		}
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	}
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
	return
}

func DeleteInvalidRedemption(c *gin.Context) {
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

type RedemptionBatchStatus struct {
	Ids    []int `json:"ids"`
	Status int   `json:"status"`
}

// BatchUpdateRedemptionStatus 批量更新兑换码状态
func BatchUpdateRedemptionStatus(c *gin.Context) {
	var req RedemptionBatchStatus
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	// 仅允许设置为合法状态
	if req.Status != common.RedemptionCodeStatusEnabled &&
		req.Status != common.RedemptionCodeStatusDisabled &&
		req.Status != common.RedemptionCodeStatusUsed {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "非法的兑换码状态",
		})
		return
	}

	count, err := model.BatchUpdateRedemptionsStatus(req.Ids, req.Status)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func validateExpiredTime(expired int64) error {
	if expired != 0 && expired < common.GetTimestamp() {
		return errors.New("过期时间不能早于当前时间")
	}
	return nil
}

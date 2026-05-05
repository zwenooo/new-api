package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"one-api/common"
	"one-api/model"
	"one-api/setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminPaygTopupRequest struct {
	ProductId int `json:"product_id" binding:"required"`
	Quota     int `json:"quota" binding:"required"`
}

// AdminTopupUserPayg credits PAYG quota into a user's PAYG balance item (by product_id).
// It keeps payg_user_balances, user.payg_quota and user.payg_allowed_groups consistent.
func AdminTopupUserPayg(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	var req adminPaygTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if req.ProductId <= 0 {
		common.ApiErrorMsg(c, "product_id 无效")
		return
	}
	if req.Quota <= 0 {
		common.ApiErrorMsg(c, "quota 必须大于 0")
		return
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := lockForUpdate(tx).
			Select("id").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}

		var product model.PaygProduct
		if err := tx.Where("id = ?", req.ProductId).First(&product).Error; err != nil {
			return err
		}

		groupIDs, err := model.GetPaygProductAllowedGroupIDsTx(tx, product.Id)
		if err != nil {
			return err
		}
		if len(groupIDs) == 0 {
			return errors.New("按量付费商品可用分组为空")
		}

		if err := model.UpsertPaygUserBalanceTx(
			tx,
			userId,
			product.Id,
			product.Name,
			product.SortOrder,
			groupIDs,
			req.Quota,
		); err != nil {
			return err
		}

		// Rebuild union groups for payg_allowed_groups from positive balances.
		balances, err := model.GetUserPaygBalancesTx(tx, userId, true)
		if err != nil {
			return err
		}
		unionGroupsJSON, err := model.UnionPaygAllowedGroupsFromBalances(balances)
		if err != nil {
			return err
		}

		return tx.Model(&model.User{}).Where("id = ?", userId).Updates(map[string]interface{}{
			"payg_quota":          gorm.Expr("payg_quota + ?", req.Quota),
			"payg_history_quota":  gorm.Expr("payg_history_quota + ?", req.Quota),
			"payg_allowed_groups": unionGroupsJSON,
			"quota":               gorm.Expr("quota + ?", req.Quota),
		}).Error
	}); err != nil {
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

type adminPaygGroupTopupRequest struct {
	GroupId int `json:"group_id" binding:"required"`
	Quota   int `json:"quota" binding:"required"`
}

// AdminTopupUserPaygByGroup credits PAYG quota into a user's group-specific PAYG balance item.
// It keeps payg_user_balances, user.payg_quota and user.payg_allowed_groups consistent.
func AdminTopupUserPaygByGroup(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	var req adminPaygGroupTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	groupID := req.GroupId
	if groupID <= 0 {
		common.ApiErrorMsg(c, "group_id 无效")
		return
	}
	if req.Quota <= 0 {
		common.ApiErrorMsg(c, "quota 必须大于 0")
		return
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := lockForUpdate(tx).
			Select("id").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}

		// Use a stable synthetic product_id per group to make balances independent per group.
		// Keep away from legacy sentinel (-1) and avoid colliding with configured PAYG product IDs (positive).
		productId := -1000000 - groupID
		groupIDs := []int{groupID}
		group, err := model.GetGroupByID(tx, groupID)
		if err != nil {
			return err
		}

		if err := model.UpsertPaygUserBalanceTx(
			tx,
			userId,
			productId,
			group.Code,
			1000000,
			groupIDs,
			req.Quota,
		); err != nil {
			return err
		}

		// Rebuild union groups for payg_allowed_groups from positive balances.
		balances, err := model.GetUserPaygBalancesTx(tx, userId, true)
		if err != nil {
			return err
		}
		unionGroupsJSON, err := model.UnionPaygAllowedGroupsFromBalances(balances)
		if err != nil {
			return err
		}

		return tx.Model(&model.User{}).Where("id = ?", userId).Updates(map[string]interface{}{
			"payg_quota":          gorm.Expr("payg_quota + ?", req.Quota),
			"payg_history_quota":  gorm.Expr("payg_history_quota + ?", req.Quota),
			"payg_allowed_groups": unionGroupsJSON,
			"quota":               gorm.Expr("quota + ?", req.Quota),
		}).Error
	}); err != nil {
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

type adminPaygBalanceUpdateRequest struct {
	AllowedGroupIds []int `json:"allowed_group_ids" binding:"required"`
}

type adminReorderUserPaygBalancesRequest struct {
	ProductIds []int `json:"product_ids" binding:"required"`
}

func AdminReorderUserPaygBalances(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}

	var req adminReorderUserPaygBalancesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	if err := model.ReorderUserPaygBalances(userId, req.ProductIds); err != nil {
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

// AdminUpdateUserPaygBalanceAllowedGroups updates allowed_group_ids for one PAYG balance item.
// For product-based balances (product_id > 0), this will enable per-user override of product groups.
func AdminUpdateUserPaygBalanceAllowedGroups(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	productId, err := strconv.Atoi(c.Param("productId"))
	if err != nil || productId == 0 {
		common.ApiErrorMsg(c, "product_id 无效")
		return
	}

	var req adminPaygBalanceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	groupIDs := model.NormalizeUniqueSortedIDs(req.AllowedGroupIds)
	if len(groupIDs) == 0 {
		common.ApiErrorMsg(c, "可用分组不能为空")
		return
	}
	if err := model.ValidateGroupIDsExist(nil, groupIDs); err != nil {
		common.ApiError(c, err)
		return
	}
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		if !setting.GroupInEnabledGroups(gid) {
			label, ok := model.GetGroupLabelByID(gid)
			if !ok {
				common.ApiErrorMsg(c, "分组不存在")
				return
			}
			common.ApiErrorMsg(c, fmt.Sprintf("分组 %s 已被禁用", label))
			return
		}
	}

	allowedJSON, err := model.MarshalGroupIDsJSON(groupIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := lockForUpdate(tx).
			Select("id").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}

		var bal model.PaygUserBalance
		if err := lockForUpdate(tx).
			Where("user_id = ? AND product_id = ?", userId, productId).
			First(&bal).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"allowed_group_ids": allowedJSON,
		}
		if productId > 0 {
			updates["override_allowed_group_ids"] = true
		}
		if err := tx.Model(&model.PaygUserBalance{}).Where("id = ?", bal.Id).Updates(updates).Error; err != nil {
			return err
		}

		// Rebuild union groups for payg_allowed_groups from positive balances.
		balances, err := model.GetUserPaygBalancesTx(tx, userId, true)
		if err != nil {
			return err
		}
		unionGroupsJSON, err := model.UnionPaygAllowedGroupsFromBalances(balances)
		if err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", userId).Update("payg_allowed_groups", unionGroupsJSON).Error
	}); err != nil {
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

// AdminDeleteUserPaygBalance clears remaining_quota for one PAYG balance item and keeps user totals consistent.
func AdminDeleteUserPaygBalance(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid user id"})
		return
	}
	productId, err := strconv.Atoi(c.Param("productId"))
	if err != nil || productId == 0 {
		common.ApiErrorMsg(c, "product_id 无效")
		return
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := model.SyncUserPaygSnapshotFromBalancesTx(tx, userId); err != nil {
			return err
		}

		var user model.User
		if err := lockForUpdate(tx).
			Select("id", "quota", "payg_quota").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}

		var bal model.PaygUserBalance
		if err := lockForUpdate(tx).
			Select("id", "remaining_quota").
			Where("user_id = ? AND product_id = ?", userId, productId).
			First(&bal).Error; err != nil {
			return err
		}

		remaining := bal.RemainingQuota
		if remaining < 0 {
			return errors.New("remaining_quota 数据错误")
		}

		if remaining > 0 {
			if err := tx.Model(&model.PaygUserBalance{}).Where("id = ?", bal.Id).
				Update("remaining_quota", 0).Error; err != nil {
				return err
			}
		}
		if err := model.ResetProductBackedPaygBalanceGroupsToProductTx(tx, bal.Id, productId); err != nil {
			return err
		}

		// Rebuild union groups for payg_allowed_groups from positive balances.
		balances, err := model.GetUserPaygBalancesTx(tx, userId, true)
		if err != nil {
			return err
		}
		unionGroupsJSON, err := model.UnionPaygAllowedGroupsFromBalances(balances)
		if err != nil {
			return err
		}

		updates := map[string]interface{}{
			"payg_allowed_groups": unionGroupsJSON,
		}
		if remaining > 0 {
			if user.PayAsYouGoQuota < remaining {
				return errors.New("payg_quota 数据错误")
			}
			if user.Quota < remaining {
				return errors.New("quota 数据错误")
			}
			updates["payg_quota"] = gorm.Expr("payg_quota - ?", remaining)
			updates["quota"] = gorm.Expr("quota - ?", remaining)
		}
		return tx.Model(&model.User{}).Where("id = ?", userId).Updates(updates).Error
	}); err != nil {
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

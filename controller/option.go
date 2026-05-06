package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/setting"
	"one-api/setting/console_setting"
	"one-api/setting/payg_setting"
	"one-api/setting/ratio_setting"
	"one-api/setting/system_setting"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetOptions(c *gin.Context) {
	var options []*model.Option
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		// Hide sensitive and internal options from the generic settings endpoint.
		if strings.HasSuffix(k, "Token") || strings.HasSuffix(k, "Secret") || strings.HasSuffix(k, "Key") {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: common.Interface2String(v),
		})
	}
	common.OptionMapRWMutex.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
	return
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

var productManagementOptionKeys = map[string]struct{}{
	"payg.credit_usd_per_cny":              {},
	"payg.credit_requests_per_cny":         {},
	"payg.credit_tokens_per_cny":           {},
	"payg.products":                        {},
	"payg.pay_request_products":            {},
	"payg.pay_token_products":              {},
	"ProductManagementHideArchivedEnabled": {},
}

var productManagementDisplayOptionKeys = []string{
	"ProductManagementHideArchivedEnabled",
}

func decodeOptionUpdateRequest(c *gin.Context) (OptionUpdateRequest, bool) {
	var option OptionUpdateRequest
	err := json.NewDecoder(c.Request.Body).Decode(&option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return OptionUpdateRequest{}, false
	}
	switch v := option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(v)
	case float64:
		option.Value = common.Interface2String(v)
	case int:
		option.Value = common.Interface2String(v)
	case []interface{}, map[string]interface{}:
		// 对数组/对象保持 JSON 语义，避免 fmt.Sprintf 生成的 "[a b]" 之类字符串导致配置无法解析
		if b, mErr := json.Marshal(v); mErr == nil {
			option.Value = string(b)
		} else {
			option.Value = fmt.Sprintf("%v", v)
		}
	default:
		option.Value = fmt.Sprintf("%v", v)
	}
	return option, true
}

func isProductManagementOptionKey(key string) bool {
	_, ok := productManagementOptionKeys[key]
	return ok
}

func GetProductManagementOptions(c *gin.Context) {
	options := make([]*model.Option, 0, len(productManagementDisplayOptionKeys))
	common.OptionMapRWMutex.RLock()
	for _, key := range productManagementDisplayOptionKeys {
		options = append(options, &model.Option{
			Key:   key,
			Value: common.Interface2String(common.OptionMap[key]),
		})
	}
	common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

func parseNormalizedPaygProducts(raw string) ([]payg_setting.PaygProduct, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "<nil>" {
		raw = "[]"
	}
	var products []payg_setting.PaygProduct
	if err := json.Unmarshal([]byte(raw), &products); err != nil {
		return nil, fmt.Errorf("按量付费商品解析失败")
	}
	normalized, err := payg_setting.NormalizePaygProducts(products)
	if err != nil {
		return nil, err
	}
	for i := range normalized {
		if err := normalizePayProductAllowedGroups(nil, &normalized[i].AllowedGroupIds, &normalized[i].AllowedGroups, normalized[i].Enabled, normalized[i].Archived, "按量付费商品"); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func refreshPaygProductsOptionCache(products []payg_setting.PaygProduct, raw string) {
	common.OptionMapRWMutex.Lock()
	common.OptionMap["payg.products"] = raw
	common.OptionMapRWMutex.Unlock()
	payg_setting.GetPaygSettings().Products = products
}

func normalizePayProductAllowedGroups(tx *gorm.DB, groupIDs *[]int, legacyGroups *[]string, enabled bool, archived bool, label string) error {
	if groupIDs == nil || legacyGroups == nil {
		return fmt.Errorf("%s可用分组无效", label)
	}
	if len(*groupIDs) == 0 && len(*legacyGroups) > 0 {
		ids, err := model.LegacyGroupIDsFromCodes(tx, *legacyGroups)
		if err != nil {
			return err
		}
		*groupIDs = ids
	}
	*groupIDs = model.NormalizeUniqueSortedIDs(*groupIDs)
	*legacyGroups = nil
	if len(*groupIDs) == 0 {
		if enabled && !archived {
			return fmt.Errorf("%s可用分组不能为空", label)
		}
		return nil
	}
	if tx != nil {
		return model.ValidateGroupIDsExist(tx, *groupIDs)
	}
	return model.ValidateGroupIDsExist(nil, *groupIDs)
}

func syncPaygProductsToDB(products []payg_setting.PaygProduct) error {
	tx := model.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	keepProductIDs := make([]int, 0, len(products))
	for _, p := range products {
		keepProductIDs = append(keepProductIDs, p.Id)
		groupIDs := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
		if len(groupIDs) == 0 && p.Enabled && !p.Archived {
			_ = tx.Rollback()
			return fmt.Errorf("按量付费商品 #%d 可用分组为空", p.Id)
		}
		if len(groupIDs) > 0 {
			if err := model.ValidateGroupIDsExist(tx, groupIDs); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if p.Archived {
			p.Enabled = false
		}
		if err := tx.Select("id", "name", "description", "enabled", "archived", "sort_order", "stock").Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "description", "enabled", "archived", "sort_order", "stock"}),
		}).Create(&model.PaygProduct{
			Id:          p.Id,
			Name:        p.Name,
			Description: p.Description,
			Enabled:     p.Enabled,
			Archived:    p.Archived,
			SortOrder:   p.SortOrder,
			Stock:       p.Stock,
		}).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Where("product_id = ?", p.Id).Delete(&model.PaygProductGroup{}).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
		if len(groupIDs) > 0 {
			rows := make([]model.PaygProductGroup, 0, len(groupIDs))
			for _, groupID := range groupIDs {
				rows = append(rows, model.PaygProductGroup{ProductId: p.Id, GroupId: groupID})
			}
			if err := tx.Create(&rows).Error; err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		var groupsJSON model.JSONValue
		if len(groupIDs) > 0 {
			if b, gErr := common.Marshal(groupIDs); gErr == nil {
				groupsJSON = model.JSONValue(b)
			} else {
				_ = tx.Rollback()
				return gErr
			}
		}
		if _, err := model.EnsureCurrentPaygProductRevisionTx(tx, p.Id); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Model(&model.PaygUserBalance{}).
			Where("product_id = ?", p.Id).
			Updates(map[string]interface{}{
				"product_name": p.Name,
				"sort_order":   p.SortOrder,
			}).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Model(&model.PaygUserBalance{}).
			Where("product_id = ? AND (override_allowed_group_ids IS NULL OR override_allowed_group_ids = 0)", p.Id).
			Update("allowed_group_ids", groupsJSON).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	keepProductIDs = model.NormalizeUniqueSortedIDs(keepProductIDs)
	var missingProductIDs []int
	missingProductsQuery := tx.Model(&model.PaygProduct{}).
		Where("(archived = ? OR enabled = ?)", false, true)
	if len(keepProductIDs) > 0 {
		missingProductsQuery = missingProductsQuery.Where("id NOT IN ?", keepProductIDs)
	}
	if err := missingProductsQuery.Order("id ASC").Pluck("id", &missingProductIDs).Error; err != nil {
		_ = tx.Rollback()
		return err
	}
	if len(missingProductIDs) > 0 {
		if err := tx.Model(&model.PaygProduct{}).
			Where("id IN ?", missingProductIDs).
			Updates(map[string]interface{}{
				"enabled":  false,
				"archived": true,
			}).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
		for _, productID := range missingProductIDs {
			if _, err := model.EnsureCurrentPaygProductRevisionTx(tx, productID); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	if err := model.BackfillUsersPaygSnapshotFromBalances(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := model.ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := model.ValidateClawBoxProductModeConfigTx(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	dbProducts, err := model.ListPaygProductsForOptionTx(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	dbProductsJSON, err := json.Marshal(dbProducts)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&model.Option{
		Key:   "payg.products",
		Value: string(dbProductsJSON),
	}).Error; err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	refreshPaygProductsOptionCache(dbProducts, string(dbProductsJSON))
	return nil
}

func rollbackPaygProductsOption(oldVal string) error {
	products, err := parseNormalizedPaygProducts(oldVal)
	if err != nil {
		return err
	}
	if err := model.UpdateOption("payg.products", oldVal); err != nil {
		return err
	}
	if err := syncPaygProductsToDB(products); err != nil {
		return err
	}
	return model.RefreshGroupSettings()
}

func UpdateOption(c *gin.Context) {
	option, ok := decodeOptionUpdateRequest(c)
	if !ok {
		return
	}
	updateOptionInternal(c, option)
}

func UpdateProductManagementOption(c *gin.Context) {
	option, ok := decodeOptionUpdateRequest(c)
	if !ok {
		return
	}
	if !isProductManagementOptionKey(option.Key) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前接口仅支持商品管理相关配置项",
		})
		return
	}
	updateOptionInternal(c, option)
}

func updateOptionInternal(c *gin.Context, option OptionUpdateRequest) {

	var (
		err                      error
		paygProductsToSync       []payg_setting.PaygProduct
		paygProductsOldVal       string
		clawBoxOptionOldVal      string
		clawBoxOptionRollbackKey string
		payRequestProductsToSync []payg_setting.PayRequestProduct
		payRequestProductsOldVal string
		payTokenProductsToSync   []payg_setting.PayTokenProduct
		payTokenProductsOldVal   string
		refreshGroupSettings     bool
	)
	switch option.Key {
	case "GroupRatio":
		fallthrough
	case "UserUsableGroups":
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "分组配置已迁移至「分组管理」（/console/setting?tab=ratio），请在该页面修改分组倍率/显示名/可选性",
		})
		return
	case "payg.products":
		common.OptionMapRWMutex.RLock()
		paygProductsOldVal = common.Interface2String(common.OptionMap["payg.products"])
		common.OptionMapRWMutex.RUnlock()
		normalized, err := parseNormalizedPaygProducts(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		if b, err := json.Marshal(normalized); err == nil {
			option.Value = string(b)
		}
		paygProductsToSync = normalized
	case "ClawBoxProductModeEnabled":
		common.OptionMapRWMutex.RLock()
		clawBoxOptionOldVal = common.Interface2String(common.OptionMap[option.Key])
		common.OptionMapRWMutex.RUnlock()
		clawBoxOptionRollbackKey = option.Key
	case "payg.pay_request_products":
		common.OptionMapRWMutex.RLock()
		payRequestProductsOldVal = common.Interface2String(common.OptionMap["payg.pay_request_products"])
		common.OptionMapRWMutex.RUnlock()

		raw := strings.TrimSpace(option.Value.(string))
		if raw == "" || raw == "null" {
			raw = "[]"
		}
		var products []payg_setting.PayRequestProduct
		if err := json.Unmarshal([]byte(raw), &products); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "按次付费商品解析失败",
			})
			return
		}
		normalized, err := payg_setting.NormalizePayRequestProducts(products)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		// Normalize legacy allowed_groups -> allowed_group_ids, and validate group ids exist.
		for i := range normalized {
			if err := normalizePayProductAllowedGroups(nil, &normalized[i].AllowedGroupIds, &normalized[i].AllowedGroups, normalized[i].Enabled, normalized[i].Archived, "按次付费商品"); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		}
		if b, err := json.Marshal(normalized); err == nil {
			option.Value = string(b)
		}
		payRequestProductsToSync = normalized
	case "payg.pay_token_products":
		common.OptionMapRWMutex.RLock()
		payTokenProductsOldVal = common.Interface2String(common.OptionMap["payg.pay_token_products"])
		common.OptionMapRWMutex.RUnlock()

		raw := strings.TrimSpace(option.Value.(string))
		if raw == "" || raw == "null" {
			raw = "[]"
		}
		var products []payg_setting.PayTokenProduct
		if err := json.Unmarshal([]byte(raw), &products); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "按token付费商品解析失败",
			})
			return
		}
		normalized, err := payg_setting.NormalizePayTokenProducts(products)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		// Normalize legacy allowed_groups -> allowed_group_ids, and validate group ids exist.
		for i := range normalized {
			if err := normalizePayProductAllowedGroups(nil, &normalized[i].AllowedGroupIds, &normalized[i].AllowedGroups, normalized[i].Enabled, normalized[i].Archived, "按token付费商品"); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		}
		if b, err := json.Marshal(normalized); err == nil {
			option.Value = string(b)
		}
		payTokenProductsToSync = normalized
	case "AutoGroups":
		raw := strings.TrimSpace(option.Value.(string))
		if raw == "" || raw == "null" {
			raw = "[]"
		}
		var groups []int
		if err := json.Unmarshal([]byte(raw), &groups); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "AutoGroups 解析失败，必须为 JSON 数组，例如：[4,5]",
			})
			return
		}
		// AutoGroups order matters (pick from the first usable group_id). Dedup while preserving order.
		seen := make(map[int]struct{}, len(groups))
		normalized := make([]int, 0, len(groups))
		for _, gid := range groups {
			if gid <= 0 {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "AutoGroups 必须为正整数数组，例如：[4,5]",
				})
				return
			}
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			normalized = append(normalized, gid)
		}
		if len(normalized) > 0 {
			if err := model.ValidateGroupIDsExist(nil, normalized); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		}
		if b, err := json.Marshal(normalized); err == nil {
			option.Value = string(b)
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "cx_compat.opencode.instructions":
		fallthrough
	case "cx_compat.opencode.instructions_meta":
		fallthrough
	case "cx_compat.opencode.pinned_instructions":
		fallthrough
	case "cx_compat.opencode.pinned_meta":
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "OpenCode instructions 不允许直接修改，请使用「cx模型兼容性配置」中的按钮操作（同步/恢复默认/设为默认版本）",
		})
		return
	case "cx_compat.responses.body_patch_json":
		raw := strings.TrimSpace(option.Value.(string))
		if raw == "" || raw == "null" || raw == "<nil>" {
			option.Value = ""
			break
		}
		var patch map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &patch); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "cx_compat.responses.body_patch_json 解析失败，必须为 JSON 对象，例如：{\"stream\":true}",
			})
			return
		}
	case "ClawBoxProductId":
		common.OptionMapRWMutex.RLock()
		clawBoxOptionOldVal = common.Interface2String(common.OptionMap[option.Key])
		common.OptionMapRWMutex.RUnlock()
		clawBoxOptionRollbackKey = option.Key
		trimmed := strings.TrimSpace(option.Value.(string))
		if trimmed != "" {
			productID, err := strconv.Atoi(trimmed)
			if err != nil || productID <= 0 {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "ClawBox 商品 ID 必须为正整数",
				})
				return
			}
			option.Value = strconv.Itoa(productID)
		} else {
			option.Value = ""
		}
	case "ClawBoxSignupShrimpQuota":
		trimmed := strings.TrimSpace(option.Value.(string))
		if trimmed == "" {
			trimmed = "0"
		}
		quota, err := strconv.Atoi(trimmed)
		if err != nil || quota < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ClawBox 注册赠送虾粮必须为大于等于 0 的整数",
			})
			return
		}
		option.Value = strconv.Itoa(quota)
	case "ClawBoxInitialShrimp":
		trimmed := strings.TrimSpace(option.Value.(string))
		if trimmed == "" {
			option.Value = "0"
			break
		}
		initialShrimp, err := strconv.Atoi(trimmed)
		if err != nil || initialShrimp < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ClawBox 初始虾粮必须为大于等于 0 的整数",
			})
			return
		}
		option.Value = strconv.Itoa(initialShrimp)
	case model.ClawBoxManagedOpenClawConfigOption:
		normalized, err := model.NormalizeClawBoxManagedOpenClawConfigValue(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		option.Value = normalized
	case "ServerAddress":
		trimmed := strings.TrimSpace(option.Value.(string))
		trimmed = strings.TrimRight(trimmed, "/")
		if strings.HasSuffix(trimmed, "/v1") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "服务器地址不要包含 /v1，/v1 请配置在「基址」里（例如：https://yourdomain.com/v1）",
			})
			return
		}
	case "CustomCallbackAddress":
		trimmed := strings.TrimSpace(option.Value.(string))
		trimmed = strings.TrimRight(trimmed, "/")
		if trimmed != "" && strings.HasSuffix(trimmed, "/v1") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "回调地址不要包含 /v1，/v1 请配置在「基址」里（例如：https://yourdomain.com/v1）",
			})
			return
		}
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})

			return
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token！",
			})
			return
		}
	case "payg.credit_usd_per_cny":
		v := strings.TrimSpace(option.Value.(string))
		f, convErr := strconv.ParseFloat(v, 64)
		if convErr != nil || f <= 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "payg.credit_usd_per_cny 必须为大于 0 的数字",
			})
			return
		}
	case "payg.credit_requests_per_cny":
		v := strings.TrimSpace(option.Value.(string))
		i, convErr := strconv.Atoi(v)
		if convErr != nil || i < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "payg.credit_requests_per_cny 必须为大于等于 0 的整数",
			})
			return
		}
	case "payg.credit_tokens_per_cny":
		v := strings.TrimSpace(option.Value.(string))
		i, convErr := strconv.Atoi(v)
		if convErr != nil || i < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "payg.credit_tokens_per_cny 必须为大于等于 0 的整数",
			})
			return
		}
	case "payg.enabled":
		v := strings.TrimSpace(option.Value.(string))
		if v != "true" && v != "false" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "payg.enabled 必须为 true 或 false",
			})
			return
		}
	case "payg.description":
		v := strings.TrimSpace(option.Value.(string))
		if utf8.RuneCountInString(v) > 2048 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "payg.description 不能超过 2048 字符",
			})
			return
		}
	case "payg.allowed_groups":
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "payg.allowed_groups 已废弃，请在商品管理为每个按量付费商品配置可用分组",
		})
		return
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "图片倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频补全倍率设置失败: " + err.Error(),
			})
			return
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "ModelRequestConcurrencyLimit":
		limit, convErr := strconv.Atoi(option.Value.(string))
		if convErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ModelRequestConcurrencyLimit 必须为整数",
			})
			return
		}
		if limit < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ModelRequestConcurrencyLimit 必须大于等于0",
			})
			return
		}
	case "ModelRequestConcurrencyLimitWaitSeconds":
		waitSeconds, convErr := strconv.Atoi(option.Value.(string))
		if convErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ModelRequestConcurrencyLimitWaitSeconds 必须为整数",
			})
			return
		}
		if waitSeconds < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ModelRequestConcurrencyLimitWaitSeconds 必须大于等于0",
			})
			return
		}
	case "ModelRequestConcurrencyLimitGroup":
		err = setting.CheckModelRequestConcurrencyLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	err = model.UpdateOption(option.Key, option.Value.(string))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if option.Key == "payg.products" && paygProductsToSync != nil {
		syncErr := syncPaygProductsToDB(paygProductsToSync)
		if syncErr != nil {
			if rollErr := rollbackPaygProductsOption(paygProductsOldVal); rollErr != nil {
				common.ApiError(c, fmt.Errorf("同步按量商品余额失败: %v；回滚按量商品配置失败: %v", syncErr, rollErr))
				return
			}
			common.ApiError(c, fmt.Errorf("同步按量商品余额失败，已回滚按量商品配置: %v", syncErr))
			return
		}
		refreshGroupSettings = true
	}

	if option.Key == "payg.pay_request_products" && payRequestProductsToSync != nil {
		syncErr := func() error {
			tx := model.DB.Begin()
			if tx.Error != nil {
				return tx.Error
			}
			keepProductIDs := make([]int, 0, len(payRequestProductsToSync))
			for _, p := range payRequestProductsToSync {
				keepProductIDs = append(keepProductIDs, p.Id)
				groupIDs := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
				if len(groupIDs) == 0 && p.Enabled && !p.Archived {
					_ = tx.Rollback()
					return fmt.Errorf("按次付费商品 #%d 可用分组为空", p.Id)
				}
				if len(groupIDs) > 0 {
					if err := model.ValidateGroupIDsExist(tx, groupIDs); err != nil {
						_ = tx.Rollback()
						return err
					}
				}
				if err := tx.Where("product_id = ?", p.Id).Delete(&model.PayRequestProductGroup{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if len(groupIDs) > 0 {
					rows := make([]model.PayRequestProductGroup, 0, len(groupIDs))
					for _, groupID := range groupIDs {
						rows = append(rows, model.PayRequestProductGroup{ProductId: p.Id, GroupId: groupID})
					}
					if err := tx.Create(&rows).Error; err != nil {
						_ = tx.Rollback()
						return err
					}
				}
				if err := tx.Select("id", "name", "description", "enabled", "archived", "sort_order", "stock", "created_at", "updated_at").Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "id"}},
					DoUpdates: clause.AssignmentColumns([]string{"name", "description", "enabled", "archived", "sort_order", "stock"}),
				}).Create(&model.PayRequestProduct{
					Id:          p.Id,
					Name:        p.Name,
					Description: p.Description,
					Enabled:     p.Enabled,
					Archived:    p.Archived,
					SortOrder:   p.SortOrder,
					Stock:       p.Stock,
				}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if _, err := model.EnsureCurrentPayRequestProductRevisionTx(tx, p.Id); err != nil {
					_ = tx.Rollback()
					return err
				}
				var groupsJSON model.JSONValue
				if len(groupIDs) > 0 {
					if b, gErr := common.Marshal(groupIDs); gErr == nil {
						groupsJSON = model.JSONValue(b)
					} else {
						_ = tx.Rollback()
						return gErr
					}
				}
				if err := tx.Model(&model.PayRequestUserBalance{}).
					Where("product_id = ?", p.Id).
					Updates(map[string]interface{}{
						"product_name":      p.Name,
						"sort_order":        p.SortOrder,
						"allowed_group_ids": groupsJSON,
					}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
			}
			// Delete products removed from payg.pay_request_products so that DB-backed /api/user/topup/info stays consistent.
			keepProductIDs = model.NormalizeUniqueSortedIDs(keepProductIDs)
			if len(keepProductIDs) == 0 {
				if err := tx.Where("1 = 1").Delete(&model.PayRequestProductGroup{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if err := tx.Where("1 = 1").Delete(&model.PayRequestProduct{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
			} else {
				if err := tx.Where("product_id NOT IN ?", keepProductIDs).Delete(&model.PayRequestProductGroup{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if err := tx.Where("id NOT IN ?", keepProductIDs).Delete(&model.PayRequestProduct{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
			}
			if err := model.BackfillUsersPayRequestSnapshotFromBalances(tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			if _, err := model.ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			return tx.Commit().Error
		}()
		if syncErr != nil {
			// Roll back option to keep config and balances consistent.
			if rollErr := model.UpdateOption(option.Key, payRequestProductsOldVal); rollErr != nil {
				common.ApiError(c, fmt.Errorf("同步按次商品余额失败: %v；回滚配置失败: %v", syncErr, rollErr))
				return
			}
			common.ApiError(c, fmt.Errorf("同步按次商品余额失败，已回滚配置: %v", syncErr))
			return
		}
		refreshGroupSettings = true
	}

	if option.Key == "payg.pay_token_products" && payTokenProductsToSync != nil {
		syncErr := func() error {
			tx := model.DB.Begin()
			if tx.Error != nil {
				return tx.Error
			}
			keepProductIDs := make([]int, 0, len(payTokenProductsToSync))
			for _, p := range payTokenProductsToSync {
				keepProductIDs = append(keepProductIDs, p.Id)
				groupIDs := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
				if len(groupIDs) == 0 && p.Enabled && !p.Archived {
					_ = tx.Rollback()
					return fmt.Errorf("按token付费商品 #%d 可用分组为空", p.Id)
				}
				if len(groupIDs) > 0 {
					if err := model.ValidateGroupIDsExist(tx, groupIDs); err != nil {
						_ = tx.Rollback()
						return err
					}
				}
				if err := tx.Where("product_id = ?", p.Id).Delete(&model.PayTokenProductGroup{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if len(groupIDs) > 0 {
					rows := make([]model.PayTokenProductGroup, 0, len(groupIDs))
					for _, groupID := range groupIDs {
						rows = append(rows, model.PayTokenProductGroup{ProductId: p.Id, GroupId: groupID})
					}
					if err := tx.Create(&rows).Error; err != nil {
						_ = tx.Rollback()
						return err
					}
				}
				if err := tx.Select("id", "name", "description", "enabled", "archived", "sort_order", "stock", "created_at", "updated_at").Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "id"}},
					DoUpdates: clause.AssignmentColumns([]string{"name", "description", "enabled", "archived", "sort_order", "stock"}),
				}).Create(&model.PayTokenProduct{
					Id:          p.Id,
					Name:        p.Name,
					Description: p.Description,
					Enabled:     p.Enabled,
					Archived:    p.Archived,
					SortOrder:   p.SortOrder,
					Stock:       p.Stock,
				}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if _, err := model.EnsureCurrentPayTokenProductRevisionTx(tx, p.Id); err != nil {
					_ = tx.Rollback()
					return err
				}
				var groupsJSON model.JSONValue
				if len(groupIDs) > 0 {
					if b, gErr := common.Marshal(groupIDs); gErr == nil {
						groupsJSON = model.JSONValue(b)
					} else {
						_ = tx.Rollback()
						return gErr
					}
				}
				if err := tx.Model(&model.PayTokenUserBalance{}).
					Where("product_id = ?", p.Id).
					Updates(map[string]interface{}{
						"product_name":      p.Name,
						"sort_order":        p.SortOrder,
						"allowed_group_ids": groupsJSON,
					}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
			}

			keepProductIDs = model.NormalizeUniqueSortedIDs(keepProductIDs)
			if len(keepProductIDs) == 0 {
				if err := tx.Where("1 = 1").Delete(&model.PayTokenProductGroup{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if err := tx.Where("1 = 1").Delete(&model.PayTokenProduct{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
			} else {
				if err := tx.Where("product_id NOT IN ?", keepProductIDs).Delete(&model.PayTokenProductGroup{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
				if err := tx.Where("id NOT IN ?", keepProductIDs).Delete(&model.PayTokenProduct{}).Error; err != nil {
					_ = tx.Rollback()
					return err
				}
			}
			if err := model.BackfillUsersPayTokenSnapshotFromBalances(tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			if _, err := model.ReconcileGroupNoBillingProductKeysTx(tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			return tx.Commit().Error
		}()
		if syncErr != nil {
			if rollErr := model.UpdateOption(option.Key, payTokenProductsOldVal); rollErr != nil {
				common.ApiError(c, fmt.Errorf("同步按token付费商品余额失败: %v；回滚配置失败: %v", syncErr, rollErr))
				return
			}
			common.ApiError(c, fmt.Errorf("同步按token付费商品余额失败，已回滚配置: %v", syncErr))
			return
		}
		refreshGroupSettings = true
	}

	if refreshGroupSettings {
		if err := model.RefreshGroupSettings(); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	if clawBoxOptionRollbackKey != "" && model.ClawBoxProductModeEnabled() {
		if err := model.ValidateClawBoxProductModeConfigTx(nil); err != nil {
			if rollErr := model.UpdateOption(clawBoxOptionRollbackKey, clawBoxOptionOldVal); rollErr != nil {
				common.ApiError(c, fmt.Errorf("ClawBox 商品配置无效: %v；回滚配置失败: %v", err, rollErr))
				return
			}
			common.ApiError(c, fmt.Errorf("ClawBox 商品配置无效，已回滚配置: %v", err))
			return
		}
	}

	if option.Key == "payg.products" || option.Key == "ClawBoxProductModeEnabled" || option.Key == "ClawBoxProductId" {
		if err := model.SyncAllClawBoxRelayTokens(); err != nil {
			if option.Key == "payg.products" {
				if rollErr := rollbackPaygProductsOption(paygProductsOldVal); rollErr != nil {
					common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败: %v；回滚按量商品失败: %v", err, rollErr))
					return
				}
				if resyncErr := model.SyncAllClawBoxRelayTokens(); resyncErr != nil {
					common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败，已回滚按量商品但恢复 token 失败: %v；恢复错误: %v", err, resyncErr))
					return
				}
				common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败，已回滚按量商品: %v", err))
				return
			}
			if clawBoxOptionRollbackKey != "" {
				if rollErr := model.UpdateOption(clawBoxOptionRollbackKey, clawBoxOptionOldVal); rollErr != nil {
					common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败: %v；回滚配置失败: %v", err, rollErr))
					return
				}
				if resyncErr := model.SyncAllClawBoxRelayTokens(); resyncErr != nil {
					common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败，已回滚配置但恢复 token 失败: %v；恢复错误: %v", err, resyncErr))
					return
				}
				common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败，已回滚配置: %v", err))
				return
			}
			common.ApiError(c, fmt.Errorf("ClawBox relay token 同步失败: %v", err))
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

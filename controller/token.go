package controller

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/setting"
	"one-api/setting/ratio_setting"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type tokenMutationRequest struct {
	Id                 int             `json:"id"`
	Status             int             `json:"status"`
	Name               string          `json:"name"`
	ExpiredTime        int64           `json:"expired_time"`
	RemainQuota        *int            `json:"remain_quota"`
	UnlimitedQuota     bool            `json:"unlimited_quota"`
	ModelLimitsEnabled bool            `json:"model_limits_enabled"`
	ModelLimits        string          `json:"model_limits"`
	AllowIps           *string         `json:"allow_ips"`
	DailyQuotaLimit    int             `json:"daily_quota_limit"`
	Group              string          `json:"group"`
	GroupId            int             `json:"group_id"`
	DefaultGroupId     int             `json:"default_group_id,omitempty"`
	AllowedGroups      model.JSONValue `json:"allowed_groups"`
	AllowedGroupIds    model.JSONValue `json:"allowed_group_ids"`
}

type tokenPublicUsageRow struct {
	TokenId          int `gorm:"column:token_id"`
	VisibleUsedQuota int `gorm:"column:visible_used_quota"`
	CostUsedQuota    int `gorm:"column:cost_used_quota"`
}

type publicTokenView struct {
	Id                 int             `json:"id"`
	UserId             int             `json:"user_id"`
	Key                string          `json:"key"`
	Status             int             `json:"status"`
	Name               string          `json:"name"`
	CreatedTime        int64           `json:"created_time"`
	AccessedTime       int64           `json:"accessed_time"`
	ExpiredTime        int64           `json:"expired_time"`
	UnlimitedQuota     bool            `json:"unlimited_quota"`
	ModelLimitsEnabled bool            `json:"model_limits_enabled"`
	ModelLimits        string          `json:"model_limits"`
	AllowIps           *string         `json:"allow_ips"`
	RemainQuota        int             `json:"remain_quota"`
	DailyQuotaLimit    int             `json:"daily_quota_limit"`
	Group              string          `json:"group"`
	GroupId            int             `json:"group_id"`
	DefaultGroupId     int             `json:"default_group_id,omitempty"`
	AllowedGroups      model.JSONValue `json:"allowed_groups"`
	AllowedGroupIds    model.JSONValue `json:"allowed_group_ids"`
	UsedQuota          int             `json:"used_quota"`
	VisibleUsedQuota   int             `json:"visible_used_quota"`
	CostUsedQuota      int             `json:"cost_used_quota"`
	QuotaDetailsHidden bool            `json:"quota_details_hidden"`
}

func getPublicTokenUsageByIDs(tokenIDs []int) (map[int]tokenPublicUsageRow, error) {
	if len(tokenIDs) == 0 {
		return map[int]tokenPublicUsageRow{}, nil
	}
	uniqueIDs := make([]int, 0, len(tokenIDs))
	seen := make(map[int]struct{}, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if tokenID <= 0 {
			continue
		}
		if _, ok := seen[tokenID]; ok {
			continue
		}
		seen[tokenID] = struct{}{}
		uniqueIDs = append(uniqueIDs, tokenID)
	}
	if len(uniqueIDs) == 0 {
		return map[int]tokenPublicUsageRow{}, nil
	}

	rows := make([]tokenPublicUsageRow, 0, len(uniqueIDs))
	if err := model.LOG_DB.Table("logs").
		Select("token_id, COALESCE(SUM(visible_quota), 0) AS visible_used_quota, COALESCE(SUM(cost_quota), 0) AS cost_used_quota").
		Where("type = ? AND token_id IN ?", model.LogTypeConsume, uniqueIDs).
		Group("token_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	usageByTokenID := make(map[int]tokenPublicUsageRow, len(rows))
	for _, row := range rows {
		usageByTokenID[row.TokenId] = row
	}
	return usageByTokenID, nil
}

func buildPublicTokenView(token *model.Token, usage tokenPublicUsageRow) publicTokenView {
	if token == nil {
		return publicTokenView{}
	}
	return publicTokenView{
		Id:                 token.Id,
		UserId:             token.UserId,
		Key:                token.Key,
		Status:             token.Status,
		Name:               token.Name,
		CreatedTime:        token.CreatedTime,
		AccessedTime:       token.AccessedTime,
		ExpiredTime:        token.ExpiredTime,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		RemainQuota:        token.RemainQuota,
		DailyQuotaLimit:    token.DailyQuotaLimit,
		Group:              token.Group,
		GroupId:            token.GroupId,
		DefaultGroupId:     token.DefaultGroupId,
		AllowedGroups:      token.AllowedGroups,
		AllowedGroupIds:    token.AllowedGroupIds,
		UsedQuota:          token.UsedQuota,
		VisibleUsedQuota:   usage.VisibleUsedQuota,
		CostUsedQuota:      usage.CostUsedQuota,
	}
}

func fastPublicTokenUsage(token *model.Token) tokenPublicUsageRow {
	if token == nil {
		return tokenPublicUsageRow{}
	}
	// The list page only needs a stable value for rendering. Avoid summing the
	// historical logs table on every page open; token.UsedQuota is maintained on
	// consume and is already the primary quota shown in the UI.
	return tokenPublicUsageRow{
		TokenId:          token.Id,
		VisibleUsedQuota: token.UsedQuota,
		CostUsedQuota:    token.UsedQuota,
	}
}

func buildPublicTokenViews(tokens []*model.Token, includeUsage bool) ([]publicTokenView, error) {
	usageByTokenID := map[int]tokenPublicUsageRow{}
	if includeUsage {
		tokenIDs := make([]int, 0, len(tokens))
		for _, token := range tokens {
			if token == nil || token.Id <= 0 {
				continue
			}
			tokenIDs = append(tokenIDs, token.Id)
		}
		var err error
		usageByTokenID, err = getPublicTokenUsageByIDs(tokenIDs)
		if err != nil {
			return nil, err
		}
	}

	views := make([]publicTokenView, 0, len(tokens))
	for _, token := range tokens {
		if token == nil {
			continue
		}
		usage := usageByTokenID[token.Id]
		if !includeUsage {
			usage = fastPublicTokenUsage(token)
		}
		views = append(views, buildPublicTokenView(token, usage))
	}
	return views, nil
}

func shouldIncludeTokenUsage(c *gin.Context) bool {
	return strings.ToLower(strings.TrimSpace(c.Query("with_usage"))) != "false"
}

func validateTokenGroupsForUser(userId int, userRole int, tokenGroupIDs []int) (bool, string, error) {
	if len(tokenGroupIDs) == 0 {
		return true, "", nil
	}

	isAdmin := userRole >= common.RoleAdminUser
	// Treat admin/root users as unrestricted by user_selectable (only "enabled" matters).
	// For common users, allow selecting groups that are either:
	// - globally user_selectable, OR
	// - currently billable (owned via subscriptions / pay-as-you-go balances).

	usable := setting.GetUserUsableGroupsCopy()
	allowed := map[int]struct{}{}
	if !isAdmin {
		allowed = make(map[int]struct{}, len(usable)+8)
		for gid := range usable {
			if gid <= 0 {
				continue
			}
			allowed[gid] = struct{}{}
		}

		if userId > 0 {
			owned, err := model.GetUserBillableGroupIDs(userId)
			if err != nil {
				return false, "", err
			}
			for _, gid := range owned {
				if gid <= 0 {
					continue
				}
				allowed[gid] = struct{}{}
			}
		}
	}

	for _, gid := range tokenGroupIDs {
		if gid <= 0 {
			return false, "分组 id 无效", nil
		}
		label, labelOK := model.GetGroupLabelByID(gid)
		if !labelOK {
			return false, "分组不存在", nil
		}
		if !setting.GroupInEnabledGroups(gid) {
			return false, fmt.Sprintf("分组 %s 已被禁用", label), nil
		}
		if !isAdmin && model.IsInternalDefaultModelGroupID(nil, gid) {
			return false, fmt.Sprintf("分组 %s 不允许用户直接选择", label), nil
		}
		if !isAdmin {
			if _, ok := allowed[gid]; !ok {
				return false, fmt.Sprintf("分组 %s 不可用", label), nil
			}
		}
		if !ratio_setting.ContainsGroupRatio(gid) {
			return false, fmt.Sprintf("分组 %s 已被弃用", label), nil
		}
	}
	return true, "", nil
}

func requestedTokenPrimaryGroupID(groupId int, defaultGroupId int) (int, string) {
	requested := 0
	if groupId > 0 {
		requested = groupId
	}
	if defaultGroupId > 0 {
		if requested > 0 && requested != defaultGroupId {
			return 0, "group_id 与 default_group_id 不一致"
		}
		requested = defaultGroupId
	}
	return requested, ""
}

func resolveForcedClawBoxTokenGroups(userId int, tokenName string) ([]int, model.JSONValue, bool, error) {
	if !strings.EqualFold(strings.TrimSpace(tokenName), model.ClawBoxRelayTokenName) {
		return nil, nil, false, nil
	}
	allowedGroupIDs, _, err := model.ResolveClawBoxRelayGroupIDsTx(nil, userId)
	if err != nil {
		return nil, nil, true, err
	}
	normalizedAllowedGroupIDs, err := model.MarshalGroupIDsJSONKeepOrder(allowedGroupIDs)
	if err != nil {
		return nil, nil, true, err
	}
	return allowedGroupIDs, normalizedAllowedGroupIDs, true, nil
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.FillTokensAllowedGroupIDs(tokens); err != nil {
		common.ApiError(c, err)
		return
	}
	publicTokens, err := buildPublicTokenViews(tokens, shouldIncludeTokenUsage(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(publicTokens)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")
	tokens, err := model.SearchUserTokens(userId, keyword, token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.FillTokensAllowedGroupIDs(tokens); err != nil {
		common.ApiError(c, err)
		return
	}
	publicTokens, err := buildPublicTokenViews(tokens, shouldIncludeTokenUsage(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    publicTokens,
	})
	return
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usageByTokenID, err := getPublicTokenUsageByIDs([]int{token.Id})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildPublicTokenView(token, usageByTokenID[token.Id]),
	})
	return
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	publicUsedQuota, err := getPublicUsageQuotaByToken(token.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":               "credit_summary",
		"total_granted":        nil,
		"total_used":           publicUsedQuota,
		"total_available":      nil,
		"expires_at":           expiredAt * 1000,
		"quota_details_hidden": true,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	publicUsedQuota, err := getPublicUsageQuotaByToken(token.Id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":               "token_usage",
			"name":                 token.Name,
			"total_granted":        nil,
			"total_used":           publicUsedQuota,
			"total_available":      nil,
			"unlimited_quota":      token.UnlimitedQuota,
			"model_limits":         token.GetModelLimitsMap(),
			"model_limits_enabled": token.ModelLimitsEnabled,
			"expires_at":           expiredAt,
			"quota_details_hidden": true,
		},
	})
}

func AddToken(c *gin.Context) {
	req := tokenMutationRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if req.DailyQuotaLimit < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "每日额度必须大于等于0",
		})
		return
	}
	if len(req.Name) > 30 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称过长",
		})
		return
	}
	userID := c.GetInt("id")
	userRole := c.GetInt("role")
	if len(req.AllowedGroups) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "allowed_groups 已废弃，请使用 allowed_group_ids",
		})
		return
	}
	if strings.TrimSpace(req.Group) != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "group 已废弃，请使用 allowed_group_ids",
		})
		return
	}
	requestedPrimaryGroupID, groupIDMessage := requestedTokenPrimaryGroupID(req.GroupId, req.DefaultGroupId)
	if groupIDMessage != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": groupIDMessage,
		})
		return
	}

	allowedGroupIDs, err := model.ParseGroupIDsJSONKeepOrder(req.AllowedGroupIds)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	forcedGroupIDs, forcedAllowedGroupsJSON, forceClawBoxGroups, err := resolveForcedClawBoxTokenGroups(userID, req.Name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if forceClawBoxGroups {
		allowedGroupIDs = forcedGroupIDs
		req.AllowedGroupIds = forcedAllowedGroupsJSON
	} else {
		if len(allowedGroupIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "请选择可用分组",
			})
			return
		}
		if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		normalizedAllowedGroupIDs, err := model.MarshalGroupIDsJSONKeepOrder(allowedGroupIDs)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		req.AllowedGroupIds = normalizedAllowedGroupIDs

		if ok, message, err := validateTokenGroupsForUser(userID, userRole, allowedGroupIDs); err != nil {
			common.ApiError(c, err)
			return
		} else if !ok {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": message,
			})
			return
		}
	}
	if requestedPrimaryGroupID > 0 && requestedPrimaryGroupID != allowedGroupIDs[0] {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "group_id/default_group_id 已废弃，请直接调整 allowed_group_ids 顺序",
		})
		return
	}
	remainQuota := 0
	if req.RemainQuota != nil {
		remainQuota = *req.RemainQuota
	}
	if !req.UnlimitedQuota && req.RemainQuota == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请输入额度",
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成令牌失败",
		})
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             userID,
		Name:               req.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        req.ExpiredTime,
		RemainQuota:        remainQuota,
		UnlimitedQuota:     req.UnlimitedQuota,
		ModelLimitsEnabled: req.ModelLimitsEnabled,
		ModelLimits:        req.ModelLimits,
		AllowIps:           req.AllowIps,
		AllowedGroupIds:    req.AllowedGroupIds,
		DailyQuotaLimit:    req.DailyQuotaLimit,
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Ensure the new token takes effect immediately even if Redis has stale or partially-written data.
	_ = model.InvalidateTokenCache(cleanToken.Key)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
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

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	userRole := c.GetInt("role")
	statusOnly := c.Query("status_only")
	req := tokenMutationRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(req.Name) > 30 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称过长",
		})
		return
	}
	requestedPrimaryGroupID, groupIDMessage := requestedTokenPrimaryGroupID(req.GroupId, req.DefaultGroupId)
	if groupIDMessage != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": groupIDMessage,
		})
		return
	}
	cleanToken, err := model.GetTokenByIds(req.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if len(req.AllowedGroups) > 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "allowed_groups 已废弃，请使用 allowed_group_ids",
			})
			return
		}
		if strings.TrimSpace(req.Group) != "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "group 已废弃，请使用 allowed_group_ids",
			})
			return
		}

		allowedGroupIDs, err := model.ParseGroupIDsJSONKeepOrder(req.AllowedGroupIds)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		effectiveTokenName := strings.TrimSpace(req.Name)
		if effectiveTokenName == "" {
			effectiveTokenName = strings.TrimSpace(cleanToken.Name)
		}
		forcedGroupIDs, forcedAllowedGroupsJSON, forceClawBoxGroups, err := resolveForcedClawBoxTokenGroups(userId, effectiveTokenName)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if forceClawBoxGroups {
			allowedGroupIDs = forcedGroupIDs
			req.AllowedGroupIds = forcedAllowedGroupsJSON
		} else {
			if len(allowedGroupIDs) == 0 {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "请选择可用分组",
				})
				return
			}
			if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			normalizedAllowedGroupIDs, err := model.MarshalGroupIDsJSONKeepOrder(allowedGroupIDs)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			req.AllowedGroupIds = normalizedAllowedGroupIDs
			if ok, message, err := validateTokenGroupsForUser(userId, userRole, allowedGroupIDs); err != nil {
				common.ApiError(c, err)
				return
			} else if !ok {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": message,
				})
				return
			}
		}
		if requestedPrimaryGroupID > 0 && requestedPrimaryGroupID != allowedGroupIDs[0] {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "group_id/default_group_id 已废弃，请直接调整 allowed_group_ids 顺序",
			})
			return
		}
	}
	if req.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌已过期，无法启用，请先修改令牌过期时间，或者设置为永不过期",
			})
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌可用额度已用尽，无法启用，请先修改令牌剩余额度，或者设置为无限额度",
			})
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = req.Status
	} else {
		if !req.UnlimitedQuota && cleanToken.UnlimitedQuota && req.RemainQuota == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "关闭无限额度时必须设置新的额度",
			})
			return
		}
		// If you add more fields, please also update token.Update()
		cleanToken.Name = req.Name
		cleanToken.ExpiredTime = req.ExpiredTime
		if req.RemainQuota != nil {
			cleanToken.RemainQuota = *req.RemainQuota
		}
		cleanToken.UnlimitedQuota = req.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = req.ModelLimitsEnabled
		cleanToken.ModelLimits = req.ModelLimits
		cleanToken.AllowIps = req.AllowIps
		cleanToken.AllowedGroupIds = req.AllowedGroupIds
		cleanToken.DailyQuotaLimit = req.DailyQuotaLimit
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Prevent short-lived mismatch between DB and Redis cache (e.g. allowed_groups not taking effect immediately).
	_ = model.InvalidateTokenCache(cleanToken.Key)
	usageByTokenID, err := getPublicTokenUsageByIDs([]int{cleanToken.Id})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildPublicTokenView(cleanToken, usageByTokenID[cleanToken.Id]),
	})
	return
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
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

package middleware

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/logger"
	"one-api/model"
	relayhelper "one-api/relay/helper"
	"one-api/setting"
	"one-api/setting/ratio_setting"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func validUserInfo(username string, role int) bool {
	// check username is empty
	if strings.TrimSpace(username) == "" {
		return false
	}
	if !common.IsValidateRole(role) {
		return false
	}
	return true
}

func authHelper(c *gin.Context, minRole int) {
	session := sessions.Default(c)
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	useAccessToken := false
	if username == nil {
		// Check access token
		accessToken := strings.TrimSpace(c.Request.Header.Get("Authorization"))
		if accessToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录且未提供 access token",
			})
			c.Abort()
			return
		}
		// Compatibility:
		// - Transfer API uses "Authorization: <token>"
		// - Some upstream clients (e.g. Aether's New API template) send "Authorization: Bearer <token>"
		accessTokenLower := strings.ToLower(accessToken)
		if strings.HasPrefix(accessTokenLower, "bearer ") {
			accessToken = strings.TrimSpace(accessToken[7:])
		}
		user := model.ValidateAccessToken(accessToken)
		if user != nil && user.Username != "" {
			if !validUserInfo(user.Username, user.Role) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，用户信息无效",
				})
				c.Abort()
				return
			}
			// Token is valid
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，access token 无效",
			})
			c.Abort()
			return
		}
	}
	// get header Transfer-Api-User
	// Compatibility: accept New-Api-User as an alias (New API style).
	apiUserIdStr := c.Request.Header.Get("Transfer-Api-User")
	if apiUserIdStr == "" {
		apiUserIdStr = c.Request.Header.Get("New-Api-User")
	}
	if apiUserIdStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，未提供 Transfer-Api-User/New-Api-User",
		})
		c.Abort()
		return
	}
	apiUserId, err := strconv.Atoi(apiUserIdStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，Transfer-Api-User/New-Api-User 格式错误",
		})
		c.Abort()
		return

	}
	if id != apiUserId {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，Transfer-Api-User/New-Api-User 与登录用户不匹配",
		})
		c.Abort()
		return
	}
	if status.(int) == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		c.Abort()
		return
	}
	if role.(int) < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，权限不足",
		})
		c.Abort()
		return
	}
	if !validUserInfo(username.(string), role.(int)) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，用户信息无效",
		})
		c.Abort()
		return
	}
	c.Set("username", username)
	c.Set("role", role)
	c.Set("id", id)
	c.Set("use_access_token", useAccessToken)

	userId, ok := id.(int)
	if !ok || userId <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，用户信息无效",
		})
		c.Abort()
		return
	}
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		c.Abort()
		return
	}
	userCache.WriteContext(c)

	c.Next()
}

func TryUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id := session.Get("id")
		if id != nil {
			c.Set("id", id)
		}
		c.Next()
	}
}

func UserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

func AdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

// AdminPageAuth is intended for browser navigations / HTML pages where we cannot
// rely on custom headers like Transfer-Api-User.
func AdminPageAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		username := session.Get("username")
		role := session.Get("role")
		id := session.Get("id")
		status := session.Get("status")

		if username == nil || role == nil || id == nil || status == nil {
			c.String(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}

		roleInt, ok := role.(int)
		if !ok {
			c.String(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		statusInt, ok := status.(int)
		if !ok {
			c.String(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}

		if statusInt == common.UserStatusDisabled {
			c.String(http.StatusForbidden, "forbidden")
			c.Abort()
			return
		}
		if roleInt < common.RoleAdminUser {
			c.String(http.StatusForbidden, "forbidden")
			c.Abort()
			return
		}
		usernameStr, ok := username.(string)
		if !ok || strings.TrimSpace(usernameStr) == "" || !common.IsValidateRole(roleInt) {
			c.String(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}

		c.Set("username", username)
		c.Set("role", roleInt)
		c.Set("id", id)
		c.Set("use_access_token", false)

		userId, ok := id.(int)
		if !ok || userId <= 0 {
			c.String(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		userCache, err := model.GetUserCache(userId)
		if err != nil {
			c.String(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		userCache.WriteContext(c)
		c.Next()
	}
}

// AdminPageOrHeaderAuth authenticates admin requests in two modes:
// - API/AJAX requests that include custom headers (Transfer-Api-User / New-Api-User)
// - Browser navigations (e.g. window.open/download URLs) where custom headers cannot be set.
//
// It prefers header-based auth when the user id header is present; otherwise it falls back to AdminPageAuth.
func AdminPageOrHeaderAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		apiUserIdStr := strings.TrimSpace(c.Request.Header.Get("Transfer-Api-User"))
		if apiUserIdStr == "" {
			apiUserIdStr = strings.TrimSpace(c.Request.Header.Get("New-Api-User"))
		}
		if apiUserIdStr != "" {
			authHelper(c, common.RoleAdminUser)
			return
		}
		AdminPageAuth()(c)
	}
}

func RootAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

func RequireAdminModulePermission(module string) func(c *gin.Context) {
	return func(c *gin.Context) {
		role := c.GetInt("role")
		if role < common.RoleAdminUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，权限不足",
			})
			c.Abort()
			return
		}

		adminPermissions, ok := common.GetContextKeyType[model.AdminPermissions](c, constant.ContextKeyUserAdminPermissions)
		if !ok {
			userId := c.GetInt("id")
			userCache, err := model.GetUserCache(userId)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，用户信息无效",
				})
				c.Abort()
				return
			}
			adminPermissions = userCache.GetAdminPermissions()
		}

		if !model.HasAdminModulePermission(role, adminPermissions, module) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，权限不足",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func WssAuth(c *gin.Context) {

}

func TokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 先检测是否为ws
		if c.Request.Header.Get("Sec-WebSocket-Protocol") != "" {
			// Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.sk-xxx, openai-beta.realtime-v1
			// read sk from Sec-WebSocket-Protocol
			key := c.Request.Header.Get("Sec-WebSocket-Protocol")
			parts := strings.Split(key, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "openai-insecure-api-key") {
					key = strings.TrimPrefix(part, "openai-insecure-api-key.")
					break
				}
			}
			c.Request.Header.Set("Authorization", "Bearer "+key)
		}
		// 检查path包含/v1/messages
		if strings.Contains(c.Request.URL.Path, "/v1/messages") {
			anthropicKey := c.Request.Header.Get("x-api-key")
			if anthropicKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+anthropicKey)
			}
		}
		// gemini api 从query中获取key
		if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1beta/openai/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
			skKey := c.Query("key")
			if skKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+skKey)
			}
			// 从x-goog-api-key header中获取key
			xGoogKey := c.Request.Header.Get("x-goog-api-key")
			if xGoogKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+xGoogKey)
			}
		}
		extractAuthToken := func(headerValue string) string {
			raw := strings.TrimSpace(headerValue)
			if raw == "" {
				return ""
			}
			fields := strings.Fields(raw)
			if len(fields) >= 2 && strings.EqualFold(fields[0], "bearer") {
				return strings.TrimSpace(fields[1])
			}
			// Compatibility:
			// - Transfer API uses "Authorization: <token>"
			// - Some upstream clients send "Authorization: Bearer <token>"
			return raw
		}

		tokenRaw := extractAuthToken(c.Request.Header.Get("Authorization"))
		if tokenRaw == "" || tokenRaw == "midjourney-proxy" {
			tokenRaw = extractAuthToken(c.Request.Header.Get("mj-api-secret"))
		}
		tokenRaw = strings.TrimSpace(tokenRaw)
		tokenRaw = strings.TrimPrefix(tokenRaw, "sk-")
		parts := strings.SplitN(tokenRaw, "-", 2)
		key := parts[0]

		token, err := model.ValidateUserToken(key)
		var userCache *model.UserBase
		if token != nil && c.GetInt("id") == 0 {
			c.Set("id", token.UserId)
		}

		if err != nil {
			isQuotaError := token != nil && (token.Status == common.TokenStatusExhausted || (!token.UnlimitedQuota && token.RemainQuota <= 0))
			if isQuotaError {
				cache, cacheErr := model.GetUserCache(token.UserId)
				if cacheErr != nil {
					abortWithOpenAiMessage(c, http.StatusInternalServerError, cacheErr.Error())
					return
				}
				userCache = cache
			}
			if err != nil {
				if isQuotaError {
					if strings.Contains(c.GetHeader("Accept"), "text/event-stream") &&
						strings.HasPrefix(c.Request.URL.Path, "/v1/responses") {
						message := common.MessageWithRequestId(err.Error(), c.GetString(common.RequestIdKey))
						relayhelper.ResponsesFailed(c, "insufficient_quota", message)
						c.Abort()
						logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", c.GetInt("id"), err.Error()))
						return
					}
					abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error(), "insufficient_quota")
					return
				}
				abortWithOpenAiMessage(c, http.StatusUnauthorized, err.Error())
				return
			}
		}

		allowIpsMap := token.GetIpLimitsMap()
		if len(allowIpsMap) != 0 {
			clientIp := c.ClientIP()
			if _, ok := allowIpsMap[clientIp]; !ok {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "您的 IP 不在令牌允许访问的列表中")
				return
			}
		}

		if userCache == nil {
			userCache, err = model.GetUserCache(token.UserId)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusInternalServerError, err.Error())
				return
			}
		}
		userEnabled := userCache.Status == common.UserStatusEnabled
		if !userEnabled {
			tokenFingerprint := ""
			if token.Key != "" {
				hash := common.GenerateHMAC(token.Key)
				if len(hash) > 16 {
					tokenFingerprint = hash[:16]
				} else {
					tokenFingerprint = hash
				}
			}
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("[token_auth_user_disabled] user_id=%d token_id=%d token_fp=%s cache_status=%d token_status=%d", token.UserId, token.Id, tokenFingerprint, userCache.Status, token.Status))
			abortWithOpenAiMessage(c, http.StatusBadRequest, "用户已被封禁")
			return
		}

		userCache.WriteContext(c)

		defaultModelGroupID := userCache.GroupId
		tokenAllowedGroupIDs, err := model.ParseGroupIDsJSONKeepOrder(token.AllowedGroupIds)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error())
			return
		}
		if len(tokenAllowedGroupIDs) == 0 {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "当前令牌分组为空")
			return
		}
		for _, gid := range tokenAllowedGroupIDs {
			label, exists := model.GetGroupLabelByID(gid)
			if !exists {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("[token_auth_allowed_group_not_found] user_id=%d token_id=%d group_id=%d", token.UserId, token.Id, gid))
				abortWithOpenAiMessage(c, http.StatusBadRequest, "当前令牌分组不存在，请联系管理员")
				return
			}
			if !setting.GroupInEnabledGroups(gid) {
				abortWithOpenAiMessage(c, http.StatusBadRequest, fmt.Sprintf("当前令牌分组 %s 已被禁用", label))
				return
			}
			if !ratio_setting.ContainsGroupRatio(gid) {
				abortWithOpenAiMessage(c, http.StatusBadRequest, fmt.Sprintf("分组 %s 已被弃用", label))
				return
			}
		}
		tokenGroupID := model.FirstGroupIDKeepOrder(tokenAllowedGroupIDs)
		if defaultModelGroupID <= 0 && tokenGroupID <= 0 {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "用户默认模型分组无效")
			return
		}
		common.SetContextKey(c, constant.ContextKeyTokenAllowedGroupIds, tokenAllowedGroupIDs)
		common.SetContextKey(c, constant.ContextKeyTokenGroupId, tokenGroupID)

		usingGroupID := defaultModelGroupID
		if tokenGroupID > 0 {
			usingGroupID = tokenGroupID
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroupId, usingGroupID)

		err = SetupContextForToken(c, token, parts...)
		if err != nil {
			return
		}
		c.Next()
	}
}

func SetupContextForToken(c *gin.Context, token *model.Token, parts ...string) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	c.Set("id", token.UserId)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Set("token_name", token.Name)
	c.Set("token_unlimited_quota", token.UnlimitedQuota)
	if !token.UnlimitedQuota {
		c.Set("token_quota", token.RemainQuota)
	}
	common.SetContextKey(c, constant.ContextKeyTokenDailyQuotaLimit, token.DailyQuotaLimit)
	common.SetContextKey(c, constant.ContextKeyTokenDailyQuotaUsed, token.DailyQuotaUsed)
	common.SetContextKey(c, constant.ContextKeyTokenDailyQuotaResetDate, token.DailyQuotaResetDate)
	if token.ModelLimitsEnabled {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", token.GetModelLimitsMap())
	} else {
		c.Set("token_model_limit_enabled", false)
	}
	if len(parts) > 1 {
		if c.GetInt("role") >= common.RoleAdminUser {
			c.Set("specific_channel_id", parts[1])
		} else {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "普通用户不支持指定渠道")
			return fmt.Errorf("普通用户不支持指定渠道")
		}
	}
	return nil
}

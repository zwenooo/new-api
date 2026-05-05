package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"one-api/billing"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	"one-api/setting"
	"one-api/setting/personal_setting"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func displayDiscreteUnits(units int) float64 {
	return billing.StoredUnitsToDisplay(units)
}

func applyDiscreteUserResponseData(responseData map[string]interface{}, user *model.User) {
	if responseData == nil || user == nil {
		return
	}
	responseData["tokens_quota"] = displayDiscreteUnits(user.TokensQuota)
	responseData["tokens_history_quota"] = displayDiscreteUnits(user.TokensHistoryQuota)
	responseData["pay_request_quota"] = displayDiscreteUnits(user.PayRequestQuota)
	responseData["pay_request_history_quota"] = displayDiscreteUnits(user.PayRequestHistoryQuota)
	responseData["pay_token_quota"] = displayDiscreteUnits(user.PayTokenQuota)
	responseData["pay_token_history_quota"] = displayDiscreteUnits(user.PayTokenHistoryQuota)
}

func applyPublicUsageResponseData(responseData map[string]interface{}, user *model.User) {
	if responseData == nil || user == nil {
		return
	}
	responseData["used_quota"] = user.UsedQuota
	responseData["visible_used_quota"] = user.VisibleUsedQuota
	responseData["cost_used_quota"] = user.CostUsedQuota
}

type userPaygBalanceDTO struct {
	ProductId       int    `json:"product_id"`
	ProductName     string `json:"product_name"`
	SortOrder       int    `json:"sort_order"`
	RemainingQuota  int    `json:"remaining_quota"`
	AllowedGroupIds []int  `json:"allowed_group_ids"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

func buildUserPaygBalances(balances []model.PaygUserBalance) []userPaygBalanceDTO {
	if len(balances) == 0 {
		return []userPaygBalanceDTO{}
	}
	items := make([]userPaygBalanceDTO, 0, len(balances))
	for _, b := range balances {
		groupIDs, gErr := model.ResolvePaygBalanceAllowedGroupIDs(b)
		if gErr != nil {
			common.SysLog("failed to parse payg balance allowed_group_ids: " + gErr.Error())
			groupIDs = nil
		}
		items = append(items, userPaygBalanceDTO{
			ProductId:       b.ProductId,
			ProductName:     b.ProductName,
			SortOrder:       b.SortOrder,
			RemainingQuota:  b.RemainingQuota,
			AllowedGroupIds: groupIDs,
			CreatedAt:       b.CreatedAt,
			UpdatedAt:       b.UpdatedAt,
		})
	}
	return items
}

func applyQuotaBalanceResponseData(responseData map[string]interface{}, user *model.User) {
	if responseData == nil || user == nil {
		return
	}
	responseData["quota"] = user.Quota
	responseData["payg_quota"] = user.PayAsYouGoQuota
	responseData["payg_history_quota"] = user.PayAsYouGoHistoryQuota
	responseData["payg_allowed_groups"] = user.PayAsYouGoAllowedGroups
	responseData["redeem_quota"] = user.RedeemQuota
	responseData["redeem_quota_expire_at"] = user.RedeemQuotaExpireAt
}

func applyLegacyClawBoxTotalQuotaCompat(responseData map[string]interface{}, user *model.User) {
	if responseData == nil || user == nil {
		return
	}
	paygRemaining := user.PayAsYouGoQuota
	if paygRemaining < 0 {
		paygRemaining = 0
	}
	subscriptionRemaining := user.RedeemQuota
	if subscriptionRemaining < 0 {
		subscriptionRemaining = 0
	}
	freeRemaining := user.Quota - user.RedeemQuota - user.PayAsYouGoQuota
	if freeRemaining < 0 {
		freeRemaining = 0
	}
	// Old ClawBox builds fall back to /api/user/self.quota when PAYG-specific fields
	// are absent. Expose the effective visible remaining quota instead of the raw
	// internal snapshot, because raw user.Quota may be negative while redeem_quota
	// still holds active subscription balance.
	responseData["quota"] = freeRemaining + subscriptionRemaining + paygRemaining
}

func stripRetiredXiaotuanFields(responseData map[string]interface{}) {
	if responseData == nil {
		return
	}
	delete(responseData, "plan_type")
	delete(responseData, "plan_start_at")
	delete(responseData, "plan_expire_at")
	delete(responseData, "xiaotuan_active")
	delete(responseData, "xiaotuan_channel_count")
}

func Login(c *gin.Context) {
	if !common.PasswordLoginEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员关闭了密码登录",
			"success": false,
		})
		return
	}
	var loginRequest LoginRequest
	err := json.NewDecoder(c.Request.Body).Decode(&loginRequest)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "无效的参数",
			"success": false,
		})
		return
	}
	username := loginRequest.Username
	password := loginRequest.Password
	if username == "" || password == "" {
		c.JSON(http.StatusOK, gin.H{
			"message": "无效的参数",
			"success": false,
		})
		return
	}
	user := model.User{
		Username: username,
		Password: password,
	}
	err = user.ValidateAndFill()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}

	// 检查是否启用2FA
	if model.IsTwoFAEnabled(user.Id) {
		// 设置pending session，等待2FA验证
		session := sessions.Default(c)
		session.Set("pending_username", user.Username)
		session.Set("pending_user_id", user.Id)
		err := session.Save()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message": "无法保存会话信息，请重试",
				"success": false,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "请输入两步验证码",
			"success": true,
			"data": map[string]interface{}{
				"require_2fa": true,
			},
		})
		return
	}

	setupLogin(&user, c)
}

// setup session & cookies and then return user info
func setupLogin(user *model.User, c *gin.Context) {
	session := sessions.Default(c)
	session.Set("id", user.Id)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Set("status", user.Status)
	session.Set("group_id", user.GroupId)
	err := session.Save()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "无法保存会话信息，请重试",
			"success": false,
		})
		return
	}
	if err := user.EnsureAvatarSeed(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
		"data": gin.H{
			"id":           user.Id,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"avatar_seed":  user.AvatarSeed,
			"role":         user.Role,
			"status":       user.Status,
			"group_id":     user.GroupId,
		},
	})
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	err := session.Save()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
	})
}

func Register(c *gin.Context) {
	if !common.RegisterEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员关闭了新用户注册",
			"success": false,
		})
		return
	}
	if !common.PasswordRegisterEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员关闭了通过密码进行注册，请使用第三方账户验证的形式进行注册",
			"success": false,
		})
		return
	}
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if user.BaseMultiplier == 0 {
		user.BaseMultiplier = 1
	}
	if user.BaseMultiplier <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "基础倍率必须大于0",
		})
		return
	}
	if err := common.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法 " + common.ValidationErrorMessage(err),
		})
		return
	}
	if user.DailyQuotaLimit < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "每日额度必须大于等于0",
		})
		return
	}

	if common.EmailVerificationEnabled {
		if user.Email == "" || user.VerificationCode == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员开启了邮箱验证，请输入邮箱地址和验证码",
			})
			return
		}
		if !common.VerifyCodeWithKey(user.Email, user.VerificationCode, common.EmailVerificationPurpose) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "验证码错误或已过期",
			})
			return
		}
	}
	exist, err := model.CheckUserExistOrDeleted(user.Username, user.Email)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "数据库错误，请稍后重试",
		})
		common.SysLog(fmt.Sprintf("CheckUserExistOrDeleted error: %v", err))
		return
	}
	if exist {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户名已存在，或已注销",
		})
		return
	}
	affCode := user.AffCode // this code is the inviter's code, not the user's own code
	inviterId, _ := model.GetUserIdByAffCode(affCode)
	cleanUser := model.User{
		Username:        user.Username,
		Password:        user.Password,
		DisplayName:     user.Username,
		Remark:          strings.TrimSpace(user.Remark),
		InviterId:       inviterId,
		Role:            common.RoleCommonUser,
		DailyQuotaLimit: user.DailyQuotaLimit,
		RegisterIP:      c.ClientIP(),
	}
	if common.EmailVerificationEnabled {
		cleanUser.Email = user.Email
	}
	if err := cleanUser.Insert(inviterId); err != nil {
		common.ApiError(c, err)
		return
	}

	// 获取插入后的用户ID
	var insertedUser model.User
	if err := model.DB.Where("username = ?", cleanUser.Username).First(&insertedUser).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户注册失败或用户ID获取失败",
		})
		return
	}
	// 生成默认令牌
	if constant.GenerateDefaultToken {
		key, err := common.GenerateKey()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "生成默认令牌失败",
			})
			common.SysLog("failed to generate token key: " + err.Error())
			return
		}
		// 生成默认令牌
		token := model.Token{
			UserId:             insertedUser.Id, // 使用插入后的用户ID
			Name:               cleanUser.Username + "的初始令牌",
			Key:                key,
			CreatedTime:        common.GetTimestamp(),
			AccessedTime:       common.GetTimestamp(),
			ExpiredTime:        -1,     // 永不过期
			RemainQuota:        500000, // 示例额度
			UnlimitedQuota:     true,
			ModelLimitsEnabled: false,
		}
		allowedGroupIDs := make([]int, 0)
		audienceGroupID := insertedUser.UserGroupId
		if audienceGroupID <= 0 {
			audienceGroupID = insertedUser.GroupId
		}
		for gid := range setting.GetUserUsableGroups(audienceGroupID) {
			allowedGroupIDs = append(allowedGroupIDs, gid)
		}
		allowedGroupIDs = model.NormalizeUniqueSortedIDs(allowedGroupIDs)
		if len(allowedGroupIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "",
			})
			return
		}
		allowedGroupIDsJSON, err := model.MarshalGroupIDsJSON(allowedGroupIDs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		token.AllowedGroupIds = allowedGroupIDsJSON
		if err := token.Insert(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "创建默认令牌失败",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func GetAllUsers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	users, total, err := model.GetAllUsers(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	// 为用户列表补充订阅日限信息，用于前端展示“每日额度”列。
	enriched := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		// 基础用户字段
		userBytes, _ := json.Marshal(u)
		item := map[string]interface{}{}
		_ = json.Unmarshal(userBytes, &item)

		// 订阅日限汇总：累加所有订阅的日限额度，若存在任一订阅不限日产生“每日不限额”标签。
		if breakdown, err := model.GetUserQuotaBreakdown(u.Id); err == nil && breakdown != nil {
			item["subscription_daily_limit"] = breakdown.SubscriptionDailyLimit
			item["subscription_daily_used"] = breakdown.SubscriptionDailyUsed
			item["subscription_daily_limit_unlimited"] = breakdown.SubscriptionDailyLimitUnlimited
		}
		enriched = append(enriched, item)
	}
	if err := attachUserGroupLabels(enriched); err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(enriched)

	common.ApiSuccess(c, pageInfo)
	return
}

func SearchUsers(c *gin.Context) {
	keyword := c.Query("keyword")
	groupID := 0
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			groupID = v
		} else {
			common.ApiErrorMsg(c, "group_id 无效（该字段表示默认模型分组）")
			return
		}
	}
	userGroupID := 0
	if raw := strings.TrimSpace(c.Query("user_group_id")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			userGroupID = v
		} else {
			common.ApiErrorMsg(c, "user_group_id 无效")
			return
		}
	}
	pageInfo := common.GetPageQuery(c)
	users, total, err := model.SearchUsers(keyword, groupID, userGroupID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	// 为搜索结果补充订阅日限信息，结构与 GetAllUsers 保持一致。
	enriched := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		userBytes, _ := json.Marshal(u)
		item := map[string]interface{}{}
		_ = json.Unmarshal(userBytes, &item)

		if breakdown, err := model.GetUserQuotaBreakdown(u.Id); err == nil && breakdown != nil {
			item["subscription_daily_limit"] = breakdown.SubscriptionDailyLimit
			item["subscription_daily_used"] = breakdown.SubscriptionDailyUsed
			item["subscription_daily_limit_unlimited"] = breakdown.SubscriptionDailyLimitUnlimited
		}
		enriched = append(enriched, item)
	}
	if err := attachUserGroupLabels(enriched); err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(enriched)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权获取同级或更高等级用户的信息",
		})
		return
	}

	userBytes, _ := json.Marshal(user)
	responseData := map[string]interface{}{}
	_ = json.Unmarshal(userBytes, &responseData)
	responseData["admin_permissions"] = user.GetAdminPermissions()
	if labelMap, err := model.UserGroupIDNameMap(nil, []int{user.UserGroupId}); err == nil {
		responseData["user_group_label"] = labelMap[user.UserGroupId]
	}
	if label, ok := model.GetPricingProfileLabel(user.PricingProfileId); ok {
		responseData["pricing_profile_label"] = label
	}
	if overrides, err := model.ListUserGroupPriceOverrides(nil, id); err == nil {
		responseData["group_price_overrides"] = overrides
	}
	responseData["resolved_group_pricing"] = model.BuildUserGroupPricingPreview(user.ToBaseUser())
	applyDiscreteUserResponseData(responseData, user)

	includes := getUserDetailIncludes(c)

	if includes.quotaBreakdown {
		if breakdown, err := model.GetUserQuotaBreakdown(id); err == nil {
			responseData["quota_breakdown"] = breakdown
		}
	}
	if includes.requestSubscriptionBreakdown {
		if breakdown, err := model.GetUserRequestSubscriptionBreakdown(id); err == nil {
			responseData["request_subscription_breakdown"] = breakdown
		}
	}
	if includes.tokenSubscriptionBreakdown {
		if breakdown, err := model.GetUserTokenSubscriptionBreakdown(id); err == nil {
			responseData["token_subscription_breakdown"] = breakdown
		}
	}
	// quota_breakdown / token_subscription_breakdown 会触发订阅刷新逻辑（到期清理/顺延生效），
	// 为避免“先读 user 再刷新”导致额度字段回显不一致，仅在请求这些明细时重新读取最新余额字段。
	if includes.quotaBreakdown || includes.tokenSubscriptionBreakdown {
		if refreshed, err := model.GetUserById(id, false); err == nil && refreshed != nil {
			responseData["quota"] = refreshed.Quota
			responseData["payg_quota"] = refreshed.PayAsYouGoQuota
			responseData["payg_history_quota"] = refreshed.PayAsYouGoHistoryQuota
			responseData["payg_allowed_groups"] = refreshed.PayAsYouGoAllowedGroups
			responseData["pay_token_allowed_groups"] = refreshed.PayTokenAllowedGroups
			responseData["redeem_quota"] = refreshed.RedeemQuota
			responseData["redeem_quota_expire_at"] = refreshed.RedeemQuotaExpireAt
			applyDiscreteUserResponseData(responseData, refreshed)
		}
	}
	if includes.paygBalances {
		if balances, err := model.GetUserPaygBalances(id, true); err != nil {
			common.SysLog("failed to build payg balances: " + err.Error())
		} else {
			type paygBalanceDTO struct {
				ProductId       int    `json:"product_id"`
				ProductName     string `json:"product_name"`
				SortOrder       int    `json:"sort_order"`
				RemainingQuota  int    `json:"remaining_quota"`
				AllowedGroupIds []int  `json:"allowed_group_ids"`
				CreatedAt       int64  `json:"created_at"`
				UpdatedAt       int64  `json:"updated_at"`
			}
			items := make([]paygBalanceDTO, 0, len(balances))
			for _, b := range balances {
				groupIDs, gErr := model.ResolvePaygBalanceAllowedGroupIDs(b)
				if gErr != nil {
					common.SysLog("failed to parse payg balance allowed_group_ids: " + gErr.Error())
					groupIDs = nil
				}
				items = append(items, paygBalanceDTO{
					ProductId:       b.ProductId,
					ProductName:     b.ProductName,
					SortOrder:       b.SortOrder,
					RemainingQuota:  b.RemainingQuota,
					AllowedGroupIds: groupIDs,
					CreatedAt:       b.CreatedAt,
					UpdatedAt:       b.UpdatedAt,
				})
			}
			responseData["payg_balances"] = items
		}
	}
	if includes.payRequestBalances {
		if balances, err := model.GetUserPayRequestBalances(id, true); err != nil {
			common.SysLog("failed to build pay_request balances: " + err.Error())
		} else {
			type payRequestBalanceDTO struct {
				ProductId         int     `json:"product_id"`
				ProductName       string  `json:"product_name"`
				SortOrder         int     `json:"sort_order"`
				RemainingRequests float64 `json:"remaining_requests"`
				AllowedGroupIds   []int   `json:"allowed_group_ids"`
				CreatedAt         int64   `json:"created_at"`
				UpdatedAt         int64   `json:"updated_at"`
			}
			items := make([]payRequestBalanceDTO, 0, len(balances))
			total := 0
			for _, b := range balances {
				total += b.RemainingRequests
				groupIDs, gErr := model.ResolvePayRequestBalanceAllowedGroupIDs(b)
				if gErr != nil {
					common.SysLog("failed to parse pay_request balance allowed_group_ids: " + gErr.Error())
					groupIDs = nil
				}
				items = append(items, payRequestBalanceDTO{
					ProductId:         b.ProductId,
					ProductName:       b.ProductName,
					SortOrder:         b.SortOrder,
					RemainingRequests: displayDiscreteUnits(b.RemainingRequests),
					AllowedGroupIds:   groupIDs,
					CreatedAt:         b.CreatedAt,
					UpdatedAt:         b.UpdatedAt,
				})
			}
			responseData["pay_request_balances"] = items
			responseData["pay_request_quota"] = displayDiscreteUnits(total)
			if unionGroupsJSON, err := model.UnionPayRequestAllowedGroupsFromBalances(balances); err != nil {
				common.SysLog("failed to build pay_request allowed_groups: " + err.Error())
			} else {
				responseData["pay_request_allowed_groups"] = unionGroupsJSON
			}
		}
	}
	if includes.payTokenBalances {
		if balances, err := model.GetUserPayTokenBalances(id, true); err != nil {
			common.SysLog("failed to build pay_token balances: " + err.Error())
		} else {
			type payTokenBalanceDTO struct {
				ProductId       int     `json:"product_id"`
				ProductName     string  `json:"product_name"`
				SortOrder       int     `json:"sort_order"`
				RemainingTokens float64 `json:"remaining_tokens"`
				AllowedGroupIds []int   `json:"allowed_group_ids"`
				CreatedAt       int64   `json:"created_at"`
				UpdatedAt       int64   `json:"updated_at"`
			}
			items := make([]payTokenBalanceDTO, 0, len(balances))
			total := 0
			for _, b := range balances {
				total += b.RemainingTokens
				groupIDs, gErr := model.ResolvePayTokenBalanceAllowedGroupIDs(b)
				if gErr != nil {
					common.SysLog("failed to parse pay_token balance allowed_group_ids: " + gErr.Error())
					groupIDs = nil
				}
				items = append(items, payTokenBalanceDTO{
					ProductId:       b.ProductId,
					ProductName:     b.ProductName,
					SortOrder:       b.SortOrder,
					RemainingTokens: displayDiscreteUnits(b.RemainingTokens),
					AllowedGroupIds: groupIDs,
					CreatedAt:       b.CreatedAt,
					UpdatedAt:       b.UpdatedAt,
				})
			}
			responseData["pay_token_balances"] = items
			responseData["pay_token_quota"] = displayDiscreteUnits(total)
			if unionGroupsJSON, err := model.UnionPayTokenAllowedGroupsFromBalances(balances); err != nil {
				common.SysLog("failed to build pay_token allowed_groups: " + err.Error())
			} else {
				responseData["pay_token_allowed_groups"] = unionGroupsJSON
			}
		}
	}
	stripRetiredXiaotuanFields(responseData)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responseData,
	})
	return
}

type userDetailIncludes struct {
	quotaBreakdown               bool
	requestSubscriptionBreakdown bool
	tokenSubscriptionBreakdown   bool
	paygBalances                 bool
	payRequestBalances           bool
	payTokenBalances             bool
}

func parseOptionalQueryBool(c *gin.Context, key string) (bool, bool) {
	raw, ok := c.GetQuery(key)
	if !ok {
		return false, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true, true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return true, false
	}
	return true, value
}

func getUserDetailIncludes(c *gin.Context) userDetailIncludes {
	defaults := userDetailIncludes{
		quotaBreakdown:               true,
		requestSubscriptionBreakdown: false,
		tokenSubscriptionBreakdown:   true,
		paygBalances:                 true,
		payRequestBalances:           true,
		payTokenBalances:             true,
	}
	scoped := userDetailIncludes{}
	hasScopedIncludes := false

	apply := func(key string, target *bool) {
		present, value := parseOptionalQueryBool(c, key)
		if !present {
			return
		}
		hasScopedIncludes = true
		*target = value
	}

	apply("include_quota_breakdown", &scoped.quotaBreakdown)
	apply("include_request_subscription_breakdown", &scoped.requestSubscriptionBreakdown)
	apply("include_token_subscription_breakdown", &scoped.tokenSubscriptionBreakdown)
	apply("include_payg_balances", &scoped.paygBalances)
	apply("include_pay_request_balances", &scoped.payRequestBalances)
	apply("include_pay_token_balances", &scoped.payTokenBalances)

	if hasScopedIncludes {
		return scoped
	}
	return defaults
}

func GenerateAccessToken(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// get rand int 28-32
	randI := common.GetRandomInt(4)
	key, err := common.GenerateRandomKey(29 + randI)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成失败",
		})
		common.SysLog("failed to generate key: " + err.Error())
		return
	}
	user.SetAccessToken(key)

	if model.DB.Where("access_token = ?", user.AccessToken).First(user).RowsAffected != 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请重试，系统生成的 UUID 竟然重复了！",
		})
		return
	}

	if err := user.Update(false); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AccessToken,
	})
	return
}

type TransferAffQuotaRequest struct {
	AmountFen int64 `json:"amount_fen" binding:"required"`
}

func TransferAffQuota(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tran := TransferAffQuotaRequest{}
	if err := c.ShouldBindJSON(&tran); err != nil {
		common.ApiError(c, err)
		return
	}
	err = user.TransferAffQuotaToBalance(tran.AmountFen)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "划转失败 " + err.Error(),
		})
		return
	}
	_ = model.InvalidateUserCache(id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "划转成功",
	})
}

func GetAffCode(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.AffCode == "" {
		user.AffCode = common.GetRandomString(4)
		if err := user.Update(false); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AffCode,
	})
	return
}

func GetSelf(c *gin.Context) {
	id := c.GetInt("id")
	userRole := c.GetInt("role")
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := user.EnsureAvatarSeed(); err != nil {
		common.ApiError(c, err)
		return
	}
	// Hide admin remarks: set to empty to trigger omitempty tag, ensuring the remark field is not included in JSON returned to regular users
	user.Remark = ""
	adminPermissions := user.GetAdminPermissions()

	// 计算用户权限信息
	permissions := calculateUserPermissions(userRole, adminPermissions)

	// 获取用户设置并提取sidebar_modules
	userSetting := user.GetSetting()

	// 构建响应数据，包含用户信息和权限
	responseData := map[string]interface{}{
		"id":                        user.Id,
		"username":                  user.Username,
		"display_name":              user.DisplayName,
		"avatar_seed":               user.AvatarSeed,
		"created_at":                user.CreatedAt,
		"role":                      user.Role,
		"status":                    user.Status,
		"email":                     user.Email,
		"group_id":                  user.GroupId,
		"quota":                     user.Quota,
		"tokens_quota":              displayDiscreteUnits(user.TokensQuota),
		"tokens_history_quota":      displayDiscreteUnits(user.TokensHistoryQuota),
		"payg_quota":                user.PayAsYouGoQuota,
		"payg_history_quota":        user.PayAsYouGoHistoryQuota,
		"payg_allowed_groups":       user.PayAsYouGoAllowedGroups,
		"pay_request_quota":         displayDiscreteUnits(user.PayRequestQuota),
		"pay_request_history_quota": displayDiscreteUnits(user.PayRequestHistoryQuota),
		"pay_token_quota":           displayDiscreteUnits(user.PayTokenQuota),
		"pay_token_history_quota":   displayDiscreteUnits(user.PayTokenHistoryQuota),
		"pay_token_allowed_groups":  user.PayTokenAllowedGroups,
		"balance_fen":               user.BalanceFen,
		"daily_quota_limit":         user.DailyQuotaLimit,
		"daily_quota_used":          user.DailyQuotaUsed,
		"daily_quota_reset_date":    user.DailyQuotaResetDate,
		"used_quota":                user.UsedQuota,
		"visible_used_quota":        user.VisibleUsedQuota,
		"cost_used_quota":           user.CostUsedQuota,
		"request_count":             user.RequestCount,
		"customer_type":             user.CustomerType,
		"pricing_profile_id":        user.PricingProfileId,
		"aff_code":                  user.AffCode,
		"aff_count":                 user.AffCount,
		"aff_quota":                 user.AffQuota,
		"aff_history_quota":         user.AffHistoryQuota,
		"inviter_id":                user.InviterId,
		"linux_do_id":               user.LinuxDOId,
		"setting":                   user.Setting,
		"stripe_customer":           user.StripeCustomer,
		"sidebar_modules":           userSetting.SidebarModules, // 正确提取sidebar_modules字段
		"permissions":               permissions,                // 新增权限字段
		"admin_permissions":         adminPermissions,
		"redeem_quota_expire_at":    user.RedeemQuotaExpireAt,
		"redeem_quota":              user.RedeemQuota,
	}
	applyPublicUsageResponseData(responseData, user)
	applyQuotaBalanceResponseData(responseData, user)
	applyLegacyClawBoxTotalQuotaCompat(responseData, user)
	if label, ok := model.GetPricingProfileLabel(user.PricingProfileId); ok {
		responseData["pricing_profile_label"] = label
	}

	if breakdown, err := model.GetUserQuotaBreakdown(id); err != nil {
		common.SysLog("failed to build quota breakdown: " + err.Error())
	} else {
		responseData["quota_breakdown"] = breakdown
	}
	if breakdown, err := model.GetUserRequestSubscriptionBreakdown(id); err != nil {
		common.SysLog("failed to build request subscription breakdown: " + err.Error())
	} else {
		responseData["request_subscription_breakdown"] = breakdown
	}
	if breakdown, err := model.GetUserTokenSubscriptionBreakdown(id); err != nil {
		common.SysLog("failed to build token subscription breakdown: " + err.Error())
	} else {
		responseData["token_subscription_breakdown"] = breakdown
	}
	// quota_breakdown / token_subscription_breakdown 会触发订阅刷新逻辑（到期清理/顺延生效），
	// 为避免“先读 user 再刷新”导致额度字段回显不一致，这里重新读取一次用户最新余额字段。
	if refreshed, err := model.GetUserById(id, false); err == nil && refreshed != nil {
		responseData["pay_token_allowed_groups"] = refreshed.PayTokenAllowedGroups
		applyDiscreteUserResponseData(responseData, refreshed)
		applyPublicUsageResponseData(responseData, refreshed)
		applyQuotaBalanceResponseData(responseData, refreshed)
		applyLegacyClawBoxTotalQuotaCompat(responseData, refreshed)
	}
	if balances, err := model.GetUserPaygBalances(id, true); err != nil {
		common.SysLog("failed to build payg balances: " + err.Error())
	} else {
		responseData["payg_balances"] = buildUserPaygBalances(balances)
	}
	if balances, err := model.GetUserPayRequestBalances(id, true); err != nil {
		common.SysLog("failed to build pay_request balances: " + err.Error())
	} else {
		type payRequestBalanceDTO struct {
			ProductId         int     `json:"product_id"`
			ProductName       string  `json:"product_name"`
			SortOrder         int     `json:"sort_order"`
			RemainingRequests float64 `json:"remaining_requests"`
			AllowedGroupIds   []int   `json:"allowed_group_ids"`
			CreatedAt         int64   `json:"created_at"`
			UpdatedAt         int64   `json:"updated_at"`
		}
		items := make([]payRequestBalanceDTO, 0, len(balances))
		total := 0
		for _, b := range balances {
			total += b.RemainingRequests
			groupIDs, gErr := model.ResolvePayRequestBalanceAllowedGroupIDs(b)
			if gErr != nil {
				common.SysLog("failed to parse pay_request balance allowed_group_ids: " + gErr.Error())
				groupIDs = nil
			}
			items = append(items, payRequestBalanceDTO{
				ProductId:         b.ProductId,
				ProductName:       b.ProductName,
				SortOrder:         b.SortOrder,
				RemainingRequests: displayDiscreteUnits(b.RemainingRequests),
				AllowedGroupIds:   groupIDs,
				CreatedAt:         b.CreatedAt,
				UpdatedAt:         b.UpdatedAt,
			})
		}
		responseData["pay_request_balances"] = items
		responseData["pay_request_quota"] = displayDiscreteUnits(total)
		if unionGroupsJSON, err := model.UnionPayRequestAllowedGroupsFromBalances(balances); err != nil {
			common.SysLog("failed to build pay_request allowed_groups: " + err.Error())
		} else {
			responseData["pay_request_allowed_groups"] = unionGroupsJSON
		}
	}
	if balances, err := model.GetUserPayTokenBalances(id, true); err != nil {
		common.SysLog("failed to build pay_token balances: " + err.Error())
	} else {
		type payTokenBalanceDTO struct {
			ProductId       int     `json:"product_id"`
			ProductName     string  `json:"product_name"`
			SortOrder       int     `json:"sort_order"`
			RemainingTokens float64 `json:"remaining_tokens"`
			AllowedGroupIds []int   `json:"allowed_group_ids"`
			CreatedAt       int64   `json:"created_at"`
			UpdatedAt       int64   `json:"updated_at"`
		}
		items := make([]payTokenBalanceDTO, 0, len(balances))
		total := 0
		for _, b := range balances {
			total += b.RemainingTokens
			groupIDs, gErr := model.ResolvePayTokenBalanceAllowedGroupIDs(b)
			if gErr != nil {
				common.SysLog("failed to parse pay_token balance allowed_group_ids: " + gErr.Error())
				groupIDs = nil
			}
			items = append(items, payTokenBalanceDTO{
				ProductId:       b.ProductId,
				ProductName:     b.ProductName,
				SortOrder:       b.SortOrder,
				RemainingTokens: displayDiscreteUnits(b.RemainingTokens),
				AllowedGroupIds: groupIDs,
				CreatedAt:       b.CreatedAt,
				UpdatedAt:       b.UpdatedAt,
			})
		}
		responseData["pay_token_balances"] = items
		responseData["pay_token_quota"] = displayDiscreteUnits(total)
		if unionGroupsJSON, err := model.UnionPayTokenAllowedGroupsFromBalances(balances); err != nil {
			common.SysLog("failed to build pay_token allowed_groups: " + err.Error())
		} else {
			responseData["pay_token_allowed_groups"] = unionGroupsJSON
		}
	}
	stripRetiredXiaotuanFields(responseData)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responseData,
	})
	return
}

func UpdateAvatar(c *gin.Context) {
	userId := c.GetInt("id")
	seed := common.GetRandomString(16)
	result := model.DB.Model(&model.User{}).Where("id = ?", userId).Update("avatar_seed", seed)
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"avatar_seed": seed,
		},
	})
}

// 计算用户权限的辅助函数
func calculateUserPermissions(userRole int, adminPermissions model.AdminPermissions) map[string]interface{} {
	permissions := map[string]interface{}{}
	sidebarModules := map[string]interface{}{}

	// 根据用户角色计算权限
	if userRole == common.RoleRootUser {
		// 超级管理员不需要边栏设置功能
		permissions["sidebar_settings"] = false
	} else if userRole == common.RoleAdminUser {
		// 管理员可以设置边栏，但不包含系统设置功能
		permissions["sidebar_settings"] = true
		sidebarModules["admin"] = map[string]interface{}{
			"product_management": adminPermissions.ProductManagement,
			"order":              adminPermissions.Order,
			"setting":            false, // 管理员不能访问系统设置
		}
	} else {
		// 普通用户只能设置个人功能，不包含管理员区域
		permissions["sidebar_settings"] = true
		sidebarModules["admin"] = false // 普通用户不能访问管理员区域
	}

	permissions["sidebar_modules"] = sidebarModules
	return permissions
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfig(userRole int) string {
	defaultConfig := map[string]interface{}{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]interface{}{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]interface{}{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"stomp_king": true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]interface{}{
		"enabled":  true,
		"topup":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":            true,
			"channel":            true,
			"models":             true,
			"redemption":         true,
			"user":               true,
			"product_management": false,
			"order":              false,
			"setting":            false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":            true,
			"channel":            true,
			"models":             true,
			"redemption":         true,
			"user":               true,
			"product_management": true,
			"order":              true,
			"setting":            true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := json.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

func anyToPositiveInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		id, _ := strconv.Atoi(strings.TrimSpace(v))
		if id > 0 {
			return id
		}
	}
	return 0
}

func attachUserGroupLabels(items []map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}
	userGroupIDs := make([]int, 0, len(items))
	for _, item := range items {
		groupID := anyToPositiveInt(item["user_group_id"])
		if groupID <= 0 {
			continue
		}
		userGroupIDs = append(userGroupIDs, groupID)
	}
	labelByID, err := model.UserGroupIDNameMap(nil, userGroupIDs)
	if err != nil {
		return err
	}
	for _, item := range items {
		groupID := anyToPositiveInt(item["user_group_id"])
		if groupID <= 0 {
			item["user_group_label"] = ""
			continue
		}
		item["user_group_label"] = labelByID[groupID]
	}
	return nil
}

func GetUserModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		id = c.GetInt("id")
	}
	user, err := model.GetUserCache(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userRole := c.GetInt("role")
	isAdmin := userRole >= common.RoleAdminUser

	groupIDSet := make(map[int]struct{}, 16)
	audienceGroupID := user.UserGroupId
	if audienceGroupID <= 0 {
		audienceGroupID = user.GroupId
	}
	for groupID := range setting.GetUserUsableGroups(audienceGroupID) {
		if groupID <= 0 {
			continue
		}
		groupIDSet[groupID] = struct{}{}
	}

	// For common users, also include models enabled under groups that the user can currently bill from
	// (e.g. subscription groups that are not globally user_selectable).
	if !isAdmin {
		if owned, err := model.GetUserBillableGroupIDs(id); err != nil {
			common.ApiError(c, err)
			return
		} else {
			for _, gid := range owned {
				if gid <= 0 {
					continue
				}
				groupIDSet[gid] = struct{}{}
			}
		}
	}

	models := make([]string, 0)
	for groupID := range groupIDSet {
		for _, g := range model.GetGroupEnabledModels(groupID) {
			if !common.StringsContains(models, g) {
				models = append(models, g)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    models,
	})
	return
}

func resolveUpdateUserLegacyGroupID(originGroupID int, requestedGroupID int, hasGroupID bool) (int, bool, error) {
	if hasGroupID && requestedGroupID < 0 {
		return 0, false, fmt.Errorf("group_id 无效")
	}

	// group_id is a legacy default model-group fallback. The current user edit
	// form does not expose it, and older form state may still submit 0.
	if hasGroupID && requestedGroupID > 0 {
		return requestedGroupID, true, nil
	}
	if originGroupID > 0 {
		return originGroupID, false, nil
	}
	groupID, err := model.ResolveLegacyDefaultModelGroupID(nil)
	if err != nil {
		return 0, false, err
	}
	if groupID <= 0 {
		return 0, false, fmt.Errorf("group_id 无效")
	}
	return groupID, false, nil
}

func UpdateUser(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	var updatedUser model.User
	if err := json.Unmarshal(body, &updatedUser); err != nil || updatedUser.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	requestFields := map[string]json.RawMessage{}
	_ = json.Unmarshal(body, &requestFields)
	_, hasPaygQuota := requestFields["payg_quota"]
	_, hasPaygAllowedGroups := requestFields["payg_allowed_groups"]
	_, hasGroupID := requestFields["group_id"]
	_, hasAdminPermissions := requestFields["admin_permissions"]
	_, hasInviterID := requestFields["inviter_id"]
	_, hasLegacyGroup := requestFields["group"]
	_, hasCustomerType := requestFields["customer_type"]
	_, hasPricingProfileID := requestFields["pricing_profile_id"]
	_, hasGroupPriceOverrides := requestFields["group_price_overrides"]
	_, hasUserGroupID := requestFields["user_group_id"]
	if hasLegacyGroup && !hasGroupID {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "group 已废弃，请使用 group_id",
		})
		return
	}

	if updatedUser.Password == "" {
		updatedUser.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if updatedUser.RedeemQuotaExpireAt < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "限时额度到期时间不能小于0",
		})
		return
	}
	if updatedUser.DailyQuotaLimit < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "每日额度必须大于等于0",
		})
		return
	}
	originUser, err := model.GetUserById(updatedUser.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	groupID, explicitGroupID, err := resolveUpdateUserLegacyGroupID(originUser.GroupId, updatedUser.GroupId, hasGroupID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	updatedUser.GroupId = groupID
	if explicitGroupID {
		if err := model.ValidateGroupIDsExist(nil, []int{updatedUser.GroupId}); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		if !setting.GroupInEnabledGroups(updatedUser.GroupId) {
			label, ok := model.GetGroupLabelByID(updatedUser.GroupId)
			if !ok {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "分组不存在",
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("分组 %s 已被禁用", label),
			})
			return
		}
	}

	if !hasPaygAllowedGroups {
		updatedUser.PayAsYouGoAllowedGroups = originUser.PayAsYouGoAllowedGroups
	} else {
		ids, err := model.ParseGroupIDsJSON(updatedUser.PayAsYouGoAllowedGroups)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		if err := model.ValidateGroupIDsExist(nil, ids); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		for _, gid := range ids {
			if gid <= 0 {
				continue
			}
			if !setting.GroupInEnabledGroups(gid) {
				label, ok := model.GetGroupLabelByID(gid)
				if !ok {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": "分组不存在",
					})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": fmt.Sprintf("分组 %s 已被禁用", label),
				})
				return
			}
		}
		normalized, err := model.MarshalGroupIDsJSON(ids)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		updatedUser.PayAsYouGoAllowedGroups = normalized
	}

	if !hasAdminPermissions {
		updatedUser.AdminPermissions = originUser.AdminPermissions
	} else {
		if c.GetInt("role") != common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "仅超级管理员可调整管理员模块权限",
			})
			return
		}
		perms, err := model.ParseAdminPermissionsJSON(updatedUser.AdminPermissions)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员模块权限格式无效",
			})
			return
		}
		if originUser.Role != common.RoleAdminUser && perms.HasAny() {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "仅支持为普通管理员设置管理员模块权限",
			})
			return
		}
		normalized, err := model.MarshalAdminPermissionsJSON(perms)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		updatedUser.AdminPermissions = normalized
	}
	if !hasInviterID {
		updatedUser.InviterId = originUser.InviterId
	} else if updatedUser.InviterId < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "邀请人 ID 无效",
		})
		return
	}
	if !hasCustomerType {
		updatedUser.CustomerType = originUser.CustomerType
	}
	if !hasPricingProfileID {
		updatedUser.PricingProfileId = originUser.PricingProfileId
	}
	if !hasUserGroupID {
		updatedUser.UserGroupId = originUser.UserGroupId
	} else if updatedUser.UserGroupId > 0 {
		if _, err := model.GetUserGroupByID(nil, updatedUser.UserGroupId); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "user_group_id 无效",
			})
			return
		}
	}
	if !hasGroupPriceOverrides {
		existingOverrides, err := model.ListUserGroupPriceOverrides(nil, originUser.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		groupPriceOverridesJSON, err := model.MarshalPriceGroupFactorsJSON(existingOverrides)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		updatedUser.GroupPriceOverrides = groupPriceOverridesJSON
	}

	// payg_quota 需要显式传入才允许修改；否则保持原值，避免老客户端误清空
	if !hasPaygQuota {
		updatedUser.PayAsYouGoQuota = originUser.PayAsYouGoQuota
		updatedUser.PayAsYouGoHistoryQuota = originUser.PayAsYouGoHistoryQuota
	} else {
		if updatedUser.PayAsYouGoQuota < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "按量付费余额不能小于0",
			})
			return
		}
		if updatedUser.Quota < updatedUser.PayAsYouGoQuota {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "用户额度不能小于按量付费余额",
			})
			return
		}
		updatedUser.PayAsYouGoHistoryQuota = originUser.PayAsYouGoHistoryQuota
		if updatedUser.PayAsYouGoHistoryQuota < updatedUser.PayAsYouGoQuota {
			updatedUser.PayAsYouGoHistoryQuota = updatedUser.PayAsYouGoQuota
		}

		// 管理员直接设置 payg_quota 时，不再隐式填充分组；必须显式配置 payg_allowed_groups（或先通过按量商品充值）。
		if updatedUser.PayAsYouGoQuota > 0 {
			existingGroups, err := model.ParseGroupIDsJSON(updatedUser.PayAsYouGoAllowedGroups)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			if len(existingGroups) == 0 {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "按量付费可用分组为空，无法设置按量付费余额，请先为用户配置 payg_allowed_groups 或通过按量商品充值",
				})
				return
			}
			if err := model.ValidateGroupIDsExist(nil, existingGroups); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			for _, gid := range existingGroups {
				if gid <= 0 {
					continue
				}
				if !setting.GroupInEnabledGroups(gid) {
					label, ok := model.GetGroupLabelByID(gid)
					if !ok {
						c.JSON(http.StatusOK, gin.H{
							"success": false,
							"message": "分组不存在",
						})
						return
					}
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": fmt.Sprintf("分组 %s 已被禁用", label),
					})
					return
				}
			}
			normalizedGroupsJSON, err := model.MarshalGroupIDsJSON(existingGroups)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			updatedUser.PayAsYouGoAllowedGroups = normalizedGroupsJSON
		}
	}

	if updatedUser.BaseMultiplier == 0 {
		updatedUser.BaseMultiplier = originUser.BaseMultiplier
	}
	if updatedUser.BaseMultiplier <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "基础倍率必须大于0",
		})
		return
	}
	if err := common.Validate.Struct(&updatedUser); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法 " + common.ValidationErrorMessage(err),
		})
		return
	}

	myRole := c.GetInt("role")
	if myRole <= originUser.Role && myRole != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权更新同权限等级或更高权限等级的用户信息",
		})
		return
	}
	if myRole <= updatedUser.Role && myRole != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权将其他用户权限等级提升到大于等于自己的权限等级",
		})
		return
	}
	if updatedUser.Password == "$I_LOVE_U" {
		updatedUser.Password = "" // rollback to what it should be
	}
	updatePassword := updatedUser.Password != ""
	if err := updatedUser.Edit(updatePassword); err != nil {
		common.ApiError(c, err)
		return
	}
	if originUser.RedeemQuotaExpireAt != updatedUser.RedeemQuotaExpireAt {
		from := originUser.RedeemQuotaExpireAt
		to := updatedUser.RedeemQuotaExpireAt
		model.RecordLog(originUser.Id, model.LogTypeManage, fmt.Sprintf("管理员将用户限时额度到期从 %d 修改为 %d", from, to))
	}
	if originUser.Quota != updatedUser.Quota {
		model.RecordLog(originUser.Id, model.LogTypeManage, fmt.Sprintf("管理员将用户额度从 %s 修改为 %s", logger.LogQuota(originUser.Quota), logger.LogQuota(updatedUser.Quota)))
	}
	if originUser.PayAsYouGoQuota != updatedUser.PayAsYouGoQuota {
		model.RecordLog(originUser.Id, model.LogTypeManage, fmt.Sprintf("管理员将用户按量付费余额从 %s 修改为 %s", logger.LogQuota(originUser.PayAsYouGoQuota), logger.LogQuota(updatedUser.PayAsYouGoQuota)))
	}
	if originUser.DailyQuotaLimit != updatedUser.DailyQuotaLimit {
		fromQuota := "无限"
		toQuota := "无限"
		if originUser.DailyQuotaLimit > 0 {
			fromQuota = logger.LogQuota(originUser.DailyQuotaLimit)
		}
		if updatedUser.DailyQuotaLimit > 0 {
			toQuota = logger.LogQuota(updatedUser.DailyQuotaLimit)
		}
		model.RecordLog(originUser.Id, model.LogTypeManage, fmt.Sprintf("管理员将用户每日额度从 %s 修改为 %s", fromQuota, toQuota))
	}
	if originUser.InviterId != updatedUser.InviterId {
		fromInviter := "无邀请人"
		toInviter := "无邀请人"
		if originUser.InviterId > 0 {
			fromInviter = strconv.Itoa(originUser.InviterId)
		}
		if updatedUser.InviterId > 0 {
			toInviter = strconv.Itoa(updatedUser.InviterId)
		}
		model.RecordLog(originUser.Id, model.LogTypeManage, fmt.Sprintf("管理员将用户邀请人从 %s 修改为 %s", fromInviter, toInviter))
	}
	if originUser.CustomerType != updatedUser.CustomerType || originUser.PricingProfileId != updatedUser.PricingProfileId {
		fromProfile := "-"
		toProfile := "-"
		if label, ok := model.GetPricingProfileLabel(originUser.PricingProfileId); ok {
			fromProfile = label
		}
		if label, ok := model.GetPricingProfileLabel(updatedUser.PricingProfileId); ok {
			toProfile = label
		}
		model.RecordLog(originUser.Id, model.LogTypeManage, fmt.Sprintf("管理员将用户价格规则从 %s/%s 修改为 %s/%s", originUser.CustomerType, fromProfile, updatedUser.CustomerType, toProfile))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateSelf(c *gin.Context) {
	var requestData map[string]interface{}
	err := json.NewDecoder(c.Request.Body).Decode(&requestData)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	// 检查是否是sidebar_modules更新请求
	if sidebarModules, exists := requestData["sidebar_modules"]; exists {
		userId := c.GetInt("id")
		user, err := model.GetUserById(userId, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}

		// 获取当前用户设置
		currentSetting := user.GetSetting()

		// 更新sidebar_modules字段
		if sidebarModulesStr, ok := sidebarModules.(string); ok {
			currentSetting.SidebarModules = sidebarModulesStr
		}

		// 保存更新后的设置
		user.SetSetting(currentSetting)
		if err := user.Update(false); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "更新设置失败: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "设置更新成功",
		})
		return
	}

	// 原有的用户信息更新逻辑
	var user model.User
	requestDataBytes, err := json.Marshal(requestData)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	err = json.Unmarshal(requestDataBytes, &user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	if user.Password == "" {
		user.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if err := common.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法 " + common.ValidationErrorMessage(err),
		})
		return
	}

	cleanUser := model.User{
		Id:          c.GetInt("id"),
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if user.Password == "$I_LOVE_U" {
		user.Password = "" // rollback to what it should be
		cleanUser.Password = ""
	}
	updatePassword, err := checkUpdatePassword(user.OriginalPassword, user.Password, cleanUser.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := cleanUser.Update(updatePassword); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func checkUpdatePassword(originalPassword string, newPassword string, userId int) (updatePassword bool, err error) {
	var currentUser *model.User
	currentUser, err = model.GetUserById(userId, true)
	if err != nil {
		return
	}
	if !common.ValidatePasswordAndHash(originalPassword, currentUser.Password) {
		err = fmt.Errorf("原密码错误")
		return
	}
	if newPassword == "" {
		return
	}
	updatePassword = true
	return
}

func DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	originUser, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权删除同权限等级或更高权限等级的用户",
		})
		return
	}
	err = model.HardDeleteUserById(id)
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

func DeleteSelf(c *gin.Context) {
	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)

	if user.Role == common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不能删除超级管理员账户",
		})
		return
	}

	err := model.DeleteUserById(id)
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

func CreateUser(c *gin.Context) {
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	user.Username = strings.TrimSpace(user.Username)
	if err != nil || user.Username == "" || user.Password == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if user.BaseMultiplier == 0 {
		user.BaseMultiplier = 1
	}
	if err := common.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法 " + common.ValidationErrorMessage(err),
		})
		return
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Username
	}
	myRole := c.GetInt("role")
	if user.Role >= myRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法创建权限大于等于自己的用户",
		})
		return
	}
	// Even for admin users, we cannot fully trust them!
	cleanUser := model.User{
		Username:            user.Username,
		Password:            user.Password,
		DisplayName:         user.DisplayName,
		Role:                user.Role, // 保持管理员设置的角色
		BaseMultiplier:      user.BaseMultiplier,
		CustomerType:        user.CustomerType,
		PricingProfileId:    user.PricingProfileId,
		UserGroupId:         user.UserGroupId,
		GroupPriceOverrides: user.GroupPriceOverrides,
		Remark:              user.Remark,
		RegisterIP:          c.ClientIP(),
	}
	if err := cleanUser.Insert(0); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

type ManageRequest struct {
	Id     int    `json:"id"`
	Action string `json:"action"`
}

// ManageUser Only admin user can do this
func ManageUser(c *gin.Context) {
	var req ManageRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	user := model.User{
		Id: req.Id,
	}
	// Fill attributes
	model.DB.Unscoped().Where(&user).First(&user)
	if user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权更新同权限等级或更高权限等级的用户信息",
		})
		return
	}
	switch req.Action {
	case "disable":
		user.Status = common.UserStatusDisabled
		if user.Role == common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法禁用超级管理员用户",
			})
			return
		}
	case "enable":
		user.Status = common.UserStatusEnabled
	case "delete":
		if user.Role == common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法删除超级管理员用户",
			})
			return
		}
		if err := user.Delete(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "promote":
		if myRole != common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "普通管理员用户无法提升其他用户为管理员",
			})
			return
		}
		if user.Role >= common.RoleAdminUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该用户已经是管理员",
			})
			return
		}
		user.Role = common.RoleAdminUser
		user.AdminPermissions = model.EmptyAdminPermissionsJSON()
	case "demote":
		if user.Role == common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法降级超级管理员用户",
			})
			return
		}
		if user.Role == common.RoleCommonUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该用户已经是普通用户",
			})
			return
		}
		user.Role = common.RoleCommonUser
		user.AdminPermissions = model.EmptyAdminPermissionsJSON()
	}

	if err := user.Update(false); err != nil {
		common.ApiError(c, err)
		return
	}
	clearUser := model.User{
		Role:   user.Role,
		Status: user.Status,
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    clearUser,
	})
	return
}

func EmailBind(c *gin.Context) {
	if !personal_setting.IsBindingVisible(personal_setting.BindingEmail) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员已禁用该绑定方式",
		})
		return
	}

	email := c.Query("email")
	code := c.Query("code")
	if !common.VerifyCodeWithKey(email, code, common.EmailVerificationPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "验证码错误或已过期",
		})
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	user := model.User{
		Id: id.(int),
	}
	err := user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user.Email = email
	// no need to check if this email already taken, because we have used verification code to check it
	err = user.Update(false)
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

type topUpRequest struct {
	Key string `json:"key"`
	// 当用户存在不限时额度且兑换的是“限时额度”时，需要显式确认后才继续兑换
	ConfirmOverride bool `json:"confirm_override"`
	// stack / defer（仅对订阅类兑换码生效）
	ApplyMode string `json:"apply_mode"`
}

var topUpLocks sync.Map
var topUpCreateLock sync.Mutex

type topUpTryLock struct {
	ch chan struct{}
}

func newTopUpTryLock() *topUpTryLock {
	return &topUpTryLock{ch: make(chan struct{}, 1)}
}

func (l *topUpTryLock) TryLock() bool {
	select {
	case l.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *topUpTryLock) Unlock() {
	select {
	case <-l.ch:
	default:
	}
}

func getTopUpLock(userID int) *topUpTryLock {
	if v, ok := topUpLocks.Load(userID); ok {
		return v.(*topUpTryLock)
	}
	topUpCreateLock.Lock()
	defer topUpCreateLock.Unlock()
	if v, ok := topUpLocks.Load(userID); ok {
		return v.(*topUpTryLock)
	}
	l := newTopUpTryLock()
	topUpLocks.Store(userID, l)
	return l
}

func TopUp(c *gin.Context) {
	id := c.GetInt("id")
	userRole := c.GetInt("role")
	lock := getTopUpLock(id)
	if !lock.TryLock() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "充值处理中，请稍后重试",
		})
		return
	}
	defer lock.Unlock()
	req := topUpRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ApplyMode != "" &&
		req.ApplyMode != model.SubscriptionApplyModeStack &&
		req.ApplyMode != model.SubscriptionApplyModeDefer {
		common.ApiErrorMsg(c, "apply_mode 无效")
		return
	}
	redeem, err := model.RedeemDetail(req.Key, id, req.ApplyMode)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	addedQuota := 0
	if redeem != nil {
		addedQuota = redeem.AddedQuota
	}

	responseData := map[string]interface{}{}
	if user, userErr := model.GetUserById(id, false); userErr == nil {
		user.Remark = ""
		userBytes, _ := json.Marshal(user)
		_ = json.Unmarshal(userBytes, &responseData)
		permissions := calculateUserPermissions(userRole, user.GetAdminPermissions())
		userSetting := user.GetSetting()
		responseData["sidebar_modules"] = userSetting.SidebarModules
		responseData["permissions"] = permissions
		responseData["admin_permissions"] = user.GetAdminPermissions()
		applyDiscreteUserResponseData(responseData, user)
		applyPublicUsageResponseData(responseData, user)
		applyQuotaBalanceResponseData(responseData, user)
		applyLegacyClawBoxTotalQuotaCompat(responseData, user)
		if breakdown, err := model.GetUserQuotaBreakdown(id); err == nil {
			responseData["quota_breakdown"] = breakdown
		}
		// quota_breakdown 会触发订阅刷新逻辑（到期清理/顺延生效），
		// 为避免“先读 user 再刷新”导致额度字段回显不一致，这里重新读取一次用户最新余额字段。
		if refreshed, err := model.GetUserById(id, false); err == nil && refreshed != nil {
			responseData["pay_request_allowed_groups"] = refreshed.PayRequestAllowedGroups
			responseData["pay_token_allowed_groups"] = refreshed.PayTokenAllowedGroups
			applyDiscreteUserResponseData(responseData, refreshed)
			applyPublicUsageResponseData(responseData, refreshed)
			applyQuotaBalanceResponseData(responseData, refreshed)
			applyLegacyClawBoxTotalQuotaCompat(responseData, refreshed)
		}
	}
	stripRetiredXiaotuanFields(responseData)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"added_quota": addedQuota,
			"user":        responseData,
			"redeem":      redeem,
		},
	})
}

type UpdateUserSettingRequest struct {
	QuotaWarningType           string  `json:"notify_type"`
	QuotaWarningThreshold      float64 `json:"quota_warning_threshold"`
	WebhookUrl                 string  `json:"webhook_url,omitempty"`
	WebhookSecret              string  `json:"webhook_secret,omitempty"`
	NotificationEmail          string  `json:"notification_email,omitempty"`
	BarkUrl                    string  `json:"bark_url,omitempty"`
	AcceptUnsetModelRatioModel bool    `json:"accept_unset_model_ratio_model"`
	RecordIpLog                bool    `json:"record_ip_log"`
}

func UpdateUserSetting(c *gin.Context) {
	var req UpdateUserSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	// 验证预警类型
	if req.QuotaWarningType != dto.NotifyTypeEmail && req.QuotaWarningType != dto.NotifyTypeWebhook && req.QuotaWarningType != dto.NotifyTypeBark {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的预警类型",
		})
		return
	}

	// 验证预警阈值
	if req.QuotaWarningThreshold <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "预警阈值必须大于0",
		})
		return
	}

	// 如果是webhook类型,验证webhook地址
	if req.QuotaWarningType == dto.NotifyTypeWebhook {
		if req.WebhookUrl == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Webhook地址不能为空",
			})
			return
		}
		// 验证URL格式
		if _, err := url.ParseRequestURI(req.WebhookUrl); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无效的Webhook地址",
			})
			return
		}
	}

	// 如果是邮件类型，验证邮箱地址
	if req.QuotaWarningType == dto.NotifyTypeEmail && req.NotificationEmail != "" {
		// 验证邮箱格式
		if !strings.Contains(req.NotificationEmail, "@") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无效的邮箱地址",
			})
			return
		}
	}

	// 如果是Bark类型，验证Bark URL
	if req.QuotaWarningType == dto.NotifyTypeBark {
		if req.BarkUrl == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Bark推送URL不能为空",
			})
			return
		}
		// 验证URL格式
		if _, err := url.ParseRequestURI(req.BarkUrl); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无效的Bark推送URL",
			})
			return
		}
		// 检查是否是HTTP或HTTPS
		if !strings.HasPrefix(req.BarkUrl, "https://") && !strings.HasPrefix(req.BarkUrl, "http://") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Bark推送URL必须以http://或https://开头",
			})
			return
		}
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 构建设置
	settings := dto.UserSetting{
		NotifyType:            req.QuotaWarningType,
		QuotaWarningThreshold: req.QuotaWarningThreshold,
		AcceptUnsetRatioModel: req.AcceptUnsetModelRatioModel,
		RecordIpLog:           req.RecordIpLog,
	}

	if user.Role != common.RoleAdminUser && user.Role != common.RoleRootUser {
		settings.AcceptUnsetRatioModel = false
		settings.RecordIpLog = true
	}

	// 如果是webhook类型,添加webhook相关设置
	if req.QuotaWarningType == dto.NotifyTypeWebhook {
		settings.WebhookUrl = req.WebhookUrl
		if req.WebhookSecret != "" {
			settings.WebhookSecret = req.WebhookSecret
		}
	}

	// 如果提供了通知邮箱，添加到设置中
	if req.QuotaWarningType == dto.NotifyTypeEmail && req.NotificationEmail != "" {
		settings.NotificationEmail = req.NotificationEmail
	}

	// 如果是Bark类型，添加Bark URL到设置中
	if req.QuotaWarningType == dto.NotifyTypeBark {
		settings.BarkUrl = req.BarkUrl
	}

	// 更新用户设置
	user.SetSetting(settings)
	if err := user.Update(false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新设置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "设置已更新",
	})
}

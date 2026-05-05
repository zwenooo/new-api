package model

import (
	"fmt"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const userCacheSchemaVersion = 4

// UserBase struct remains the same as it represents the cached data structure
type UserBase struct {
	CacheSchemaVersion      int       `json:"cache_schema_version"`
	Id                      int       `json:"id"`
	Role                    int       `json:"role"`
	GroupId                 int       `json:"group_id"`
	UserGroupId             int       `json:"user_group_id"`
	Group                   string    `json:"group"`
	Email                   string    `json:"email"`
	Quota                   int       `json:"quota"`
	TokensQuota             int       `json:"tokens_quota"`
	TokensHistoryQuota      int       `json:"tokens_history_quota"`
	PayAsYouGoQuota         int       `json:"payg_quota"`
	PayAsYouGoHistoryQuota  int       `json:"payg_history_quota"`
	PayAsYouGoAllowedGroups JSONValue `json:"payg_allowed_groups"`
	PayRequestQuota         int       `json:"pay_request_quota"`
	PayRequestHistoryQuota  int       `json:"pay_request_history_quota"`
	PayRequestAllowedGroups JSONValue `json:"pay_request_allowed_groups"`
	PayTokenQuota           int       `json:"pay_token_quota"`
	PayTokenHistoryQuota    int       `json:"pay_token_history_quota"`
	PayTokenAllowedGroups   JSONValue `json:"pay_token_allowed_groups"`
	Status                  int       `json:"status"`
	Username                string    `json:"username"`
	Setting                 string    `json:"setting"`
	AdminPermissions        JSONValue `json:"admin_permissions"`
	DailyQuotaLimit         int       `json:"daily_quota_limit"`
	DailyQuotaUsed          int       `json:"daily_quota_used"`
	DailyQuotaResetDate     int       `json:"daily_quota_reset_date"`
	BaseMultiplier          float64   `json:"base_multiplier"`
	CustomerType            string    `json:"customer_type"`
	PricingProfileId        int       `json:"pricing_profile_id"`
	BalanceFen              int64     `json:"balance_fen"`
	// 追加缓存字段：可过期兑换额度余额及其到期时间
	RedeemQuota         int    `json:"redeem_quota"`
	RedeemQuotaExpireAt int64  `json:"redeem_quota_expire_at"`
	PlanType            string `json:"plan_type"`
	PlanStartAt         int64  `json:"plan_start_at"`
	PlanExpireAt        int64  `json:"plan_expire_at"`
}

func (user *UserBase) WriteContext(c *gin.Context) {
	audienceGroupID := user.UserGroupId
	if audienceGroupID <= 0 {
		audienceGroupID = user.GroupId
	}
	common.SetContextKey(c, constant.ContextKeyUserGroupId, audienceGroupID)
	common.SetContextKey(c, constant.ContextKeyDefaultModelGroupId, user.GroupId)
	if common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId) <= 0 && user.GroupId > 0 {
		common.SetContextKey(c, constant.ContextKeyUsingGroupId, user.GroupId)
	}
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyUserStatus, user.Status)
	common.SetContextKey(c, constant.ContextKeyUserEmail, user.Email)
	common.SetContextKey(c, constant.ContextKeyUserName, user.Username)
	common.SetContextKey(c, constant.ContextKeyUserSetting, user.GetSetting())
	common.SetContextKey(c, constant.ContextKeyUserAdminPermissions, user.GetAdminPermissions())
	common.SetContextKey(c, constant.ContextKeyUserDailyQuotaLimit, user.DailyQuotaLimit)
	common.SetContextKey(c, constant.ContextKeyUserDailyQuotaUsed, user.DailyQuotaUsed)
	common.SetContextKey(c, constant.ContextKeyUserBaseMultiplier, user.BaseMultiplier)
	common.SetContextKey(c, constant.ContextKeyUserPlanType, user.PlanType)
	common.SetContextKey(c, constant.ContextKeyUserPlanStartAt, user.PlanStartAt)
	common.SetContextKey(c, constant.ContextKeyUserPlanExpireAt, user.PlanExpireAt)
	c.Set("username", user.Username)
	c.Set("role", user.Role)
}

func (user *UserBase) GetSetting() dto.UserSetting {
	// 优先从字符串反序列化
	setting := dto.UserSetting{}
	if user.Setting != "" {
		// 先解出结构体
		if err := common.Unmarshal([]byte(user.Setting), &setting); err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		} else {
			// 再解成 map 判断字段是否缺失，以便对历史老数据进行默认值回填
			var raw map[string]interface{}
			if err := common.Unmarshal([]byte(user.Setting), &raw); err == nil {
				if _, ok := raw["record_ip_log"]; !ok {
					// 老用户设置中没有该字段时，默认开启 IP 记录
					setting.RecordIpLog = true
				}
			}
		}
	} else {
		// 新用户尚无设置时，按默认开启 IP 记录
		setting.RecordIpLog = true
	}
	return setting
}

// getUserCacheKey returns the key for user cache
func getUserCacheKey(userId int) string {
	return fmt.Sprintf("user:%d", userId)
}

// invalidateUserCache clears user cache
func invalidateUserCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisDelKey(getUserCacheKey(userId))
}

func InvalidateUserCache(userId int) error {
	return invalidateUserCache(userId)
}

// updateUserCache updates all user cache fields using hash
func updateUserCache(user User) error {
	if !common.RedisEnabled {
		return nil
	}

	return common.RedisHSetObj(
		getUserCacheKey(user.Id),
		user.ToBaseUser(),
		time.Duration(common.RedisKeyCacheSeconds())*time.Second,
	)
}

// GetUserCache gets complete user cache from hash
func GetUserCache(userId int) (userCache *UserBase, err error) {
	var dbUser *User
	var fromDB bool
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && dbUser != nil {
			gopool.Go(func() {
				if err := updateUserCache(*dbUser); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()

	// Try getting from Redis first
	userCache, err = cacheGetUserBase(userId)
	if err == nil {
		if userCache.CacheSchemaVersion != userCacheSchemaVersion {
			common.SysLog(fmt.Sprintf("user cache schema mismatch, refreshing from DB: user_id=%d cached=%d expected=%d", userId, userCache.CacheSchemaVersion, userCacheSchemaVersion))
			_ = invalidateUserCache(userId)
		} else if userCache.RedeemQuotaExpireAt > 0 && userCache.RedeemQuota > 0 && userCache.RedeemQuotaExpireAt < time.Now().Unix() {
			_ = invalidateUserCache(userId)
		} else if userCache.GroupId <= 0 {
			common.SysLog(fmt.Sprintf("user cache group_id missing, refreshing from DB: user_id=%d", userId))
			_ = invalidateUserCache(userId)
		} else if userCache.Status != common.UserStatusEnabled && userCache.Status != common.UserStatusDisabled {
			common.SysLog(fmt.Sprintf("user cache status invalid, refreshing from DB: user_id=%d status=%d", userId, userCache.Status))
			_ = invalidateUserCache(userId)
		} else {
			return userCache, nil
		}
	}

	// If Redis fails, get from DB
	fromDB = true
	var userEntity User
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}
		return tx.Omit("password").Where("id = ?", userId).First(&userEntity).Error
	})
	if err != nil {
		return nil, err
	}
	dbUser = &userEntity
	userCache = dbUser.ToBaseUser()
	return userCache, nil
}

func cacheGetUserBase(userId int) (*UserBase, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var userCache UserBase
	// Try getting from Redis first
	err := common.RedisHGetObj(getUserCacheKey(userId), &userCache)
	if err != nil {
		return nil, err
	}
	return &userCache, nil
}

// Add atomic quota operations using hash fields
func cacheIncrUserQuota(userId int, delta int64) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHIncrBy(getUserCacheKey(userId), "Quota", delta)
}

func cacheDecrUserQuota(userId int, delta int64) error {
	return cacheIncrUserQuota(userId, -delta)
}

func cacheIncrUserTokensQuota(userId int, delta int64) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHIncrBy(getUserCacheKey(userId), "TokensQuota", delta)
}

func cacheDecrUserTokensQuota(userId int, delta int64) error {
	return cacheIncrUserTokensQuota(userId, -delta)
}

func cacheIncrUserPayTokenQuota(userId int, delta int64) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHIncrBy(getUserCacheKey(userId), "PayTokenQuota", delta)
}

func cacheDecrUserPayTokenQuota(userId int, delta int64) error {
	return cacheIncrUserPayTokenQuota(userId, -delta)
}

// Helper functions to get individual fields if needed
func getUserAudienceGroupIdCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	return cache.UserGroupId, nil
}

func getUserGroupIdCache(userId int) (int, error) {
	return getUserAudienceGroupIdCache(userId)
}

func getUserQuotaCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	// GetUserCache 已统一清理过期兑换额度，这里直接返回实时可用额度
	return cache.Quota, nil
}

func getUserStatusCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	return cache.Status, nil
}

func getUserNameCache(userId int) (string, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return "", err
	}
	return cache.Username, nil
}

func getUserSettingCache(userId int) (dto.UserSetting, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return dto.UserSetting{}, err
	}
	return cache.GetSetting(), nil
}

// New functions for individual field updates
func updateUserStatusCache(userId int, status bool) error {
	if !common.RedisEnabled {
		return nil
	}
	statusInt := common.UserStatusEnabled
	if !status {
		statusInt = common.UserStatusDisabled
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Status", fmt.Sprintf("%d", statusInt))
}

func updateUserQuotaCache(userId int, quota int) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Quota", fmt.Sprintf("%d", quota))
}

func updateUserAudienceGroupIdCache(userId int, groupId int) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetField(getUserCacheKey(userId), "UserGroupId", fmt.Sprintf("%d", groupId))
}

func updateUserGroupIdCache(userId int, groupId int) error {
	return updateUserAudienceGroupIdCache(userId, groupId)
}

func updateUserNameCache(userId int, username string) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Username", username)
}

func updateUserSettingCache(userId int, setting string) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Setting", setting)
}

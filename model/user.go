package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id               int     `json:"id"`
	Username         string  `json:"username" gorm:"unique;index" validate:"max=20"`
	Password         string  `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	OriginalPassword string  `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName      string  `json:"display_name" gorm:"index" validate:"max=20"`
	AvatarSeed       string  `json:"avatar_seed" gorm:"type:varchar(64);column:avatar_seed;index"`
	Role             int     `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status           int     `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email            string  `json:"email" gorm:"index" validate:"max=50"`
	GitHubId         string  `json:"github_id" gorm:"column:github_id;index"`
	OidcId           string  `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId         string  `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId       string  `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode string  `json:"verification_code" gorm:"-:all"`                                    // this field is only for Email verification, don't save it to database!
	AccessToken      *string `json:"access_token" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	Quota            int     `json:"quota" gorm:"type:int;default:0"`
	// TokensQuota stores token-bucket balance in scaled discrete units.
	TokensQuota int `json:"tokens_quota" gorm:"type:bigint;default:0;column:tokens_quota"`
	// TokensHistoryQuota stores cumulative credited token balance in scaled discrete units.
	TokensHistoryQuota int `json:"tokens_history_quota" gorm:"type:bigint;default:0;column:tokens_history_quota"`
	// PayAsYouGoQuota 表示“按量付费（不限时）额度”的当前余额，必须与订阅额度分桶扣费。
	PayAsYouGoQuota int `json:"payg_quota" gorm:"type:int;default:0;column:payg_quota"`
	// PayAsYouGoHistoryQuota 表示“按量付费（不限时）额度”的累计入账总额，用于展示统计（可选）。
	PayAsYouGoHistoryQuota int `json:"payg_history_quota" gorm:"type:int;default:0;column:payg_history_quota"`
	// PayAsYouGoAllowedGroups 表示按量付费额度当前可用的分组集合（tier），用于控制可用模型/倍率。
	PayAsYouGoAllowedGroups JSONValue `json:"payg_allowed_groups" gorm:"type:json;column:payg_allowed_groups"`
	// PayRequestQuota stores pay-request balance in scaled discrete units.
	PayRequestQuota int `json:"pay_request_quota" gorm:"type:bigint;default:0;column:pay_request_quota"`
	// PayRequestHistoryQuota stores cumulative pay-request credits in scaled discrete units.
	PayRequestHistoryQuota int `json:"pay_request_history_quota" gorm:"type:bigint;default:0;column:pay_request_history_quota"`
	// PayRequestAllowedGroups 表示按次付费次数当前可用的分组集合（tier），用于控制可用模型/倍率。
	PayRequestAllowedGroups JSONValue `json:"pay_request_allowed_groups" gorm:"type:json;column:pay_request_allowed_groups"`
	// PayTokenQuota stores pay-token balance in scaled discrete units.
	PayTokenQuota int `json:"pay_token_quota" gorm:"type:bigint;default:0;column:pay_token_quota"`
	// PayTokenHistoryQuota stores cumulative pay-token credits in scaled discrete units.
	PayTokenHistoryQuota int `json:"pay_token_history_quota" gorm:"type:bigint;default:0;column:pay_token_history_quota"`
	// PayTokenAllowedGroups 表示按token付费tokens当前可用的分组集合（tier），用于控制可用模型/倍率。
	PayTokenAllowedGroups JSONValue `json:"pay_token_allowed_groups" gorm:"type:json;column:pay_token_allowed_groups"`
	UsedQuota             int       `json:"used_quota" gorm:"type:int;default:0;column:used_quota"`                 // used quota
	VisibleUsedQuota      int       `json:"visible_used_quota" gorm:"type:int;default:0;column:visible_used_quota"` // visible used quota
	CostUsedQuota         int       `json:"cost_used_quota" gorm:"type:int;default:0;column:cost_used_quota"`       // cost used quota
	RequestCount          int       `json:"request_count" gorm:"type:int;default:0;"`                               // request number
	DailyQuotaLimit       int       `json:"daily_quota_limit" gorm:"type:int;default:0"`                            // 0 表示不限额
	DailyQuotaUsed        int       `json:"daily_quota_used" gorm:"type:int;default:0"`                             // 当日已用额度
	DailyQuotaResetDate   int       `json:"-" gorm:"type:int;default:0;column:daily_quota_reset_date"`              // YYYYMMDD???????????
	BaseMultiplier        float64   `json:"base_multiplier" gorm:"type:double precision;default:1" validate:"gt=0"`
	CustomerType          string    `json:"customer_type" gorm:"type:varchar(32);not null;default:'retail';column:customer_type;index"`
	PricingProfileId      int       `json:"pricing_profile_id" gorm:"type:int;default:0;column:pricing_profile_id;index"`
	// GroupId is the legacy default model-group fallback field.
	// It is not the new audience/user-group field.
	GroupId int    `json:"group_id" gorm:"type:int;default:0;index;column:group_id"`
	Group   string `json:"group" gorm:"type:varchar(64);default:'default'"`
	// UserGroupId is the actual audience/user-group field used for user segmentation and ops.
	UserGroupId         int            `json:"user_group_id" gorm:"type:int;default:0;index;column:user_group_id"`
	UserGroupLabel      string         `json:"user_group_label,omitempty" gorm:"-"`
	GroupPriceOverrides JSONValue      `json:"group_price_overrides,omitempty" gorm:"-:all"`
	BalanceFen          int64          `json:"balance_fen" gorm:"type:bigint;default:0;column:balance_fen"` // 人民币余额（分）
	AffCode             string         `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount            int            `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota            int64          `json:"aff_quota" gorm:"type:bigint;default:0;column:aff_quota"`           // 邀请剩余额度（分）
	AffHistoryQuota     int64          `json:"aff_history_quota" gorm:"type:bigint;default:0;column:aff_history"` // 邀请历史额度（分）
	InviterId           int            `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	DeletedAt           gorm.DeletedAt `gorm:"index"`
	LinuxDOId           string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting             string         `json:"setting" gorm:"type:text;column:setting"`
	AdminPermissions    JSONValue      `json:"admin_permissions" gorm:"type:json;column:admin_permissions"`
	Remark              string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer      string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt           int64          `json:"created_at" gorm:"bigint;autoCreateTime"`
	RegisterIP          string         `json:"register_ip" gorm:"type:varchar(64);column:register_ip"`
	// 下面两个字段用于实现“兑换额度有效期”逻辑：
	// RedeemQuota 表示当前仍处于有效期（未过期）的兑换来源额度余额；与总额度 Quota 保持：Quota >= RedeemQuota
	RedeemQuota int `json:"redeem_quota" gorm:"type:int;default:0;column:redeem_quota"`
	// RedeemQuotaExpireAt 为兑换额度的统一到期时间（秒级 Unix 时间戳）。
	// 0 表示不过期；>0 表示在该时间点到期。每次兑换可按“叠加订阅”规则续期。
	RedeemQuotaExpireAt int64 `json:"redeem_quota_expire_at" gorm:"bigint;default:0;column:redeem_quota_expire_at"`

	// PlanType / PlanExpireAt 用于表示用户的订阅计划（例如小团订阅）。
	PlanType     string `json:"plan_type" gorm:"type:varchar(32);default:'default';column:plan_type;index"`
	PlanStartAt  int64  `json:"plan_start_at" gorm:"bigint;default:0;column:plan_start_at"`
	PlanExpireAt int64  `json:"plan_expire_at" gorm:"bigint;default:0;column:plan_expire_at"`
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		CacheSchemaVersion:      userCacheSchemaVersion,
		Id:                      user.Id,
		Role:                    user.Role,
		GroupId:                 user.GroupId,
		UserGroupId:             user.UserGroupId,
		Group:                   user.Group,
		Quota:                   user.Quota,
		TokensQuota:             user.TokensQuota,
		TokensHistoryQuota:      user.TokensHistoryQuota,
		PayAsYouGoQuota:         user.PayAsYouGoQuota,
		PayAsYouGoHistoryQuota:  user.PayAsYouGoHistoryQuota,
		PayAsYouGoAllowedGroups: user.PayAsYouGoAllowedGroups,
		PayRequestQuota:         user.PayRequestQuota,
		PayRequestHistoryQuota:  user.PayRequestHistoryQuota,
		PayRequestAllowedGroups: user.PayRequestAllowedGroups,
		PayTokenQuota:           user.PayTokenQuota,
		PayTokenHistoryQuota:    user.PayTokenHistoryQuota,
		PayTokenAllowedGroups:   user.PayTokenAllowedGroups,
		Status:                  user.Status,
		Username:                user.Username,
		Setting:                 user.Setting,
		AdminPermissions:        user.AdminPermissions,
		Email:                   user.Email,
		DailyQuotaLimit:         user.DailyQuotaLimit,
		DailyQuotaUsed:          user.DailyQuotaUsed,
		DailyQuotaResetDate:     user.DailyQuotaResetDate,
		BaseMultiplier:          user.BaseMultiplier,
		CustomerType:            user.CustomerType,
		PricingProfileId:        user.PricingProfileId,
		BalanceFen:              user.BalanceFen,
		RedeemQuota:             user.RedeemQuota,
		RedeemQuotaExpireAt:     user.RedeemQuotaExpireAt,
		PlanType:                user.PlanType,
		PlanStartAt:             user.PlanStartAt,
		PlanExpireAt:            user.PlanExpireAt,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

func (user *User) GetSetting() dto.UserSetting {
	// 读取用户设置，若缺失字段则回填默认值
	setting := dto.UserSetting{}
	if user.Setting != "" {
		if err := json.Unmarshal([]byte(user.Setting), &setting); err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		} else {
			// 检查原始 JSON 是否包含 record_ip_log 字段，不包含则默认开启
			var raw map[string]interface{}
			if err := common.Unmarshal([]byte(user.Setting), &raw); err == nil {
				if _, ok := raw["record_ip_log"]; !ok {
					setting.RecordIpLog = true
				}
			}
		}
	} else {
		// 无任何个性化设置时，默认开启 IP 记录
		setting.RecordIpLog = true
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := json.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
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
		"enabled":      true,
		"subscription": true,
		"topup":        true,
		"personal":     true,
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

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Unscoped().Model(&User{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	err = tx.Unscoped().Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func SearchUsers(keyword string, groupID int, userGroupID int, startIdx int, num int) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})
	if groupID > 0 {
		query = query.Where("group_id = ?", groupID)
	}
	if userGroupID > 0 {
		query = query.Where("user_group_id = ?", userGroupID)
	}

	// 构建搜索条件
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"

	// 尝试将关键字转换为整数ID
	keywordInt, err := strconv.Atoi(keyword)
	if err == nil {
		// 如果是数字，同时搜索ID和其他字段
		likeCondition = "id = ? OR " + likeCondition
		query = query.Where(likeCondition,
			keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	} else {
		// 非数字关键字，只搜索字符串字段
		query = query.Where(likeCondition,
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Omit("password").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&ChannelUserBinding{}).Error; err != nil {
			return err
		}
		var subscriptionIDs []int
		if err := tx.Model(&UserSubscription{}).
			Select("id").
			Where("user_id = ?", id).
			Pluck("id", &subscriptionIDs).Error; err != nil {
			return err
		}
		if len(subscriptionIDs) > 0 {
			if err := tx.Where("subscription_id IN ?", subscriptionIDs).Delete(&UserSubscriptionGroup{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", id).Delete(&UserSubscription{}).Error; err != nil {
			return err
		}

		var requestSubscriptionIDs []int
		if err := tx.Model(&UserRequestSubscription{}).
			Select("id").
			Where("user_id = ?", id).
			Pluck("id", &requestSubscriptionIDs).Error; err != nil {
			return err
		}
		if len(requestSubscriptionIDs) > 0 {
			if err := tx.Where("subscription_id IN ?", requestSubscriptionIDs).Delete(&UserRequestSubscriptionGroup{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", id).Delete(&UserRequestSubscription{}).Error; err != nil {
			return err
		}

		return tx.Unscoped().Delete(&User{}, "id = ?", id).Error
	}); err != nil {
		return err
	}
	if err := invalidateUserCache(id); err != nil {
		return err
	}
	return InitChannelBindingCache()
}

func inviteUser(inviterId int) error {
	if inviterId <= 0 {
		return errors.New("inviterId 无效！")
	}
	return DB.Model(&User{}).
		Where("id = ?", inviterId).
		Update("aff_count", gorm.Expr("aff_count + ?", 1)).Error
}

func validateInvitationBindingTx(tx *gorm.DB, userId int, inviterId int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if inviterId < 0 {
		return errors.New("邀请人 ID 无效")
	}
	if inviterId == 0 {
		return nil
	}
	if inviterId == userId {
		return errors.New("邀请人不能是自己")
	}

	visited := make(map[int]struct{}, 8)
	currentID := inviterId
	for currentID > 0 {
		if currentID == userId {
			return errors.New("邀请绑定不能形成循环")
		}
		if _, ok := visited[currentID]; ok {
			return errors.New("邀请链存在循环，请先修复相关用户的邀请绑定")
		}
		visited[currentID] = struct{}{}

		current := User{}
		if err := tx.Select("id", "inviter_id").First(&current, "id = ?", currentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("邀请人不存在")
			}
			return err
		}
		currentID = current.InviterId
	}
	return nil
}

func syncInvitationAffCountsTx(tx *gorm.DB, inviterIDs []int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	ids := normalizeUniqueSortedIDs(inviterIDs)
	if len(ids) == 0 {
		return nil
	}

	var inviters []User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		Where("id IN ?", ids).
		Find(&inviters).Error; err != nil {
		return err
	}
	for _, inviter := range inviters {
		var count int64
		if err := tx.Model(&User{}).
			Where("inviter_id = ?", inviter.Id).
			Count(&count).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).
			Where("id = ?", inviter.Id).
			Update("aff_count", int(count)).Error; err != nil {
			return err
		}
	}
	return nil
}

func (user *User) TransferAffQuotaToBalance(amountFen int64) error {
	if amountFen <= 0 {
		return errors.New("转移金额必须大于0！")
	}

	// 开始数据库事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback() // 确保在函数退出时事务能回滚

	// 加锁查询用户以确保数据一致性
	var current User
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "aff_quota", "balance_fen").
		First(&current, user.Id).Error
	if err != nil {
		return err
	}

	// 再次检查用户的AffQuota是否足够
	if current.AffQuota < amountFen {
		return errors.New("邀请返利不足！")
	}

	balanceBeforeFen := current.BalanceFen
	balanceAfterFen := balanceBeforeFen + amountFen
	if balanceAfterFen < balanceBeforeFen {
		return errors.New("余额溢出")
	}

	// 更新返利与余额（单位：分）
	if err := tx.Model(&User{}).
		Where("id = ?", user.Id).
		Updates(map[string]interface{}{
			"aff_quota":   gorm.Expr("aff_quota - ?", amountFen),
			"balance_fen": gorm.Expr("balance_fen + ?", amountFen),
		}).Error; err != nil {
		return err
	}

	if err := CreateBalanceRecord(
		tx,
		user.Id,
		BalanceRecordTypeAffTransferIn,
		amountFen,
		balanceBeforeFen,
		balanceAfterFen,
		"",
	); err != nil {
		return err
	}

	// 提交事务
	return tx.Commit().Error
}

func prepareUserPricingConfigTx(tx *gorm.DB, user *User, assignDefaultRetailProfile bool) ([]PriceGroupFactor, error) {
	if user == nil {
		return nil, errors.New("user 为空")
	}
	customerType, err := normalizeCustomerType(user.CustomerType)
	if err != nil {
		return nil, err
	}
	user.CustomerType = customerType

	if assignDefaultRetailProfile && user.PricingProfileId <= 0 && customerType == CustomerTypeRetail {
		retailProfileID, err := ensureDefaultRetailPricingProfileTx(tx)
		if err != nil {
			return nil, err
		}
		if retailProfileID > 0 {
			user.PricingProfileId = retailProfileID
		}
	}
	if err := validatePricingProfileForUserTx(tx, customerType, user.PricingProfileId); err != nil {
		return nil, err
	}

	overrides, err := ParsePriceGroupFactorsJSON(user.GroupPriceOverrides)
	if err != nil {
		return nil, fmt.Errorf("解析分组价格覆写失败: %w", err)
	}
	if len(overrides) > 0 {
		if err := ValidateGroupIDsExist(tx, extractPriceGroupFactorIDs(overrides)); err != nil {
			return nil, err
		}
	}
	return overrides, nil
}

func (user *User) Insert(inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(user.AvatarSeed) == "" {
		user.AvatarSeed = common.GetRandomString(16)
	}
	user.Quota = common.QuotaForNewUser
	//user.SetAccessToken(common.GetUUID())
	user.AffCode = common.GetRandomString(4)
	if user.BaseMultiplier <= 0 {
		user.BaseMultiplier = 1
	}

	// 初始化用户设置：默认开启记录请求/错误日志的 IP
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{RecordIpLog: true}
		// 这里暂不设置 SidebarModules，待创建后按角色补齐
		user.SetSetting(defaultSetting)
	}

	overrides, err := prepareUserPricingConfigTx(DB, user, true)
	if err != nil {
		return err
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if user.UserGroupId <= 0 {
			userGroupID, err := ResolveUserGroupIDForUserTx(tx, user.UserGroupId, user.GroupId)
			if err != nil {
				return err
			}
			user.UserGroupId = userGroupID
		} else {
			if _, err := GetUserGroupByID(tx, user.UserGroupId); err != nil {
				return err
			}
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return replaceUserGroupPriceOverridesTx(tx, user.Id, overrides)
	}); err != nil {
		return err
	}
	if err := RefreshPricingRuleCache(); err != nil {
		return err
	}

	// 用户创建成功后，根据角色初始化边栏配置
	// 需要重新获取用户以确保有正确的ID和Role
	var createdUser User
	if err := DB.Where("username = ?", user.Username).First(&createdUser).Error; err == nil {
		// 生成基于角色的默认边栏配置
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	if inviterId != 0 {
		_ = inviteUser(inviterId)
		if common.QuotaForInvitee > 0 {
			_ = IncreaseUserQuota(user.Id, common.QuotaForInvitee, true)
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
		}
	}
	return nil
}

func (user *User) Update(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	DB.First(&user, user.Id)
	if err = DB.Model(user).Updates(newUser).Error; err != nil {
		return err
	}

	// Update cache
	return updateUserCache(*user)
}

func (user *User) Edit(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	if newUser.GroupId <= 0 {
		return errors.New("用户分组无效")
	}
	cacheUser := User{}
	refreshPricingRules := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		originUser := User{}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&originUser, user.Id).Error; err != nil {
			return err
		}
		overrides, err := prepareUserPricingConfigTx(tx, &newUser, false)
		if err != nil {
			return err
		}
		if newUser.UserGroupId > 0 {
			if _, err := GetUserGroupByID(tx, newUser.UserGroupId); err != nil {
				return err
			}
		}
		if originUser.InviterId != newUser.InviterId {
			if err := validateInvitationBindingTx(tx, originUser.Id, newUser.InviterId); err != nil {
				return err
			}
		}

		if originUser.PayAsYouGoQuota != newUser.PayAsYouGoQuota || string(originUser.PayAsYouGoAllowedGroups) != string(newUser.PayAsYouGoAllowedGroups) {
			if err := syncEditableLegacyPaygBalanceTx(tx, originUser.Id, newUser.PayAsYouGoQuota, newUser.PayAsYouGoAllowedGroups); err != nil {
				return err
			}
		}

		updates := map[string]interface{}{
			"username":            newUser.Username,
			"display_name":        newUser.DisplayName,
			"group_id":            newUser.GroupId,
			"user_group_id":       newUser.UserGroupId,
			"quota":               newUser.Quota,
			"payg_quota":          newUser.PayAsYouGoQuota,
			"payg_history_quota":  newUser.PayAsYouGoHistoryQuota,
			"payg_allowed_groups": newUser.PayAsYouGoAllowedGroups,
			"admin_permissions":   newUser.AdminPermissions,
			"remark":              newUser.Remark,
			"daily_quota_limit":   newUser.DailyQuotaLimit,
			"base_multiplier":     newUser.BaseMultiplier,
			"customer_type":       newUser.CustomerType,
			"pricing_profile_id":  newUser.PricingProfileId,
			"inviter_id":          newUser.InviterId,
			// 允许管理员直接设置限时额度的到期时间；可为 0（代表不限时/清空到期）
			"redeem_quota_expire_at": newUser.RedeemQuotaExpireAt,
		}
		if updatePassword {
			updates["password"] = newUser.Password
		}

		dailyQuotaUsed := originUser.DailyQuotaUsed
		if originUser.DailyQuotaLimit != newUser.DailyQuotaLimit {
			if newUser.DailyQuotaLimit > 0 && originUser.DailyQuotaUsed > newUser.DailyQuotaLimit {
				dailyQuotaUsed = newUser.DailyQuotaLimit
				updates["daily_quota_used"] = dailyQuotaUsed
			}
		}
		if err = tx.Model(&originUser).Updates(updates).Error; err != nil {
			return err
		}
		if err := replaceUserGroupPriceOverridesTx(tx, originUser.Id, overrides); err != nil {
			return err
		}
		if originUser.InviterId != newUser.InviterId {
			if err := syncInvitationAffCountsTx(tx, []int{originUser.InviterId, newUser.InviterId}); err != nil {
				return err
			}
		}

		originUser.Username = newUser.Username
		originUser.DisplayName = newUser.DisplayName
		originUser.GroupId = newUser.GroupId
		originUser.UserGroupId = newUser.UserGroupId
		originUser.Quota = newUser.Quota
		originUser.PayAsYouGoQuota = newUser.PayAsYouGoQuota
		originUser.PayAsYouGoHistoryQuota = newUser.PayAsYouGoHistoryQuota
		originUser.PayAsYouGoAllowedGroups = newUser.PayAsYouGoAllowedGroups
		originUser.AdminPermissions = newUser.AdminPermissions
		originUser.Remark = newUser.Remark
		originUser.DailyQuotaLimit = newUser.DailyQuotaLimit
		originUser.BaseMultiplier = newUser.BaseMultiplier
		originUser.CustomerType = newUser.CustomerType
		originUser.PricingProfileId = newUser.PricingProfileId
		originUser.DailyQuotaUsed = dailyQuotaUsed
		originUser.InviterId = newUser.InviterId
		if updatePassword {
			originUser.Password = newUser.Password
		}
		cacheUser = originUser
		refreshPricingRules = true
		return nil
	})
	if err != nil {
		return err
	}
	if refreshPricingRules {
		if err := RefreshPricingRuleCache(); err != nil {
			return err
		}
	}

	// Update cache with latest values
	return updateUserCache(cacheUser)
}

func syncEditableLegacyPaygBalanceTx(tx *gorm.DB, userId int, desiredQuota int, desiredAllowedGroups JSONValue) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if !tx.Migrator().HasTable(&PaygUserBalance{}) {
		return nil
	}

	var activeNonLegacyCount int64
	if err := tx.Model(&PaygUserBalance{}).
		Where("user_id = ? AND product_id <> ? AND remaining_quota > 0", userId, -1).
		Count(&activeNonLegacyCount).Error; err != nil {
		return err
	}
	if activeNonLegacyCount > 0 {
		return errors.New("用户存在按量付费明细余额，请使用按量付费管理接口调整，不能直接编辑 payg_quota 或 payg_allowed_groups")
	}

	groupIDs := make([]int, 0)
	if len(desiredAllowedGroups) > 0 {
		ids, err := ParseGroupIDsJSON(desiredAllowedGroups)
		if err != nil {
			return err
		}
		groupIDs = normalizeUniqueSortedIDs(ids)
	}
	if desiredQuota > 0 {
		if len(groupIDs) == 0 {
			return errors.New("按量付费可用分组为空")
		}
		if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
			return err
		}
	}

	var groupIDsJSON JSONValue
	if len(groupIDs) > 0 {
		marshaled, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}
		groupIDsJSON = marshaled
	}

	var legacyBalance PaygUserBalance
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND product_id = ?", userId, -1).
		First(&legacyBalance).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if desiredQuota <= 0 {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		now := time.Now().Unix()
		updates := map[string]interface{}{
			"product_name":               "",
			"sort_order":                 0,
			"allowed_group_ids":          groupIDsJSON,
			"allowed_groups":             nil,
			"override_allowed_group_ids": false,
			"remaining_quota":            0,
			"updated_at":                 now,
		}
		return tx.Model(&PaygUserBalance{}).Where("id = ?", legacyBalance.Id).Updates(updates).Error
	}

	now := time.Now().Unix()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		balance := &PaygUserBalance{
			UserId:                  userId,
			ProductId:               -1,
			ProductName:             "",
			SortOrder:               0,
			AllowedGroupIds:         groupIDsJSON,
			AllowedGroups:           nil,
			OverrideAllowedGroupIds: false,
			RemainingQuota:          desiredQuota,
			HistoryQuota:            desiredQuota,
			CreatedAt:               now,
			UpdatedAt:               now,
		}
		return tx.Create(balance).Error
	}

	historyQuota := legacyBalance.HistoryQuota
	if historyQuota < desiredQuota {
		historyQuota = desiredQuota
	}
	return tx.Model(&PaygUserBalance{}).Where("id = ?", legacyBalance.Id).Updates(map[string]interface{}{
		"product_name":               "",
		"sort_order":                 0,
		"allowed_group_ids":          groupIDsJSON,
		"allowed_groups":             nil,
		"override_allowed_group_ids": false,
		"remaining_quota":            desiredQuota,
		"history_quota":              historyQuota,
		"updated_at":                 now,
	}).Error
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.Id).Delete(&ChannelUserBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(user).Error
	}); err != nil {
		return err
	}
	if err := invalidateUserCache(user.Id); err != nil {
		return err
	}
	return InitChannelBindingCache()
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	return HardDeleteUserById(user.Id)
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return errors.New("用户名或密码为空")
	}
	// find buy username or email
	DB.Where("username = ? OR email = ?", username, username).First(user)
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return errors.New("用户名或密码错误，或用户已被封禁")
	}
	return nil
}

func (user *User) EnsureAvatarSeed() error {
	if user == nil || user.Id == 0 {
		return errors.New("id 为空！")
	}
	if strings.TrimSpace(user.AvatarSeed) != "" {
		return nil
	}

	seed := fmt.Sprintf("user-%d", user.Id)
	result := DB.Model(&User{}).
		Where("id = ? AND (avatar_seed IS NULL OR avatar_seed = '')", user.Id).
		Update("avatar_seed", seed)
	if result.Error != nil {
		common.SysLog(fmt.Sprintf("failed to init avatar_seed: user_id=%d err=%s", user.Id, result.Error.Error()))
		user.AvatarSeed = seed
		return nil
	}
	if result.RowsAffected == 0 {
		var latest User
		if err := DB.Select("avatar_seed").First(&latest, "id = ?", user.Id).Error; err != nil {
			common.SysLog(fmt.Sprintf("failed to load avatar_seed: user_id=%d err=%s", user.Id, err.Error()))
			user.AvatarSeed = seed
			return nil
		}
		if strings.TrimSpace(latest.AvatarSeed) == "" {
			user.AvatarSeed = seed
		} else {
			user.AvatarSeed = latest.AvatarSeed
		}
		return nil
	}

	user.AvatarSeed = seed
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	return DB.Unscoped().Where("email = ?", email).Find(&User{}).RowsAffected == 1
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	err = DB.Model(&User{}).Where("email = ?", email).Update("password", hashedPassword).Error
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (user *User) {
	if token == "" {
		return nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user = &User{}
	if DB.Where("access_token = ?", token).First(user).RowsAffected == 1 {
		return user
	}
	return nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserQuotaCache(id, quota); err != nil {
					common.SysLog("failed to update user quota cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			return quota, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, id); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	})
	if err != nil {
		return 0, err
	}
	if quota < 0 {
		quota = 0
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserAudienceGroupID gets user_group_id from Redis first, falls back to DB if needed.
// Compatibility:
// - prefer users.user_group_id
// - if still empty, derive from legacy users.group_id when possible
// - if user_groups table does not exist yet, fall back to legacy users.group_id directly
func GetUserAudienceGroupID(id int, fromDB bool) (groupID int, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && groupID > 0 {
			gopool.Go(func() {
				if err := updateUserAudienceGroupIdCache(id, groupID); err != nil {
					common.SysLog("failed to update user audience_group_id cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		groupID, err := getUserAudienceGroupIdCache(id)
		if err == nil && groupID > 0 {
			return groupID, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	type row struct {
		UserGroupId int `gorm:"column:user_group_id"`
		GroupId     int `gorm:"column:group_id"`
	}
	var userRow row
	err = DB.Model(&User{}).Where("id = ?", id).Select("user_group_id", "group_id").First(&userRow).Error
	if err != nil {
		return 0, err
	}
	if userRow.UserGroupId > 0 {
		return userRow.UserGroupId, nil
	}
	if DB != nil && DB.Migrator().HasTable(&UserGroup{}) {
		resolvedUserGroupID, resolveErr := ResolveUserGroupIDForUserTx(DB, 0, userRow.GroupId)
		if resolveErr != nil {
			return 0, resolveErr
		}
		if resolvedUserGroupID > 0 {
			if updateErr := DB.Model(&User{}).Where("id = ?", id).Update("user_group_id", resolvedUserGroupID).Error; updateErr != nil {
				return 0, updateErr
			}
			return resolvedUserGroupID, nil
		}
	}
	if userRow.GroupId > 0 {
		return userRow.GroupId, nil
	}
	return 0, errors.New("用户分组无效")
}

// GetUserGroup is a deprecated compatibility wrapper.
// Use GetUserAudienceGroupID instead.
func GetUserGroup(id int, fromDB bool) (int, error) {
	return GetUserAudienceGroupID(id, fromDB)
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog("failed to update user setting cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&setting).Error
	if err != nil {
		return settingMap, err
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheIncrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to increase user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, quota)
		return nil
	}
	return increaseUserQuota(id, quota)
}

func increaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func ReturnUserQuota(id int, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	userDelta := 0
	userDelta, err = restoreUserQuota(id, quota)
	if err != nil {
		return err
	}
	if userDelta > 0 {
		gopool.Go(func() {
			err := cacheIncrUserQuota(id, int64(userDelta))
			if err != nil {
				common.SysLog("failed to restore user quota: " + err.Error())
			}
		})
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return nil
}

func DecreaseUserQuotaByBucket(id int, quota int, bucket string, usingGroupID int, paygProductId int) (selectedPaygProductId int, err error) {
	selectedPaygProductId, _, err = DecreaseUserQuotaByBucketWithAllocations(id, quota, bucket, usingGroupID, paygProductId)
	return selectedPaygProductId, err
}

func DecreaseUserQuotaByBucketWithAllocations(id int, quota int, bucket string, usingGroupID int, paygProductId int) (selectedPaygProductId int, subscriptionAllocations []relaycommon.SubscriptionUnitAllocation, err error) {
	if quota < 0 {
		return 0, nil, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return 0, nil, nil
	}
	userDelta := 0
	selectedPaygProductId, subscriptionAllocations, userDelta, err = consumeUserQuotaByBucket(id, quota, bucket, usingGroupID, paygProductId)
	if err != nil {
		if errors.Is(err, ErrUserDailyQuotaExceeded) {
			if errInvalidate := invalidateUserCache(id); errInvalidate != nil {
				common.SysLog("failed to invalidate user cache after daily quota exceeded: " + errInvalidate.Error())
			}
		}
		return 0, nil, err
	}
	if userDelta > 0 {
		gopool.Go(func() {
			var err error
			if bucket == UserQuotaBucketTokens {
				err = cacheDecrUserTokensQuota(id, int64(userDelta))
			} else if bucket == UserQuotaBucketPayToken {
				err = cacheDecrUserPayTokenQuota(id, int64(userDelta))
			} else {
				err = cacheDecrUserQuota(id, int64(userDelta))
			}
			if err != nil {
				common.SysLog("failed to decrease user quota: " + err.Error())
			}
		})
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return selectedPaygProductId, subscriptionAllocations, nil
}

func ReturnUserQuotaByBucket(id int, quota int, bucket string, usingGroupID int, paygProductId int) (err error) {
	return ReturnUserQuotaByBucketWithAllocations(id, quota, bucket, usingGroupID, paygProductId, nil)
}

func ReturnUserQuotaByBucketWithAllocations(id int, quota int, bucket string, usingGroupID int, paygProductId int, subscriptionAllocations []relaycommon.SubscriptionUnitAllocation) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	userDelta := 0
	userDelta, err = restoreUserQuotaByBucket(id, quota, bucket, usingGroupID, paygProductId, subscriptionAllocations)
	if err != nil {
		return err
	}
	if userDelta > 0 {
		gopool.Go(func() {
			var err error
			if bucket == UserQuotaBucketTokens {
				err = cacheIncrUserTokensQuota(id, int64(userDelta))
			} else if bucket == UserQuotaBucketPayToken {
				err = cacheIncrUserPayTokenQuota(id, int64(userDelta))
			} else {
				err = cacheIncrUserQuota(id, int64(userDelta))
			}
			if err != nil {
				common.SysLog("failed to restore user quota: " + err.Error())
			}
		})
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return nil
}

func restoreUserQuota(id int, quota int) (userDelta int, err error) {
	userDelta = 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		if err := ensureUserQuotaSubscriptionsReadyForConsume(tx, id, now); err != nil {
			return err
		}
		restored, restoredDeducted, err := restoreQuotaToSubscriptions(tx, id, quota)
		if err != nil {
			return err
		}
		left := quota - restored
		if left < 0 {
			left = 0
		}

		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}

		delta := restoredDeducted + left
		if delta < 0 {
			delta = 0
		}
		updates := map[string]interface{}{}
		if delta > 0 {
			updates["quota"] = gorm.Expr("quota + ?", delta)
		}
		if restoredDeducted > 0 {
			updates["redeem_quota"] = gorm.Expr("redeem_quota + ?", restoredDeducted)
		}
		// 自由额度不记录用户级每日用量

		if len(updates) > 0 {
			if err := tx.Model(&User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				return err
			}
		}

		userDelta = delta
		return nil
	})
	if err != nil {
		return 0, err
	}
	return userDelta, nil
}

func restoreUserQuotaByBucket(id int, quota int, bucket string, usingGroupID int, paygProductId int, subscriptionAllocations []relaycommon.SubscriptionUnitAllocation) (userDelta int, err error) {
	userDelta = 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		switch bucket {
		case UserQuotaBucketTokens:
			if err := ensureUserSubscriptionsActiveAt(tx, id, now); err != nil {
				return err
			}
			var restored, restoredDeducted int
			var err error
			if len(subscriptionAllocations) > 0 {
				restored, restoredDeducted, err = restoreTokensToSubscriptionsWithAllocations(tx, id, subscriptionAllocations)
			} else {
				if usingGroupID <= 0 {
					return errors.New("tokens 订阅缺少分组信息")
				}
				restored, restoredDeducted, err = restoreTokensToSubscriptionsByGroup(tx, id, quota, usingGroupID)
			}
			if err != nil {
				return err
			}
			if restored != quota {
				return errors.New("tokens 额度归还失败")
			}
			if restoredDeducted > 0 {
				if err := tx.Model(&User{}).Where("id = ?", id).Update("tokens_quota", gorm.Expr("tokens_quota + ?", restoredDeducted)).Error; err != nil {
					return err
				}
			}
			userDelta = restoredDeducted
			return nil
		case UserQuotaBucketSubscription:
			if err := ensureUserQuotaSubscriptionsReadyForConsume(tx, id, now); err != nil {
				return err
			}
			var restored, restoredDeducted int
			var err error
			if len(subscriptionAllocations) > 0 {
				restored, restoredDeducted, err = restoreQuotaToSubscriptionsWithAllocations(tx, id, subscriptionAllocations)
			} else {
				restored, restoredDeducted, err = restoreQuotaToSubscriptionsByGroup(tx, id, quota, usingGroupID)
			}
			if err != nil {
				return err
			}
			if restored != quota {
				return errors.New("订阅额度归还失败")
			}
			if restoredDeducted > 0 {
				if err := tx.Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
					"quota":        gorm.Expr("quota + ?", restoredDeducted),
					"redeem_quota": gorm.Expr("redeem_quota + ?", restoredDeducted),
				}).Error; err != nil {
					return err
				}
			}
			userDelta = restoredDeducted
			return nil
		case UserQuotaBucketPayg:
			if paygProductId == 0 {
				return errors.New("按量付费商品未指定，无法返还额度")
			}

			var user User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id").
				Where("id = ?", id).
				First(&user).Error; err != nil {
				return err
			}

			var bal PaygUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND product_id = ?", id, paygProductId).
				First(&bal).Error; err != nil {
				return err
			}

			if err := tx.Model(&PaygUserBalance{}).Where("id = ?", bal.Id).
				Update("remaining_quota", gorm.Expr("remaining_quota + ?", quota)).Error; err != nil {
				return err
			}

			// Rebuild union groups for payg_allowed_groups from positive balances.
			var balances []PaygUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "product_id", "allowed_group_ids", "override_allowed_group_ids", "remaining_quota", "sort_order").
				Where("user_id = ? AND remaining_quota > 0", id).
				Order("sort_order DESC, product_id DESC, id DESC").
				Find(&balances).Error; err != nil {
				return err
			}

			dustCleared, err := clearPaygDustFromBalancesTx(tx, balances, common.PreConsumedQuota)
			if err != nil {
				return err
			}
			unionGroupsJSON, err := UnionPaygAllowedGroupsFromBalances(balances)
			if err != nil {
				return err
			}

			delta := quota - dustCleared
			if delta < 0 {
				delta = 0
			}
			userDelta = delta
			return tx.Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
				"payg_quota":          gorm.Expr("payg_quota + ?", delta),
				"quota":               gorm.Expr("quota + ?", delta),
				"payg_allowed_groups": unionGroupsJSON,
			}).Error
		case UserQuotaBucketPayToken:
			if paygProductId == 0 {
				return errors.New("按token付费商品未指定，无法返还tokens")
			}

			var user User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id").
				Where("id = ?", id).
				First(&user).Error; err != nil {
				return err
			}

			var bal PayTokenUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND product_id = ?", id, paygProductId).
				First(&bal).Error; err != nil {
				return err
			}

			if err := tx.Model(&PayTokenUserBalance{}).Where("id = ?", bal.Id).
				Update("remaining_tokens", gorm.Expr("remaining_tokens + ?", quota)).Error; err != nil {
				return err
			}

			// Rebuild union groups for pay_tokens_allowed_groups from positive balances.
			var balances []PayTokenUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "product_id", "allowed_group_ids", "remaining_tokens", "sort_order").
				Where("user_id = ? AND remaining_tokens > 0", id).
				Order("sort_order DESC, product_id DESC, id DESC").
				Find(&balances).Error; err != nil {
				return err
			}

			unionGroupsJSON, err := UnionPayTokenAllowedGroupsFromBalances(balances)
			if err != nil {
				return err
			}

			userDelta = quota
			return tx.Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
				"pay_token_quota":          gorm.Expr("pay_token_quota + ?", quota),
				"pay_token_allowed_groups": unionGroupsJSON,
			}).Error
		case UserQuotaBucketFree:
			userDelta = quota
			return tx.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
		default:
			return errors.New("bucket 无效")
		}
	})
	if err != nil {
		return 0, err
	}
	return userDelta, nil
}

func DecreaseUserQuota(id int, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	userDelta := 0
	userDelta, err = consumeUserQuota(id, quota)
	if err != nil {
		if errors.Is(err, ErrUserDailyQuotaExceeded) {
			if errInvalidate := invalidateUserCache(id); errInvalidate != nil {
				common.SysLog("failed to invalidate user cache after daily quota exceeded: " + errInvalidate.Error())
			}
		}
		return err
	}
	if userDelta > 0 {
		gopool.Go(func() {
			err := cacheDecrUserQuota(id, int64(userDelta))
			if err != nil {
				common.SysLog("failed to decrease user quota: " + err.Error())
			}
		})
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return nil
}

func consumeUserQuota(id int, quota int) (userDelta int, err error) {
	userDelta = 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, id); err != nil {
			return err
		}

		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "quota", "redeem_quota", "payg_quota", "payg_history_quota", "payg_allowed_groups", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date").
			Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if _, err := syncLockedUserPaygSnapshotFromBalancesTx(tx, &user); err != nil {
			return err
		}

		consumedFromSub, deductedFromSub, err := consumeQuotaFromSubscriptions(tx, id, quota)
		if err != nil {
			return err
		}
		remainingAfterSub := quota - consumedFromSub
		remaining := remainingAfterSub

		paygQuota := user.PayAsYouGoQuota
		freeQuota := user.Quota - user.RedeemQuota - paygQuota
		if freeQuota < 0 {
			freeQuota = 0
		}

		paygConsume := 0
		if remaining > 0 && paygQuota > 0 {
			if paygQuota >= remaining {
				paygConsume = remaining
				remaining = 0
			} else {
				paygConsume = paygQuota
				remaining -= paygQuota
			}
		}
		if remaining > freeQuota {
			return errors.New("用户额度不足")
		}

		paygDustCleared := 0
		var paygAllowedGroupsJSON JSONValue
		if paygConsume > 0 {
			// Keep per-product PAYG balances consistent when legacy (bucket-less) deduction consumes PAYG quota.
			var balances []PaygUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "allowed_group_ids", "override_allowed_group_ids", "remaining_quota", "product_id", "sort_order").
				Where("user_id = ? AND remaining_quota > 0", id).
				Order("sort_order DESC, product_id DESC, id DESC").
				Find(&balances).Error; err != nil {
				return err
			}
			if len(balances) == 0 {
				return errors.New("payg_quota 数据错误")
			}
			left := paygConsume
			for i := range balances {
				if left <= 0 {
					break
				}
				usable := balances[i].RemainingQuota
				if usable <= 0 {
					continue
				}
				consume := usable
				if consume > left {
					consume = left
				}
				if err := tx.Model(&PaygUserBalance{}).Where("id = ?", balances[i].Id).
					Update("remaining_quota", gorm.Expr("remaining_quota - ?", consume)).Error; err != nil {
					return err
				}
				balances[i].RemainingQuota -= consume
				left -= consume
			}
			if left > 0 {
				return errors.New("payg_quota 数据错误")
			}
			dustCleared, err := clearPaygDustFromBalancesTx(tx, balances, common.PreConsumedQuota)
			if err != nil {
				return err
			}
			paygDustCleared = dustCleared
			union, err := UnionPaygAllowedGroupsFromBalances(balances)
			if err != nil {
				return err
			}
			paygAllowedGroupsJSON = union
		}

		updates := map[string]interface{}{}
		if paygConsume > 0 {
			updates["payg_quota"] = gorm.Expr("payg_quota - ?", paygConsume+paygDustCleared)
			updates["payg_allowed_groups"] = paygAllowedGroupsJSON
		}
		// 自由额度不设用户级日限，订阅的日限在订阅记录中单独校验

		totalDeduct := deductedFromSub + remainingAfterSub + paygDustCleared
		if totalDeduct < 0 {
			totalDeduct = 0
		}
		if totalDeduct > 0 {
			updates["quota"] = gorm.Expr("quota - ?", totalDeduct)
		}
		if len(updates) > 0 {
			if err := tx.Model(&User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				return err
			}
		}
		userDelta = totalDeduct

		return refreshUserSubscriptionSnapshot(tx, id, common.GetTimestamp())
	})
	if err != nil {
		return 0, err
	}
	return userDelta, nil
}

func sumSubscriptionRemainingByGroup(tx *gorm.DB, userId int, groupID int) (int, bool, error) {
	if tx == nil {
		tx = DB
	}
	if groupID <= 0 {
		return 0, false, errors.New("group_id 无效")
	}
	var subs []UserSubscription
	now := time.Now().Unix()
	if err := tx.Select("id", "total_quota", "remaining_quota", "source_preset_id", "source_redemption_id").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Find(&subs).Error; err != nil {
		return 0, false, err
	}
	allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
	if err != nil {
		return 0, false, err
	}
	total := 0
	hasUnlimited := false
	for _, sub := range subs {
		allowed := false
		for _, gid := range allowedBySubID[sub.Id] {
			if gid == groupID {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		if sub.TotalQuota == 0 {
			hasUnlimited = true
			continue
		}
		if sub.RemainingQuota > 0 {
			total += sub.RemainingQuota
		}
	}
	return total, hasUnlimited, nil
}

func consumeUserQuotaByBucket(id int, quota int, bucket string, usingGroupID int, paygProductId int) (selectedPaygProductId int, subscriptionAllocations []relaycommon.SubscriptionUnitAllocation, userDelta int, err error) {
	selectedPaygProductId = 0
	subscriptionAllocations = nil
	userDelta = 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		switch bucket {
		case UserQuotaBucketTokens:
			if usingGroupID <= 0 {
				return errors.New("tokens 订阅缺少分组信息")
			}
			if err := ensureUserSubscriptionsActiveAt(tx, id, now); err != nil {
				return err
			}

			covered, deducted, allocations, err := consumeTokensFromSubscriptionsByGroupWithAllocations(tx, id, quota, usingGroupID)
			if err != nil {
				return err
			}
			if covered != quota {
				remainingTotal, hasUnlimited, err := sumTokenSubscriptionRemainingByGroup(tx, id, usingGroupID)
				if err != nil {
					return err
				}
				if hasUnlimited || remainingTotal >= quota {
					return ErrUserDailyQuotaExceeded
				}
				return errors.New("tokens 额度不足")
			}

			if deducted > 0 {
				if err := tx.Model(&User{}).Where("id = ?", id).Update("tokens_quota", gorm.Expr("tokens_quota - ?", deducted)).Error; err != nil {
					return err
				}
			}
			subscriptionAllocations = allocations
			userDelta = deducted
			return nil

		case UserQuotaBucketSubscription:
			if err := ensureUserQuotaSubscriptionsReadyForConsume(tx, id, now); err != nil {
				return err
			}

			covered, deducted, allocations, err := consumeQuotaFromSubscriptionsByGroupWithAllocations(tx, id, quota, usingGroupID)
			if err != nil {
				return err
			}
			if covered != quota {
				remainingTotal, hasUnlimited, err := sumSubscriptionRemainingByGroup(tx, id, usingGroupID)
				if err != nil {
					return err
				}
				if hasUnlimited || remainingTotal >= quota {
					return ErrUserDailyQuotaExceeded
				}
				return errors.New("订阅额度不足")
			}

			if deducted > 0 {
				if err := tx.Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
					"quota":                  gorm.Expr("quota - ?", deducted),
					"redeem_quota":           gorm.Expr("CASE WHEN redeem_quota >= ? THEN redeem_quota - ? ELSE 0 END", deducted, deducted),
					"redeem_quota_expire_at": gorm.Expr("CASE WHEN redeem_quota <= ? THEN 0 ELSE redeem_quota_expire_at END", deducted),
				}).Error; err != nil {
					return err
				}
			}
			subscriptionAllocations = allocations
			userDelta = deducted
			return nil

		case UserQuotaBucketPayg:
			if usingGroupID <= 0 {
				return errors.New("按量付费缺少分组信息")
			}

			var user User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "quota", "payg_quota", "payg_history_quota", "payg_allowed_groups").
				Where("id = ?", id).First(&user).Error; err != nil {
				return err
			}
			if _, err := syncLockedUserPaygSnapshotFromBalancesTx(tx, &user); err != nil {
				return err
			}
			if user.PayAsYouGoQuota < quota {
				return errors.New("按量付费额度不足")
			}

			var balances []PaygUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "product_id", "product_name", "sort_order", "allowed_group_ids", "override_allowed_group_ids", "remaining_quota").
				Where("user_id = ? AND remaining_quota > 0", id).
				Order("sort_order DESC, product_id DESC, id DESC").
				Find(&balances).Error; err != nil {
				return err
			}
			if len(balances) == 0 {
				return errors.New("按量付费额度不足")
			}

			type candidate struct {
				Idx int
			}
			var chosen *candidate
			if paygProductId != 0 {
				for i := range balances {
					if balances[i].ProductId != paygProductId {
						continue
					}
					ids, err := resolvePaygBalanceAllowedGroupIDs(balances[i])
					if err != nil {
						return err
					}
					allowed := false
					for _, gid := range ids {
						if gid == usingGroupID {
							allowed = true
							break
						}
					}
					if !allowed {
						return errors.New("按量付费商品分组不匹配")
					}
					if balances[i].RemainingQuota < quota {
						return errors.New("按量付费额度不足")
					}
					chosen = &candidate{Idx: i}
					break
				}
				if chosen == nil {
					return errors.New("按量付费商品余额不存在")
				}
			} else {
				for i := range balances {
					if balances[i].RemainingQuota < quota {
						continue
					}
					ids, err := resolvePaygBalanceAllowedGroupIDs(balances[i])
					if err != nil {
						return err
					}
					for _, gid := range ids {
						if gid == usingGroupID {
							chosen = &candidate{Idx: i}
							break
						}
					}
					if chosen != nil {
						break
					}
				}
				if chosen == nil {
					return errors.New("按量付费额度不足")
				}
			}

			target := balances[chosen.Idx]
			if err := tx.Model(&PaygUserBalance{}).Where("id = ?", target.Id).
				Update("remaining_quota", gorm.Expr("remaining_quota - ?", quota)).Error; err != nil {
				return err
			}
			selectedPaygProductId = target.ProductId

			// Update union groups after consuming.
			balances[chosen.Idx].RemainingQuota = target.RemainingQuota - quota
			dustCleared, err := clearPaygDustFromBalancesTx(tx, balances, common.PreConsumedQuota)
			if err != nil {
				return err
			}
			unionGroupsJSON, err := UnionPaygAllowedGroupsFromBalances(balances)
			if err != nil {
				return err
			}

			totalDeduct := quota + dustCleared
			userDelta = totalDeduct
			return tx.Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
				"payg_quota":          gorm.Expr("payg_quota - ?", totalDeduct),
				"quota":               gorm.Expr("quota - ?", totalDeduct),
				"payg_allowed_groups": unionGroupsJSON,
			}).Error

		case UserQuotaBucketPayToken:
			if usingGroupID <= 0 {
				return errors.New("按token付费缺少分组信息")
			}

			var user User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "pay_token_quota", "pay_token_history_quota", "pay_token_allowed_groups").
				Where("id = ?", id).First(&user).Error; err != nil {
				return err
			}
			if _, err := syncLockedUserPayTokenSnapshotFromBalancesTx(tx, &user); err != nil {
				return err
			}
			if user.PayTokenQuota < quota {
				return errors.New("按token付费余额不足")
			}

			var balances []PayTokenUserBalance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "product_id", "product_name", "sort_order", "allowed_group_ids", "remaining_tokens").
				Where("user_id = ? AND remaining_tokens > 0", id).
				Order("sort_order DESC, product_id DESC, id DESC").
				Find(&balances).Error; err != nil {
				return err
			}
			if len(balances) == 0 {
				return errors.New("按token付费余额不足")
			}

			type candidate struct {
				Idx int
			}
			var chosen *candidate
			if paygProductId != 0 {
				for i := range balances {
					if balances[i].ProductId != paygProductId {
						continue
					}
					ids, err := resolvePayTokenBalanceAllowedGroupIDs(balances[i])
					if err != nil {
						return err
					}
					allowed := false
					for _, gid := range ids {
						if gid == usingGroupID {
							allowed = true
							break
						}
					}
					if !allowed {
						return errors.New("按token付费商品分组不匹配")
					}
					if balances[i].RemainingTokens < quota {
						return errors.New("按token付费余额不足")
					}
					chosen = &candidate{Idx: i}
					break
				}
				if chosen == nil {
					return errors.New("按token付费商品余额不存在")
				}
			} else {
				for i := range balances {
					if balances[i].RemainingTokens < quota {
						continue
					}
					ids, err := resolvePayTokenBalanceAllowedGroupIDs(balances[i])
					if err != nil {
						return err
					}
					for _, gid := range ids {
						if gid == usingGroupID {
							chosen = &candidate{Idx: i}
							break
						}
					}
					if chosen != nil {
						break
					}
				}
				if chosen == nil {
					return errors.New("按token付费余额不足")
				}
			}

			target := balances[chosen.Idx]
			if err := tx.Model(&PayTokenUserBalance{}).Where("id = ?", target.Id).
				Update("remaining_tokens", gorm.Expr("remaining_tokens - ?", quota)).Error; err != nil {
				return err
			}
			selectedPaygProductId = target.ProductId

			// Update union groups after consuming.
			balances[chosen.Idx].RemainingTokens = target.RemainingTokens - quota
			unionGroupsJSON, err := UnionPayTokenAllowedGroupsFromBalances(balances)
			if err != nil {
				return err
			}

			userDelta = quota
			return tx.Model(&User{}).Where("id = ?", id).Updates(map[string]interface{}{
				"pay_token_quota":          gorm.Expr("pay_token_quota - ?", quota),
				"pay_token_allowed_groups": unionGroupsJSON,
			}).Error

		case UserQuotaBucketFree:
			var user User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("quota", "redeem_quota", "payg_quota").
				Where("id = ?", id).First(&user).Error; err != nil {
				return err
			}
			if user.Quota < quota {
				return errors.New("用户额度不足")
			}
			freeRemaining := user.Quota - user.RedeemQuota - user.PayAsYouGoQuota
			if freeRemaining < 0 {
				freeRemaining = 0
			}
			if freeRemaining < quota {
				return errors.New("用户额度不足")
			}
			userDelta = quota
			return tx.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
		default:
			return errors.New("bucket 无效")
		}
	})
	return selectedPaygProductId, subscriptionAllocations, userDelta, err
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	UpdateUserUsageAndRequestCount(id, quota, quota, quota)
}

func UpdateUserUsageAndRequestCount(id int, quota int, visibleQuota int, costQuota int) {
	UpdateUserUsageMetrics(id, quota, visibleQuota, costQuota, 1)
}

func UpdateUserUsageQuotas(id int, quota int, visibleQuota int, costQuota int) {
	UpdateUserUsageMetrics(id, quota, visibleQuota, costQuota, 0)
}

func UpdateUserUsageMetrics(id int, quota int, visibleQuota int, costQuota int, count int) {
	if common.BatchUpdateEnabled {
		if quota != 0 {
			addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		}
		if visibleQuota != 0 {
			addNewRecord(BatchUpdateTypeVisibleUsedQuota, id, visibleQuota)
		}
		if costQuota != 0 {
			addNewRecord(BatchUpdateTypeCostUsedQuota, id, costQuota)
		}
		if count != 0 {
			addNewRecord(BatchUpdateTypeRequestCount, id, count)
		}
		return
	}
	updateUserUsageAndRequestCount(id, quota, visibleQuota, costQuota, count)
}

func updateUserUsageAndRequestCount(id int, quota int, visibleQuota int, costQuota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":         gorm.Expr("used_quota + ?", quota),
			"visible_used_quota": gorm.Expr("visible_used_quota + ?", visibleQuota),
			"cost_used_quota":    gorm.Expr("cost_used_quota + ?", costQuota),
			"request_count":      gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserVisibleUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"visible_used_quota": gorm.Expr("visible_used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user visible used quota: " + err.Error())
	}
}

func updateUserCostUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"cost_used_quota": gorm.Expr("cost_used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user cost used quota: " + err.Error())
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog("failed to update user name cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}

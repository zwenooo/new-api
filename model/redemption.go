package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"one-api/logger"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type Redemption struct {
	Id                int            `json:"id"`
	UserId            int            `json:"user_id"`
	Key               string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status            int            `json:"status" gorm:"default:1"`
	Name              string         `json:"name" gorm:"index"`
	Mode              string         `json:"mode" gorm:"type:varchar(32);default:'subscription';column:mode"`
	PriceFen          int64          `json:"price_fen" gorm:"type:bigint;default:0;column:price_fen"` // 兑换码对应的结算金额（分），用于分销/返佣
	InviterId         int            `json:"inviter_id" gorm:"type:int;default:0;index;column:inviter_id"`
	CommissionPercent int            `json:"commission_percent" gorm:"type:int;default:0;column:commission_percent"`
	CommissionFen     int64          `json:"commission_fen" gorm:"type:bigint;default:0;column:commission_fen"`
	IsFirstPurchase   bool           `json:"is_first_purchase" gorm:"type:boolean;default:false;column:is_first_purchase"`
	Quota             int            `json:"quota" gorm:"default:100"`
	DailyQuotaLimit   int            `json:"daily_quota_limit" gorm:"default:0"`
	DailyRequestLimit int            `json:"daily_request_limit" gorm:"default:0;column:daily_request_limit"`
	CreatedTime       int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime      int64          `json:"redeemed_time" gorm:"bigint"`
	Count             int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId        int            `json:"used_user_id"`
	DeletedAt         gorm.DeletedAt `gorm:"index"`
	ExpiredTime       int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	// QuotaValidDays 表示订阅额度的有效期（按天）。
	// 对于订阅类兑换码（每日额度 > 0）：
	//   - 0 表示仅在兑换当日有效，到当日结束（约 23:59:59）过期；
	//   - >0 表示从兑换时刻起累加对应天数。
	QuotaValidDays int `json:"quota_valid_days" gorm:"default:0"`

	// Deprecated retired fields kept for schema compatibility only.
	PlanValidDays int       `json:"plan_valid_days" gorm:"default:0;column:plan_valid_days"`
	ChannelIds    JSONValue `json:"channel_ids" gorm:"type:json;column:channel_ids"`
	// AllowedGroups limits which channel groups (tiers) this redemption can be consumed from.
	AllowedGroups   JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	AllowedGroupIds JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
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

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	for _, redemption := range redemptions {
		NormalizeCompatibleRedemptionMode(redemption)
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	for _, redemption := range redemptions {
		NormalizeCompatibleRedemptionMode(redemption)
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	NormalizeCompatibleRedemptionMode(&redemption)
	return &redemption, err
}

type RedeemResult struct {
	Mode       string `json:"mode"`
	AddedQuota int    `json:"added_quota"`
}

func resolveRedeemMode(redemption *Redemption) (string, error) {
	if redemption == nil {
		return "", errors.New("兑换码为空")
	}
	mode := ResolveCompatibleRedemptionMode(redemption)
	if mode == "" {
		return "", errors.New("该兑换码类型已下线或缺少显式 mode")
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

func RedeemDetail(key string, userId int, applyMode string) (*RedeemResult, error) {
	if key == "" {
		return nil, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return nil, errors.New("无效的 user id")
	}
	if applyMode == "" {
		applyMode = SubscriptionApplyModeStack
	}
	if applyMode != SubscriptionApplyModeStack && applyMode != SubscriptionApplyModeDefer {
		return nil, errors.New("apply_mode 无效")
	}
	redemption := &Redemption{}
	result := &RedeemResult{}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	inviterId := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error; err != nil {
			return errors.New("无效的兑换码")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}

		mode, err := resolveRedeemMode(redemption)
		if err != nil {
			return err
		}

		switch mode {
		case "activation":
			return errors.New("该激活码仅用于 ClawBox 首次激活")
		case "request":
			if redemption.DailyRequestLimit < 0 {
				return errors.New("次数订阅兑换码每日次数无效")
			}
			if redemption.Quota < 0 {
				return errors.New("次数订阅兑换码总次数无效")
			}
			if len(redemption.AllowedGroupIds) == 0 {
				return errors.New("次数订阅兑换码缺少可用分组")
			}

			now := time.Now().Unix()
			if now <= 0 || now > common.MaxSupportedUnixTimestamp {
				return errors.New("系统时间异常")
			}

			startAt := now
			if applyMode == SubscriptionApplyModeDefer {
				maxExpire, err := GetUserRequestSubscriptionMaxExpireAt(tx, userId, now)
				if err != nil {
					return err
				}
				if maxExpire >= startAt {
					startAt = maxExpire + 1
				}
			}
			if startAt > common.MaxSupportedUnixTimestamp {
				return errors.New("订阅开始时间过大")
			}
			if redemption.QuotaValidDays < 0 {
				return errors.New("订阅有效期无效")
			}

			var expireAt int64
			if redemption.QuotaValidDays > 0 {
				extendSeconds := int64(redemption.QuotaValidDays) * common.SecondsPerDay
				if extendSeconds > common.MaxSupportedUnixTimestamp-startAt {
					return errors.New("订阅有效期过大")
				}
				expireAt = startAt + extendSeconds
			} else if redemption.QuotaValidDays == 0 {
				// 0 表示永久
				expireAt = 0
			}

			var groupIDs []int
			if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &groupIDs); err != nil {
				return errors.New("可用分组解析失败")
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return errors.New("次数订阅兑换码缺少可用分组")
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return err
			}

			if _, err := CreateUserRequestSubscriptionTx(
				tx,
				userId,
				startAt,
				float64(redemption.DailyRequestLimit),
				float64(redemption.Quota),
				expireAt,
				groupIDs,
				fmt.Sprintf("redeem:%d", redemption.Id),
				UserRequestSubscriptionSourceRef{RedemptionId: redemption.Id},
			); err != nil {
				return err
			}

			result.Mode = "request"
			result.AddedQuota = 0

		case "payg":
			if redemption.Quota <= 0 {
				return errors.New("按量付费兑换码额度必须大于 0")
			}
			if len(redemption.AllowedGroupIds) == 0 {
				return errors.New("按量付费兑换码缺少可用分组")
			}

			var user User
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, "id = ?", userId).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("用户不存在")
				}
				return err
			}

			var groupIDs []int
			if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &groupIDs); err != nil {
				return errors.New("可用分组解析失败")
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return errors.New("按量付费兑换码缺少可用分组")
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return err
			}

			// PAYG redemption codes are credited into the legacy PAYG balance item (product_id=-1).
			if err := UpsertPaygUserBalanceTx(tx, userId, -1, "", 0, groupIDs, redemption.Quota); err != nil {
				return err
			}

			// Rebuild union groups for payg_allowed_groups from positive balances.
			balances, err := GetUserPaygBalancesTx(tx, userId, true)
			if err != nil {
				return err
			}
			unionGroupsJSON, err := UnionPaygAllowedGroupsFromBalances(balances)
			if err != nil {
				return err
			}

			if err := tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
				"payg_quota":          gorm.Expr("payg_quota + ?", redemption.Quota),
				"payg_history_quota":  gorm.Expr("payg_history_quota + ?", redemption.Quota),
				"payg_allowed_groups": unionGroupsJSON,
				"quota":               gorm.Expr("quota + ?", redemption.Quota),
			}).Error; err != nil {
				return err
			}

			result.Mode = "payg"
			result.AddedQuota = redemption.Quota

		case "pay_request":
			if redemption.Quota <= 0 {
				return errors.New("按次付费兑换码次数必须大于 0")
			}
			if len(redemption.AllowedGroupIds) == 0 {
				return errors.New("按次付费兑换码缺少可用分组")
			}

			var user User
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, "id = ?", userId).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("用户不存在")
				}
				return err
			}

			var groupIDs []int
			if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &groupIDs); err != nil {
				return errors.New("可用分组解析失败")
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return errors.New("按次付费兑换码缺少可用分组")
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return err
			}

			// Pay-request redemption codes are credited into the legacy pay-request balance item (product_id=-1).
			if err := UpsertPayRequestUserBalanceTx(tx, userId, -1, "", 0, groupIDs, redemption.Quota); err != nil {
				return err
			}

			if _, err := SyncUserPayRequestSnapshotFromBalancesTx(tx, userId); err != nil {
				return err
			}

			result.Mode = "pay_request"
			result.AddedQuota = redemption.Quota

		case "pay_token":
			if redemption.Quota <= 0 {
				return errors.New("按token付费兑换码tokens必须大于 0")
			}
			if len(redemption.AllowedGroupIds) == 0 {
				return errors.New("按token付费兑换码缺少可用分组")
			}

			var user User
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, "id = ?", userId).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("用户不存在")
				}
				return err
			}

			var groupIDs []int
			if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &groupIDs); err != nil {
				return errors.New("可用分组解析失败")
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return errors.New("按token付费兑换码缺少可用分组")
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return err
			}

			// Pay-token redemption codes are credited into the legacy pay-token balance item (product_id=-1).
			if err := UpsertPayTokenUserBalanceTx(tx, userId, -1, "", 0, groupIDs, redemption.Quota); err != nil {
				return err
			}

			if _, err := SyncUserPayTokenSnapshotFromBalancesTx(tx, userId); err != nil {
				return err
			}

			result.Mode = "pay_token"
			result.AddedQuota = redemption.Quota

		case "subscription", "tokens":
			if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
				return err
			}

			now := time.Now().Unix()
			if now <= 0 || now > common.MaxSupportedUnixTimestamp {
				return errors.New("系统时间异常")
			}

			startAt := now
			if applyMode == SubscriptionApplyModeDefer {
				billingUnit := UserSubscriptionBillingUnitQuota
				if mode == "tokens" {
					billingUnit = UserSubscriptionBillingUnitTokens
				}
				maxExpire, err := GetUserSubscriptionMaxExpireAtWithBillingUnit(tx, userId, now, billingUnit)
				if err != nil {
					return err
				}
				if maxExpire >= startAt {
					startAt = maxExpire + 1
				}
			}

			if startAt > common.MaxSupportedUnixTimestamp {
				return errors.New("订阅开始时间过大")
			}
			if redemption.QuotaValidDays < 0 {
				return errors.New("订阅有效期无效")
			}

			var expireAt int64
			if redemption.QuotaValidDays > 0 {
				extendSeconds := int64(redemption.QuotaValidDays) * common.SecondsPerDay
				if extendSeconds > common.MaxSupportedUnixTimestamp-startAt {
					return errors.New("订阅有效期过大")
				}
				expireAt = startAt + extendSeconds
			} else if redemption.QuotaValidDays == 0 {
				expireAt = 0
			}

			if len(redemption.AllowedGroupIds) == 0 {
				return errors.New("订阅兑换码缺少可用分组")
			}
			var groupIDs []int
			if err := common.Unmarshal([]byte(redemption.AllowedGroupIds), &groupIDs); err != nil {
				return errors.New("可用分组解析失败")
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return errors.New("订阅兑换码缺少可用分组")
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return err
			}
			billingUnit := UserSubscriptionBillingUnitQuota
			if mode == "tokens" {
				billingUnit = UserSubscriptionBillingUnitTokens
			}
			if _, err := createUserSubscription(
				tx,
				userId,
				startAt,
				redemption.Quota,
				redemption.Quota,
				redemption.DailyQuotaLimit,
				expireAt,
				groupIDs,
				billingUnit,
				fmt.Sprintf("redeem:%d", redemption.Id),
				UserSubscriptionSourceRef{RedemptionId: redemption.Id},
			); err != nil {
				return err
			}
			result.Mode = mode
			result.AddedQuota = redemption.Quota
		default:
			return errors.New("无效的兑换码类型")
		}

		// 邀请返利：按兑换码的结算金额（分）计算分佣，并记录到兑换码上用于后续查询展示。
		if redemption.PriceFen > 0 {
			paidCount, err := CountUserSuccessfulCommissionablePaidEventsTx(tx, userId)
			if err != nil {
				return err
			}
			isFirstPaid := paidCount == 0
			redemption.IsFirstPurchase = isFirstPaid

			inviterId, commissionPercent, commissionFen, err := ApplyInvitationCommissionTx(tx, userId, redemption.PriceFen, isFirstPaid)
			if err != nil {
				return err
			}
			redemption.InviterId = inviterId
			redemption.CommissionPercent = commissionPercent
			redemption.CommissionFen = commissionFen
		}

		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		return tx.Save(redemption).Error
	})
	if err != nil {
		return nil, errors.New("兑换失败，" + err.Error())
	}

	switch result.Mode {
	case "request":
		dailyLabel := "不限"
		if redemption.DailyRequestLimit > 0 {
			dailyLabel = fmt.Sprintf("%d", redemption.DailyRequestLimit)
		}
		totalLabel := "不限"
		if redemption.Quota > 0 {
			totalLabel = fmt.Sprintf("%d", redemption.Quota)
		}
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码开通次数订阅：每日 %s 次，总 %s 次，有效期 %d 天，兑换码ID %d", dailyLabel, totalLabel, redemption.QuotaValidDays, redemption.Id))
	case "pay_request":
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值按次付费次数 %d 次，兑换码ID %d", redemption.Quota, redemption.Id))
	case "pay_token":
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值按token付费tokens %d，兑换码ID %d", redemption.Quota, redemption.Id))
	default:
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	}

	// 更新缓存：使得新额度和到期时间尽快对前端/限额检查可见
	_ = invalidateUserCache(userId)
	if inviterId > 0 {
		_ = invalidateUserCache(inviterId)
	}
	return result, nil
}

func Redeem(key string, userId int) (quota int, err error) {
	result, err := RedeemDetail(key, userId, "")
	if err != nil {
		return 0, err
	}
	return result.AddedQuota, nil
}

// RedeemWithClearPermanent 为历史兼容接口，现阶段不会再清空用户的不限时额度。
// 返回值维持原有签名，其中 cleared 恒为 0，避免旧调用方出错。
func RedeemWithClearPermanent(key string, userId int) (quota int, cleared int, err error) {
	quota, err = Redeem(key, userId)
	return quota, 0, err
}

func fetchActivationCode(tx *gorm.DB, key string, forUpdate bool) (*Redemption, error) {
	if key == "" {
		return nil, errors.New("未提供激活码")
	}
	if tx == nil {
		tx = DB
	}
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	query := tx
	if forUpdate {
		query = tx.Set("gorm:query_option", "FOR UPDATE")
	}
	redemption := &Redemption{}
	if err := query.Where(keyCol+" = ?", key).First(redemption).Error; err != nil {
		return nil, errors.New("无效的激活码")
	}
	if redemption.Mode != "activation" {
		return nil, errors.New("无效的激活码")
	}
	if redemption.Status != common.RedemptionCodeStatusEnabled {
		return nil, errors.New("该激活码已被使用")
	}
	if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
		return nil, errors.New("该激活码已过期")
	}
	return redemption, nil
}

func consumeActivationCodeTx(tx *gorm.DB, key string, usedUserId int) error {
	if tx == nil {
		return errors.New("db transaction is required")
	}
	common.RandomSleep()
	redemption, err := fetchActivationCode(tx, key, true)
	if err != nil {
		return err
	}
	redemption.RedeemedTime = common.GetTimestamp()
	redemption.Status = common.RedemptionCodeStatusUsed
	redemption.UsedUserId = usedUserId
	return tx.Model(redemption).
		Select("redeemed_time", "status", "used_user_id").
		Updates(redemption).
		Error
}

func ValidateActivationCode(key string) error {
	_, err := fetchActivationCode(DB, key, false)
	return err
}

func ConsumeActivationCode(key string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return consumeActivationCodeTx(tx, key, 0)
	})
}

func ConsumeActivationCodeWithUser(tx *gorm.DB, key string, usedUserId int) error {
	return consumeActivationCodeTx(tx, key, usedUserId)
}

func (redemption *Redemption) Insert() error {
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "mode", "price_fen", "quota", "daily_quota_limit", "daily_request_limit", "redeemed_time", "expired_time", "quota_valid_days", "plan_valid_days", "channel_ids", "allowed_groups", "allowed_group_ids").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

// BatchUpdateRedemptionsStatus 批量更新兑换码状态，返回成功更新的数量
func BatchUpdateRedemptionsStatus(ids []int, status int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	result := DB.Model(&Redemption{}).Where("id IN ?", ids).Update("status", status)
	if result.Error != nil {
		return 0, result.Error
	}

	return int(result.RowsAffected), nil
}

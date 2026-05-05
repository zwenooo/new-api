package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"strings"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Token struct {
	Id                  int            `json:"id"`
	UserId              int            `json:"user_id" gorm:"index"`
	Key                 string         `json:"key" gorm:"type:char(48);uniqueIndex"`
	Status              int            `json:"status" gorm:"default:1"`
	Name                string         `json:"name" gorm:"index" `
	CreatedTime         int64          `json:"created_time" gorm:"bigint"`
	AccessedTime        int64          `json:"accessed_time" gorm:"bigint"`
	ExpiredTime         int64          `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota         int            `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota      bool           `json:"unlimited_quota"`
	ModelLimitsEnabled  bool           `json:"model_limits_enabled"`
	ModelLimits         string         `json:"model_limits" gorm:"type:varchar(1024);default:''"`
	AllowIps            *string        `json:"allow_ips" gorm:"default:''"`
	UsedQuota           int            `json:"used_quota" gorm:"default:0"`                               // used quota
	DailyQuotaLimit     int            `json:"daily_quota_limit" gorm:"type:int;default:0"`               // 0 ?????
	DailyQuotaUsed      int            `json:"daily_quota_used" gorm:"type:int;default:0"`                // ??????
	DailyQuotaResetDate int            `json:"-" gorm:"type:int;default:0;column:daily_quota_reset_date"` // YYYYMMDD???????????
	Group               string         `json:"group" gorm:"default:''"`
	GroupId             int            `json:"group_id" gorm:"-"`
	DefaultGroupId      int            `json:"default_group_id,omitempty" gorm:"type:int;default:0;index;column:default_group_id"`
	AllowedGroups       JSONValue      `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	AllowedGroupIds     JSONValue      `json:"allowed_group_ids" gorm:"-"`
	DeletedAt           gorm.DeletedAt `gorm:"index"`
}

func (token *Token) Clean() {
	token.Key = ""
}

func (token *Token) GetIpLimitsMap() map[string]any {
	// delete empty spaces
	//split with \n
	ipLimitsMap := make(map[string]any)
	if token.AllowIps == nil {
		return ipLimitsMap
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimitsMap
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if common.IsIP(ip) {
			ipLimitsMap[ip] = true
		}
	}
	return ipLimitsMap
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = DB.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

func SearchUserTokens(userId int, keyword string, token string) (tokens []*Token, err error) {
	if token != "" {
		token = strings.Trim(token, "sk-")
	}
	err = DB.Where("user_id = ?", userId).Where("name LIKE ?", "%"+keyword+"%").Where(commonKeyCol+" LIKE ?", "%"+token+"%").Find(&tokens).Error
	return tokens, err
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, errors.New("未提供令牌")
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted {
			keyPrefix := key[:3]
			keySuffix := key[len(key)-3:]
			return token, errors.New("该令牌额度已用尽 TokenStatusExhausted[sk-" + keyPrefix + "***" + keySuffix + "]")
		} else if token.Status == common.TokenStatusExpired {
			return token, errors.New("该令牌已过期")
		}
		if token.Status != common.TokenStatusEnabled {
			return token, errors.New("该令牌状态不可用")
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, errors.New("该令牌已过期")
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				// in this case, we can make sure the token is exhausted
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			keyPrefix := key[:3]
			keySuffix := key[len(key)-3:]
			return token, errors.New(fmt.Sprintf("[sk-%s***%s] 该令牌额度已用尽 !token.UnlimitedQuota && token.RemainQuota = %d", keyPrefix, keySuffix, token.RemainQuota))
		}
		return token, nil
	}
	return nil, errors.New("无效的令牌")
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	if err == nil {
		_ = hydrateTokenGroupFields(&token)
	}
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	if err == nil {
		_ = hydrateTokenGroupFields(&token)
	}
	if shouldUpdateRedis(true, err) {
		gopool.Go(func() {
			if err := cacheSetToken(token); err != nil {
				common.SysLog("failed to update user status cache: " + err.Error())
			}
		})
	}
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			gopool.Go(func() {
				if err := cacheSetToken(*token); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			switch token.Status {
			case common.TokenStatusEnabled, common.TokenStatusDisabled, common.TokenStatusExpired, common.TokenStatusExhausted:
				return token, nil
			default:
				common.SysLog(fmt.Sprintf("token cache status invalid, refreshing from DB: token_id=%d user_id=%d status=%d", token.Id, token.UserId, token.Status))
				if delErr := cacheDeleteToken(key); delErr != nil {
					common.SysLog("failed to delete invalid token cache: " + delErr.Error())
				}
			}
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Where(commonKeyCol+" = ?", key).First(&token).Error
	if err == nil {
		_ = hydrateTokenGroupFields(token)
	}
	return token, err
}

func hydrateTokenGroupFields(token *Token) error {
	if token == nil || token.Id <= 0 || DB == nil {
		return nil
	}
	groupIDs, err := getTokenAllowedGroupIDsTx(DB, token.Id)
	if err != nil {
		return err
	}
	if len(groupIDs) > 0 {
		if b, err := common.Marshal(groupIDs); err == nil {
			token.AllowedGroupIds = JSONValue(b)
		}
		codes, err := GroupCodesFromIDs(DB, groupIDs)
		if err != nil {
			return err
		}
		groupsJSON, err := MarshalGroupNamesJSON(codes)
		if err != nil {
			return err
		}
		token.AllowedGroups = groupsJSON
	}
	primaryGroupID := FirstGroupIDKeepOrder(groupIDs)
	token.DefaultGroupId = 0
	if primaryGroupID > 0 {
		token.GroupId = primaryGroupID
		group, err := GetGroupByID(DB, primaryGroupID)
		if err == nil && group != nil {
			token.Group = group.Code
		}
	} else {
		token.GroupId = 0
		token.Group = ""
	}
	return nil
}

func FillTokensAllowedGroupIDsTx(tx *gorm.DB, tokens []*Token) error {
	if len(tokens) == 0 {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	tokenIDs := make([]int, 0, len(tokens))
	seen := make(map[int]struct{}, len(tokens))
	for _, t := range tokens {
		if t == nil || t.Id <= 0 {
			continue
		}
		if _, ok := seen[t.Id]; ok {
			continue
		}
		seen[t.Id] = struct{}{}
		tokenIDs = append(tokenIDs, t.Id)
	}
	tokenIDs = normalizeUniqueSortedIDs(tokenIDs)
	if len(tokenIDs) == 0 {
		return nil
	}

	type row struct {
		TokenId   int `gorm:"column:token_id"`
		GroupId   int `gorm:"column:group_id"`
		SortOrder int `gorm:"column:sort_order"`
	}
	var rows []row
	if err := tx.Model(&TokenAllowedGroup{}).
		Select("token_id", "group_id", "sort_order").
		Where("token_id IN ?", tokenIDs).
		Order("token_id ASC").
		Order("sort_order ASC").
		Order("group_id ASC").
		Find(&rows).Error; err != nil {
		return err
	}
	byToken := make(map[int][]int, len(tokenIDs))
	for _, r := range rows {
		if r.TokenId <= 0 || r.GroupId <= 0 {
			continue
		}
		byToken[r.TokenId] = append(byToken[r.TokenId], r.GroupId)
	}

	for _, t := range tokens {
		if t == nil || t.Id <= 0 {
			continue
		}
		ids := normalizeUniquePositiveIDsKeepOrder(byToken[t.Id])
		if len(ids) == 0 {
			return fmt.Errorf("token#%d 缺少可用分组", t.Id)
		}
		b, err := MarshalGroupIDsJSONKeepOrder(ids)
		if err != nil {
			return err
		}
		t.AllowedGroupIds = b
		t.DefaultGroupId = 0
		t.GroupId = FirstGroupIDKeepOrder(ids)
	}

	return nil
}

func FillTokensAllowedGroupIDs(tokens []*Token) error {
	return FillTokensAllowedGroupIDsTx(DB, tokens)
}

func (token *Token) Insert() error {
	if token == nil {
		return errors.New("token 为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		token.DefaultGroupId = 0
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		groupIDs, err := ParseGroupIDsJSONKeepOrder(token.AllowedGroupIds)
		if err != nil {
			return err
		}
		if len(groupIDs) == 0 {
			return errors.New("当前令牌分组为空")
		}
		if err := upsertTokenAllowedGroupsTx(tx, token.Id, groupIDs); err != nil {
			return err
		}
		return nil
	})
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Transaction(func(tx *gorm.DB) error {
		token.DefaultGroupId = 0
		if err := tx.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
			"model_limits_enabled", "model_limits", "daily_quota_limit", "allow_ips", "default_group_id").Updates(token).Error; err != nil {
			return err
		}
		groupIDs, err := ParseGroupIDsJSONKeepOrder(token.AllowedGroupIds)
		if err != nil {
			return err
		}
		if len(groupIDs) == 0 {
			return errors.New("当前令牌分组为空")
		}
		if err := upsertTokenAllowedGroupsTx(tx, token.Id, groupIDs); err != nil {
			return err
		}
		return nil
	})
	return err
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Delete(token).Error
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	if err := restoreTokenQuota(id, quota); err != nil {
		return err
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheIncrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to increase token quota: " + err.Error())
			}
		})
	}
	if err := cacheDeleteToken(key); err != nil {
		common.SysLog("failed to invalidate token cache: " + err.Error())
	}
	return nil
}

func restoreTokenQuota(id int, quota int) (err error) {
	if quota <= 0 {
		return nil
	}
	now := common.GetTimestamp()
	today := common.GetTodayDateInt()

	updates := map[string]interface{}{
		"remain_quota":  gorm.Expr("remain_quota + ?", quota),
		"used_quota":    gorm.Expr("CASE WHEN used_quota >= ? THEN used_quota - ? ELSE 0 END", quota, quota),
		"accessed_time": now,
		"daily_quota_used": gorm.Expr(
			"CASE WHEN daily_quota_limit > 0 THEN (CASE WHEN daily_quota_reset_date = ? THEN (CASE WHEN daily_quota_used >= ? THEN daily_quota_used - ? ELSE 0 END) ELSE 0 END) ELSE daily_quota_used END",
			today, quota, quota,
		),
		"daily_quota_reset_date": gorm.Expr("CASE WHEN daily_quota_limit > 0 THEN ? ELSE daily_quota_reset_date END", today),
	}
	return DB.Model(&Token{}).Where("id = ?", id).Updates(updates).Error
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	if err := consumeTokenQuota(id, quota); err != nil {
		if errors.Is(err, ErrTokenDailyQuotaExceeded) {
			if errDelete := cacheDeleteToken(key); errDelete != nil {
				common.SysLog("failed to invalidate token cache after daily quota exceeded: " + errDelete.Error())
			}
		}
		return err
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheDecrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to decrease token quota: " + err.Error())
			}
		})
	}
	if err := cacheDeleteToken(key); err != nil {
		common.SysLog("failed to invalidate token cache: " + err.Error())
	}
	return nil
}

func consumeTokenQuota(id int, quota int) (err error) {
	if quota <= 0 {
		return nil
	}
	now := common.GetTimestamp()
	today := common.GetTodayDateInt()

	updates := map[string]interface{}{
		"remain_quota": gorm.Expr(
			"CASE WHEN unlimited_quota THEN remain_quota - ? WHEN remain_quota >= ? THEN remain_quota - ? ELSE 0 END",
			quota, quota, quota,
		),
		"used_quota":    gorm.Expr("used_quota + ?", quota),
		"accessed_time": now,
		"daily_quota_used": gorm.Expr(
			"CASE WHEN daily_quota_limit > 0 THEN (CASE WHEN daily_quota_reset_date = ? THEN daily_quota_used + ? ELSE ? END) ELSE daily_quota_used END",
			today, quota, quota,
		),
		"daily_quota_reset_date": gorm.Expr("CASE WHEN daily_quota_limit > 0 THEN ? ELSE daily_quota_reset_date END", today),
	}

	tx := DB.Model(&Token{}).
		Where("id = ?", id).
		Where("daily_quota_limit <= 0 OR ((CASE WHEN daily_quota_reset_date = ? THEN daily_quota_used ELSE 0 END) + ? <= daily_quota_limit)", today, quota).
		Updates(updates)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected > 0 {
		return nil
	}

	// Distinguish daily-limit exceeded from other unexpected cases.
	var token Token
	if err := DB.Model(&Token{}).
		Select("id", "daily_quota_limit", "daily_quota_used", "daily_quota_reset_date").
		Where("id = ?", id).First(&token).Error; err != nil {
		return err
	}
	if token.DailyQuotaLimit > 0 {
		usedDaily := token.DailyQuotaUsed
		if token.DailyQuotaResetDate != today {
			usedDaily = 0
		}
		remaining := token.DailyQuotaLimit - usedDaily
		if remaining <= 0 || quota > remaining {
			// Best-effort mark the token as exhausted for today.
			_ = DB.Model(&Token{}).Where("id = ?", id).Updates(map[string]interface{}{
				"daily_quota_used":       token.DailyQuotaLimit,
				"daily_quota_reset_date": today,
				"accessed_time":          now,
			}).Error
			return ErrTokenDailyQuotaExceeded
		}
	}
	return errors.New("failed to consume token quota")
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()

	var tokens []Token
	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}

package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"one-api/setting"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	ClawBoxRelayTokenName           = "ClawBox"
	ClawBoxProductModeEnabledOption = "ClawBoxProductModeEnabled"
	ClawBoxProductIdOption          = "ClawBoxProductId"
)

func isClawBoxProductModeEnabled() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.EqualFold(strings.TrimSpace(common.OptionMap[ClawBoxProductModeEnabledOption]), "true")
}

func ClawBoxProductModeEnabled() bool {
	return isClawBoxProductModeEnabled()
}

func configuredClawBoxProductID() int {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	value := strings.TrimSpace(common.OptionMap[ClawBoxProductIdOption])
	if value == "" {
		return 0
	}
	productID, err := strconv.Atoi(value)
	if err != nil || productID <= 0 {
		return 0
	}
	return productID
}

func resolveClawBoxProductIDTx(tx *gorm.DB) (int, error) {
	configured := configuredClawBoxProductID()
	if configured > 0 {
		var product PaygProduct
		if err := tx.Where("id = ?", configured).First(&product).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, fmt.Errorf("ClawBox 商品 #%d 不存在", configured)
			}
			return 0, err
		}
		if !product.Enabled {
			return 0, fmt.Errorf("ClawBox 商品 #%d 已下架", configured)
		}
		return configured, nil
	}

	var products []PaygProduct
	if err := tx.Where("enabled = ?", true).Order("sort_order DESC, id ASC").Find(&products).Error; err != nil {
		return 0, err
	}
	if len(products) == 0 {
		return 0, errors.New("当前没有可用的按量付费商品")
	}
	if len(products) > 1 {
		return 0, errors.New("ClawBox 商品模式已开启，但未指定 ClawBox 商品 ID，且当前存在多个按量付费商品")
	}
	return products[0].Id, nil
}

func ResolveClawBoxProductIDTx(tx *gorm.DB) (int, error) {
	if tx == nil {
		tx = DB
	}
	return resolveClawBoxProductIDTx(tx)
}

func ValidateClawBoxProductModeConfigTx(tx *gorm.DB) error {
	if !isClawBoxProductModeEnabled() {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	groupIDs, _, err := ResolveClawBoxRelayGroupIDsTx(tx, 0)
	if err != nil {
		return err
	}
	return ValidateGroupIDsExist(tx, groupIDs)
}

func resolveLegacyClawBoxAllowedGroupIDsTx(tx *gorm.DB, userID int) ([]int, error) {
	groups, err := ListGroups(tx)
	if err != nil {
		return nil, err
	}

	ownedSet := map[int]struct{}{}
	if userID > 0 {
		owned, err := GetUserBillableGroupIDs(userID)
		if err != nil {
			return nil, err
		}
		for _, groupID := range owned {
			if groupID > 0 {
				ownedSet[groupID] = struct{}{}
			}
		}
	}

	ordered := make([]int, 0, len(groups))
	seen := map[int]struct{}{}
	appendGroup := func(groupID int) {
		if groupID <= 0 {
			return
		}
		if _, ok := seen[groupID]; ok {
			return
		}
		seen[groupID] = struct{}{}
		ordered = append(ordered, groupID)
	}

	for _, group := range groups {
		if group.Id <= 0 || !group.Enabled {
			continue
		}
		if _, ok := ownedSet[group.Id]; ok {
			appendGroup(group.Id)
		}
	}
	for _, group := range groups {
		if group.Id <= 0 || !group.Enabled {
			continue
		}
		if group.UserSelectable {
			appendGroup(group.Id)
		}
	}

	if len(ordered) == 0 {
		usable := setting.GetUserUsableGroupsCopy()
		ids := make([]int, 0, len(usable))
		for groupID := range usable {
			if groupID > 0 {
				ids = append(ids, groupID)
			}
		}
		sort.Ints(ids)
		for _, groupID := range ids {
			appendGroup(groupID)
		}
	}

	if len(ordered) == 0 {
		return nil, errors.New("当前账号没有可用于 ClawBox 的分组")
	}
	return ordered, nil
}

func ResolveClawBoxRelayGroupIDsTx(tx *gorm.DB, userID int) ([]int, int, error) {
	if tx == nil {
		tx = DB
	}
	if isClawBoxProductModeEnabled() {
		productID, err := resolveClawBoxProductIDTx(tx)
		if err != nil {
			return nil, 0, err
		}
		groupIDs, err := GetPaygProductAllowedGroupIDsTx(tx, productID)
		if err != nil {
			return nil, 0, err
		}
		groupIDs = normalizeUniquePositiveIDsKeepOrder(groupIDs)
		if len(groupIDs) == 0 {
			return nil, 0, fmt.Errorf("ClawBox 商品 #%d 未配置可用分组", productID)
		}
		return groupIDs, productID, nil
	}

	groupIDs, err := resolveLegacyClawBoxAllowedGroupIDsTx(tx, userID)
	if err != nil {
		return nil, 0, err
	}
	return groupIDs, 0, nil
}

func ensureClawBoxRelayTokenTx(tx *gorm.DB, userID int) (*Token, error) {
	if tx == nil {
		tx = DB
	}
	if userID <= 0 {
		return nil, errors.New("userId 无效")
	}

	allowedGroupIDs, _, err := ResolveClawBoxRelayGroupIDsTx(tx, userID)
	if err != nil {
		return nil, err
	}
	if err := ValidateGroupIDsExist(tx, allowedGroupIDs); err != nil {
		return nil, err
	}
	allowedGroupIDsJSON, err := MarshalGroupIDsJSONKeepOrder(allowedGroupIDs)
	if err != nil {
		return nil, err
	}

	var tokens []Token
	if err := tx.Where("user_id = ? AND name = ?", userID, ClawBoxRelayTokenName).Order("id DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}

	var token *Token
	if len(tokens) > 0 {
		token = &tokens[0]
		token.Name = ClawBoxRelayTokenName
		token.Status = common.TokenStatusEnabled
		token.ExpiredTime = -1
		token.UnlimitedQuota = true
		token.RemainQuota = 0
		token.DailyQuotaLimit = 0
		token.AllowIps = nil
		token.AllowedGroupIds = allowedGroupIDsJSON
		token.DefaultGroupId = 0
		if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
			"name":              token.Name,
			"status":            token.Status,
			"expired_time":      token.ExpiredTime,
			"remain_quota":      token.RemainQuota,
			"unlimited_quota":   token.UnlimitedQuota,
			"daily_quota_limit": token.DailyQuotaLimit,
			"allow_ips":         token.AllowIps,
			"default_group_id":  token.DefaultGroupId,
		}).Error; err != nil {
			return nil, err
		}
		if err := upsertTokenAllowedGroupsTx(tx, token.Id, allowedGroupIDs); err != nil {
			return nil, err
		}
	} else {
		key, err := common.GenerateKey()
		if err != nil {
			return nil, err
		}
		now := common.GetTimestamp()
		token = &Token{
			UserId:          userID,
			Name:            ClawBoxRelayTokenName,
			Key:             key,
			Status:          common.TokenStatusEnabled,
			CreatedTime:     now,
			AccessedTime:    now,
			ExpiredTime:     -1,
			RemainQuota:     0,
			UnlimitedQuota:  true,
			AllowedGroupIds: allowedGroupIDsJSON,
			DefaultGroupId:  0,
			DailyQuotaLimit: 0,
		}
		if err := tx.Create(token).Error; err != nil {
			return nil, err
		}
		if err := upsertTokenAllowedGroupsTx(tx, token.Id, allowedGroupIDs); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func rotateClawBoxRelayTokenTx(tx *gorm.DB, userID int) (*Token, error) {
	if tx == nil {
		tx = DB
	}
	if userID <= 0 {
		return nil, errors.New("userId 无效")
	}

	allowedGroupIDs, _, err := ResolveClawBoxRelayGroupIDsTx(tx, userID)
	if err != nil {
		return nil, err
	}
	if err := ValidateGroupIDsExist(tx, allowedGroupIDs); err != nil {
		return nil, err
	}

	var tokens []Token
	if err := tx.Where("user_id = ? AND name = ?", userID, ClawBoxRelayTokenName).Order("id DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return ensureClawBoxRelayTokenTx(tx, userID)
	}

	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	token := &tokens[0]
	token.Key = key
	token.Name = ClawBoxRelayTokenName
	token.Status = common.TokenStatusEnabled
	token.ExpiredTime = -1
	token.UnlimitedQuota = true
	token.RemainQuota = 0
	token.DailyQuotaLimit = 0
	token.AllowIps = nil
	token.DefaultGroupId = 0

	if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"key":               token.Key,
		"name":              token.Name,
		"status":            token.Status,
		"expired_time":      token.ExpiredTime,
		"remain_quota":      token.RemainQuota,
		"unlimited_quota":   token.UnlimitedQuota,
		"daily_quota_limit": token.DailyQuotaLimit,
		"allow_ips":         token.AllowIps,
		"default_group_id":  token.DefaultGroupId,
	}).Error; err != nil {
		return nil, err
	}
	if err := upsertTokenAllowedGroupsTx(tx, token.Id, allowedGroupIDs); err != nil {
		return nil, err
	}

	if len(tokens) > 1 {
		staleIDs := make([]int, 0, len(tokens)-1)
		for _, stale := range tokens[1:] {
			if stale.Id > 0 {
				staleIDs = append(staleIDs, stale.Id)
			}
		}
		if len(staleIDs) > 0 {
			if err := tx.Where("token_id IN ?", staleIDs).Delete(&TokenAllowedGroup{}).Error; err != nil {
				return nil, err
			}
			if err := tx.Where("id IN ?", staleIDs).Delete(&Token{}).Error; err != nil {
				return nil, err
			}
		}
	}

	return token, nil
}

func EnsureClawBoxRelayToken(userID int) (*Token, error) {
	var token *Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		resolved, err := ensureClawBoxRelayTokenTx(tx, userID)
		if err != nil {
			return err
		}
		token = resolved
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = InvalidateTokenCache(token.Key)
	return token, nil
}

func RotateClawBoxRelayToken(userID int) (*Token, error) {
	var (
		token   *Token
		oldKeys []string
	)
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing []Token
		if err := tx.Where("user_id = ? AND name = ?", userID, ClawBoxRelayTokenName).Find(&existing).Error; err != nil {
			return err
		}
		for _, item := range existing {
			if strings.TrimSpace(item.Key) != "" {
				oldKeys = append(oldKeys, item.Key)
			}
		}

		resolved, err := rotateClawBoxRelayTokenTx(tx, userID)
		if err != nil {
			return err
		}
		token = resolved
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, key := range oldKeys {
		_ = InvalidateTokenCache(key)
	}
	if token != nil {
		_ = InvalidateTokenCache(token.Key)
	}
	return token, nil
}

func SyncAllClawBoxRelayTokens() error {
	var userIDs []int
	if err := DB.Model(&Token{}).
		Distinct("user_id").
		Where("name = ?", ClawBoxRelayTokenName).
		Order("user_id ASC").
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	for _, userID := range normalizeUniqueSortedIDs(userIDs) {
		if userID <= 0 {
			continue
		}
		if _, err := EnsureClawBoxRelayToken(userID); err != nil {
			return err
		}
	}
	return nil
}

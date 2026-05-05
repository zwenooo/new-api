package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupCleanupSummary struct {
	ArchivedGroupId           int `json:"archived_group_id"`
	UpdatedChannels           int `json:"updated_channels"`
	DisabledChannels          int `json:"disabled_channels"`
	UpdatedTokens             int `json:"updated_tokens"`
	DeletedTokens             int `json:"deleted_tokens"`
	UpdatedProducts           int `json:"updated_products"`
	ArchivedProducts          int `json:"archived_products"`
	UpdatedSubscriptions      int `json:"updated_subscriptions"`
	InvalidatedSubscriptions  int `json:"invalidated_subscriptions"`
	UpdatedPaygBalances       int `json:"updated_payg_balances"`
	ZeroedPaygBalances        int `json:"zeroed_payg_balances"`
	UpdatedPayRequestBalances int `json:"updated_pay_request_balances"`
	ZeroedPayRequestBalances  int `json:"zeroed_pay_request_balances"`
	UpdatedPayTokenBalances   int `json:"updated_pay_token_balances"`
	ZeroedPayTokenBalances    int `json:"zeroed_pay_token_balances"`
	RemappedUsers             int `json:"remapped_users"`
	CleanedPricingRefs        int `json:"cleaned_pricing_refs"`
}

type GroupTokenRemapSummary struct {
	SourceGroupId int `json:"source_group_id"`
	TargetGroupId int `json:"target_group_id"`
	UpdatedTokens int `json:"updated_tokens"`
	SkippedTokens int `json:"skipped_tokens"`
}

func removeOrderedGroupIDKeepOrder(ids []int, target int) []int {
	if len(ids) == 0 {
		return nil
	}
	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if id == target {
			continue
		}
		filtered = append(filtered, id)
	}
	return normalizeUniquePositiveIDsKeepOrder(filtered)
}

func replaceOrderedGroupIDKeepOrder(ids []int, source int, target int) []int {
	if len(ids) == 0 {
		return nil
	}
	replaced := make([]int, 0, len(ids))
	for _, id := range ids {
		switch {
		case id == source:
			replaced = append(replaced, target)
		default:
			replaced = append(replaced, id)
		}
	}
	return normalizeUniquePositiveIDsKeepOrder(replaced)
}

func equalOrderedPositiveIDs(left []int, right []int) bool {
	left = normalizeUniquePositiveIDsKeepOrder(left)
	right = normalizeUniquePositiveIDsKeepOrder(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func filterGroupLimitMap(groupLimitByID map[int]int, removedGroupID int) map[int]int {
	if len(groupLimitByID) == 0 {
		return map[int]int{}
	}
	next := make(map[int]int, len(groupLimitByID))
	for groupID, limit := range groupLimitByID {
		if groupID <= 0 || groupID == removedGroupID {
			continue
		}
		next[groupID] = limit
	}
	return next
}

func firstFallbackEnabledGroupTx(tx *gorm.DB, excludedGroupID int) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	var group Group
	if err := activeGroupScope(tx).
		Where("enabled = ? AND id <> ?", true, excludedGroupID).
		Order("id ASC").
		First(&group).Error; err != nil {
		return nil, err
	}
	group.NormalizeForResponse()
	return &group, nil
}

func groupContainsID(ids []int, groupID int) bool {
	for _, id := range ids {
		if id == groupID {
			return true
		}
	}
	return false
}

func archiveGroupCode(code string, groupID int) string {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, " ", "_")
	if code == "" {
		code = "group"
	}
	return fmt.Sprintf("__archived_group_%d__%s", groupID, code)
}

func removeGroupIDFromArrayOptionTx(tx *gorm.DB, key string, groupID int) error {
	raw, ok, err := readLegacyOptionValue(tx, key)
	if err != nil || !ok {
		return err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	ids, err := ParseGroupIDsJSON(JSONValue(trimmed))
	if err != nil {
		return err
	}
	next := removeIntID(ids, groupID)
	if equalSortedIDs(ids, next) {
		return nil
	}
	value := "[]"
	if b, err := MarshalGroupIDsJSON(next); err == nil && len(b) > 0 {
		value = string(b)
	} else if err != nil {
		return err
	}
	return updateOptionValueTx(tx, key, value)
}

func removeGroupIDFromObjectOptionTx(tx *gorm.DB, key string, groupID int) error {
	raw, ok, err := readLegacyOptionValue(tx, key)
	if err != nil || !ok {
		return err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := common.Unmarshal([]byte(trimmed), &payload); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	delete(payload, fmt.Sprintf("%d", groupID))
	value := "{}"
	if len(payload) > 0 {
		b, err := common.Marshal(payload)
		if err != nil {
			return err
		}
		value = string(b)
	}
	return updateOptionValueTx(tx, key, value)
}

func cleanupChannelBindingsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) error {
	channelIDSet := make(map[int]struct{})
	var primaryChannelIDs []int
	if err := tx.Model(&ChannelGroup{}).Distinct("channel_id").Where("group_id = ?", groupID).Pluck("channel_id", &primaryChannelIDs).Error; err != nil {
		return err
	}
	for _, id := range primaryChannelIDs {
		if id > 0 {
			channelIDSet[id] = struct{}{}
		}
	}
	var backupChannelIDs []int
	if err := tx.Model(&ChannelBackupGroup{}).Distinct("channel_id").Where("group_id = ?", groupID).Pluck("channel_id", &backupChannelIDs).Error; err != nil {
		return err
	}
	for _, id := range backupChannelIDs {
		if id > 0 {
			channelIDSet[id] = struct{}{}
		}
	}
	if len(channelIDSet) == 0 {
		return nil
	}

	channelIDs := make([]int, 0, len(channelIDSet))
	for id := range channelIDSet {
		channelIDs = append(channelIDs, id)
	}
	sort.Ints(channelIDs)

	var channels []*Channel
	if err := tx.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return err
	}
	byID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.Id > 0 {
			byID[channel.Id] = channel
		}
	}

	for _, channelID := range channelIDs {
		channel := byID[channelID]
		if channel == nil {
			continue
		}
		groupIDs, err := getChannelGroupIDsTx(tx, channelID)
		if err != nil {
			return err
		}
		backupIDs, err := getChannelBackupGroupIDsTx(tx, channelID)
		if err != nil {
			return err
		}
		nextGroupIDs := removeIntID(groupIDs, groupID)
		nextBackupIDs := removeIntID(backupIDs, groupID)

		if len(nextGroupIDs) == 0 {
			if err := tx.Where("channel_id = ?", channelID).Delete(&ChannelGroup{}).Error; err != nil {
				return err
			}
			if err := tx.Where("channel_id = ?", channelID).Delete(&ChannelBackupGroup{}).Error; err != nil {
				return err
			}
			if err := tx.Where("channel_id = ?", channelID).Delete(&Ability{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channelID).Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
				return err
			}
			summary.DisabledChannels++
			continue
		}

		nextBackupIDs = filterChannelBackupGroupIDs(nextGroupIDs, nextBackupIDs)
		if !equalSortedIDs(groupIDs, nextGroupIDs) || !equalSortedIDs(backupIDs, nextBackupIDs) {
			if err := upsertChannelGroupsTx(tx, channelID, nextGroupIDs); err != nil {
				return err
			}
			if err := upsertChannelBackupGroupsTx(tx, channelID, nextBackupIDs); err != nil {
				return err
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
			summary.UpdatedChannels++
		}
	}
	return nil
}

func cleanupTokenBindingsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) ([]string, error) {
	var tokenIDs []int
	if err := tx.Model(&TokenAllowedGroup{}).
		Distinct("token_id").
		Where("group_id = ?", groupID).
		Pluck("token_id", &tokenIDs).Error; err != nil {
		return nil, err
	}
	tokenIDs = normalizeUniqueSortedIDs(tokenIDs)
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	var tokens []Token
	if err := tx.Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
		return nil, err
	}
	tokenByID := make(map[int]Token, len(tokens))
	for _, token := range tokens {
		if token.Id > 0 {
			tokenByID[token.Id] = token
		}
	}

	keys := make([]string, 0, len(tokens))
	for _, tokenID := range tokenIDs {
		token, ok := tokenByID[tokenID]
		if !ok {
			continue
		}
		groupIDs, err := getTokenAllowedGroupIDsTx(tx, tokenID)
		if err != nil {
			return nil, err
		}
		nextGroupIDs := removeOrderedGroupIDKeepOrder(groupIDs, groupID)
		if len(nextGroupIDs) == 0 {
			if err := tx.Where("token_id = ?", tokenID).Delete(&TokenAllowedGroup{}).Error; err != nil {
				return nil, err
			}
			if err := tx.Delete(&Token{}, "id = ?", tokenID).Error; err != nil {
				return nil, err
			}
			keys = append(keys, token.Key)
			summary.DeletedTokens++
			continue
		}
		if equalOrderedPositiveIDs(groupIDs, nextGroupIDs) {
			continue
		}
		if err := upsertTokenAllowedGroupsTx(tx, tokenID, nextGroupIDs); err != nil {
			return nil, err
		}
		keys = append(keys, token.Key)
		summary.UpdatedTokens++
	}
	return keys, nil
}

func cleanupSubscriptionPresetGroupsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) ([]int, error) {
	if !tx.Migrator().HasTable(&SubscriptionProductGroup{}) {
		return nil, nil
	}
	var productIDs []int
	if err := tx.Model(&SubscriptionProductGroup{}).
		Distinct("product_id").
		Where("group_id = ?", groupID).
		Pluck("product_id", &productIDs).Error; err != nil {
		return nil, err
	}
	productIDs = normalizeUniqueSortedIDs(productIDs)
	if len(productIDs) == 0 {
		return nil, nil
	}

	groupLimitsByProductID, err := getSubscriptionProductGroupDailyLimitsByProductIDsTx(tx, productIDs)
	if err != nil {
		return nil, err
	}

	archivedPresetIDs := make([]int, 0)
	for _, productID := range productIDs {
		groupIDs, err := getSubscriptionProductGroupIDsTx(tx, productID)
		if err != nil {
			return nil, err
		}
		nextGroupIDs := removeIntID(groupIDs, groupID)
		nextLimits := filterGroupLimitMap(groupLimitsByProductID[productID], groupID)
		if len(nextGroupIDs) == 0 {
			if err := tx.Model(&RedemptionPreset{}).
				Where("id = ?", productID).
				Updates(map[string]interface{}{
					"enabled":  false,
					"archived": true,
				}).Error; err != nil {
				return nil, err
			}
			if err := tx.Where("product_id = ?", productID).Delete(&SubscriptionProductGroup{}).Error; err != nil {
				return nil, err
			}
			if err := tx.Where("product_id = ?", productID).Delete(&SubscriptionProductGroupDailyLimit{}).Error; err != nil {
				return nil, err
			}
			archivedPresetIDs = append(archivedPresetIDs, productID)
			summary.ArchivedProducts++
			continue
		}
		if err := upsertSubscriptionProductGroupsTx(tx, productID, nextGroupIDs); err != nil {
			return nil, err
		}
		if err := upsertSubscriptionProductGroupDailyLimitsTx(tx, productID, nextLimits); err != nil {
			return nil, err
		}
		summary.UpdatedProducts++
	}

	if tx.Migrator().HasTable(&RedemptionPresetRevisionGroup{}) {
		if err := tx.Where("group_id = ?", groupID).Delete(&RedemptionPresetRevisionGroup{}).Error; err != nil {
			return nil, err
		}
	}
	if tx.Migrator().HasTable(&RedemptionPresetRevisionGroupDailyLimit{}) {
		if err := tx.Where("group_id = ?", groupID).Delete(&RedemptionPresetRevisionGroupDailyLimit{}).Error; err != nil {
			return nil, err
		}
	}
	return archivedPresetIDs, nil
}

func cleanupPaygProductsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) error {
	if !tx.Migrator().HasTable(&PaygProductGroup{}) {
		return nil
	}
	var productIDs []int
	if err := tx.Model(&PaygProductGroup{}).Distinct("product_id").Where("group_id = ?", groupID).Pluck("product_id", &productIDs).Error; err != nil {
		return err
	}
	productIDs = normalizeUniqueSortedIDs(productIDs)
	for _, productID := range productIDs {
		groupIDs, err := getPaygProductGroupIDsTx(tx, productID)
		if err != nil {
			return err
		}
		nextGroupIDs := removeIntID(groupIDs, groupID)
		if len(nextGroupIDs) == 0 {
			if err := archivePaygProductTx(tx, productID); err != nil {
				return err
			}
			if err := tx.Where("product_id = ?", productID).Delete(&PaygProductGroup{}).Error; err != nil {
				return err
			}
			summary.ArchivedProducts++
			continue
		}
		if err := tx.Where("product_id = ?", productID).Delete(&PaygProductGroup{}).Error; err != nil {
			return err
		}
		rows := make([]PaygProductGroup, 0, len(nextGroupIDs))
		for _, nextGroupID := range nextGroupIDs {
			rows = append(rows, PaygProductGroup{ProductId: productID, GroupId: nextGroupID})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
		summary.UpdatedProducts++
	}
	return nil
}

func cleanupPayRequestProductsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) error {
	if !tx.Migrator().HasTable(&PayRequestProductGroup{}) {
		return nil
	}
	var productIDs []int
	if err := tx.Model(&PayRequestProductGroup{}).Distinct("product_id").Where("group_id = ?", groupID).Pluck("product_id", &productIDs).Error; err != nil {
		return err
	}
	productIDs = normalizeUniqueSortedIDs(productIDs)
	for _, productID := range productIDs {
		groupIDs, err := getPayRequestProductGroupIDsTx(tx, productID)
		if err != nil {
			return err
		}
		nextGroupIDs := removeIntID(groupIDs, groupID)
		if len(nextGroupIDs) == 0 {
			if err := archivePayRequestProductTx(tx, productID); err != nil {
				return err
			}
			if err := tx.Where("product_id = ?", productID).Delete(&PayRequestProductGroup{}).Error; err != nil {
				return err
			}
			summary.ArchivedProducts++
			continue
		}
		if err := tx.Where("product_id = ?", productID).Delete(&PayRequestProductGroup{}).Error; err != nil {
			return err
		}
		rows := make([]PayRequestProductGroup, 0, len(nextGroupIDs))
		for _, nextGroupID := range nextGroupIDs {
			rows = append(rows, PayRequestProductGroup{ProductId: productID, GroupId: nextGroupID})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
		summary.UpdatedProducts++
	}
	return nil
}

func cleanupPayTokenProductsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) error {
	if !tx.Migrator().HasTable(&PayTokenProductGroup{}) {
		return nil
	}
	var productIDs []int
	if err := tx.Model(&PayTokenProductGroup{}).Distinct("product_id").Where("group_id = ?", groupID).Pluck("product_id", &productIDs).Error; err != nil {
		return err
	}
	productIDs = normalizeUniqueSortedIDs(productIDs)
	for _, productID := range productIDs {
		groupIDs, err := getPayTokenProductGroupIDsTx(tx, productID)
		if err != nil {
			return err
		}
		nextGroupIDs := removeIntID(groupIDs, groupID)
		if len(nextGroupIDs) == 0 {
			if err := archivePayTokenProductTx(tx, productID); err != nil {
				return err
			}
			if err := tx.Where("product_id = ?", productID).Delete(&PayTokenProductGroup{}).Error; err != nil {
				return err
			}
			summary.ArchivedProducts++
			continue
		}
		if err := tx.Where("product_id = ?", productID).Delete(&PayTokenProductGroup{}).Error; err != nil {
			return err
		}
		rows := make([]PayTokenProductGroup, 0, len(nextGroupIDs))
		for _, nextGroupID := range nextGroupIDs {
			rows = append(rows, PayTokenProductGroup{ProductId: productID, GroupId: nextGroupID})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
		summary.UpdatedProducts++
	}
	return nil
}

func archivePaygProductTx(tx *gorm.DB, productID int) error {
	return tx.Model(&PaygProduct{}).Where("id = ?", productID).Updates(map[string]interface{}{
		"enabled":  false,
		"archived": true,
	}).Error
}

func archivePayRequestProductTx(tx *gorm.DB, productID int) error {
	return tx.Model(&PayRequestProduct{}).Where("id = ?", productID).Updates(map[string]interface{}{
		"enabled":  false,
		"archived": true,
	}).Error
}

func archivePayTokenProductTx(tx *gorm.DB, productID int) error {
	return tx.Model(&PayTokenProduct{}).Where("id = ?", productID).Updates(map[string]interface{}{
		"enabled":  false,
		"archived": true,
	}).Error
}

func cleanupSubscriptionsForGroupTx(tx *gorm.DB, groupID int, archivedPresetIDs []int, summary *GroupCleanupSummary) error {
	now := time.Now().Unix()

	if tx.Migrator().HasTable(&UserSubscriptionGroup{}) {
		var subIDs []int
		if err := tx.Model(&UserSubscriptionGroup{}).Distinct("subscription_id").Where("group_id = ?", groupID).Pluck("subscription_id", &subIDs).Error; err != nil {
			return err
		}
		subIDs = normalizeUniqueSortedIDs(subIDs)
		groupDailyLimitsBySubID, err := getUserSubscriptionGroupDailyLimitsBySubscriptionIDsTx(tx, subIDs)
		if err != nil {
			return err
		}
		for _, subID := range subIDs {
			groupIDs, err := getUserSubscriptionGroupIDsTx(tx, subID)
			if err != nil {
				return err
			}
			nextGroupIDs := removeIntID(groupIDs, groupID)
			nextLimits := filterGroupLimitMap(groupDailyLimitsBySubID[subID], groupID)
			if len(nextGroupIDs) == 0 {
				if err := tx.Where("subscription_id = ?", subID).Delete(&UserSubscriptionGroup{}).Error; err != nil {
					return err
				}
				if tx.Migrator().HasTable(&UserSubscriptionGroupDailyLimit{}) {
					if err := tx.Where("subscription_id = ?", subID).Delete(&UserSubscriptionGroupDailyLimit{}).Error; err != nil {
						return err
					}
				}
				if err := tx.Model(&UserSubscription{}).Where("id = ? AND invalid_at = 0", subID).Update("invalid_at", now).Error; err != nil {
					return err
				}
				summary.InvalidatedSubscriptions++
				continue
			}
			if err := upsertUserSubscriptionGroupsTx(tx, subID, nextGroupIDs); err != nil {
				return err
			}
			if err := upsertUserSubscriptionGroupDailyLimitsTx(tx, subID, nextLimits); err != nil {
				return err
			}
			summary.UpdatedSubscriptions++
		}
	}

	if tx.Migrator().HasTable(&UserRequestSubscriptionGroup{}) {
		var requestSubIDs []int
		if err := tx.Model(&UserRequestSubscriptionGroup{}).Distinct("subscription_id").Where("group_id = ?", groupID).Pluck("subscription_id", &requestSubIDs).Error; err != nil {
			return err
		}
		requestSubIDs = normalizeUniqueSortedIDs(requestSubIDs)
		for _, subID := range requestSubIDs {
			groupIDs, err := getUserRequestSubscriptionGroupIDsTx(tx, subID)
			if err != nil {
				return err
			}
			nextGroupIDs := removeIntID(groupIDs, groupID)
			if len(nextGroupIDs) == 0 {
				if err := tx.Where("subscription_id = ?", subID).Delete(&UserRequestSubscriptionGroup{}).Error; err != nil {
					return err
				}
				if err := tx.Model(&UserRequestSubscription{}).Where("id = ? AND invalid_at = 0", subID).Update("invalid_at", now).Error; err != nil {
					return err
				}
				summary.InvalidatedSubscriptions++
				continue
			}
			if err := upsertUserRequestSubscriptionGroupsTx(tx, subID, nextGroupIDs); err != nil {
				return err
			}
			summary.UpdatedSubscriptions++
		}
	}

	archivedPresetIDs = normalizeUniqueSortedIDs(archivedPresetIDs)
	if len(archivedPresetIDs) > 0 {
		if tx.Migrator().HasTable(&UserSubscription{}) {
			if err := tx.Model(&UserSubscription{}).
				Where("source_preset_id IN ? AND invalid_at = 0", archivedPresetIDs).
				Update("invalid_at", now).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable(&UserRequestSubscription{}) {
			if err := tx.Model(&UserRequestSubscription{}).
				Where("source_preset_id IN ? AND invalid_at = 0", archivedPresetIDs).
				Update("invalid_at", now).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanupPaygBalancesForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) ([]int, error) {
	var balances []PaygUserBalance
	if !tx.Migrator().HasTable(&PaygUserBalance{}) {
		return nil, nil
	}
	if err := tx.Select("id", "user_id", "product_id", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota").
		Find(&balances).Error; err != nil {
		return nil, err
	}
	affectedUsers := make(map[int]struct{})
	for _, balance := range balances {
		if balance.Id <= 0 || balance.UserId <= 0 {
			continue
		}
		if !balance.OverrideAllowedGroupIds {
			productGroupIDs, err := getPaygProductGroupIDsTx(tx, balance.ProductId)
			if err == nil && len(productGroupIDs) > 0 {
				continue
			}
			if err := tx.Model(&PaygUserBalance{}).Where("id = ?", balance.Id).Updates(map[string]interface{}{
				"allowed_group_ids":          nil,
				"allowed_groups":             nil,
				"override_allowed_group_ids": true,
				"remaining_quota":            0,
			}).Error; err != nil {
				return nil, err
			}
			if balance.RemainingQuota > 0 {
				if err := tx.Model(&User{}).Where("id = ?", balance.UserId).Updates(map[string]interface{}{
					"payg_quota": gorm.Expr("CASE WHEN payg_quota >= ? THEN payg_quota - ? ELSE 0 END", balance.RemainingQuota, balance.RemainingQuota),
					"quota":      gorm.Expr("CASE WHEN quota >= ? THEN quota - ? ELSE 0 END", balance.RemainingQuota, balance.RemainingQuota),
				}).Error; err != nil {
					return nil, err
				}
			}
			affectedUsers[balance.UserId] = struct{}{}
			summary.ZeroedPaygBalances++
			continue
		}

		groupIDs, err := ParseGroupIDsJSON(balance.AllowedGroupIds)
		if err != nil {
			return nil, err
		}
		if !groupContainsID(groupIDs, groupID) {
			continue
		}
		nextGroupIDs := removeIntID(groupIDs, groupID)
		updatePayload := map[string]interface{}{
			"allowed_groups": nil,
		}
		if len(nextGroupIDs) == 0 {
			updatePayload["allowed_group_ids"] = nil
			updatePayload["remaining_quota"] = 0
			if balance.RemainingQuota > 0 {
				if err := tx.Model(&User{}).Where("id = ?", balance.UserId).Updates(map[string]interface{}{
					"payg_quota": gorm.Expr("CASE WHEN payg_quota >= ? THEN payg_quota - ? ELSE 0 END", balance.RemainingQuota, balance.RemainingQuota),
					"quota":      gorm.Expr("CASE WHEN quota >= ? THEN quota - ? ELSE 0 END", balance.RemainingQuota, balance.RemainingQuota),
				}).Error; err != nil {
					return nil, err
				}
			}
			summary.ZeroedPaygBalances++
		} else {
			nextJSON, err := MarshalGroupIDsJSON(nextGroupIDs)
			if err != nil {
				return nil, err
			}
			updatePayload["allowed_group_ids"] = nextJSON
			summary.UpdatedPaygBalances++
		}
		if err := tx.Model(&PaygUserBalance{}).Where("id = ?", balance.Id).Updates(updatePayload).Error; err != nil {
			return nil, err
		}
		affectedUsers[balance.UserId] = struct{}{}
	}
	userIDs := make([]int, 0, len(affectedUsers))
	for userID := range affectedUsers {
		userIDs = append(userIDs, userID)
	}
	sort.Ints(userIDs)
	for _, userID := range userIDs {
		if _, err := SyncUserPaygSnapshotFromBalancesTx(tx, userID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return userIDs, nil
}

func cleanupPayRequestBalancesForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) ([]int, error) {
	if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
		return nil, nil
	}
	var balances []PayRequestUserBalance
	if err := tx.Select("id", "user_id", "product_id", "allowed_group_ids", "remaining_requests").
		Find(&balances).Error; err != nil {
		return nil, err
	}
	affectedUsers := make(map[int]struct{})
	for _, balance := range balances {
		if balance.Id <= 0 || balance.UserId <= 0 {
			continue
		}
		productGroupIDs, err := getPayRequestProductGroupIDsTx(tx, balance.ProductId)
		if err == nil && len(productGroupIDs) > 0 {
			continue
		}
		if err := tx.Model(&PayRequestUserBalance{}).Where("id = ?", balance.Id).Updates(map[string]interface{}{
			"allowed_group_ids":  nil,
			"allowed_groups":     nil,
			"remaining_requests": 0,
		}).Error; err != nil {
			return nil, err
		}
		affectedUsers[balance.UserId] = struct{}{}
		summary.ZeroedPayRequestBalances++
	}
	userIDs := make([]int, 0, len(affectedUsers))
	for userID := range affectedUsers {
		userIDs = append(userIDs, userID)
	}
	sort.Ints(userIDs)
	for _, userID := range userIDs {
		if _, err := SyncUserPayRequestSnapshotFromBalancesTx(tx, userID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return userIDs, nil
}

func cleanupPayTokenBalancesForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) ([]int, error) {
	if !tx.Migrator().HasTable(&PayTokenUserBalance{}) {
		return nil, nil
	}
	var balances []PayTokenUserBalance
	if err := tx.Select("id", "user_id", "product_id", "allowed_group_ids", "remaining_tokens").
		Find(&balances).Error; err != nil {
		return nil, err
	}
	affectedUsers := make(map[int]struct{})
	for _, balance := range balances {
		if balance.Id <= 0 || balance.UserId <= 0 {
			continue
		}
		productGroupIDs, err := getPayTokenProductGroupIDsTx(tx, balance.ProductId)
		if err == nil && len(productGroupIDs) > 0 {
			continue
		}
		if err := tx.Model(&PayTokenUserBalance{}).Where("id = ?", balance.Id).Updates(map[string]interface{}{
			"allowed_group_ids": nil,
			"allowed_groups":    nil,
			"remaining_tokens":  0,
		}).Error; err != nil {
			return nil, err
		}
		affectedUsers[balance.UserId] = struct{}{}
		summary.ZeroedPayTokenBalances++
	}
	userIDs := make([]int, 0, len(affectedUsers))
	for userID := range affectedUsers {
		userIDs = append(userIDs, userID)
	}
	sort.Ints(userIDs)
	for _, userID := range userIDs {
		if _, err := SyncUserPayTokenSnapshotFromBalancesTx(tx, userID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return userIDs, nil
}

func cleanupUserPrimaryGroupsForDeletedGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) ([]int, error) {
	var userIDs []int
	if !tx.Migrator().HasTable(&User{}) {
		return nil, nil
	}
	if err := tx.Model(&User{}).Where("group_id = ?", groupID).Pluck("id", &userIDs).Error; err != nil {
		return nil, err
	}
	userIDs = normalizeUniqueSortedIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, nil
	}
	fallbackGroup, err := firstFallbackEnabledGroupTx(tx, groupID)
	if err != nil {
		return nil, fmt.Errorf("删除该分组前至少需要保留一个启用中的备用分组，以便迁移用户默认分组: %w", err)
	}
	if err := tx.Model(&User{}).Where("group_id = ?", groupID).Updates(map[string]interface{}{
		"group_id": fallbackGroup.Id,
		"group":    fallbackGroup.Code,
	}).Error; err != nil {
		return nil, err
	}
	summary.RemappedUsers = len(userIDs)
	return userIDs, nil
}

func cleanupPricingRefsForGroupTx(tx *gorm.DB, groupID int, summary *GroupCleanupSummary) error {
	cleaned := 0
	if tx.Migrator().HasTable(&PricingProfileGroupFactor{}) {
		res := tx.Where("group_id = ?", groupID).Delete(&PricingProfileGroupFactor{})
		if res.Error != nil {
			return res.Error
		}
		cleaned += int(res.RowsAffected)
	}
	if tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		res := tx.Where("group_id = ?", groupID).Delete(&UserGroupPriceOverride{})
		if res.Error != nil {
			return res.Error
		}
		cleaned += int(res.RowsAffected)
	}
	summary.CleanedPricingRefs = cleaned
	return nil
}

func SoftDeleteGroupByID(tx *gorm.DB, groupID int) (*GroupCleanupSummary, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}
	if groupID <= 0 {
		return nil, errors.New("分组 id 无效")
	}

	summary := &GroupCleanupSummary{ArchivedGroupId: groupID}
	tokenKeys := make([]string, 0)
	invalidateUserIDs := make(map[int]struct{})

	if err := tx.Transaction(func(tx *gorm.DB) error {
		group, err := GetGroupByID(tx, groupID)
		if err != nil {
			return err
		}
		if group.Archived {
			return errors.New("该分组已删除")
		}
		if strings.TrimSpace(group.Code) == "default" {
			return fmt.Errorf("该分组当前被系统逻辑依赖（code=%s，动态 fallback 默认分组），不可删除", group.Code)
		}
		if err := cleanupChannelBindingsForGroupTx(tx, groupID, summary); err != nil {
			return err
		}
		keys, err := cleanupTokenBindingsForGroupTx(tx, groupID, summary)
		if err != nil {
			return err
		}
		tokenKeys = append(tokenKeys, keys...)

		archivedPresetIDs, err := cleanupSubscriptionPresetGroupsForGroupTx(tx, groupID, summary)
		if err != nil {
			return err
		}
		if err := cleanupPaygProductsForGroupTx(tx, groupID, summary); err != nil {
			return err
		}
		if err := cleanupPayRequestProductsForGroupTx(tx, groupID, summary); err != nil {
			return err
		}
		if err := cleanupPayTokenProductsForGroupTx(tx, groupID, summary); err != nil {
			return err
		}

		if err := cleanupSubscriptionsForGroupTx(tx, groupID, archivedPresetIDs, summary); err != nil {
			return err
		}

		userIDs, err := cleanupUserPrimaryGroupsForDeletedGroupTx(tx, groupID, summary)
		if err != nil {
			return err
		}
		for _, userID := range userIDs {
			invalidateUserIDs[userID] = struct{}{}
		}

		paygUsers, err := cleanupPaygBalancesForGroupTx(tx, groupID, summary)
		if err != nil {
			return err
		}
		for _, userID := range paygUsers {
			invalidateUserIDs[userID] = struct{}{}
		}
		payRequestUsers, err := cleanupPayRequestBalancesForGroupTx(tx, groupID, summary)
		if err != nil {
			return err
		}
		for _, userID := range payRequestUsers {
			invalidateUserIDs[userID] = struct{}{}
		}
		payTokenUsers, err := cleanupPayTokenBalancesForGroupTx(tx, groupID, summary)
		if err != nil {
			return err
		}
		for _, userID := range payTokenUsers {
			invalidateUserIDs[userID] = struct{}{}
		}

		if err := cleanupPricingRefsForGroupTx(tx, groupID, summary); err != nil {
			return err
		}

		if err := removeGroupIDFromArrayOptionTx(tx, "AutoGroups", groupID); err != nil {
			return err
		}
		if err := removeGroupIDFromObjectOptionTx(tx, "ModelRequestRateLimitGroup", groupID); err != nil {
			return err
		}
		if err := removeGroupIDFromObjectOptionTx(tx, "ModelRequestConcurrencyLimitGroup", groupID); err != nil {
			return err
		}
		if err := removeGroupIDFromObjectOptionTx(tx, "GroupGroupRatio", groupID); err != nil {
			return err
		}
		if err := removeGroupIDFromArrayOptionTx(tx, "cx_pool.cx_group_ids", groupID); err != nil {
			return err
		}

		archivedCode := archiveGroupCode(group.Code, groupID)
		archivedName := archiveGroupCode(group.DisplayName, groupID)
		if err := tx.Model(&Group{}).Where("id = ?", groupID).Updates(map[string]interface{}{
			"code":                    archivedCode,
			"name":                    archivedName,
			"description":             strings.TrimSpace(group.Description),
			"enabled":                 false,
			"user_selectable":         false,
			"no_billing":              false,
			"no_billing_product_keys": nil,
			"archived":                true,
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	InitOptionMap()
	if err := RefreshGroupSettings(); err != nil {
		return nil, err
	}
	if err := RefreshPricingRuleCache(); err != nil {
		return nil, err
	}
	BumpChannelCacheRevision()
	InitChannelCache()

	tokenKeys = normalizeUniqueStringOrderless(tokenKeys)
	for _, key := range tokenKeys {
		_ = InvalidateTokenCache(key)
	}
	for userID := range invalidateUserIDs {
		_ = invalidateUserCache(userID)
	}
	return summary, nil
}

func BulkRemapTokenAllowedGroups(tx *gorm.DB, sourceGroupID int, targetGroupID int) (*GroupTokenRemapSummary, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}
	if sourceGroupID <= 0 || targetGroupID <= 0 {
		return nil, errors.New("分组 id 无效")
	}
	if sourceGroupID == targetGroupID {
		return nil, errors.New("源分组和目标分组不能相同")
	}
	if err := ValidateGroupIDsExist(tx, []int{targetGroupID}); err != nil {
		return nil, err
	}

	summary := &GroupTokenRemapSummary{
		SourceGroupId: sourceGroupID,
		TargetGroupId: targetGroupID,
	}
	tokenKeys := make([]string, 0)
	err := tx.Transaction(func(tx *gorm.DB) error {
		var tokenIDs []int
		if err := tx.Model(&TokenAllowedGroup{}).
			Distinct("token_id").
			Where("group_id = ?", sourceGroupID).
			Pluck("token_id", &tokenIDs).Error; err != nil {
			return err
		}
		tokenIDs = normalizeUniqueSortedIDs(tokenIDs)
		if len(tokenIDs) == 0 {
			return nil
		}

		var tokens []Token
		if err := tx.Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
			return err
		}
		tokenByID := make(map[int]Token, len(tokens))
		for _, token := range tokens {
			if token.Id > 0 {
				tokenByID[token.Id] = token
			}
		}

		for _, tokenID := range tokenIDs {
			token, ok := tokenByID[tokenID]
			if !ok {
				continue
			}
			groupIDs, err := getTokenAllowedGroupIDsTx(tx, tokenID)
			if err != nil {
				return err
			}
			if !groupContainsID(groupIDs, sourceGroupID) {
				summary.SkippedTokens++
				continue
			}
			nextGroupIDs := replaceOrderedGroupIDKeepOrder(groupIDs, sourceGroupID, targetGroupID)
			if equalOrderedPositiveIDs(groupIDs, nextGroupIDs) {
				summary.SkippedTokens++
				continue
			}
			if err := upsertTokenAllowedGroupsTx(tx, tokenID, nextGroupIDs); err != nil {
				return err
			}
			tokenKeys = append(tokenKeys, token.Key)
			summary.UpdatedTokens++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	tokenKeys = normalizeUniqueStringOrderless(tokenKeys)
	for _, key := range tokenKeys {
		_ = InvalidateTokenCache(key)
	}
	return summary, nil
}

func normalizeUniqueStringOrderless(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

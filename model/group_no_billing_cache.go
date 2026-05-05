package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"one-api/common"

	"gorm.io/gorm"
)

var (
	groupNoBillingByID            map[int]struct{}
	groupNoBillingProductKeysByID map[int][]string
	groupNoBillingByIDMu          sync.RWMutex
)

func setGroupNoBillingLocked(ids map[int]struct{}, productKeysByID map[int][]string) {
	groupNoBillingByID = ids
	groupNoBillingProductKeysByID = productKeysByID
}

func syncGroupNoBillingFromGroups(groups []Group) error {
	ids := make(map[int]struct{}, len(groups))
	productKeysByID := make(map[int][]string, len(groups))
	for _, g := range groups {
		if g.Id <= 0 || !g.Enabled || !g.NoBilling {
			continue
		}
		ids[g.Id] = struct{}{}
		refs, err := ParseGroupNoBillingProductKeysJSON(g.NoBillingProductKeys)
		if err != nil {
			return fmt.Errorf("分组 %s 不计费限定商品配置无效: %w", strings.TrimSpace(g.Code), err)
		}
		if len(refs) == 0 {
			continue
		}
		keys := make([]string, 0, len(refs))
		for _, ref := range refs {
			key := ref.Key()
			if key == "" {
				continue
			}
			keys = append(keys, key)
		}
		if len(keys) > 0 {
			productKeysByID[g.Id] = keys
		}
	}
	groupNoBillingByIDMu.Lock()
	setGroupNoBillingLocked(ids, productKeysByID)
	groupNoBillingByIDMu.Unlock()
	return nil
}

func IsGroupNoBilling(groupID int) bool {
	if groupID <= 0 {
		return false
	}
	groupNoBillingByIDMu.RLock()
	_, ok := groupNoBillingByID[groupID]
	groupNoBillingByIDMu.RUnlock()
	return ok
}

func GetNoBillingGroupsCopy() map[int]struct{} {
	groupNoBillingByIDMu.RLock()
	defer groupNoBillingByIDMu.RUnlock()

	out := make(map[int]struct{}, len(groupNoBillingByID))
	for id := range groupNoBillingByID {
		out[id] = struct{}{}
	}
	return out
}

func getNoBillingGroupProductKeysCopy() map[int][]string {
	groupNoBillingByIDMu.RLock()
	defer groupNoBillingByIDMu.RUnlock()

	out := make(map[int][]string, len(groupNoBillingProductKeysByID))
	for groupID, keys := range groupNoBillingProductKeysByID {
		if len(keys) == 0 {
			continue
		}
		copied := make([]string, len(keys))
		copy(copied, keys)
		out[groupID] = copied
	}
	return out
}

func addOwnedNoBillingProductGroups(dst map[string]map[int]struct{}, key string, groupIDs []int) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if len(groupIDs) == 0 {
		return
	}
	groupSet, ok := dst[key]
	if !ok {
		groupSet = make(map[int]struct{}, len(groupIDs))
		dst[key] = groupSet
	}
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		groupSet[groupID] = struct{}{}
	}
}

func loadUserOwnedNoBillingProductGroupsTx(tx *gorm.DB, userId int) (map[string]map[int]struct{}, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	owned := make(map[string]map[int]struct{}, 16)

	if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
		return nil, err
	}

	now := time.Now().Unix()

	var quotaSubs []UserSubscription
	if err := tx.Select("id", "source_preset_id").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')", userId, true, now, now, UserSubscriptionBillingUnitQuota).
		Find(&quotaSubs).Error; err != nil {
		return nil, err
	}
	if len(quotaSubs) > 0 {
		allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, quotaSubs)
		if err != nil {
			return nil, err
		}
		for _, sub := range quotaSubs {
			if sub.SourcePresetId <= 0 {
				continue
			}
			addOwnedNoBillingProductGroups(
				owned,
				BuildGroupNoBillingProductKey(GroupNoBillingProductKindSubscription, sub.SourcePresetId),
				allowedBySubID[sub.Id],
			)
		}
	}

	var tokenSubs []UserSubscription
	if err := tx.Select("id", "source_preset_id").
		Where("user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
		Find(&tokenSubs).Error; err != nil {
		return nil, err
	}
	if len(tokenSubs) > 0 {
		allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, tokenSubs)
		if err != nil {
			return nil, err
		}
		for _, sub := range tokenSubs {
			if sub.SourcePresetId <= 0 {
				continue
			}
			addOwnedNoBillingProductGroups(
				owned,
				BuildGroupNoBillingProductKey(GroupNoBillingProductKindTokens, sub.SourcePresetId),
				allowedBySubID[sub.Id],
			)
		}
	}

	today := common.GetTodayDateInt()
	var requestSubs []UserRequestSubscription
	if err := tx.Select("id", "source_preset_id", "daily_request_limit", "daily_request_used", "daily_request_reset_date", "total_request_limit", "total_request_used").
		Where("user_id = ? AND invalid_at = 0 AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?)", userId, now, now).
		Find(&requestSubs).Error; err != nil {
		return nil, err
	}
	if len(requestSubs) > 0 {
		groupIDsBySubID, err := resolveUserRequestSubscriptionAllowedGroupsTx(tx, requestSubs)
		if err != nil {
			return nil, err
		}
		for _, sub := range requestSubs {
			if sub.SourcePresetId <= 0 {
				continue
			}
			if !canConsumeUserRequestSubscription(sub, today, 1) {
				continue
			}
			addOwnedNoBillingProductGroups(
				owned,
				BuildGroupNoBillingProductKey(GroupNoBillingProductKindRequest, sub.SourcePresetId),
				groupIDsBySubID[sub.Id],
			)
		}
	}

	var paygBalances []PaygUserBalance
	if err := tx.Select("product_id", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota").
		Where("user_id = ? AND remaining_quota > 0", userId).
		Find(&paygBalances).Error; err != nil {
		return nil, err
	}
	for _, balance := range paygBalances {
		if balance.ProductId <= 0 {
			continue
		}
		groupIDs, err := ResolvePaygBalanceAllowedGroupIDs(balance)
		if err != nil {
			return nil, err
		}
		addOwnedNoBillingProductGroups(
			owned,
			BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayg, balance.ProductId),
			groupIDs,
		)
	}

	var payRequestBalances []PayRequestUserBalance
	if err := tx.Select("product_id", "allowed_group_ids", "allowed_groups", "remaining_requests").
		Where("user_id = ? AND remaining_requests > 0", userId).
		Find(&payRequestBalances).Error; err != nil {
		return nil, err
	}
	for _, balance := range payRequestBalances {
		if balance.ProductId <= 0 {
			continue
		}
		groupIDs, err := ResolvePayRequestBalanceAllowedGroupIDs(balance)
		if err != nil {
			return nil, err
		}
		addOwnedNoBillingProductGroups(
			owned,
			BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayRequest, balance.ProductId),
			groupIDs,
		)
	}

	var payTokenBalances []PayTokenUserBalance
	if err := tx.Select("product_id", "allowed_group_ids", "allowed_groups", "remaining_tokens").
		Where("user_id = ? AND remaining_tokens > 0", userId).
		Find(&payTokenBalances).Error; err != nil {
		return nil, err
	}
	for _, balance := range payTokenBalances {
		if balance.ProductId <= 0 {
			continue
		}
		groupIDs, err := ResolvePayTokenBalanceAllowedGroupIDs(balance)
		if err != nil {
			return nil, err
		}
		addOwnedNoBillingProductGroups(
			owned,
			BuildGroupNoBillingProductKey(GroupNoBillingProductKindPayToken, balance.ProductId),
			groupIDs,
		)
	}

	return owned, nil
}

func GetUserEligibleNoBillingGroupSet(userId int) (map[int]struct{}, error) {
	if userId <= 0 {
		return map[int]struct{}{}, nil
	}

	productKeysByID := getNoBillingGroupProductKeysCopy()
	if len(productKeysByID) == 0 {
		return map[int]struct{}{}, nil
	}

	ownedProductGroups := make(map[string]map[int]struct{}, 16)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		ownedProductGroups, err = loadUserOwnedNoBillingProductGroupsTx(tx, userId)
		return err
	}); err != nil {
		return nil, err
	}

	out := make(map[int]struct{}, len(productKeysByID))
	for groupID, keys := range productKeysByID {
		for _, key := range keys {
			groupSet, ok := ownedProductGroups[key]
			if !ok {
				continue
			}
			if _, ok := groupSet[groupID]; !ok {
				continue
			}
			out[groupID] = struct{}{}
			break
		}
	}
	return out, nil
}

func GetUserEligibleNoBillingGroupIDs(userId int) ([]int, error) {
	groupSet, err := GetUserEligibleNoBillingGroupSet(userId)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]int, 0, len(groupSet))
	for groupID := range groupSet {
		if groupID <= 0 {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	return normalizeUniqueSortedIDs(groupIDs), nil
}

func IsUserEligibleForNoBillingGroup(userId int, groupID int) (bool, error) {
	if userId <= 0 || groupID <= 0 {
		return false, nil
	}
	groupSet, err := GetUserEligibleNoBillingGroupSet(userId)
	if err != nil {
		return false, err
	}
	_, ok := groupSet[groupID]
	return ok, nil
}

package model

import (
	"errors"
	"strings"
	"unicode/utf8"

	"one-api/common"
	relaycommon "one-api/relay/common"
	"one-api/setting/payg_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func resolvePaygBalanceAllowedGroupIDs(b PaygUserBalance) ([]int, error) {
	return resolveEffectivePrepaidBalanceAllowedGroupIDsTx(DB, effectivePrepaidBalanceAllowedGroupLookup{
		LoadCurrentProductGroupIDs: getPaygProductGroupIDsTx,
		LoadConfiguredGroupIDs: func(productID int) ([]int, bool) {
			p, ok := payg_setting.FindPaygProductByID(productID)
			if !ok {
				return nil, false
			}
			return p.AllowedGroupIds, true
		},
	}, effectivePrepaidBalanceAllowedGroupOptions{
		ProductID:              b.ProductId,
		FollowCurrentProduct:   !b.OverrideAllowedGroupIds,
		StoredGroupIDs:         b.AllowedGroupIds,
		StoredGroups:           b.AllowedGroups,
		EmptyProductMessage:    "按量付费商品可用分组为空",
		EmptySnapshotMessage:   "按量付费可用分组为空",
		MissingSnapshotMessage: "按量付费缺少 allowed_group_ids",
	})
}

// ResolvePaygBalanceAllowedGroupIDs returns the effective allowed group ids for a balance item.
// For product-based balances, it follows current product config; otherwise it falls back to stored snapshot.
func ResolvePaygBalanceAllowedGroupIDs(b PaygUserBalance) ([]int, error) {
	return resolvePaygBalanceAllowedGroupIDs(b)
}

// ResolvePaygBalanceAllowedGroups returns the effective allowed group codes for a balance item.
// It is derived from ResolvePaygBalanceAllowedGroupIDs for display/legacy compatibility.
func ResolvePaygBalanceAllowedGroups(b PaygUserBalance) ([]string, error) {
	ids, err := resolvePaygBalanceAllowedGroupIDs(b)
	if err != nil {
		return nil, err
	}
	codes, err := GroupCodesFromIDs(DB, ids)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeGroupNames(codes)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, errors.New("按量付费可用分组为空")
	}
	return normalized, nil
}

func cloneProductQuotaAllocations(allocations []relaycommon.ProductQuotaAllocation) []relaycommon.ProductQuotaAllocation {
	if len(allocations) == 0 {
		return nil
	}
	out := make([]relaycommon.ProductQuotaAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.ProductId == 0 || allocation.Quota <= 0 {
			continue
		}
		out = append(out, relaycommon.ProductQuotaAllocation{
			ProductId: allocation.ProductId,
			Quota:     allocation.Quota,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildPaygConsumableAllocationsFromBalances(balances []PaygUserBalance, groupID int, requiredQuota int) ([]relaycommon.ProductQuotaAllocation, bool, error) {
	if groupID <= 0 {
		return nil, false, errors.New("group_id 无效")
	}
	if requiredQuota <= 0 {
		requiredQuota = 1
	}
	left := requiredQuota
	allocations := make([]relaycommon.ProductQuotaAllocation, 0, len(balances))
	for _, balance := range balances {
		if balance.RemainingQuota <= 0 {
			continue
		}
		ids, err := resolvePaygBalanceAllowedGroupIDs(balance)
		if err != nil {
			return nil, false, err
		}
		allowed := false
		for _, gid := range ids {
			if gid == groupID {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		useQuota := balance.RemainingQuota
		if useQuota > left {
			useQuota = left
		}
		if useQuota <= 0 {
			continue
		}
		allocations = append(allocations, relaycommon.ProductQuotaAllocation{
			ProductId: balance.ProductId,
			Quota:     useQuota,
		})
		left -= useQuota
		if left <= 0 {
			return allocations, true, nil
		}
	}
	return nil, false, nil
}

// PaygUserBalance is the historical name for a user's prepaid-credit balance.
// One (user_id, product_id) => one row.
// allowed_groups are the groups that can be consumed by this balance item.
type PaygUserBalance struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId    int `json:"user_id" gorm:"not null;index;uniqueIndex:idx_payg_user_product"`
	ProductId int `json:"product_id" gorm:"not null;index;uniqueIndex:idx_payg_user_product;column:product_id"`

	ProductName string `json:"product_name" gorm:"type:varchar(64);default:'';column:product_name"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`

	// AllowedGroupIds is the source of truth. allowed_groups is legacy-only snapshot.
	AllowedGroupIds JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`
	AllowedGroups   JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	// OverrideAllowedGroupIds forces the balance item to use the stored allowed_group_ids snapshot
	// instead of following product group config (ProductId > 0). This enables per-user adjustments.
	OverrideAllowedGroupIds bool `json:"override_allowed_group_ids" gorm:"type:boolean;not null;default:false;column:override_allowed_group_ids"`
	RemainingQuota          int  `json:"remaining_quota" gorm:"type:int;default:0;column:remaining_quota"`
	HistoryQuota            int  `json:"history_quota" gorm:"type:int;default:0;column:history_quota"`

	CreatedAt int64 `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (b *PaygUserBalance) Validate() error {
	if b == nil {
		return errors.New("balance 为空")
	}
	if b.UserId <= 0 {
		return errors.New("user_id 无效")
	}
	if b.ProductId == 0 {
		return errors.New("product_id 无效")
	}
	name := strings.TrimSpace(b.ProductName)
	if name != "" && utf8.RuneCountInString(name) > 64 {
		return errors.New("product_name 过长")
	}
	if b.SortOrder < 0 {
		return errors.New("sort_order 不能小于 0")
	}
	if b.RemainingQuota < 0 {
		return errors.New("remaining_quota 不能小于 0")
	}
	if b.HistoryQuota < 0 {
		return errors.New("history_quota 不能小于 0")
	}
	if len(b.AllowedGroupIds) == 0 {
		return errors.New("allowed_group_ids 不能为空")
	}
	groupIDs, err := ParseGroupIDsJSON(b.AllowedGroupIds)
	if err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return errors.New("allowed_group_ids 不能为空")
	}
	return nil
}

func GetUserPaygBalances(userId int, onlyPositive bool) ([]PaygUserBalance, error) {
	return GetUserPaygBalancesTx(DB, userId, onlyPositive)
}

func GetUserPaygBalancesTx(tx *gorm.DB, userId int, onlyPositive bool) ([]PaygUserBalance, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	var balances []PaygUserBalance
	q := tx.Model(&PaygUserBalance{}).Where("user_id = ?", userId)
	if onlyPositive {
		q = q.Where("remaining_quota > 0")
	}
	if err := q.Order("sort_order DESC, product_id DESC, id DESC").Find(&balances).Error; err != nil {
		return nil, err
	}
	return balances, nil
}

func ReorderUserPaygBalances(userId int, orderedProductIDs []int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	productIDs := normalizeOrderedUniqueNonZeroIDs(orderedProductIDs)
	if len(productIDs) == 0 {
		return errors.New("product_ids 不能为空")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			ProductId int `gorm:"column:product_id"`
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&PaygUserBalance{}).
			Select("product_id").
			Where("user_id = ? AND product_id IN ?", userId, productIDs).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(productIDs) {
			return errors.New("按量付费记录不存在")
		}

		caseSQL := "CASE product_id"
		args := make([]interface{}, 0, len(productIDs)*2)
		for idx, productID := range productIDs {
			caseSQL += " WHEN ? THEN ?"
			args = append(args, productID, len(productIDs)-idx)
		}
		caseSQL += " ELSE sort_order END"

		return tx.Model(&PaygUserBalance{}).
			Where("user_id = ? AND product_id IN ?", userId, productIDs).
			Update("sort_order", gorm.Expr(caseSQL, args...)).Error
	})
}

func UpsertPaygUserBalanceTx(tx *gorm.DB, userId int, productId int, productName string, sortOrder int, allowedGroupIDs []int, addQuota int) error {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if productId == 0 {
		return errors.New("productId 无效")
	}
	if addQuota <= 0 {
		return errors.New("addQuota 必须大于 0")
	}
	if sortOrder < 0 {
		return errors.New("sort_order 不能小于 0")
	}
	if strings.TrimSpace(productName) != "" && utf8.RuneCountInString(strings.TrimSpace(productName)) > 64 {
		return errors.New("product_name 过长")
	}

	// Validate groups
	ids := normalizeUniqueSortedIDs(allowedGroupIDs)
	if len(ids) == 0 {
		return errors.New("allowed_group_ids 不能为空")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	normalizedGroupIDsJSON, err := MarshalGroupIDsJSON(ids)
	if err != nil {
		return err
	}

	var existing PaygUserBalance
	err = lockForUpdate(tx).
		Where("user_id = ? AND product_id = ?", userId, productId).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			b := &PaygUserBalance{
				UserId:          userId,
				ProductId:       productId,
				ProductName:     strings.TrimSpace(productName),
				SortOrder:       sortOrder,
				AllowedGroupIds: normalizedGroupIDsJSON,
				AllowedGroups:   nil,
				RemainingQuota:  addQuota,
				HistoryQuota:    addQuota,
			}
			if vErr := b.Validate(); vErr != nil {
				return vErr
			}
			return tx.Create(b).Error
		}
		return err
	}

	updates := map[string]interface{}{
		"remaining_quota": gorm.Expr("remaining_quota + ?", addQuota),
		"history_quota":   gorm.Expr("history_quota + ?", addQuota),
		"sort_order":      sortOrder,
	}
	// Treat zeroed product-backed balances as freshly re-added goods:
	// revive them from the current product group bindings instead of
	// keeping a stale per-user override snapshot from a previously deleted row.
	if existing.ProductId > 0 && existing.RemainingQuota <= 0 {
		updates["allowed_group_ids"] = normalizedGroupIDsJSON
		updates["allowed_groups"] = nil
		updates["override_allowed_group_ids"] = false
	} else if !existing.OverrideAllowedGroupIds {
		updates["allowed_group_ids"] = normalizedGroupIDsJSON
	}
	if strings.TrimSpace(productName) != "" {
		updates["product_name"] = strings.TrimSpace(productName)
	}
	return tx.Model(&PaygUserBalance{}).Where("id = ?", existing.Id).Updates(updates).Error
}

func ResetProductBackedPaygBalanceGroupsToProductTx(tx *gorm.DB, balanceID int, productID int) error {
	if tx == nil {
		tx = DB
	}
	if balanceID <= 0 {
		return errors.New("balance_id 无效")
	}
	if productID <= 0 {
		return nil
	}

	groupIDs, err := getPaygProductGroupIDsTx(tx, productID)
	if err != nil {
		return err
	}

	updates := map[string]interface{}{
		"allowed_group_ids":          nil,
		"allowed_groups":             nil,
		"override_allowed_group_ids": false,
	}
	if len(groupIDs) > 0 {
		groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}
		updates["allowed_group_ids"] = groupIDsJSON
	}

	return tx.Model(&PaygUserBalance{}).Where("id = ?", balanceID).Updates(updates).Error
}

func UnionPaygAllowedGroupsFromBalances(balances []PaygUserBalance) (JSONValue, error) {
	if len(balances) == 0 {
		return nil, nil
	}
	groupIDs := make([]int, 0, 16)
	seen := make(map[int]struct{}, 16)
	for _, b := range balances {
		if b.RemainingQuota <= 0 {
			continue
		}
		ids, err := resolvePaygBalanceAllowedGroupIDs(b)
		if err != nil {
			return nil, err
		}
		for _, gid := range ids {
			if gid <= 0 {
				continue
			}
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			groupIDs = append(groupIDs, gid)
		}
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil, nil
	}
	return MarshalGroupIDsJSON(groupIDs)
}

func FindUserPaygConsumableProductIdTx(tx *gorm.DB, userId int, groupID int, requiredQuota int) (productId int, ok bool, err error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, false, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return 0, false, errors.New("group_id 无效")
	}
	if requiredQuota <= 0 {
		requiredQuota = 1
	}

	var balances []PaygUserBalance
	if err := tx.Model(&PaygUserBalance{}).
		Select("product_id", "sort_order", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota").
		Where("user_id = ? AND remaining_quota > 0", userId).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return 0, false, err
	}
	allocations, ok, err := buildPaygConsumableAllocationsFromBalances(balances, groupID, requiredQuota)
	if err != nil {
		return 0, false, err
	}
	if !ok || len(allocations) == 0 {
		return 0, false, nil
	}
	return allocations[0].ProductId, true, nil
}

func GetUserPaygBalanceInfoTx(tx *gorm.DB, userId int) (totalRemaining int, allowedGroupIDs []int, err error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, nil, errors.New("userId 无效")
	}

	var balances []PaygUserBalance
	if err := tx.Model(&PaygUserBalance{}).
		Select("product_id", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota").
		Where("user_id = ? AND remaining_quota > 0", userId).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return 0, nil, err
	}
	total := 0
	unionIDs := make([]int, 0, 16)
	seen := make(map[int]struct{}, 16)
	for _, b := range balances {
		if b.RemainingQuota <= 0 {
			continue
		}
		total += b.RemainingQuota
		ids, err := resolvePaygBalanceAllowedGroupIDs(b)
		if err != nil {
			return 0, nil, err
		}
		for _, gid := range ids {
			if gid <= 0 {
				continue
			}
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			unionIDs = append(unionIDs, gid)
		}
	}
	unionIDs = normalizeUniqueSortedIDs(unionIDs)
	if len(unionIDs) == 0 {
		return total, nil, nil
	}
	return total, unionIDs, nil
}

func consumeUserPaygQuotaWithAllocations(userId int, groupID int, quota int) ([]relaycommon.ProductQuotaAllocation, int, error) {
	if userId <= 0 {
		return nil, 0, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return nil, 0, errors.New("group_id 无效")
	}
	if quota <= 0 {
		return nil, 0, errors.New("quota 必须大于 0")
	}

	var allocations []relaycommon.ProductQuotaAllocation
	userDelta := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).
			Select("id", "quota", "payg_quota", "payg_history_quota", "payg_allowed_groups").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}
		if _, err := syncLockedUserPaygSnapshotFromBalancesTx(tx, &user); err != nil {
			return err
		}
		if user.PayAsYouGoQuota < quota {
			return errors.New("按量付费额度不足")
		}

		var balances []PaygUserBalance
		if err := lockForUpdate(tx).
			Select("id", "product_id", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota", "sort_order").
			Where("user_id = ? AND remaining_quota > 0", userId).
			Order("sort_order DESC, product_id DESC, id DESC").
			Find(&balances).Error; err != nil {
			return err
		}
		if len(balances) == 0 {
			return errors.New("按量付费额度不足")
		}

		resolvedAllocations, ok, err := buildPaygConsumableAllocationsFromBalances(balances, groupID, quota)
		if err != nil {
			return err
		}
		if !ok || len(resolvedAllocations) == 0 {
			return errors.New("按量付费额度不足")
		}

		indexByProductID := make(map[int]int, len(balances))
		for i := range balances {
			indexByProductID[balances[i].ProductId] = i
		}
		for _, allocation := range resolvedAllocations {
			idx, ok := indexByProductID[allocation.ProductId]
			if !ok {
				return errors.New("按量付费商品余额不存在")
			}
			if balances[idx].RemainingQuota < allocation.Quota {
				return errors.New("按量付费额度不足")
			}
			if err := tx.Model(&PaygUserBalance{}).
				Where("id = ?", balances[idx].Id).
				Update("remaining_quota", gorm.Expr("remaining_quota - ?", allocation.Quota)).Error; err != nil {
				return err
			}
			balances[idx].RemainingQuota -= allocation.Quota
		}

		dustCleared, err := clearPaygDustFromBalancesTx(tx, balances, common.PreConsumedQuota)
		if err != nil {
			return err
		}
		unionGroupsJSON, err := UnionPaygAllowedGroupsFromBalances(balances)
		if err != nil {
			return err
		}
		totalDeduct := quota + dustCleared
		if err := tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
			"payg_quota":          gorm.Expr("payg_quota - ?", totalDeduct),
			"quota":               gorm.Expr("quota - ?", totalDeduct),
			"payg_allowed_groups": unionGroupsJSON,
		}).Error; err != nil {
			return err
		}

		allocations = cloneProductQuotaAllocations(resolvedAllocations)
		userDelta = totalDeduct
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return allocations, userDelta, nil
}

func DecreaseUserPaygQuotaWithAllocations(userId int, groupID int, quota int) ([]relaycommon.ProductQuotaAllocation, error) {
	allocations, userDelta, err := consumeUserPaygQuotaWithAllocations(userId, groupID, quota)
	if err != nil {
		return nil, err
	}
	if userDelta > 0 {
		gopool.Go(func() {
			if err := cacheDecrUserQuota(userId, int64(userDelta)); err != nil {
				common.SysLog("failed to decrease user quota: " + err.Error())
			}
		})
	}
	if err := invalidateUserCache(userId); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return allocations, nil
}

func restoreUserPaygQuotaWithAllocations(userId int, allocations []relaycommon.ProductQuotaAllocation) (int, error) {
	normalizedAllocations := cloneProductQuotaAllocations(allocations)
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if len(normalizedAllocations) == 0 {
		return 0, errors.New("按量付费商品未指定，无法返还额度")
	}

	totalQuota := 0
	for _, allocation := range normalizedAllocations {
		totalQuota += allocation.Quota
	}

	userDelta := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).
			Select("id").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}

		for _, allocation := range normalizedAllocations {
			var balance PaygUserBalance
			if err := lockForUpdate(tx).
				Where("user_id = ? AND product_id = ?", userId, allocation.ProductId).
				First(&balance).Error; err != nil {
				return err
			}
			if err := tx.Model(&PaygUserBalance{}).
				Where("id = ?", balance.Id).
				Update("remaining_quota", gorm.Expr("remaining_quota + ?", allocation.Quota)).Error; err != nil {
				return err
			}
		}

		var balances []PaygUserBalance
		if err := lockForUpdate(tx).
			Select("id", "product_id", "allowed_group_ids", "allowed_groups", "override_allowed_group_ids", "remaining_quota", "sort_order").
			Where("user_id = ? AND remaining_quota > 0", userId).
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
		delta := totalQuota - dustCleared
		if delta < 0 {
			delta = 0
		}
		userDelta = delta
		return tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
			"payg_quota":          gorm.Expr("payg_quota + ?", delta),
			"quota":               gorm.Expr("quota + ?", delta),
			"payg_allowed_groups": unionGroupsJSON,
		}).Error
	})
	if err != nil {
		return 0, err
	}
	return userDelta, nil
}

func ReturnUserPaygQuotaWithAllocations(userId int, allocations []relaycommon.ProductQuotaAllocation) error {
	restored, err := restoreUserPaygQuotaWithAllocations(userId, allocations)
	if err != nil {
		return err
	}
	if restored > 0 {
		gopool.Go(func() {
			if err := cacheIncrUserQuota(userId, int64(restored)); err != nil {
				common.SysLog("failed to restore user quota: " + err.Error())
			}
		})
	}
	if err := invalidateUserCache(userId); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return nil
}

func clearPaygDustFromBalancesTx(tx *gorm.DB, balances []PaygUserBalance, threshold int) (cleared int, err error) {
	if threshold <= 0 || len(balances) == 0 {
		return 0, nil
	}
	if tx == nil {
		tx = DB
	}

	ids := make([]int, 0, 8)
	total := 0
	for i := range balances {
		remaining := balances[i].RemainingQuota
		if remaining <= 0 || remaining >= threshold {
			continue
		}
		total += remaining
		ids = append(ids, balances[i].Id)
		balances[i].RemainingQuota = 0
	}
	if total <= 0 || len(ids) == 0 {
		return 0, nil
	}
	if err := tx.Model(&PaygUserBalance{}).Where("id IN ?", ids).Update("remaining_quota", 0).Error; err != nil {
		return 0, err
	}
	return total, nil
}

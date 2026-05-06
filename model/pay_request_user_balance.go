package model

import (
	"errors"
	"one-api/common"
	relaycommon "one-api/relay/common"
	"strings"
	"unicode/utf8"

	"one-api/setting/payg_setting"

	"gorm.io/gorm"
)

// resolvePayRequestBalanceAllowedGroupIDs returns the effective allowed group IDs for a balance item.
// For product-based balances, allowed groups should follow current product config so that
// product group changes (add/remove) apply immediately to all historical purchases.
func resolvePayRequestBalanceAllowedGroupIDs(b PayRequestUserBalance) ([]int, error) {
	return resolveEffectivePrepaidBalanceAllowedGroupIDsTx(DB, effectivePrepaidBalanceAllowedGroupLookup{
		LoadCurrentProductGroupIDs: getPayRequestProductGroupIDsTx,
		LoadConfiguredGroupIDs: func(productID int) ([]int, bool) {
			p, ok := payg_setting.FindPayRequestProductByID(productID)
			if !ok {
				return nil, false
			}
			return p.AllowedGroupIds, true
		},
	}, effectivePrepaidBalanceAllowedGroupOptions{
		ProductID:              b.ProductId,
		FollowCurrentProduct:   true,
		StoredGroupIDs:         b.AllowedGroupIds,
		StoredGroups:           b.AllowedGroups,
		EmptyProductMessage:    "按次付费商品可用分组为空",
		EmptySnapshotMessage:   "按次付费可用分组为空",
		MissingSnapshotMessage: "按次付费缺少 allowed_group_ids",
	})
}

// ResolvePayRequestBalanceAllowedGroupIDs returns the effective allowed group IDs for a balance item.
func ResolvePayRequestBalanceAllowedGroupIDs(b PayRequestUserBalance) ([]int, error) {
	return resolvePayRequestBalanceAllowedGroupIDs(b)
}

// ResolvePayRequestBalanceAllowedGroups returns the effective allowed group codes for a balance item.
func ResolvePayRequestBalanceAllowedGroups(b PayRequestUserBalance) ([]string, error) {
	ids, err := resolvePayRequestBalanceAllowedGroupIDs(b)
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
		return nil, errors.New("按次付费可用分组为空")
	}
	return normalized, nil
}

func buildPayRequestConsumableAllocationsFromBalances(balances []PayRequestUserBalance, groupID int, requiredRequests int) ([]relaycommon.ProductQuotaAllocation, bool, error) {
	if groupID <= 0 {
		return nil, false, errors.New("group_id 无效")
	}
	if requiredRequests <= 0 {
		requiredRequests = 1
	}
	left := requiredRequests
	allocations := make([]relaycommon.ProductQuotaAllocation, 0, len(balances))
	for _, balance := range balances {
		if balance.RemainingRequests <= 0 {
			continue
		}
		ids, err := resolvePayRequestBalanceAllowedGroupIDs(balance)
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
		useRequests := balance.RemainingRequests
		if useRequests > left {
			useRequests = left
		}
		if useRequests <= 0 {
			continue
		}
		allocations = append(allocations, relaycommon.ProductQuotaAllocation{
			ProductId: balance.ProductId,
			Quota:     useRequests,
		})
		left -= useRequests
		if left <= 0 {
			return allocations, true, nil
		}
	}
	return allocations, false, nil
}

func firstProductIDFromProductAllocations(allocations []relaycommon.ProductQuotaAllocation) int {
	for _, allocation := range allocations {
		if allocation.ProductId != 0 && allocation.Quota > 0 {
			return allocation.ProductId
		}
	}
	return 0
}

// PayRequestUserBalance stores a user's prepaid-request balance per product.
// One (user_id, product_id) => one row.
type PayRequestUserBalance struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId    int `json:"user_id" gorm:"not null;index;uniqueIndex:idx_pay_request_user_product"`
	ProductId int `json:"product_id" gorm:"not null;index;uniqueIndex:idx_pay_request_user_product;column:product_id"`

	ProductName string `json:"product_name" gorm:"type:varchar(64);default:'';column:product_name"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`

	// AllowedGroupIds is the source of truth. allowed_groups is legacy-only snapshot.
	AllowedGroupIds   JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`
	AllowedGroups     JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	RemainingRequests int       `json:"remaining_requests" gorm:"type:bigint;default:0;column:remaining_requests"`
	HistoryRequests   int       `json:"history_requests" gorm:"type:bigint;default:0;column:history_requests"`

	CreatedAt int64 `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (b *PayRequestUserBalance) Validate() error {
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
	if b.RemainingRequests < 0 {
		return errors.New("remaining_requests 不能小于 0")
	}
	if b.HistoryRequests < 0 {
		return errors.New("history_requests 不能小于 0")
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

// GetUserPayRequestBalances retrieves all pay-request balances for a user.
func GetUserPayRequestBalances(userId int, onlyPositive bool) ([]PayRequestUserBalance, error) {
	return GetUserPayRequestBalancesTx(DB, userId, onlyPositive)
}

// GetUserPayRequestBalancesTx retrieves all pay-request balances for a user within a transaction.
func GetUserPayRequestBalancesTx(tx *gorm.DB, userId int, onlyPositive bool) ([]PayRequestUserBalance, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	var balances []PayRequestUserBalance
	q := tx.Model(&PayRequestUserBalance{}).Where("user_id = ?", userId)
	if onlyPositive {
		q = q.Where("remaining_requests > 0")
	}
	if err := q.Order("sort_order DESC, product_id DESC, id DESC").Find(&balances).Error; err != nil {
		return nil, err
	}
	return balances, nil
}

// UpsertPayRequestUserBalanceTx creates or updates a user's pay-request balance for a specific product.
func UpsertPayRequestUserBalanceTx(tx *gorm.DB, userId int, productId int, productName string, sortOrder int, allowedGroupIDs []int, addRequests int) error {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if productId == 0 {
		return errors.New("productId 无效")
	}
	if addRequests <= 0 {
		return errors.New("addRequests 必须大于 0")
	}
	addStoredRequests := discreteUnitsFromDisplayInt(addRequests)
	if addStoredRequests <= 0 {
		return errors.New("addRequests 必须大于 0")
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

	var existing PayRequestUserBalance
	err = lockForUpdate(tx).
		Where("user_id = ? AND product_id = ?", userId, productId).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			b := &PayRequestUserBalance{
				UserId:            userId,
				ProductId:         productId,
				ProductName:       strings.TrimSpace(productName),
				SortOrder:         sortOrder,
				AllowedGroupIds:   normalizedGroupIDsJSON,
				AllowedGroups:     nil,
				RemainingRequests: addStoredRequests,
				HistoryRequests:   addStoredRequests,
			}
			if vErr := b.Validate(); vErr != nil {
				return vErr
			}
			return tx.Create(b).Error
		}
		return err
	}

	updates := map[string]interface{}{
		"remaining_requests": gorm.Expr("remaining_requests + ?", addStoredRequests),
		"history_requests":   gorm.Expr("history_requests + ?", addStoredRequests),
		"allowed_group_ids":  normalizedGroupIDsJSON,
		"sort_order":         sortOrder,
	}
	if strings.TrimSpace(productName) != "" {
		updates["product_name"] = strings.TrimSpace(productName)
	}
	return tx.Model(&PayRequestUserBalance{}).Where("id = ?", existing.Id).Updates(updates).Error
}

// UnionPayRequestAllowedGroupsFromBalances computes the union of all allowed group IDs from balances.
func UnionPayRequestAllowedGroupsFromBalances(balances []PayRequestUserBalance) (JSONValue, error) {
	if len(balances) == 0 {
		return nil, nil
	}
	groupIDs := make([]int, 0, 16)
	seen := make(map[int]struct{}, 16)
	for _, b := range balances {
		if b.RemainingRequests <= 0 {
			continue
		}
		ids, err := resolvePayRequestBalanceAllowedGroupIDs(b)
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

// FindUserPayRequestConsumableProductIdTx finds a product that can be consumed for the given group.
func FindUserPayRequestConsumableProductIdTx(tx *gorm.DB, userId int, groupID int, requiredRequests int) (productId int, ok bool, err error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, false, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return 0, false, errors.New("group_id 无效")
	}
	if requiredRequests <= 0 {
		requiredRequests = 1
	}

	var balances []PayRequestUserBalance
	if err := tx.Model(&PayRequestUserBalance{}).
		Select("product_id", "sort_order", "allowed_group_ids", "allowed_groups", "remaining_requests").
		Where("user_id = ? AND remaining_requests > 0", userId).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return 0, false, err
	}
	allocations, ok, err := buildPayRequestConsumableAllocationsFromBalances(balances, groupID, requiredRequests)
	if err != nil {
		return 0, false, err
	}
	if !ok || len(allocations) == 0 {
		return 0, false, nil
	}
	return allocations[0].ProductId, true, nil
}

// GetUserPayRequestBalanceInfoTx returns the total remaining requests and union of allowed group IDs.
func GetUserPayRequestBalanceInfoTx(tx *gorm.DB, userId int) (totalRemaining int, allowedGroupIDs []int, err error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, nil, errors.New("userId 无效")
	}

	var balances []PayRequestUserBalance
	if err := tx.Model(&PayRequestUserBalance{}).
		Select("product_id", "allowed_group_ids", "allowed_groups", "remaining_requests").
		Where("user_id = ? AND remaining_requests > 0", userId).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return 0, nil, err
	}
	total := 0
	unionIDs := make([]int, 0, 16)
	seen := make(map[int]struct{}, 16)
	for _, b := range balances {
		if b.RemainingRequests <= 0 {
			continue
		}
		total += b.RemainingRequests
		ids, err := resolvePayRequestBalanceAllowedGroupIDs(b)
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

func consumeUserPayRequestQuotaWithAllocations(userId int, groupID int, count int) ([]relaycommon.ProductQuotaAllocation, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return nil, errors.New("group_id 无效")
	}
	if count <= 0 {
		return nil, errors.New("count 无效")
	}

	var allocations []relaycommon.ProductQuotaAllocation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).
			Select("id", "pay_request_quota", "pay_request_history_quota", "pay_request_allowed_groups").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}
		if _, err := syncLockedUserPayRequestSnapshotFromBalancesTx(tx, &user); err != nil {
			return err
		}
		if user.PayRequestQuota < count {
			return errors.New("按次付费次数不足")
		}

		var balances []PayRequestUserBalance
		if err := lockForUpdate(tx).
			Select("id", "product_id", "allowed_group_ids", "allowed_groups", "remaining_requests", "sort_order").
			Where("user_id = ? AND remaining_requests > 0", userId).
			Order("sort_order DESC, product_id DESC, id DESC").
			Find(&balances).Error; err != nil {
			return err
		}
		if len(balances) == 0 {
			return errors.New("按次付费次数不足")
		}

		resolvedAllocations, ok, err := buildPayRequestConsumableAllocationsFromBalances(balances, groupID, count)
		if err != nil {
			return err
		}
		if !ok || len(resolvedAllocations) == 0 {
			return errors.New("按次付费次数不足")
		}

		indexByProductID := make(map[int]int, len(balances))
		for i := range balances {
			indexByProductID[balances[i].ProductId] = i
		}
		for _, allocation := range resolvedAllocations {
			idx, ok := indexByProductID[allocation.ProductId]
			if !ok {
				return errors.New("按次付费商品余额不存在")
			}
			if balances[idx].RemainingRequests < allocation.Quota {
				return errors.New("按次付费次数不足")
			}
			if err := tx.Model(&PayRequestUserBalance{}).
				Where("id = ?", balances[idx].Id).
				Update("remaining_requests", gorm.Expr("remaining_requests - ?", allocation.Quota)).Error; err != nil {
				return err
			}
			balances[idx].RemainingRequests -= allocation.Quota
		}

		if _, err := syncLockedUserPayRequestSnapshotFromBalancesTx(tx, &user); err != nil {
			return err
		}
		allocations = cloneProductQuotaAllocations(resolvedAllocations)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return allocations, nil
}

func DecreaseUserPayRequestQuotaWithAllocations(userId int, groupID int, count int) ([]relaycommon.ProductQuotaAllocation, error) {
	allocations, err := consumeUserPayRequestQuotaWithAllocations(userId, groupID, count)
	if err != nil {
		return nil, err
	}
	if err := invalidateUserCache(userId); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return allocations, nil
}

func restoreUserPayRequestQuotaWithAllocations(userId int, allocations []relaycommon.ProductQuotaAllocation) error {
	normalizedAllocations := cloneProductQuotaAllocations(allocations)
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if len(normalizedAllocations) == 0 {
		return errors.New("按次付费商品未指定，无法返还次数")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).
			Select("id", "pay_request_quota", "pay_request_history_quota", "pay_request_allowed_groups").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}

		for _, allocation := range normalizedAllocations {
			var balance PayRequestUserBalance
			if err := lockForUpdate(tx).
				Where("user_id = ? AND product_id = ?", userId, allocation.ProductId).
				First(&balance).Error; err != nil {
				return err
			}
			if balance.RemainingRequests < 0 {
				return errors.New("remaining_requests 状态错误")
			}
			if err := tx.Model(&PayRequestUserBalance{}).
				Where("id = ?", balance.Id).
				Update("remaining_requests", gorm.Expr("remaining_requests + ?", allocation.Quota)).Error; err != nil {
				return err
			}
		}

		_, err := syncLockedUserPayRequestSnapshotFromBalancesTx(tx, &user)
		return err
	})
}

func ReturnUserPayRequestQuotaWithAllocations(userId int, allocations []relaycommon.ProductQuotaAllocation) error {
	if err := restoreUserPayRequestQuotaWithAllocations(userId, allocations); err != nil {
		return err
	}
	if err := invalidateUserCache(userId); err != nil {
		common.SysLog("failed to invalidate user cache: " + err.Error())
	}
	return nil
}

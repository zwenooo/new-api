package model

import (
	"errors"
	"strings"
	"unicode/utf8"

	"one-api/setting/payg_setting"

	"gorm.io/gorm"
)

func resolvePayTokenBalanceAllowedGroupIDs(b PayTokenUserBalance) ([]int, error) {
	return resolveEffectivePrepaidBalanceAllowedGroupIDsTx(DB, effectivePrepaidBalanceAllowedGroupLookup{
		LoadCurrentProductGroupIDs: getPayTokenProductGroupIDsTx,
		LoadConfiguredGroupIDs: func(productID int) ([]int, bool) {
			p, ok := payg_setting.FindPayTokenProductByID(productID)
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
		EmptyProductMessage:    "按token付费商品可用分组为空",
		EmptySnapshotMessage:   "按token付费可用分组为空",
		MissingSnapshotMessage: "按token付费缺少 allowed_group_ids",
	})
}

// ResolvePayTokenBalanceAllowedGroupIDs returns the effective allowed group ids for a balance item.
// For product-based balances, it follows current product config; otherwise it falls back to stored snapshot.
func ResolvePayTokenBalanceAllowedGroupIDs(b PayTokenUserBalance) ([]int, error) {
	return resolvePayTokenBalanceAllowedGroupIDs(b)
}

// ResolvePayTokenBalanceAllowedGroups returns the effective allowed group codes for a balance item.
// It is derived from ResolvePayTokenBalanceAllowedGroupIDs for display/legacy compatibility.
func ResolvePayTokenBalanceAllowedGroups(b PayTokenUserBalance) ([]string, error) {
	ids, err := resolvePayTokenBalanceAllowedGroupIDs(b)
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
		return nil, errors.New("按token付费可用分组为空")
	}
	return normalized, nil
}

// PayTokenUserBalance stores a user's prepaid-token balance per product.
// One (user_id, product_id) => one row.
type PayTokenUserBalance struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId    int `json:"user_id" gorm:"not null;index;uniqueIndex:idx_pay_token_user_product"`
	ProductId int `json:"product_id" gorm:"not null;index;uniqueIndex:idx_pay_token_user_product;column:product_id"`

	ProductName string `json:"product_name" gorm:"type:varchar(64);default:'';column:product_name"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`

	// AllowedGroupIds is the source of truth. allowed_groups is legacy-only snapshot.
	AllowedGroupIds JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`
	AllowedGroups   JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`

	RemainingTokens int `json:"remaining_tokens" gorm:"type:bigint;default:0;column:remaining_tokens"`
	HistoryTokens   int `json:"history_tokens" gorm:"type:bigint;default:0;column:history_tokens"`

	CreatedAt int64 `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (PayTokenUserBalance) TableName() string {
	return "pay_token_user_balances"
}

func (b *PayTokenUserBalance) Validate() error {
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
	if b.RemainingTokens < 0 {
		return errors.New("remaining_tokens 不能小于 0")
	}
	if b.HistoryTokens < 0 {
		return errors.New("history_tokens 不能小于 0")
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

func GetUserPayTokenBalances(userId int, onlyPositive bool) ([]PayTokenUserBalance, error) {
	return GetUserPayTokenBalancesTx(DB, userId, onlyPositive)
}

func GetUserPayTokenBalancesTx(tx *gorm.DB, userId int, onlyPositive bool) ([]PayTokenUserBalance, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	var balances []PayTokenUserBalance
	q := tx.Model(&PayTokenUserBalance{}).Where("user_id = ?", userId)
	if onlyPositive {
		q = q.Where("remaining_tokens > 0")
	}
	if err := q.Order("sort_order DESC, product_id DESC, id DESC").Find(&balances).Error; err != nil {
		return nil, err
	}
	return balances, nil
}

func UpsertPayTokenUserBalanceTx(tx *gorm.DB, userId int, productId int, productName string, sortOrder int, allowedGroupIDs []int, addTokens int) error {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if productId == 0 {
		return errors.New("productId 无效")
	}
	if addTokens <= 0 {
		return errors.New("addTokens 必须大于 0")
	}
	addStoredTokens := discreteUnitsFromDisplayInt(addTokens)
	if addStoredTokens <= 0 {
		return errors.New("addTokens 必须大于 0")
	}
	if sortOrder < 0 {
		return errors.New("sort_order 不能小于 0")
	}
	if strings.TrimSpace(productName) != "" && utf8.RuneCountInString(strings.TrimSpace(productName)) > 64 {
		return errors.New("product_name 过长")
	}

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

	var existing PayTokenUserBalance
	err = lockForUpdate(tx).
		Where("user_id = ? AND product_id = ?", userId, productId).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			b := &PayTokenUserBalance{
				UserId:          userId,
				ProductId:       productId,
				ProductName:     strings.TrimSpace(productName),
				SortOrder:       sortOrder,
				AllowedGroupIds: normalizedGroupIDsJSON,
				AllowedGroups:   nil,
				RemainingTokens: addStoredTokens,
				HistoryTokens:   addStoredTokens,
			}
			if vErr := b.Validate(); vErr != nil {
				return vErr
			}
			return tx.Create(b).Error
		}
		return err
	}

	updates := map[string]interface{}{
		"remaining_tokens":  gorm.Expr("remaining_tokens + ?", addStoredTokens),
		"history_tokens":    gorm.Expr("history_tokens + ?", addStoredTokens),
		"allowed_group_ids": normalizedGroupIDsJSON,
		"sort_order":        sortOrder,
	}
	if strings.TrimSpace(productName) != "" {
		updates["product_name"] = strings.TrimSpace(productName)
	}
	return tx.Model(&PayTokenUserBalance{}).Where("id = ?", existing.Id).Updates(updates).Error
}

func UnionPayTokenAllowedGroupsFromBalances(balances []PayTokenUserBalance) (JSONValue, error) {
	if len(balances) == 0 {
		return nil, nil
	}
	groupIDs := make([]int, 0, 16)
	seen := make(map[int]struct{}, 16)
	for _, b := range balances {
		if b.RemainingTokens <= 0 {
			continue
		}
		ids, err := resolvePayTokenBalanceAllowedGroupIDs(b)
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

func FindUserPayTokenConsumableProductIdTx(tx *gorm.DB, userId int, groupID int, requiredTokens int) (productId int, ok bool, err error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, false, errors.New("userId 无效")
	}
	if groupID <= 0 {
		return 0, false, errors.New("group_id 无效")
	}
	if requiredTokens <= 0 {
		requiredTokens = 1
	}

	var balances []PayTokenUserBalance
	if err := tx.Model(&PayTokenUserBalance{}).
		Select("product_id", "sort_order", "allowed_group_ids", "allowed_groups", "remaining_tokens").
		Where("user_id = ? AND remaining_tokens > 0", userId).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return 0, false, err
	}
	for _, b := range balances {
		if b.RemainingTokens < requiredTokens {
			continue
		}
		ids, err := resolvePayTokenBalanceAllowedGroupIDs(b)
		if err != nil {
			return 0, false, err
		}
		for _, gid := range ids {
			if gid == groupID {
				return b.ProductId, true, nil
			}
		}
	}
	return 0, false, nil
}

func GetUserPayTokenBalanceInfoTx(tx *gorm.DB, userId int) (totalRemaining int, allowedGroupIDs []int, err error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, nil, errors.New("userId 无效")
	}

	var balances []PayTokenUserBalance
	if err := tx.Model(&PayTokenUserBalance{}).
		Select("product_id", "allowed_group_ids", "allowed_groups", "remaining_tokens").
		Where("user_id = ? AND remaining_tokens > 0", userId).
		Order("sort_order DESC, product_id DESC, id DESC").
		Find(&balances).Error; err != nil {
		return 0, nil, err
	}
	total := 0
	unionIDs := make([]int, 0, 16)
	seen := make(map[int]struct{}, 16)
	for _, b := range balances {
		if b.RemainingTokens <= 0 {
			continue
		}
		total += b.RemainingTokens
		ids, err := resolvePayTokenBalanceAllowedGroupIDs(b)
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

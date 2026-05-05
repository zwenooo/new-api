package model

import (
	"errors"
	"fmt"
	"one-api/common"

	"gorm.io/gorm"
)

// BackfillPayRequestUserBalancesAllowedGroupIDs fills pay_request_user_balances.allowed_group_ids for legacy rows
// that only have allowed_groups (or rely on product bindings).
func BackfillPayRequestUserBalancesAllowedGroupIDs(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
		return nil
	}

	type row struct {
		Id            int       `gorm:"column:id"`
		UserId        int       `gorm:"column:user_id"`
		ProductId     int       `gorm:"column:product_id"`
		AllowedGroups JSONValue `gorm:"column:allowed_groups"`
	}

	var rows []row
	query := tx.Model(&PayRequestUserBalance{}).
		Select("id", "user_id", "product_id", "allowed_groups").
		Where("remaining_requests > 0")
	query = query.Where(jsonColumnIsEmptyCondition("allowed_group_ids"))
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	updated := 0
	for _, r := range rows {
		if r.Id <= 0 {
			continue
		}
		var groupIDs []int

		if len(r.AllowedGroups) > 0 {
			ids, idErr := ParseGroupIDsJSON(r.AllowedGroups)
			if idErr == nil && len(ids) > 0 {
				groupIDs = ids
			} else {
				codes, codeErr := ParseGroupNamesJSON(r.AllowedGroups)
				if codeErr != nil {
					if idErr != nil {
						return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d allowed_groups 解析失败: %w", r.Id, idErr)
					}
					return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d allowed_groups 解析失败: %w", r.Id, codeErr)
				}
				if len(codes) > 0 {
					ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
					if err != nil {
						return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d 分组回填失败: %w", r.Id, err)
					}
					groupIDs = ids
				}
			}
		}

		if len(groupIDs) == 0 && r.ProductId > 0 {
			ids, err := getPayRequestProductGroupIDsTx(tx, r.ProductId)
			if err != nil {
				return err
			}
			groupIDs = ids
		}

		// Fall back to user-level pay_request_allowed_groups when both snapshot and product bindings are missing
		// (legacy rows with sentinel product_id).
		if len(groupIDs) == 0 && r.UserId > 0 {
			type urow struct {
				PayRequestAllowedGroups JSONValue `gorm:"column:pay_request_allowed_groups"`
			}
			var u urow
			if err := tx.Model(&User{}).Select("pay_request_allowed_groups").Where("id = ?", r.UserId).First(&u).Error; err != nil {
				return err
			}
			ids, idErr := ParseGroupIDsJSON(u.PayRequestAllowedGroups)
			if idErr == nil && len(ids) > 0 {
				groupIDs = ids
			} else {
				codes, codeErr := ParseGroupNamesJSON(u.PayRequestAllowedGroups)
				if codeErr != nil {
					if idErr != nil {
						return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d user#%d pay_request_allowed_groups 解析失败: %w", r.Id, r.UserId, idErr)
					}
					return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d user#%d pay_request_allowed_groups 解析失败: %w", r.Id, r.UserId, codeErr)
				}
				if len(codes) > 0 {
					ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
					if err != nil {
						return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d user#%d pay_request_allowed_groups 回填失败: %w", r.Id, r.UserId, err)
					}
					groupIDs = ids
				}
			}
		}

			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
				if err != nil {
					return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d user_id=%d product_id=%d 回退分组失败: %w", r.Id, r.UserId, r.ProductId, err)
				}
				groupIDs = []int{legacyDefaultModelGroupID}
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return fmt.Errorf("backfill pay_request_user_balances.allowed_group_ids: balance#%d 分组无效: %w", r.Id, err)
			}
		groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}

		if err := tx.Model(&PayRequestUserBalance{}).Where("id = ?", r.Id).Update("allowed_group_ids", groupIDsJSON).Error; err != nil {
			return err
		}
		updated++
	}

	common.SysLog(fmt.Sprintf("backfilled pay_request_user_balances.allowed_group_ids for %d rows", updated))
	return nil
}

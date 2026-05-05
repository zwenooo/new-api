package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BackfillPaygUserBalancesFromLegacyUsers migrates legacy user-level PAYG fields
// (users.payg_quota + users.payg_allowed_groups) into per-product balances.
//
// Rule:
//   - If a user has payg_quota > 0 AND no payg_user_balances rows, create one legacy item:
//     product_id = -1, remaining_quota = users.payg_quota, allowed_groups = users.payg_allowed_groups.
//   - This keeps existing PAYG users consumable after introducing per-product PAYG balances.
func BackfillPaygUserBalancesFromLegacyUsers(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&PaygUserBalance{}) {
		return nil
	}

	type legacyRow struct {
		Id                int       `gorm:"column:id"`
		PaygQuota         int       `gorm:"column:payg_quota"`
		PaygHistoryQuota  int       `gorm:"column:payg_history_quota"`
		PaygAllowedGroups JSONValue `gorm:"column:payg_allowed_groups"`
	}

	var rows []legacyRow
	// Find users with legacy PAYG balance but without any per-product balances.
	if err := tx.Table("users u").
		Select("u.id, u.payg_quota, u.payg_history_quota, u.payg_allowed_groups").
		Joins("LEFT JOIN payg_user_balances b ON b.user_id = u.id").
		Where("u.payg_quota > 0").
		Where("b.id IS NULL").
		Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	now := time.Now().Unix()
	if now <= 0 || now > common.MaxSupportedUnixTimestamp {
		return errors.New("系统时间异常")
	}

	for _, r := range rows {
		if r.Id <= 0 {
			continue
		}
		if r.PaygQuota <= 0 {
			continue
		}
		groupIDs, idErr := ParseGroupIDsJSON(r.PaygAllowedGroups)
		if idErr != nil || len(groupIDs) == 0 {
			codes, codeErr := ParseGroupNamesJSON(r.PaygAllowedGroups)
			if codeErr != nil {
				if idErr != nil {
					return fmt.Errorf("backfill payg_user_balances: 用户 %d payg_allowed_groups 解析失败: %w", r.Id, idErr)
				}
				return fmt.Errorf("backfill payg_user_balances: 用户 %d payg_allowed_groups 解析失败: %w", r.Id, codeErr)
			}
			if len(codes) == 0 {
				return fmt.Errorf("backfill payg_user_balances: 用户 %d payg_allowed_groups 为空", r.Id)
			}
			ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
			if err != nil {
				return fmt.Errorf("backfill payg_user_balances: 用户 %d payg_allowed_groups 回填失败: %w", r.Id, err)
			}
			groupIDs = ids
		}
		if len(groupIDs) == 0 {
			legacyPaygModelGroupID, err := ResolveLegacyPaygModelGroupID(tx)
			if err != nil {
				return fmt.Errorf("backfill payg_user_balances: 用户 %d payg_allowed_groups 回退失败: %w", r.Id, err)
			}
			groupIDs = []int{legacyPaygModelGroupID}
		}
		if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
			return fmt.Errorf("backfill payg_user_balances: 用户 %d payg_allowed_groups 无效: %w", r.Id, err)
		}
		groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}

		history := r.PaygHistoryQuota
		if history < r.PaygQuota {
			history = r.PaygQuota
		}
		bal := &PaygUserBalance{
			UserId:          r.Id,
			ProductId:       -1,
			ProductName:     "",
			SortOrder:       0,
			AllowedGroupIds: groupIDsJSON,
			AllowedGroups:   nil,
			RemainingQuota:  r.PaygQuota,
			HistoryQuota:    history,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if vErr := bal.Validate(); vErr != nil {
			return vErr
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(bal).Error; err != nil {
			return err
		}
	}
	common.SysLog(fmt.Sprintf("backfilled payg_user_balances for %d users", len(rows)))
	return nil
}

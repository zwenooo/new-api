package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"time"

	"gorm.io/gorm"
)

// BackfillPayTokenUserBalancesFromLegacyUsers migrates legacy user-level pay-token fields
// into pay_token_user_balances as a legacy balance row (product_id = -1).
func BackfillPayTokenUserBalancesFromLegacyUsers(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&PayTokenUserBalance{}) {
		return nil
	}

	type legacyRow struct {
		Id                    int       `gorm:"column:id"`
		PayTokenQuota         int       `gorm:"column:pay_token_quota"`
		PayTokenHistoryQuota  int       `gorm:"column:pay_token_history_quota"`
		PayTokenAllowedGroups JSONValue `gorm:"column:pay_token_allowed_groups"`
	}

	var rows []legacyRow
	if err := tx.Table("users u").
		Select("u.id, u.pay_token_quota, u.pay_token_history_quota, u.pay_token_allowed_groups").
		Joins("LEFT JOIN pay_token_user_balances b ON b.user_id = u.id").
		Where("u.pay_token_quota > 0").
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
			if r.Id <= 0 || r.PayTokenQuota <= 0 {
				continue
			}

			groupIDs, idErr := ParseGroupIDsJSON(r.PayTokenAllowedGroups)
			if idErr != nil || len(groupIDs) == 0 {
				codes, codeErr := ParseGroupNamesJSON(r.PayTokenAllowedGroups)
				if codeErr != nil {
					if idErr != nil {
						return fmt.Errorf("backfill pay_token_user_balances: 用户 %d pay_token_allowed_groups 解析失败: %w", r.Id, idErr)
					}
					return fmt.Errorf("backfill pay_token_user_balances: 用户 %d pay_token_allowed_groups 解析失败: %w", r.Id, codeErr)
				}
				if len(codes) == 0 {
					return fmt.Errorf("backfill pay_token_user_balances: 用户 %d pay_token_allowed_groups 为空", r.Id)
				}
				var err error
				groupIDs, _, err = existingLegacyGroupIDsFromCodes(tx, codes)
				if err != nil {
					return fmt.Errorf("backfill pay_token_user_balances: 用户 %d 分组代码转换失败: %w", r.Id, err)
				}
				if len(groupIDs) == 0 {
					legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
					if err != nil {
						return fmt.Errorf("backfill pay_token_user_balances: 用户 %d 分组代码回退失败: %w", r.Id, err)
					}
					groupIDs = []int{legacyDefaultModelGroupID}
				}
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return fmt.Errorf("backfill pay_token_user_balances: 用户 %d 分组 ID 为空", r.Id)
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return fmt.Errorf("backfill pay_token_user_balances: 用户 %d pay_token_allowed_groups 无效: %w", r.Id, err)
			}
			groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
			if err != nil {
				return fmt.Errorf("backfill pay_token_user_balances: 用户 %d 分组 ID 序列化失败: %w", r.Id, err)
			}

		historyTokens := r.PayTokenHistoryQuota
		if historyTokens < r.PayTokenQuota {
			historyTokens = r.PayTokenQuota
		}

		balance := &PayTokenUserBalance{
			UserId:          r.Id,
			ProductId:       -1,
			ProductName:     "",
			SortOrder:       0,
			AllowedGroupIds: groupIDsJSON,
			AllowedGroups:   nil,
			RemainingTokens: r.PayTokenQuota,
			HistoryTokens:   historyTokens,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(balance).Error; err != nil {
			return fmt.Errorf("backfill pay_token_user_balances: 用户 %d 创建余额记录失败: %w", r.Id, err)
		}
	}

	common.SysLog(fmt.Sprintf("backfilled %d pay_token_user_balances from legacy users", len(rows)))
	return nil
}

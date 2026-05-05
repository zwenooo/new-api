package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"time"

	"gorm.io/gorm"
)

// BackfillPayRequestUserBalancesFromLegacyUsers migrates legacy pay_request_quota from users table
// to the new pay_request_user_balances table as a legacy balance row.
func BackfillPayRequestUserBalancesFromLegacyUsers(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
		return nil
	}

	type legacyRow struct {
		Id                      int       `gorm:"column:id"`
		PayRequestQuota         int       `gorm:"column:pay_request_quota"`
		PayRequestHistoryQuota  int       `gorm:"column:pay_request_history_quota"`
		PayRequestAllowedGroups JSONValue `gorm:"column:pay_request_allowed_groups"`
	}

	var rows []legacyRow
	// Find users with legacy pay_request balance but without any per-product balances.
	if err := tx.Table("users u").
		Select("u.id, u.pay_request_quota, u.pay_request_history_quota, u.pay_request_allowed_groups").
		Joins("LEFT JOIN pay_request_user_balances b ON b.user_id = u.id").
		Where("u.pay_request_quota > 0").
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
			if r.PayRequestQuota <= 0 {
				continue
			}
			groupIDs, idErr := ParseGroupIDsJSON(r.PayRequestAllowedGroups)
			if idErr != nil || len(groupIDs) == 0 {
				codes, codeErr := ParseGroupNamesJSON(r.PayRequestAllowedGroups)
				if codeErr != nil {
					if idErr != nil {
						return fmt.Errorf("backfill pay_request_user_balances: 用户 %d pay_request_allowed_groups 解析失败: %w", r.Id, idErr)
					}
					return fmt.Errorf("backfill pay_request_user_balances: 用户 %d pay_request_allowed_groups 解析失败: %w", r.Id, codeErr)
				}
				if len(codes) == 0 {
					return fmt.Errorf("backfill pay_request_user_balances: 用户 %d pay_request_allowed_groups 为空", r.Id)
				}
				var err error
				groupIDs, _, err = existingLegacyGroupIDsFromCodes(tx, codes)
				if err != nil {
					return fmt.Errorf("backfill pay_request_user_balances: 用户 %d 分组代码转换失败: %w", r.Id, err)
				}
				if len(groupIDs) == 0 {
					legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
					if err != nil {
						return fmt.Errorf("backfill pay_request_user_balances: 用户 %d 分组代码回退失败: %w", r.Id, err)
					}
					groupIDs = []int{legacyDefaultModelGroupID}
				}
			}
			groupIDs = normalizeUniqueSortedIDs(groupIDs)
			if len(groupIDs) == 0 {
				return fmt.Errorf("backfill pay_request_user_balances: 用户 %d 分组 ID 为空", r.Id)
			}
			if err := ValidateGroupIDsExist(tx, groupIDs); err != nil {
				return fmt.Errorf("backfill pay_request_user_balances: 用户 %d pay_request_allowed_groups 无效: %w", r.Id, err)
			}
			groupIDsJSON, err := MarshalGroupIDsJSON(groupIDs)
			if err != nil {
				return fmt.Errorf("backfill pay_request_user_balances: 用户 %d 分组 ID 序列化失败: %w", r.Id, err)
			}

		historyRequests := r.PayRequestHistoryQuota
		if historyRequests < r.PayRequestQuota {
			historyRequests = r.PayRequestQuota
		}

		balance := &PayRequestUserBalance{
			UserId:            r.Id,
			ProductId:         -1,
			ProductName:       "",
			SortOrder:         0,
			AllowedGroupIds:   groupIDsJSON,
			AllowedGroups:     nil,
			RemainingRequests: r.PayRequestQuota,
			HistoryRequests:   historyRequests,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.Create(balance).Error; err != nil {
			return fmt.Errorf("backfill pay_request_user_balances: 用户 %d 创建余额记录失败: %w", r.Id, err)
		}
	}

	common.SysLog(fmt.Sprintf("backfilled %d pay_request_user_balances from legacy users", len(rows)))
	return nil
}

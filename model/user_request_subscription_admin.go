package model

import (
	"errors"
	"strings"
	"time"

	"one-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RequestSubscriptionSummary struct {
	Id int `json:"id"`

	DailyRequestLimit     float64 `json:"daily_request_limit"`
	DailyRequestUsed      float64 `json:"daily_request_used"`
	DailyRequestRemaining float64 `json:"daily_request_remaining"`
	DailyRequestResetDate int     `json:"daily_request_reset_date"`

	TotalRequestLimit     float64 `json:"total_request_limit"`
	TotalRequestUsed      float64 `json:"total_request_used"`
	TotalRequestRemaining float64 `json:"total_request_remaining"`

	StartAt   int64 `json:"start_at"`
	ExpireAt  int64 `json:"expire_at"`
	InvalidAt int64 `json:"invalid_at"`

	AllowedGroupIds      []int  `json:"allowed_group_ids"`
	SortOrder            int    `json:"sort_order"`
	Source               string `json:"source"`
	SourceOrderId        int    `json:"source_order_id,omitempty"`
	SourceOrderTradeNo   string `json:"source_order_trade_no,omitempty"`
	SourceOrderQuantity  int    `json:"source_order_quantity,omitempty"`
	SourcePresetId       int    `json:"source_preset_id"`
	SourcePresetName     string `json:"source_preset_name,omitempty"`
	SourceRedemptionId   int    `json:"source_redemption_id"`
	SourceRedemptionName string `json:"source_redemption_name,omitempty"`
}

type RequestSubscriptionBreakdown struct {
	TodayDate int `json:"today_date"`

	DailyRequestLimitTotal     float64 `json:"daily_request_limit_total"`
	DailyRequestUsedTotal      float64 `json:"daily_request_used_total"`
	DailyRequestRemainingTotal float64 `json:"daily_request_remaining_total"`
	DailyRequestLimitUnlimited bool    `json:"daily_request_limit_unlimited_total"`

	TotalRequestLimitTotal     float64 `json:"total_request_limit_total"`
	TotalRequestUsedTotal      float64 `json:"total_request_used_total"`
	TotalRequestRemainingTotal float64 `json:"total_request_remaining_total"`
	TotalRequestLimitUnlimited bool    `json:"total_request_limit_unlimited_total"`

	Subscriptions []RequestSubscriptionSummary `json:"subscriptions"`
}

func GetUserRequestSubscriptionBreakdown(userId int) (*RequestSubscriptionBreakdown, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}

	result := &RequestSubscriptionBreakdown{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		today := common.GetTodayDateInt()

		var active []UserRequestSubscription
		if err := tx.
			Where("user_id = ? AND invalid_at = 0 AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?)", userId, now, now).
			Order(userRequestSubscriptionActiveOrderExpr).
			Find(&active).Error; err != nil {
			return err
		}

		var pending []UserRequestSubscription
		if err := tx.
			Where("user_id = ? AND invalid_at = 0 AND start_at > ? AND (expire_at = 0 OR expire_at >= ?)", userId, now, now).
			Order(userRequestSubscriptionPendingOrderExpr).
			Find(&pending).Error; err != nil {
			return err
		}

		var expired []UserRequestSubscription
		if err := tx.
			Where("user_id = ? AND invalid_at = 0 AND expire_at > 0 AND expire_at < ?", userId, now).
			Order(userRequestSubscriptionExpiredOrderExpr).
			Limit(20).
			Find(&expired).Error; err != nil {
			return err
		}

		combined := make([]UserRequestSubscription, 0, len(active)+len(pending)+len(expired))
		combined = append(combined, active...)
		combined = append(combined, pending...)
		combined = append(combined, expired...)

		productIDs := make([]int, 0, len(combined))
		redemptionIDs := make([]int, 0, len(combined))
		orderIDs := make([]int, 0, len(combined))
		for _, sub := range combined {
			if sub.SourcePresetId > 0 {
				productIDs = append(productIDs, sub.SourcePresetId)
			}
			if sub.SourceRedemptionId > 0 {
				redemptionIDs = append(redemptionIDs, sub.SourceRedemptionId)
			}
			if sub.SourceOrderId > 0 {
				orderIDs = append(orderIDs, sub.SourceOrderId)
			}
		}

		presetNameByID := make(map[int]string, 0)
		if normalized := normalizeUniqueSortedIDs(productIDs); len(normalized) > 0 {
			type row struct {
				Id   int    `gorm:"column:id"`
				Name string `gorm:"column:name"`
			}
			var rows []row
			if err := tx.Model(&RedemptionPreset{}).
				Select("id", "name").
				Where("id IN ?", normalized).
				Find(&rows).Error; err != nil {
				return err
			}
			presetNameByID = make(map[int]string, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				name := strings.TrimSpace(r.Name)
				if name == "" {
					continue
				}
				presetNameByID[r.Id] = name
			}
		}

		redemptionNameByID := make(map[int]string, 0)
		if normalized := normalizeUniqueSortedIDs(redemptionIDs); len(normalized) > 0 {
			type row struct {
				Id   int    `gorm:"column:id"`
				Name string `gorm:"column:name"`
			}
			var rows []row
			if err := tx.Model(&Redemption{}).
				Select("id", "name").
				Where("id IN ?", normalized).
				Find(&rows).Error; err != nil {
				return err
			}
			redemptionNameByID = make(map[int]string, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				name := strings.TrimSpace(r.Name)
				if name == "" {
					continue
				}
				redemptionNameByID[r.Id] = name
			}
		}

		type orderInfo struct {
			TradeNo  string
			Quantity int
		}
		orderInfoByID := make(map[int]orderInfo, 0)
		if normalized := normalizeUniqueSortedIDs(orderIDs); len(normalized) > 0 {
			type row struct {
				Id       int    `gorm:"column:id"`
				TradeNo  string `gorm:"column:trade_no"`
				Quantity int    `gorm:"column:quantity"`
			}
			var rows []row
			if err := tx.Model(&SubscriptionOrder{}).
				Select("id", "trade_no", "quantity").
				Where("id IN ?", normalized).
				Find(&rows).Error; err != nil {
				return err
			}
			orderInfoByID = make(map[int]orderInfo, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				orderInfoByID[r.Id] = orderInfo{
					TradeNo:  strings.TrimSpace(r.TradeNo),
					Quantity: r.Quantity,
				}
			}
		}

		groupIDsBySubID, err := resolveUserRequestSubscriptionAllowedGroupsTx(tx, combined)
		if err != nil {
			return err
		}

		items := make([]RequestSubscriptionSummary, 0, len(combined))
		limitTotal := 0
		usedTotal := 0
		remainingTotal := 0
		limitUnlimited := false

		totalLimitTotal := 0
		totalUsedTotal := 0
		totalRemainingTotal := 0
		totalUnlimited := false

		for _, sub := range combined {
			limit := sub.DailyRequestLimit
			used := sub.DailyRequestUsed
			resetDate := sub.DailyRequestResetDate
			if resetDate != today {
				used = 0
			}

			remaining := 0
			if limit > 0 {
				remaining = limit - used
				if remaining < 0 {
					remaining = 0
				}
			}

			totalLimit := sub.TotalRequestLimit
			totalUsed := sub.TotalRequestUsed
			totalRemaining := 0
			if totalLimit > 0 {
				totalRemaining = totalLimit - totalUsed
				if totalRemaining < 0 {
					totalRemaining = 0
				}
			}

			rawIDs := normalizeUniqueSortedIDs(groupIDsBySubID[sub.Id])

			activeNow := (sub.StartAt == 0 || sub.StartAt <= now) && (sub.ExpireAt == 0 || sub.ExpireAt >= now)
			if activeNow {
				usedTotal += used
				if limit == 0 {
					limitUnlimited = true
				} else if limit > 0 {
					limitTotal += limit
					remainingTotal += remaining
				}

				totalUsedTotal += totalUsed
				if totalLimit == 0 {
					totalUnlimited = true
				} else if totalLimit > 0 {
					totalLimitTotal += totalLimit
					totalRemainingTotal += totalRemaining
				}
			}

			orderInfo := orderInfoByID[sub.SourceOrderId]
			items = append(items, RequestSubscriptionSummary{
				Id:                    sub.Id,
				DailyRequestLimit:     discreteUnitsToDisplay(limit),
				DailyRequestUsed:      discreteUnitsToDisplay(used),
				DailyRequestRemaining: discreteUnitsToDisplay(remaining),
				DailyRequestResetDate: resetDate,
				TotalRequestLimit:     discreteUnitsToDisplay(totalLimit),
				TotalRequestUsed:      discreteUnitsToDisplay(totalUsed),
				TotalRequestRemaining: discreteUnitsToDisplay(totalRemaining),
				StartAt:               sub.StartAt,
				ExpireAt:              sub.ExpireAt,
				InvalidAt:             sub.InvalidAt,
				AllowedGroupIds:       rawIDs,
				SortOrder:             sub.SortOrder,
				Source:                sub.Source,
				SourceOrderId:         sub.SourceOrderId,
				SourceOrderTradeNo:    orderInfo.TradeNo,
				SourceOrderQuantity:   orderInfo.Quantity,
				SourcePresetId:        sub.SourcePresetId,
				SourcePresetName:      presetNameByID[sub.SourcePresetId],
				SourceRedemptionId:    sub.SourceRedemptionId,
				SourceRedemptionName:  redemptionNameByID[sub.SourceRedemptionId],
			})
		}

		result.TodayDate = today
		result.DailyRequestLimitTotal = discreteUnitsToDisplay(limitTotal)
		result.DailyRequestUsedTotal = discreteUnitsToDisplay(usedTotal)
		result.DailyRequestRemainingTotal = discreteUnitsToDisplay(remainingTotal)
		result.DailyRequestLimitUnlimited = limitUnlimited

		result.TotalRequestLimitTotal = discreteUnitsToDisplay(totalLimitTotal)
		result.TotalRequestUsedTotal = discreteUnitsToDisplay(totalUsedTotal)
		result.TotalRequestRemainingTotal = discreteUnitsToDisplay(totalRemainingTotal)
		result.TotalRequestLimitUnlimited = totalUnlimited
		result.Subscriptions = items
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ReorderUserRequestSubscriptions(userId int, orderedSubIDs []int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	ids := normalizeOrderedUniquePositiveIDs(orderedSubIDs)
	if len(ids) == 0 {
		return errors.New("subscription_ids 不能为空")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			Id int `gorm:"column:id"`
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&UserRequestSubscription{}).
			Select("id").
			Where("user_id = ? AND id IN ?", userId, ids).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return errors.New("次数订阅记录不存在")
		}

		caseSQL := "CASE id"
		args := make([]interface{}, 0, len(ids)*2)
		for idx, id := range ids {
			caseSQL += " WHEN ? THEN ?"
			args = append(args, id, len(ids)-idx)
		}
		caseSQL += " ELSE sort_order END"

		return tx.Model(&UserRequestSubscription{}).
			Where("user_id = ? AND id IN ?", userId, ids).
			Update("sort_order", gorm.Expr(caseSQL, args...)).Error
	})
}

type UpdateUserRequestSubscriptionParams struct {
	DailyRequestLimit *float64
	TotalRequestLimit *float64
	StartAt           *int64
	ExpireAt          *int64
	AllowedGroupIds   *[]int
}

func updateUserRequestSubscriptionTx(tx *gorm.DB, userId int, subId int, params UpdateUserRequestSubscriptionParams) (*UserRequestSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx 为空")
	}

	var sub UserRequestSubscription
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", subId, userId).
		First(&sub).Error; err != nil {
		return nil, err
	}

	nextLimit := sub.DailyRequestLimit
	if params.DailyRequestLimit != nil {
		if *params.DailyRequestLimit < 0 {
			return nil, errors.New("每日次数必须大于等于 0")
		}
		scaled, exact := discreteUnitsFromDisplayFloatExact(*params.DailyRequestLimit)
		if *params.DailyRequestLimit > 0 && !exact {
			return nil, errors.New("每日次数最多支持 3 位小数")
		}
		nextLimit = scaled
	}
	if nextLimit < 0 {
		return nil, errors.New("每日次数必须大于等于 0")
	}

	nextTotalLimit := sub.TotalRequestLimit
	if params.TotalRequestLimit != nil {
		if *params.TotalRequestLimit < 0 {
			return nil, errors.New("总次数必须大于等于 0")
		}
		scaled, exact := discreteUnitsFromDisplayFloatExact(*params.TotalRequestLimit)
		if *params.TotalRequestLimit > 0 && !exact {
			return nil, errors.New("总次数最多支持 3 位小数")
		}
		nextTotalLimit = scaled
	}
	if nextTotalLimit < 0 {
		return nil, errors.New("总次数必须大于等于 0")
	}
	if nextTotalLimit > 0 && sub.TotalRequestUsed > nextTotalLimit {
		return nil, errors.New("总次数不能小于已使用次数")
	}

	nextStartAt := sub.StartAt
	if params.StartAt != nil {
		nextStartAt = *params.StartAt
		if nextStartAt < 0 {
			return nil, errors.New("start_at 无效")
		}
		if nextStartAt == 0 {
			nextStartAt = time.Now().Unix()
		}
	}

	nextExpireAt := sub.ExpireAt
	if params.ExpireAt != nil {
		nextExpireAt = *params.ExpireAt
		if nextExpireAt < 0 {
			return nil, errors.New("expire_at 无效")
		}
	}
	if nextExpireAt > 0 && nextStartAt > nextExpireAt {
		return nil, errors.New("expire_at 必须晚于 start_at")
	}

	var nextAllowedGroupIDs []int
	if params.AllowedGroupIds != nil {
		if sub.SourcePresetId > 0 {
			return nil, errors.New("该订阅来源于商品，可用分组由商品控制，请修改商品配置")
		}
		nextAllowedGroupIDs = normalizeUniqueSortedIDs(*params.AllowedGroupIds)
	} else {
		resolved, err := resolveUserRequestSubscriptionAllowedGroupsTx(tx, []UserRequestSubscription{sub})
		if err != nil {
			return nil, err
		}
		nextAllowedGroupIDs = resolved[sub.Id]
	}
	if len(nextAllowedGroupIDs) == 0 {
		return nil, errors.New("可用分组不能为空")
	}
	if err := ValidateGroupIDsExist(tx, nextAllowedGroupIDs); err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if nextLimit != sub.DailyRequestLimit {
		updates["daily_request_limit"] = nextLimit
	}
	if nextTotalLimit != sub.TotalRequestLimit {
		updates["total_request_limit"] = nextTotalLimit
	}
	if nextStartAt != sub.StartAt {
		updates["start_at"] = nextStartAt
	}
	if nextExpireAt != sub.ExpireAt {
		updates["expire_at"] = nextExpireAt
	}

	if len(updates) > 0 {
		if err := tx.Model(&UserRequestSubscription{}).
			Where("id = ? AND user_id = ?", subId, userId).
			Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if params.AllowedGroupIds != nil {
		if err := upsertUserRequestSubscriptionGroupsTx(tx, sub.Id, nextAllowedGroupIDs); err != nil {
			return nil, err
		}
	}

	var updated UserRequestSubscription
	if err := tx.Where("id = ? AND user_id = ?", subId, userId).First(&updated).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func UpdateUserRequestSubscription(userId int, subId int, params UpdateUserRequestSubscriptionParams) (*UserRequestSubscription, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if subId <= 0 {
		return nil, errors.New("subId 无效")
	}

	var updated *UserRequestSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		result, err := updateUserRequestSubscriptionTx(tx, userId, subId, params)
		if err != nil {
			return err
		}
		updated = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func ActivatePendingUserRequestSubscription(userId int, subId int) (*UserRequestSubscription, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if subId <= 0 {
		return nil, errors.New("subId 无效")
	}

	var updated *UserRequestSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		var sub UserRequestSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", subId, userId).
			First(&sub).Error; err != nil {
			return err
		}
		if sub.InvalidAt > 0 || (sub.ExpireAt > 0 && sub.ExpireAt < now) {
			return errors.New("订阅已失效")
		}
		if sub.StartAt <= 0 || sub.StartAt <= now {
			return errors.New("订阅已生效")
		}

		nextStartAt, nextExpireAt, err := shiftPendingSubscriptionWindowToNow(sub.StartAt, sub.ExpireAt, now)
		if err != nil {
			return err
		}
		result, err := updateUserRequestSubscriptionTx(tx, userId, subId, UpdateUserRequestSubscriptionParams{
			StartAt:  &nextStartAt,
			ExpireAt: &nextExpireAt,
		})
		if err != nil {
			return err
		}
		updated = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func DeleteUserRequestSubscription(userId int, subId int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if subId <= 0 {
		return errors.New("subId 无效")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserRequestSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", subId, userId).
			First(&sub).Error; err != nil {
			return err
		}
		if tx.Migrator().HasTable(&UserRequestSubscriptionGroup{}) {
			if err := tx.Where("subscription_id = ?", sub.Id).Delete(&UserRequestSubscriptionGroup{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&sub).Error
	})
}

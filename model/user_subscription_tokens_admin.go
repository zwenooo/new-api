package model

import (
	"errors"
	"strings"
	"time"

	"one-api/common"

	"gorm.io/gorm"
)

type TokenSubscriptionSummary struct {
	Id int `json:"id"`

	TotalTokens     float64 `json:"total_tokens"`
	RemainingTokens float64 `json:"remaining_tokens"`
	ConsumedTokens  float64 `json:"consumed_tokens"`

	DailyTokensLimit float64 `json:"daily_tokens_limit"`
	DailyTokensUsed  float64 `json:"daily_tokens_used"`

	StartAt   int64 `json:"start_at"`
	ExpireAt  int64 `json:"expire_at"`
	InvalidAt int64 `json:"invalid_at"`

	AllowedGroupIds []int  `json:"allowed_group_ids"`
	Source          string `json:"source"`

	SourceOrderId       int    `json:"source_order_id,omitempty"`
	SourceOrderTradeNo  string `json:"source_order_trade_no,omitempty"`
	SourceOrderQuantity int    `json:"source_order_quantity,omitempty"`

	SourcePresetId   int    `json:"source_preset_id"`
	SourcePresetName string `json:"source_preset_name,omitempty"`

	SourceRedemptionId   int    `json:"source_redemption_id"`
	SourceRedemptionName string `json:"source_redemption_name,omitempty"`
}

type TokenSubscriptionGroupCapacity struct {
	GroupId int `json:"group_id"`

	TotalRemaining float64 `json:"total_remaining"`
	DailyCapacity  float64 `json:"daily_capacity"`

	TotalUnlimited bool `json:"total_unlimited"`
	DailyUnlimited bool `json:"daily_unlimited"`
}

type TokenSubscriptionBreakdown struct {
	Subscriptions   []TokenSubscriptionSummary       `json:"subscriptions"`
	GroupCapacities []TokenSubscriptionGroupCapacity `json:"group_capacities"`

	// ConfigErrors records validation errors (e.g. missing group limits) encountered during breakdown building.
	// It does not affect quota enforcement; enforcement happens in quota pre-check/consume flows.
	ConfigErrors []string `json:"config_errors,omitempty"`
}

func GetUserTokenSubscriptionBreakdown(userId int) (*TokenSubscriptionBreakdown, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}

	result := &TokenSubscriptionBreakdown{
		Subscriptions:   []TokenSubscriptionSummary{},
		GroupCapacities: []TokenSubscriptionGroupCapacity{},
	}

	var active []UserSubscription
	var pending []UserSubscription
	var expired []UserSubscription
	var allowedGroupIDsBySubID map[int][]int
	var presetNameByID map[int]string
	var redemptionNameByID map[int]string
	var orderInfoByID map[int]struct {
		TradeNo  string
		Quantity int
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}

		now := time.Now().Unix()
		if err := tx.
			Where("user_id = ? AND credited = ? AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, true, now, now, UserSubscriptionBillingUnitTokens).
			Order("CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC").
			Find(&active).Error; err != nil {
			return err
		}

		if err := tx.
			Where("user_id = ? AND credited = ? AND (expire_at = 0 OR expire_at >= ?) AND billing_unit = ?", userId, false, now, UserSubscriptionBillingUnitTokens).
			Order("start_at ASC, CASE WHEN expire_at = 0 THEN 1 ELSE 0 END, expire_at ASC, id ASC").
			Find(&pending).Error; err != nil {
			return err
		}

		if err := tx.
			Where("user_id = ? AND expire_at > 0 AND expire_at < ? AND billing_unit = ?", userId, now, UserSubscriptionBillingUnitTokens).
			Order("expire_at DESC, id DESC").
			Limit(20).
			Find(&expired).Error; err != nil {
			return err
		}

		combined := make([]UserSubscription, 0, len(active)+len(pending)+len(expired))
		combined = append(combined, active...)
		combined = append(combined, pending...)
		combined = append(combined, expired...)
		if len(combined) == 0 {
			allowedGroupIDsBySubID = map[int][]int{}
			presetNameByID = map[int]string{}
			redemptionNameByID = map[int]string{}
			orderInfoByID = map[int]struct {
				TradeNo  string
				Quantity int
			}{}
			return nil
		}

		allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, combined)
		if err != nil {
			return err
		}
		allowedGroupIDsBySubID = allowedBySubID

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

		presetNameByID = make(map[int]string, 0)
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

		redemptionNameByID = make(map[int]string, 0)
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

		orderInfoByID = make(map[int]struct {
			TradeNo  string
			Quantity int
		}, 0)
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
			orderInfoByID = make(map[int]struct {
				TradeNo  string
				Quantity int
			}, len(rows))
			for _, r := range rows {
				if r.Id <= 0 {
					continue
				}
				orderInfoByID[r.Id] = struct {
					TradeNo  string
					Quantity int
				}{
					TradeNo:  strings.TrimSpace(r.TradeNo),
					Quantity: r.Quantity,
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	today := common.GetTodayDateInt()

	combined := make([]UserSubscription, 0, len(active)+len(pending)+len(expired))
	combined = append(combined, active...)
	combined = append(combined, pending...)
	combined = append(combined, expired...)

	// Subscriptions
	items := make([]TokenSubscriptionSummary, 0, len(combined))
	for _, sub := range combined {
		usedToday := sub.DailyQuotaUsed
		if sub.DailyQuotaResetDate != today {
			usedToday = 0
		}
		allowed := allowedGroupIDsBySubID[sub.Id]
		if allowed == nil {
			allowed = []int{}
		}

		total := sub.TotalQuota
		remaining := sub.RemainingQuota
		consumed := 0
		if total > 0 {
			consumed = total - remaining
			if consumed < 0 {
				consumed = 0
			}
		}

		orderTradeNo := ""
		orderQuantity := 0
		if sub.SourceOrderId > 0 {
			if info, ok := orderInfoByID[sub.SourceOrderId]; ok {
				orderTradeNo = info.TradeNo
				orderQuantity = info.Quantity
			}
		}

		items = append(items, TokenSubscriptionSummary{
			Id: sub.Id,

			TotalTokens:     discreteUnitsToDisplay(total),
			RemainingTokens: discreteUnitsToDisplay(remaining),
			ConsumedTokens:  discreteUnitsToDisplay(consumed),

			DailyTokensLimit: discreteUnitsToDisplay(sub.DailyQuotaLimit),
			DailyTokensUsed:  discreteUnitsToDisplay(usedToday),

			StartAt:   sub.StartAt,
			ExpireAt:  sub.ExpireAt,
			InvalidAt: sub.InvalidAt,

			AllowedGroupIds: allowed,
			Source:          sub.Source,

			SourceOrderId:       sub.SourceOrderId,
			SourceOrderTradeNo:  orderTradeNo,
			SourceOrderQuantity: orderQuantity,

			SourcePresetId:   sub.SourcePresetId,
			SourcePresetName: presetNameByID[sub.SourcePresetId],

			SourceRedemptionId:   sub.SourceRedemptionId,
			SourceRedemptionName: redemptionNameByID[sub.SourceRedemptionId],
		})
	}
	result.Subscriptions = items

	// Group capacities (active-only)
	groupIDSet := make(map[int]struct{}, 8)
	for _, sub := range active {
		if sub.Id <= 0 {
			continue
		}
		for _, gid := range allowedGroupIDsBySubID[sub.Id] {
			if gid <= 0 {
				continue
			}
			groupIDSet[gid] = struct{}{}
		}
	}
	groupIDs := make([]int, 0, len(groupIDSet))
	for gid := range groupIDSet {
		groupIDs = append(groupIDs, gid)
	}
	groupIDs = normalizeUniqueSortedIDs(groupIDs)

	caps := make([]TokenSubscriptionGroupCapacity, 0, len(groupIDs))
	for _, gid := range groupIDs {
		totalRemaining, dailyCapacity, totalUnlimited, dailyUnlimited, err := GetUserTokenSubscriptionCapacityForGroup(userId, gid)
		if err != nil {
			result.ConfigErrors = append(result.ConfigErrors, err.Error())
			continue
		}
		caps = append(caps, TokenSubscriptionGroupCapacity{
			GroupId:        gid,
			TotalRemaining: discreteUnitsToDisplay(totalRemaining),
			DailyCapacity:  discreteUnitsToDisplay(dailyCapacity),
			TotalUnlimited: totalUnlimited,
			DailyUnlimited: dailyUnlimited,
		})
	}
	result.GroupCapacities = caps

	return result, nil
}

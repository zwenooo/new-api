package model

import (
	"errors"
	"sort"
)

type subscriptionBreakdownOrderInfo struct {
	TradeNo  string
	Quantity int
}

func buildUserSubscriptionGroupQuotaBreakdown(
	sub UserSubscription,
	allowedGroupIDs []int,
	remaining int,
	used int,
	dailyLimit int,
	groupDailyLimits []GroupDailyQuotaLimit,
	usedByGroupID map[int]int,
	addGroupDailyLimitConfigErr func(UserSubscription, int),
) ([]SubscriptionGroupQuotaBreakdown, error) {
	groupQuotaBreakdown := make([]SubscriptionGroupQuotaBreakdown, 0, len(allowedGroupIDs))
	if len(allowedGroupIDs) == 0 {
		return groupQuotaBreakdown, nil
	}

	sortedGroups := append([]int(nil), allowedGroupIDs...)
	sort.Ints(sortedGroups)

	if len(groupDailyLimits) > 0 {
		limitByGroup := make(map[int]int, len(groupDailyLimits))
		for _, item := range groupDailyLimits {
			limitByGroup[item.GroupId] = item.DailyQuotaLimit
		}
		for _, gid := range sortedGroups {
			limit, ok := limitByGroup[gid]
			if !ok {
				addGroupDailyLimitConfigErr(sub, gid)
				continue
			}
			usedQuota := 0
			if usedByGroupID != nil {
				usedQuota = usedByGroupID[gid]
			}
			if usedQuota < 0 {
				return nil, errors.New("订阅额度日用量数据错误")
			}
			available := remaining
			if limit > 0 {
				remainToday := limit - usedQuota
				if remainToday < 0 {
					remainToday = 0
				}
				if available > remainToday {
					available = remainToday
				}
			}
			if available < 0 {
				available = 0
			}
			groupQuotaBreakdown = append(groupQuotaBreakdown, SubscriptionGroupQuotaBreakdown{
				GroupId:             gid,
				DailyQuotaUsed:      usedQuota,
				DailyQuotaAvailable: available,
				DailyQuotaLimit:     limit,
			})
		}
		return groupQuotaBreakdown, nil
	}

	available := remaining
	if dailyLimit > 0 {
		remainToday := dailyLimit - used
		if remainToday < 0 {
			remainToday = 0
		}
		if available > remainToday {
			available = remainToday
		}
	}
	if available < 0 {
		available = 0
	}
	for _, gid := range sortedGroups {
		groupQuotaBreakdown = append(groupQuotaBreakdown, SubscriptionGroupQuotaBreakdown{
			GroupId:             gid,
			DailyQuotaUsed:      used,
			DailyQuotaAvailable: available,
			DailyQuotaLimit:     dailyLimit,
		})
	}
	return groupQuotaBreakdown, nil
}

func buildUserSubscriptionBreakdownSummary(
	sub UserSubscription,
	allowedGroupIDs []int,
	used int,
	dailyLimit int,
	groupDailyLimits []GroupDailyQuotaLimit,
	usedByGroupID map[int]int,
	addGroupDailyLimitConfigErr func(UserSubscription, int),
	orderInfo subscriptionBreakdownOrderInfo,
	presetName string,
	redemptionName string,
) (SubscriptionSummary, error) {
	consumed := sub.TotalQuota - sub.RemainingQuota
	if consumed < 0 {
		consumed = 0
	}
	remaining := sub.RemainingQuota
	if remaining < 0 {
		remaining = 0
	}

	groupQuotaBreakdown, err := buildUserSubscriptionGroupQuotaBreakdown(
		sub,
		allowedGroupIDs,
		remaining,
		used,
		dailyLimit,
		groupDailyLimits,
		usedByGroupID,
		addGroupDailyLimitConfigErr,
	)
	if err != nil {
		return SubscriptionSummary{}, err
	}

	startUnix := sub.StartAt
	if startUnix <= 0 {
		startUnix = sub.CreatedAt.Unix()
	}

	return SubscriptionSummary{
		ID:                   sub.Id,
		Source:               sub.Source,
		SourceOrderId:        sub.SourceOrderId,
		SourceOrderTradeNo:   orderInfo.TradeNo,
		SourceOrderQuantity:  orderInfo.Quantity,
		SourcePresetId:       sub.SourcePresetId,
		SourcePresetName:     presetName,
		SourceRedemptionId:   sub.SourceRedemptionId,
		SourceRedemptionName: redemptionName,
		AllowedGroupIds:      allowedGroupIDs,
		SortOrder:            sub.SortOrder,
		GroupDailyLimits:     groupDailyLimits,
		GroupQuotaBreakdown:  groupQuotaBreakdown,
		TotalQuota:           sub.TotalQuota,
		RemainingQuota:       remaining,
		ConsumedQuota:        consumed,
		DailyQuotaLimit:      dailyLimit,
		DailyQuotaUsed:       used,
		StartAt:              startUnix,
		ExpireAt:             sub.ExpireAt,
		InvalidAt:            sub.InvalidAt,
	}, nil
}

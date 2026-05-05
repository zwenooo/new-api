package model

import (
	"one-api/billing"

	"github.com/shopspring/decimal"
)

func discreteUnitsFromDisplayInt(units int) int {
	return billing.DisplayIntUnitsToStored(units)
}

func discreteUnitsFromDisplayFloat(units float64) int {
	return billing.DisplayUnitsToStored(units)
}

func discreteUnitsFromDisplayFloatExact(units float64) (int, bool) {
	if units < 0 {
		return 0, false
	}
	stored := billing.DisplayUnitsToStored(units)
	if units == 0 {
		return stored, true
	}
	restored := decimal.NewFromInt(int64(stored)).Div(decimal.NewFromInt(int64(billing.DiscreteQuotaScale)))
	return stored, restored.Equal(decimal.NewFromFloat(units))
}

func discreteUnitsToDisplay(units int) float64 {
	return billing.StoredUnitsToDisplay(units)
}

func discreteUnitsToDisplayString(units int) string {
	return billing.StoredUnitsToDisplayString(units)
}

func scaleGroupLimitMapToStored(limitByGroupID map[int]int) map[int]int {
	if len(limitByGroupID) == 0 {
		return limitByGroupID
	}
	out := make(map[int]int, len(limitByGroupID))
	for gid, limit := range limitByGroupID {
		out[gid] = discreteUnitsFromDisplayInt(limit)
	}
	return out
}

func scaleGroupDailyLimitsToStored(items []GroupDailyQuotaLimit) []GroupDailyQuotaLimit {
	if len(items) == 0 {
		return items
	}
	out := make([]GroupDailyQuotaLimit, 0, len(items))
	for _, item := range items {
		out = append(out, GroupDailyQuotaLimit{
			GroupId:         item.GroupId,
			DailyQuotaLimit: discreteUnitsFromDisplayInt(item.DailyQuotaLimit),
		})
	}
	return out
}

func scaleEffectiveGroupLimitMapToStored(limitByGroupID map[int]int) map[int]int {
	if len(limitByGroupID) == 0 {
		return limitByGroupID
	}
	out := make(map[int]int, len(limitByGroupID))
	for gid, limit := range limitByGroupID {
		out[gid] = discreteUnitsFromDisplayInt(limit)
	}
	return out
}

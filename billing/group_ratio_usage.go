package billing

import (
	"one-api/setting/ratio_setting"

	"github.com/shopspring/decimal"
)

func ResolveGroupRatio(userGroupID int, usingGroupID int) float64 {
	if usingGroupID <= 0 {
		return 1
	}
	if userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(userGroupID, usingGroupID); ok {
		return userGroupRatio
	}
	return ratio_setting.GetGroupRatio(usingGroupID)
}

func ScaleTokensByGroupRatio(units int, ratio float64) int {
	if units <= 0 || ratio <= 0 {
		return 0
	}
	scaled := decimal.NewFromInt(int64(units)).Mul(discreteQuotaScaleDecimal).Mul(decimal.NewFromFloat(ratio))
	result := int(scaled.Round(0).IntPart())
	if result <= 0 {
		return 1
	}
	return result
}

func ScaleRequestsByGroupRatio(requests int, ratio float64) int {
	if requests <= 0 || ratio <= 0 {
		return 0
	}
	scaled := decimal.NewFromInt(int64(requests)).Mul(discreteQuotaScaleDecimal).Mul(decimal.NewFromFloat(ratio))
	result := int(scaled.Round(0).IntPart())
	if result <= 0 {
		return 1
	}
	return result
}

func ScaleDiscreteUsageByMultiplier(units int, multiplier float64) int {
	if units <= 0 {
		return 0
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	scaled := decimal.NewFromInt(int64(units)).Mul(decimal.NewFromFloat(multiplier))
	result := int(scaled.Round(0).IntPart())
	if result <= 0 {
		return 1
	}
	return result
}

func ComputeTokenUsage(userGroupID int, usingGroupID int, tokens int) int {
	return ScaleTokensByGroupRatio(tokens, ResolveGroupRatio(userGroupID, usingGroupID))
}

func ComputeRequestUsage(userGroupID int, usingGroupID int, requests int) int {
	return ScaleRequestsByGroupRatio(requests, ResolveGroupRatio(userGroupID, usingGroupID))
}

package billing

import "github.com/shopspring/decimal"

const DiscreteQuotaScale = 1000

var discreteQuotaScaleDecimal = decimal.NewFromInt(DiscreteQuotaScale)

func DisplayUnitsToStored(units float64) int {
	if units <= 0 {
		return 0
	}
	return int(decimal.NewFromFloat(units).Mul(discreteQuotaScaleDecimal).Round(0).IntPart())
}

func DisplayIntUnitsToStored(units int) int {
	if units <= 0 {
		return 0
	}
	return units * DiscreteQuotaScale
}

func StoredUnitsToDisplay(units int) float64 {
	if units == 0 {
		return 0
	}
	return decimal.NewFromInt(int64(units)).Div(discreteQuotaScaleDecimal).InexactFloat64()
}

func StoredUnitsToDisplayString(units int) string {
	if units == 0 {
		return "0"
	}
	return decimal.NewFromInt(int64(units)).Div(discreteQuotaScaleDecimal).String()
}

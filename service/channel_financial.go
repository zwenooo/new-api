package service

import (
	"one-api/common"
	"one-api/model"
	"one-api/setting/payg_setting"
	"strings"
)

// FillChannelFinancialFields fills derived CNY-based financial fields for a channel.
//
// - Revenue side uses channel.UsedQuota (sales caliber) and PayG credit conversion rate.
// - Cost side uses channel.CostUsedQuota (cost caliber) and channel.BuyCnyPerUsd.
// - Profit is only set when both revenue and cost are available.
func FillChannelFinancialFields(channel *model.Channel) {
	if channel == nil {
		return
	}

	mode := strings.TrimSpace(channel.BillingMode)
	if mode == "" {
		mode = model.ChannelBillingModeQuota
	}
	channel.BillingMode = mode

	if mode == model.ChannelBillingModeRequest {
		successCount := float64(channel.RequestSuccessCount)

		if channel.SellRequestsPerCny > 0 {
			revenueCny := successCount / float64(channel.SellRequestsPerCny)
			channel.RevenueCny = &revenueCny
		}

		if channel.BuyRequestsPerCny > 0 {
			costCny := successCount / float64(channel.BuyRequestsPerCny)
			channel.CostCny = &costCny
		}

		if channel.RevenueCny != nil && channel.CostCny != nil {
			profitCny := *channel.RevenueCny - *channel.CostCny
			channel.ProfitCny = &profitCny
		}
		return
	}

	revenueUsd := float64(channel.UsedQuota) / common.QuotaPerUnit
	costUsd := float64(channel.CostUsedQuota) / common.QuotaPerUnit

	// RevenueCny: derived from Pay-as-you-go credit conversion.
	// credit_usd_per_cny means: credited USD quota = RMB(yuan) * credit_usd_per_cny,
	// so RMB per USD = 1 / credit_usd_per_cny.
	creditUsdPerCny := payg_setting.GetPaygSettings().CreditUsdPerCny
	if creditUsdPerCny > 0 {
		revenueCny := revenueUsd / creditUsdPerCny
		channel.RevenueCny = &revenueCny
	}

	// CostCny: derived from channel buy price (CNY per USD).
	if channel.BuyCnyPerUsd > 0 {
		costCny := costUsd * channel.BuyCnyPerUsd
		channel.CostCny = &costCny
	}

	if channel.RevenueCny != nil && channel.CostCny != nil {
		profitCny := *channel.RevenueCny - *channel.CostCny
		channel.ProfitCny = &profitCny
	}
}

func FillChannelsFinancialFields(channels []*model.Channel) {
	for _, ch := range channels {
		FillChannelFinancialFields(ch)
	}
}

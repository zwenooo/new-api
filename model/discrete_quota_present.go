package model

import "encoding/json"

func PresentUserSubscription(sub *UserSubscription) map[string]interface{} {
	if sub == nil {
		return nil
	}
	payload := map[string]interface{}{}
	raw, _ := json.Marshal(sub)
	_ = json.Unmarshal(raw, &payload)
	if sub.BillingUnit == UserSubscriptionBillingUnitTokens {
		payload["total_quota"] = discreteUnitsToDisplay(sub.TotalQuota)
		payload["remaining_quota"] = discreteUnitsToDisplay(sub.RemainingQuota)
		payload["daily_quota_limit"] = discreteUnitsToDisplay(sub.DailyQuotaLimit)
		payload["daily_quota_used"] = discreteUnitsToDisplay(sub.DailyQuotaUsed)
	}
	return payload
}

func PresentUserRequestSubscription(sub *UserRequestSubscription) map[string]interface{} {
	if sub == nil {
		return nil
	}
	payload := map[string]interface{}{}
	raw, _ := json.Marshal(sub)
	_ = json.Unmarshal(raw, &payload)
	payload["daily_request_limit"] = discreteUnitsToDisplay(sub.DailyRequestLimit)
	payload["daily_request_used"] = discreteUnitsToDisplay(sub.DailyRequestUsed)
	payload["total_request_limit"] = discreteUnitsToDisplay(sub.TotalRequestLimit)
	payload["total_request_used"] = discreteUnitsToDisplay(sub.TotalRequestUsed)
	return payload
}

package model

import "testing"

func TestCanConsumeUserRequestSubscriptionResetsDailyUsageOnNewDay(t *testing.T) {
	sub := UserRequestSubscription{
		DailyRequestLimit:     discreteUnitsFromDisplayInt(5),
		DailyRequestUsed:      discreteUnitsFromDisplayInt(5),
		DailyRequestResetDate: 20260317,
		TotalRequestLimit:     discreteUnitsFromDisplayInt(10),
		TotalRequestUsed:      discreteUnitsFromDisplayInt(4),
	}

	if ok := canConsumeUserRequestSubscription(sub, 20260318, discreteUnitsFromDisplayInt(1)); !ok {
		t.Fatalf("expected subscription to be consumable after daily reset")
	}
}

func TestCanConsumeUserRequestSubscriptionRejectsTotalExhaustedSubscription(t *testing.T) {
	sub := UserRequestSubscription{
		DailyRequestLimit:     discreteUnitsFromDisplayInt(10),
		DailyRequestUsed:      discreteUnitsFromDisplayInt(2),
		DailyRequestResetDate: 20260318,
		TotalRequestLimit:     discreteUnitsFromDisplayInt(3),
		TotalRequestUsed:      discreteUnitsFromDisplayInt(3),
	}

	if ok := canConsumeUserRequestSubscription(sub, 20260318, discreteUnitsFromDisplayInt(1)); ok {
		t.Fatalf("expected subscription to be rejected when total requests are exhausted")
	}
}

func TestEvaluateUserRequestSubscriptionConsumptionClassifiesFailures(t *testing.T) {
	base := UserRequestSubscription{
		DailyRequestLimit:     discreteUnitsFromDisplayInt(5),
		DailyRequestUsed:      discreteUnitsFromDisplayInt(1),
		DailyRequestResetDate: 20260318,
		TotalRequestLimit:     discreteUnitsFromDisplayInt(10),
		TotalRequestUsed:      discreteUnitsFromDisplayInt(2),
	}
	allowedSet := map[int]struct{}{7: {}}

	groupMismatch := evaluateUserRequestSubscriptionConsumption(base, allowedSet, 9, 20260318, discreteUnitsFromDisplayInt(1))
	if groupMismatch.Reason != userRequestSubscriptionConsumeFailureGroupMismatch {
		t.Fatalf("expected group mismatch, got %v", groupMismatch.Reason)
	}

	dailyExhaustedSub := base
	dailyExhaustedSub.DailyRequestUsed = discreteUnitsFromDisplayInt(5)
	dailyExhausted := evaluateUserRequestSubscriptionConsumption(dailyExhaustedSub, allowedSet, 7, 20260318, discreteUnitsFromDisplayInt(1))
	if dailyExhausted.Reason != userRequestSubscriptionConsumeFailureDailyExhausted {
		t.Fatalf("expected daily exhausted, got %v", dailyExhausted.Reason)
	}

	totalExhaustedSub := base
	totalExhaustedSub.TotalRequestUsed = discreteUnitsFromDisplayInt(10)
	totalExhausted := evaluateUserRequestSubscriptionConsumption(totalExhaustedSub, allowedSet, 7, 20260318, discreteUnitsFromDisplayInt(1))
	if totalExhausted.Reason != userRequestSubscriptionConsumeFailureTotalExhausted {
		t.Fatalf("expected total exhausted, got %v", totalExhausted.Reason)
	}
}

func TestBuildUserRequestSubscriptionInsufficientError(t *testing.T) {
	tests := []struct {
		name          string
		hasGroupMatch bool
		daily         bool
		total         bool
		want          string
	}{
		{
			name:          "no_matching_group",
			hasGroupMatch: false,
			want:          "次数订阅不足",
		},
		{
			name:          "daily_exhausted",
			hasGroupMatch: true,
			daily:         true,
			want:          "次数订阅当日次数已用尽",
		},
		{
			name:          "total_exhausted",
			hasGroupMatch: true,
			total:         true,
			want:          "次数订阅总次数已用尽",
		},
		{
			name:          "daily_and_total_exhausted",
			hasGroupMatch: true,
			daily:         true,
			total:         true,
			want:          "次数订阅当日或总次数已用尽",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := buildUserRequestSubscriptionInsufficientError(tt.hasGroupMatch, tt.daily, tt.total)
			if err == nil {
				t.Fatalf("expected error")
			}
			if err.Error() != tt.want {
				t.Fatalf("got %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

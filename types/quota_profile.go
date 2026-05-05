package types

// QuotaProfile represents three billing dimensions:
// - SettledQuota: actual enforced deduction.
// - VisibleQuota: user-facing displayed quota.
// - CostQuota: upstream/provider-side cost quota.
type QuotaProfile struct {
	SettledQuota int
	VisibleQuota int
	CostQuota    int
}

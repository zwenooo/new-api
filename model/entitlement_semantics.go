package model

// EntitlementMode describes how a user obtains a billable entitlement.
// It intentionally separates acquisition mode from metering unit.
type EntitlementMode string

const (
	EntitlementModePrepaid      EntitlementMode = "prepaid"
	EntitlementModeSubscription EntitlementMode = "subscription"
)

func (m EntitlementMode) Label() string {
	switch m {
	case EntitlementModePrepaid:
		return "预付费"
	case EntitlementModeSubscription:
		return "订阅"
	default:
		return ""
	}
}

// EntitlementUnit describes what the entitlement is metered in.
type EntitlementUnit string

const (
	EntitlementUnitCredit  EntitlementUnit = "credit"
	EntitlementUnitRequest EntitlementUnit = "request"
	EntitlementUnitToken   EntitlementUnit = "token"
)

func (u EntitlementUnit) Label() string {
	switch u {
	case EntitlementUnitCredit:
		return "额度"
	case EntitlementUnitRequest:
		return "次数"
	case EntitlementUnitToken:
		return "Token"
	default:
		return ""
	}
}

// EntitlementKind is the fully-qualified semantic kind used by the billing domain.
// It combines acquisition mode and metering unit so names stay unambiguous.
type EntitlementKind string

const (
	EntitlementKindPrepaidCredit       EntitlementKind = "prepaid_credit"
	EntitlementKindPrepaidRequest      EntitlementKind = "prepaid_request"
	EntitlementKindPrepaidToken        EntitlementKind = "prepaid_token"
	EntitlementKindSubscriptionCredit  EntitlementKind = "subscription_credit"
	EntitlementKindSubscriptionRequest EntitlementKind = "subscription_request"
	EntitlementKindSubscriptionToken   EntitlementKind = "subscription_token"
)

func buildEntitlementKind(mode EntitlementMode, unit EntitlementUnit) EntitlementKind {
	switch {
	case mode == EntitlementModePrepaid && unit == EntitlementUnitCredit:
		return EntitlementKindPrepaidCredit
	case mode == EntitlementModePrepaid && unit == EntitlementUnitRequest:
		return EntitlementKindPrepaidRequest
	case mode == EntitlementModePrepaid && unit == EntitlementUnitToken:
		return EntitlementKindPrepaidToken
	case mode == EntitlementModeSubscription && unit == EntitlementUnitCredit:
		return EntitlementKindSubscriptionCredit
	case mode == EntitlementModeSubscription && unit == EntitlementUnitRequest:
		return EntitlementKindSubscriptionRequest
	case mode == EntitlementModeSubscription && unit == EntitlementUnitToken:
		return EntitlementKindSubscriptionToken
	default:
		return ""
	}
}

func (k EntitlementKind) Mode() EntitlementMode {
	switch k {
	case EntitlementKindPrepaidCredit, EntitlementKindPrepaidRequest, EntitlementKindPrepaidToken:
		return EntitlementModePrepaid
	case EntitlementKindSubscriptionCredit, EntitlementKindSubscriptionRequest, EntitlementKindSubscriptionToken:
		return EntitlementModeSubscription
	default:
		return ""
	}
}

func (k EntitlementKind) Unit() EntitlementUnit {
	switch k {
	case EntitlementKindPrepaidCredit, EntitlementKindSubscriptionCredit:
		return EntitlementUnitCredit
	case EntitlementKindPrepaidRequest, EntitlementKindSubscriptionRequest:
		return EntitlementUnitRequest
	case EntitlementKindPrepaidToken, EntitlementKindSubscriptionToken:
		return EntitlementUnitToken
	default:
		return ""
	}
}

func (k EntitlementKind) Label() string {
	mode := k.Mode().Label()
	unit := k.Unit().Label()
	if mode == "" || unit == "" {
		return ""
	}
	return mode + unit
}

const (
	// UserSubscriptionBillingUnitCredit is the semantic alias for the historical "quota" unit.
	UserSubscriptionBillingUnitCredit = UserSubscriptionBillingUnitQuota
	// UserSubscriptionBillingUnitToken matches token-based subscription entitlements.
	UserSubscriptionBillingUnitToken = UserSubscriptionBillingUnitTokens
)

func resolveUserSubscriptionEntitlementUnit(unit string) EntitlementUnit {
	normalized, err := normalizeUserSubscriptionBillingUnit(unit)
	if err != nil {
		return ""
	}
	switch normalized {
	case UserSubscriptionBillingUnitToken:
		return EntitlementUnitToken
	case UserSubscriptionBillingUnitCredit:
		return EntitlementUnitCredit
	default:
		return ""
	}
}

func (sub UserSubscription) EntitlementMode() EntitlementMode {
	return EntitlementModeSubscription
}

func (sub UserSubscription) EntitlementUnit() EntitlementUnit {
	return resolveUserSubscriptionEntitlementUnit(sub.BillingUnit)
}

func (sub UserSubscription) EntitlementKind() EntitlementKind {
	return buildEntitlementKind(sub.EntitlementMode(), sub.EntitlementUnit())
}

func (sub UserRequestSubscription) EntitlementMode() EntitlementMode {
	return EntitlementModeSubscription
}

func (sub UserRequestSubscription) EntitlementUnit() EntitlementUnit {
	return EntitlementUnitRequest
}

func (sub UserRequestSubscription) EntitlementKind() EntitlementKind {
	return buildEntitlementKind(sub.EntitlementMode(), sub.EntitlementUnit())
}

func (b PaygUserBalance) EntitlementMode() EntitlementMode {
	return EntitlementModePrepaid
}

func (b PaygUserBalance) EntitlementUnit() EntitlementUnit {
	return EntitlementUnitCredit
}

func (b PaygUserBalance) EntitlementKind() EntitlementKind {
	return buildEntitlementKind(b.EntitlementMode(), b.EntitlementUnit())
}

func (b PayRequestUserBalance) EntitlementMode() EntitlementMode {
	return EntitlementModePrepaid
}

func (b PayRequestUserBalance) EntitlementUnit() EntitlementUnit {
	return EntitlementUnitRequest
}

func (b PayRequestUserBalance) EntitlementKind() EntitlementKind {
	return buildEntitlementKind(b.EntitlementMode(), b.EntitlementUnit())
}

func (b PayTokenUserBalance) EntitlementMode() EntitlementMode {
	return EntitlementModePrepaid
}

func (b PayTokenUserBalance) EntitlementUnit() EntitlementUnit {
	return EntitlementUnitToken
}

func (b PayTokenUserBalance) EntitlementKind() EntitlementKind {
	return buildEntitlementKind(b.EntitlementMode(), b.EntitlementUnit())
}

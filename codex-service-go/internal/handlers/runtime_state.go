package handlers

import "strings"

func runtimeBlockedReasonPrefix(reason string) string {
	text := strings.ToLower(strings.TrimSpace(reason))
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, ":"); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}
	return text
}

func runtimeBlockedState(reason string) string {
	switch runtimeBlockedReasonPrefix(reason) {
	case "sleep":
		return "cooldown"
	case "channel_backoff":
		return "channel_backoff"
	case "transport_quarantine":
		return "transport_quarantine"
	case "payment_required", "deactivated_workspace":
		return "member_expired"
	case "expired", "token_revoked", "auth_missing", "auth_expired":
		return "expired"
	default:
		return "normal"
	}
}

func runtimeIsTemporaryState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "cooldown", "channel_backoff", "transport_quarantine":
		return true
	default:
		return false
	}
}

func runtimeStateLabel(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "cooldown":
		return "上游冷却"
	case "channel_backoff":
		return "渠道退避"
	case "transport_quarantine":
		return "链路隔离"
	case "member_expired":
		return "会员过期"
	case "expired":
		return "过期"
	default:
		return "正常"
	}
}

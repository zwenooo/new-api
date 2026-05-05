package personal_setting

import (
	"encoding/json"
	"strings"

	"one-api/common"
)

const (
	// Option keys stored in the options table / cache
	OptionKeyAccountBindingVisibility   = "PersonalSettingAccountBindingVisibility"
	OptionKeyWalletInvitationVisibility = "PersonalSettingWalletInvitationVisible"
	OptionKeyInvitationPageVisibility   = "PersonalSettingInvitationPageVisible"
	OptionKeyOtherSettingsVisibility    = "PersonalSettingOtherSettingsVisible"

	// Supported binding identifiers referenced by frontend and backend checks
	BindingEmail    = "email"
	BindingWeChat   = "wechat"
	BindingGitHub   = "github"
	BindingOIDC     = "oidc"
	BindingTelegram = "telegram"
	BindingLinuxDO  = "linuxdo"
)

var defaultAccountBindingVisibility = map[string]bool{
	BindingEmail:    true,
	BindingWeChat:   true,
	BindingGitHub:   true,
	BindingOIDC:     true,
	BindingTelegram: true,
	BindingLinuxDO:  true,
}

// DefaultAccountBindingVisibilityJSON provides the serialized default visibility map.
var DefaultAccountBindingVisibilityJSON = func() string {
	bytes, _ := json.Marshal(defaultAccountBindingVisibility)
	return string(bytes)
}()

// GetAccountBindingVisibility merges stored visibility overrides with defaults and returns a copy.
func GetAccountBindingVisibility() map[string]bool {
	visibility := make(map[string]bool, len(defaultAccountBindingVisibility))
	for k, v := range defaultAccountBindingVisibility {
		visibility[k] = v
	}

	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[OptionKeyAccountBindingVisibility]
	common.OptionMapRWMutex.RUnlock()

	if raw == "" {
		return visibility
	}

	var overrides map[string]bool
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return visibility
	}

	for k, v := range overrides {
		visibility[k] = v
	}

	return visibility
}

// IsBindingVisible reports whether a binding type should be exposed to end users.
func IsBindingVisible(binding string) bool {
	visibility := GetAccountBindingVisibility()
	allowed, ok := visibility[binding]
	if !ok {
		return false
	}
	return allowed
}

// IsWalletInvitationVisible reports whether the wallet invitation card should be displayed.
func IsWalletInvitationVisible() bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[OptionKeyWalletInvitationVisibility]
	common.OptionMapRWMutex.RUnlock()

	if raw == "" {
		return true
	}

	normalized := strings.TrimSpace(strings.ToLower(raw))
	return normalized != "0" && normalized != "false"
}

// IsInvitationPageVisible reports whether the invitation page/menu should be displayed.
func IsInvitationPageVisible() bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[OptionKeyInvitationPageVisibility]
	common.OptionMapRWMutex.RUnlock()

	if raw == "" {
		return IsWalletInvitationVisible()
	}

	normalized := strings.TrimSpace(strings.ToLower(raw))
	return normalized != "0" && normalized != "false"
}

// IsOtherSettingsVisible reports whether the "Other Settings" card
// in the personal center should be displayed.
func IsOtherSettingsVisible() bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[OptionKeyOtherSettingsVisibility]
	common.OptionMapRWMutex.RUnlock()

	if raw == "" {
		return true
	}

	normalized := strings.TrimSpace(strings.ToLower(raw))
	return normalized != "0" && normalized != "false"
}

package operation_setting

import "testing"

func TestAutomaticSwitchStatusCodeWhitelistFromString(t *testing.T) {
	restoreAutomaticSwitchStatusCodeSettings(t)

	err := AutomaticSwitchStatusCodeWhitelistFromString("200\n429\n200\n")
	if err != nil {
		t.Fatalf("AutomaticSwitchStatusCodeWhitelistFromString error: %v", err)
	}

	if got := AutomaticSwitchStatusCodeWhitelistToString(); got != "200\n429" {
		t.Fatalf("AutomaticSwitchStatusCodeWhitelistToString() = %q, want %q", got, "200\n429")
	}
	if !HasAutomaticSwitchStatusCodeWhitelist() {
		t.Fatalf("expected whitelist to be enabled")
	}
	if !IsAutomaticSwitchStatusCodeAllowed(200) {
		t.Fatalf("expected status code 200 to be allowed")
	}
	if IsAutomaticSwitchStatusCodeAllowed(500) {
		t.Fatalf("expected status code 500 to be rejected")
	}
}

func TestAutomaticSwitchStatusCodeWhitelistFromStringRejectsInvalidStatusCode(t *testing.T) {
	restoreAutomaticSwitchStatusCodeSettings(t)

	if err := AutomaticSwitchStatusCodeWhitelistFromString("200\n99"); err == nil {
		t.Fatalf("expected invalid status code to return error")
	}
}

func TestValidateAutomaticSwitchMaxRetries(t *testing.T) {
	if err := ValidateAutomaticSwitchMaxRetries(0); err != nil {
		t.Fatalf("ValidateAutomaticSwitchMaxRetries(0) error: %v", err)
	}
	if err := ValidateAutomaticSwitchMaxRetries(5); err != nil {
		t.Fatalf("ValidateAutomaticSwitchMaxRetries(5) error: %v", err)
	}
	if err := ValidateAutomaticSwitchMaxRetries(999); err != nil {
		t.Fatalf("ValidateAutomaticSwitchMaxRetries(999) error: %v", err)
	}
	if err := ValidateAutomaticSwitchMaxRetries(-1); err == nil {
		t.Fatalf("expected negative retry count to fail validation")
	}
}

func restoreAutomaticSwitchStatusCodeSettings(t *testing.T) {
	t.Helper()

	backupWhitelist := append([]int(nil), AutomaticSwitchStatusCodeWhitelist...)
	backupSet := make(map[int]struct{}, len(AutomaticSwitchStatusCodeWhitelistSet))
	for statusCode := range AutomaticSwitchStatusCodeWhitelistSet {
		backupSet[statusCode] = struct{}{}
	}
	backupRetries := AutomaticSwitchMaxRetries

	t.Cleanup(func() {
		AutomaticSwitchStatusCodeWhitelist = backupWhitelist
		AutomaticSwitchStatusCodeWhitelistSet = backupSet
		AutomaticSwitchMaxRetries = backupRetries
	})
}

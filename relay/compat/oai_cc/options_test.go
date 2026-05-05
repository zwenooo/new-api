package oai_cc

import (
	"testing"

	"one-api/common"
)

func TestGetPromptCacheSettings_PrefersNewCx2ccOptionKeys(t *testing.T) {
	restoreCx2ccPromptCacheOptions(t, map[string]string{
		cx2ccPromptCacheEnabledOpt:            "false",
		cx2ccPromptCacheEntriesOpt:            "1234",
		cx2ccPromptCacheSessionsOpt:           "77",
		cx2ccPromptCacheSessionTtlMsOpt:       "9000",
		legacyCx2ccPromptCacheEnabledOpt:      "true",
		legacyCx2ccPromptCacheEntriesOpt:      "5000",
		legacyCx2ccPromptCacheSessionsOpt:     "2000",
		legacyCx2ccPromptCacheSessionTtlMsOpt: "7200000",
	})

	settings := GetPromptCacheSettings()
	if settings.Enabled {
		t.Fatalf("enabled = %v, want false", settings.Enabled)
	}
	if settings.MaxEntries != 1234 {
		t.Fatalf("max entries = %d, want 1234", settings.MaxEntries)
	}
	if settings.MaxSessions != 77 {
		t.Fatalf("max sessions = %d, want 77", settings.MaxSessions)
	}
	if got := settings.SessionTTL.Milliseconds(); got != 9000 {
		t.Fatalf("session ttl = %dms, want 9000ms", got)
	}
}

func TestGetPromptCacheSettings_FallsBackToLegacyCx2ccOptionKeys(t *testing.T) {
	restoreCx2ccPromptCacheOptions(t, map[string]string{
		legacyCx2ccPromptCacheEnabledOpt:      "false",
		legacyCx2ccPromptCacheEntriesOpt:      "1300",
		legacyCx2ccPromptCacheSessionsOpt:     "79",
		legacyCx2ccPromptCacheSessionTtlMsOpt: "9100",
	})

	settings := GetPromptCacheSettings()
	if settings.Enabled {
		t.Fatalf("enabled = %v, want false", settings.Enabled)
	}
	if settings.MaxEntries != 1300 {
		t.Fatalf("max entries = %d, want 1300", settings.MaxEntries)
	}
	if settings.MaxSessions != 79 {
		t.Fatalf("max sessions = %d, want 79", settings.MaxSessions)
	}
	if got := settings.SessionTTL.Milliseconds(); got != 9100 {
		t.Fatalf("session ttl = %dms, want 9100ms", got)
	}
}

func restoreCx2ccPromptCacheOptions(t *testing.T, options map[string]string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	backup := cloneCx2ccOptionMap(common.OptionMap)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for _, key := range []string{
		cx2ccPromptCacheEnabledOpt,
		cx2ccPromptCacheEntriesOpt,
		cx2ccPromptCacheSessionsOpt,
		cx2ccPromptCacheSessionTtlMsOpt,
		legacyCx2ccPromptCacheEnabledOpt,
		legacyCx2ccPromptCacheEntriesOpt,
		legacyCx2ccPromptCacheSessionsOpt,
		legacyCx2ccPromptCacheSessionTtlMsOpt,
	} {
		delete(common.OptionMap, key)
	}
	for key, value := range options {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = backup
		common.OptionMapRWMutex.Unlock()
	})
}

func cloneCx2ccOptionMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

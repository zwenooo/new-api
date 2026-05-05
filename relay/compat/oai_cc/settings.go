package oai_cc

import (
	"time"
)

type PromptCacheSettings struct {
	Enabled     bool
	MaxEntries  int
	MaxSessions int
	SessionTTL  time.Duration
}

func GetPromptCacheSettings() PromptCacheSettings {
	enabled := parseGlobalOptionBoolAny(true, cx2ccPromptCacheEnabledOpt, legacyCx2ccPromptCacheEnabledOpt)

	maxEntries := parseGlobalOptionIntAny(5000, cx2ccPromptCacheEntriesOpt, legacyCx2ccPromptCacheEntriesOpt)
	if maxEntries < 100 {
		maxEntries = 100
	}

	maxSessions := parseGlobalOptionIntAny(2000, cx2ccPromptCacheSessionsOpt, legacyCx2ccPromptCacheSessionsOpt)
	if maxSessions < 1 {
		maxSessions = 1
	}

	ttlMs := parseGlobalOptionIntAny(2*60*60*1000, cx2ccPromptCacheSessionTtlMsOpt, legacyCx2ccPromptCacheSessionTtlMsOpt)
	if ttlMs < 0 {
		ttlMs = 0
	}

	return PromptCacheSettings{
		Enabled:     enabled,
		MaxEntries:  maxEntries,
		MaxSessions: maxSessions,
		SessionTTL:  time.Duration(ttlMs) * time.Millisecond,
	}
}

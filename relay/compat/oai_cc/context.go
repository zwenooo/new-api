package oai_cc

import (
	"strings"
	"time"
)

var promptCacheSessions = NewSessionCache[*PromptCacheSessionState](2*time.Hour, 2000)

type UsageContext struct {
	SessionKey string

	LocalTotalInputTokens  int
	UsageCacheLocal        UsageCache
	MessageStartInputTokens int
	MessageStartUsageCache UsageCache

	PromptCacheSession *PromptCacheSessionState
	PromptCacheEnabled bool
}

func getOrCreatePromptCacheSession(sessionKey string, settings PromptCacheSettings) *PromptCacheSessionState {
	promptCacheSessions.UpdateConfig(settings.SessionTTL, settings.MaxSessions)

	state, _ := promptCacheSessions.GetOrCreate(sessionKey, func() *PromptCacheSessionState {
		return &PromptCacheSessionState{
			PromptCache: NewPromptCache(settings.MaxEntries),
		}
	})
	if state != nil && state.PromptCache != nil {
		state.PromptCache.SetMaxEntries(settings.MaxEntries)
	}
	return state
}

func BuildUsageContext(sessionKey string, anthropicReq map[string]any) *UsageContext {
	settings := GetPromptCacheSettings()
	usageLocal := buildDefaultUsageCache()
	effectiveReq := anthropicReq

	var sessionState *PromptCacheSessionState
	if settings.Enabled && strings.TrimSpace(sessionKey) != "" {
		sessionState = getOrCreatePromptCacheSession(sessionKey, settings)
		if sessionState != nil && sessionState.PromptCache != nil {
			effectiveReq = addStableAutoCacheControlMarkers(anthropicReq, sessionState, "", promptCacheRebuildIdle)
			usageLocal = computePromptCacheUsage(sessionState.PromptCache, effectiveReq, false)
		}
	}

	// Mirrors openai-claude-main: token estimation happens after stable cache markers are applied,
	// so message_start usage remains consistent with prompt cache usage computation.
	localTotal := countInputTokensLocal(effectiveReq)
	messageStartCache := reconcileCacheUsage(usageLocal, localTotal, localTotal)
	messageStartInput := maxInt(0, localTotal-messageStartCache.CacheCreationInputTokens-messageStartCache.CacheReadInputTokens)

	return &UsageContext{
		SessionKey:              sessionKey,
		LocalTotalInputTokens:   localTotal,
		UsageCacheLocal:         usageLocal,
		MessageStartInputTokens: messageStartInput,
		MessageStartUsageCache:  messageStartCache,
		PromptCacheSession:      sessionState,
		PromptCacheEnabled:      settings.Enabled,
	}
}

func (c *UsageContext) FinalUsage(totalInputTokens int, outputTokens int) (uncachedInputTokens int, usageCache UsageCache) {
	if c == nil {
		return maxInt(0, totalInputTokens), buildDefaultUsageCache()
	}
	inputTokens, cache := buildUsageForTotal(c.LocalTotalInputTokens, totalInputTokens, c.UsageCacheLocal, c.PromptCacheSession)
	return inputTokens, cache
}

package oai_cc

const (
	cx2ccPromptCacheEnabledOpt      = "cx2cc.prompt_cache.enabled"
	cx2ccPromptCacheEntriesOpt      = "cx2cc.prompt_cache.entries"
	cx2ccPromptCacheSessionsOpt     = "cx2cc.prompt_cache.sessions"
	cx2ccPromptCacheSessionTtlMsOpt = "cx2cc.prompt_cache.session_ttl_ms"

	legacyCx2ccPromptCacheEnabledOpt      = "cx_pool.cx2cc.prompt_cache.enabled"
	legacyCx2ccPromptCacheEntriesOpt      = "cx_pool.cx2cc.prompt_cache.entries"
	legacyCx2ccPromptCacheSessionsOpt     = "cx_pool.cx2cc.prompt_cache.sessions"
	legacyCx2ccPromptCacheSessionTtlMsOpt = "cx_pool.cx2cc.prompt_cache.session_ttl_ms"

	cx2ccCacheSmoothingEnabledOpt       = "cx2cc.cache_smoothing.enabled"
	cx2ccCacheSmoothingDropThresholdOpt = "cx2cc.cache_smoothing.drop_threshold"
	cx2ccCacheSmoothingDampeningOpt     = "cx2cc.cache_smoothing.dampening"
	cx2ccCacheSmoothingMinPrevReadOpt   = "cx2cc.cache_smoothing.min_prev_read"

	legacyCx2ccCacheSmoothingEnabledOpt       = "cx_pool.cx2cc.cache_smoothing.enabled"
	legacyCx2ccCacheSmoothingDropThresholdOpt = "cx_pool.cx2cc.cache_smoothing.drop_threshold"
	legacyCx2ccCacheSmoothingDampeningOpt     = "cx_pool.cx2cc.cache_smoothing.dampening"
	legacyCx2ccCacheSmoothingMinPrevReadOpt   = "cx_pool.cx2cc.cache_smoothing.min_prev_read"
)

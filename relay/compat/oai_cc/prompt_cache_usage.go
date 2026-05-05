package oai_cc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CacheTtlMs int

const (
	cacheTtl5m CacheTtlMs = 300_000
	cacheTtl1h CacheTtlMs = 3_600_000
)

type UsageCacheCreationBreakdown struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
}

type UsageCache struct {
	CacheCreationInputTokens int                         `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int                         `json:"cache_read_input_tokens"`
	CacheCreation            UsageCacheCreationBreakdown `json:"cache_creation"`
	ClaudeCacheCreation5m    int                         `json:"claude_cache_creation_5_m_tokens"`
	ClaudeCacheCreation1h    int                         `json:"claude_cache_creation_1_h_tokens"`
}

type CacheSmoothingState struct {
	LastCacheRead     int
	LastCacheCreation int
	LastTotalInput    int
	RequestCount      int
	TotalCorrected    int
}

type PromptCacheSessionState struct {
	PromptCache *PromptCache

	CacheSmoothing     *CacheSmoothingState
	StableCacheMarkers map[string]*stableCacheMarkerState

	mu sync.Mutex
}

type CacheBreakpoint struct {
	Hash   string
	Tokens int
	TTL    CacheTtlMs
}

func toNonNegativeInt(value any) int {
	switch v := value.(type) {
	case int:
		if v < 0 {
			return 0
		}
		return v
	case int64:
		if v < 0 {
			return 0
		}
		if v > int64(^uint(0)>>1) {
			return int(^uint(0) >> 1)
		}
		return int(v)
	case float64:
		if !isFinite(v) || v < 0 {
			return 0
		}
		return int(math.Floor(v))
	case json.Number:
		n, _ := v.Int64()
		if n < 0 {
			return 0
		}
		if n > int64(^uint(0)>>1) {
			return int(^uint(0) >> 1)
		}
		return int(n)
	default:
		return 0
	}
}

func buildDefaultUsageCache() UsageCache {
	return UsageCache{
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     0,
		CacheCreation:            UsageCacheCreationBreakdown{Ephemeral5mInputTokens: 0, Ephemeral1hInputTokens: 0},
		ClaudeCacheCreation5m:    0,
		ClaudeCacheCreation1h:    0,
	}
}

func ttlMsForCacheControl(cacheControl any) CacheTtlMs {
	m, ok := cacheControl.(map[string]any)
	if !ok || m == nil {
		return cacheTtl5m
	}
	ttl := strings.ToLower(strings.TrimSpace(asString(m["ttl"])))
	if ttl == "1h" {
		return cacheTtl1h
	}
	typ := strings.ToLower(strings.TrimSpace(asString(m["type"])))
	if strings.Contains(typ, "1h") {
		return cacheTtl1h
	}
	return cacheTtl5m
}

func reconcileCacheUsage(base UsageCache, localTotalInputTokens int, totalInputTokens int) UsageCache {
	total := toNonNegativeInt(totalInputTokens)
	localTotal := toNonNegativeInt(localTotalInputTokens)

	readLocal := toNonNegativeInt(base.CacheReadInputTokens)
	createLocal := toNonNegativeInt(base.CacheCreationInputTokens)
	create5mLocal := toNonNegativeInt(base.CacheCreation.Ephemeral5mInputTokens)

	// Mirrors openai-claude-main: 1h buckets are not emitted.
	create1h := 0

	rawCacheTotal := readLocal + createLocal

	read := 0
	create := 0
	create5m := 0

	if rawCacheTotal <= total {
		if localTotal > 0 && total > localTotal && rawCacheTotal > 0 {
			scale := float64(total) / float64(localTotal)
			read = int(math.Floor(float64(readLocal) * scale))
			create = int(math.Floor(float64(createLocal) * scale))
			create5m = int(math.Floor(float64(create5mLocal) * scale))
			if read+create > total {
				create = maxInt(0, total-read)
			}
			if create5m > create {
				create5m = create
			}
		} else {
			read = readLocal
			create = createLocal
			create5m = create5mLocal
		}
	} else {
		scale := 0.0
		if total > 0 {
			scale = float64(total) / float64(rawCacheTotal)
		}
		read = int(math.Floor(float64(readLocal) * scale))
		create = int(math.Floor(float64(createLocal) * scale))
		create5m = int(math.Floor(float64(create5mLocal) * scale))
		if read+create > total {
			create = maxInt(0, total-read)
		}
		if create5m > create {
			create5m = create
		}
	}

	out := buildDefaultUsageCache()
	out.CacheReadInputTokens = read
	out.CacheCreationInputTokens = create
	out.CacheCreation = UsageCacheCreationBreakdown{
		Ephemeral5mInputTokens: create5m,
		Ephemeral1hInputTokens: create1h,
	}
	out.ClaudeCacheCreation5m = create5m
	out.ClaudeCacheCreation1h = create1h
	return out
}

func applyCacheSmoothing(sessionState *PromptCacheSessionState, totalInputTokens int, cache *UsageCache) {
	if !parseGlobalOptionBoolAny(true, cx2ccCacheSmoothingEnabledOpt, legacyCx2ccCacheSmoothingEnabledOpt) {
		return
	}
	if sessionState == nil || cache == nil {
		return
	}

	currentRead := toNonNegativeInt(cache.CacheReadInputTokens)
	currentCreation := toNonNegativeInt(cache.CacheCreationInputTokens)
	currentTotal := toNonNegativeInt(totalInputTokens)

	dropThreshold := clampFloat(parseGlobalOptionFloatAny(0.3, cx2ccCacheSmoothingDropThresholdOpt, legacyCx2ccCacheSmoothingDropThresholdOpt), 0.01, 1.0)
	dampening := clampFloat(parseGlobalOptionFloatAny(0.8, cx2ccCacheSmoothingDampeningOpt, legacyCx2ccCacheSmoothingDampeningOpt), 0.01, 1.0)
	minPrevRead := maxInt(0, parseGlobalOptionIntAny(1000, cx2ccCacheSmoothingMinPrevReadOpt, legacyCx2ccCacheSmoothingMinPrevReadOpt))

	sessionState.mu.Lock()
	defer sessionState.mu.Unlock()

	if sessionState.CacheSmoothing == nil {
		sessionState.CacheSmoothing = &CacheSmoothingState{
			LastCacheRead:     currentRead,
			LastCacheCreation: currentCreation,
			LastTotalInput:    currentTotal,
			RequestCount:      1,
			TotalCorrected:    0,
		}
		return
	}

	sm := sessionState.CacheSmoothing
	prevRead := toNonNegativeInt(sm.LastCacheRead)
	prevCreation := toNonNegativeInt(sm.LastCacheCreation)
	prevTotal := toNonNegativeInt(sm.LastTotalInput)
	sm.RequestCount = toNonNegativeInt(sm.RequestCount) + 1

	if prevRead < minPrevRead {
		sm.LastCacheRead = currentRead
		sm.LastCacheCreation = currentCreation
		sm.LastTotalInput = currentTotal
		return
	}

	if prevTotal > 0 && currentTotal < int(float64(prevTotal)*0.5) {
		sm.LastCacheRead = currentRead
		sm.LastCacheCreation = currentCreation
		sm.LastTotalInput = currentTotal
		return
	}

	readDrop := prevRead - currentRead
	creationSpike := currentCreation - prevCreation
	readDropRatio := 0.0
	if prevRead > 0 {
		readDropRatio = float64(readDrop) / float64(prevRead)
	}

	if readDropRatio < dropThreshold || creationSpike <= 0 || currentCreation <= 0 {
		sm.LastCacheRead = currentRead
		sm.LastCacheCreation = currentCreation
		sm.LastTotalInput = currentTotal
		return
	}

	rawCorrection := minInt(readDrop, creationSpike, currentCreation)
	correction := int(math.Floor(float64(rawCorrection) * dampening))
	if correction <= 0 {
		sm.LastCacheRead = currentRead
		sm.LastCacheCreation = currentCreation
		sm.LastTotalInput = currentTotal
		return
	}

	newRead := currentRead + correction
	newCreation := maxInt(0, currentCreation-correction)

	cache.CacheReadInputTokens = newRead
	cache.CacheCreationInputTokens = newCreation
	if cache.CacheCreation.Ephemeral5mInputTokens > newCreation {
		cache.CacheCreation.Ephemeral5mInputTokens = newCreation
	}
	if cache.CacheCreation.Ephemeral1hInputTokens > newCreation {
		cache.CacheCreation.Ephemeral1hInputTokens = newCreation
	}

	sm.LastCacheRead = newRead
	sm.LastCacheCreation = newCreation
	sm.LastTotalInput = currentTotal
	sm.TotalCorrected = toNonNegativeInt(sm.TotalCorrected) + correction
}

func stableJsonStringify(value any) string {
	b, err := json.Marshal(sortJsonValue(value))
	if err == nil {
		return string(b)
	}
	b, err = json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}

func sortJsonValue(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = sortJsonValue(v[i])
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(v))
		for _, k := range keys {
			out[k] = sortJsonValue(v[k])
		}
		return out
	default:
		return value
	}
}

func normalizeSystemTextForPromptCache(text any) string {
	raw := asString(text)
	if raw == "" {
		return ""
	}
	return reCch.ReplaceAllString(raw, "cch=<stable>")
}

func normalizeSystemTextForPromptCacheHash(text any) string {
	raw := normalizeSystemTextForPromptCache(text)
	if raw == "" {
		return ""
	}
	return normalizeDynamicTimeLines(raw)
}

func normalizeToolForCache(tool map[string]any) string {
	name := strings.TrimSpace(asString(tool["name"]))
	if name == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	parts = append(parts, "name:"+name)

	desc := strings.TrimSpace(asString(tool["description"]))
	if desc != "" {
		parts = append(parts, "desc:"+desc)
	}

	schema := tool["input_schema"]
	if schema == nil {
		schema = tool["inputSchema"]
	}
	if schema != nil {
		if sorted, err := json.Marshal(sortJsonValue(schema)); err == nil {
			parts = append(parts, "schema:"+string(sorted))
		}
	}
	return strings.Join(parts, "|")
}

type cacheCtx struct {
	toolUseIdMap map[string]string
	nextToolRef  int
}

func canonicalizeContentBlockForCache(block any, ctx *cacheCtx) map[string]any {
	getToolRef := func(rawId any) string {
		if ctx == nil || rawId == nil {
			return ""
		}
		key := strings.TrimSpace(asString(rawId))
		if key == "" {
			return ""
		}
		if ctx.toolUseIdMap == nil {
			ctx.toolUseIdMap = make(map[string]string)
		}
		if ctx.nextToolRef <= 0 {
			ctx.nextToolRef = 1
		}
		if _, ok := ctx.toolUseIdMap[key]; !ok {
			ref := "tool_" + strconv.Itoa(ctx.nextToolRef)
			ctx.toolUseIdMap[key] = ref
			ctx.nextToolRef++
		}
		return ctx.toolUseIdMap[key]
	}

	m, _ := block.(map[string]any)
	typ := ""
	if m != nil {
		typ = strings.TrimSpace(asString(m["type"]))
	}

	switch strings.ToLower(typ) {
	case "text":
		return map[string]any{"type": "text", "text": asString(m["text"])}
	case "thinking":
		return map[string]any{"type": "thinking", "thinking": asString(m["thinking"])}
	case "tool_use":
		input := m["input"]
		if input == nil {
			input = map[string]any{}
		}
		return map[string]any{
			"type":     "tool_use",
			"tool_ref": getToolRef(m["id"]),
			"name":     asString(m["name"]),
			"input":    sortJsonValue(input),
		}
	case "tool_result":
		rawToolUseId := m["tool_use_id"]
		if rawToolUseId == nil {
			rawToolUseId = m["toolUseId"]
		}
		content := m["content"]
		if content == nil {
			content = ""
		}
		return map[string]any{
			"type":     "tool_result",
			"tool_ref": getToolRef(rawToolUseId),
			"content":  sortJsonValue(content),
		}
	case "image", "input_image", "image_url":
		source, _ := m["source"].(map[string]any)
		mediaType := strings.ToLower(strings.TrimSpace(asString(firstNonNil(getNested(source, "media_type"), getNested(source, "mediaType"), m["media_type"]))))
		url := ""
		if source != nil {
			url = strings.TrimSpace(asString(source["url"]))
		}
		if url == "" {
			url = strings.TrimSpace(asString(m["image_url"]))
		}
		dataLen := 0
		if source != nil {
			if s, ok := source["data"].(string); ok {
				dataLen = len(s)
			}
		}
		bytes := 0
		if dataLen > 0 {
			bytes = int(math.Floor(float64(dataLen) * 3.0 / 4.0))
		}
		return map[string]any{
			"type":       "image",
			"media_type": mediaType,
			"url":        url,
			"bytes":      bytes,
		}
	case "document":
		source, _ := m["source"].(map[string]any)
		mediaType := strings.ToLower(strings.TrimSpace(asString(firstNonNil(getNested(source, "media_type"), getNested(source, "mediaType")))))
		url := ""
		if source != nil {
			url = strings.TrimSpace(asString(source["url"]))
		}
		dataLen := 0
		if source != nil {
			if s, ok := source["data"].(string); ok {
				dataLen = len(s)
			}
		}
		bytes := 0
		if dataLen > 0 {
			bytes = int(math.Floor(float64(dataLen) * 3.0 / 4.0))
		}
		return map[string]any{
			"type":       "document",
			"title":      asString(m["title"]),
			"media_type": mediaType,
			"url":        url,
			"bytes":      bytes,
		}
	default:
		clone := map[string]any{}
		for k, v := range m {
			clone[k] = v
		}
		for _, k := range []string{"cache_control", "signature", "id", "tool_use_id", "toolUseId", "timestamp", "created_at", "createdAt"} {
			delete(clone, k)
		}
		t := typ
		if strings.TrimSpace(t) == "" {
			t = "json"
		}
		return map[string]any{"type": t, "data": sortJsonValue(clone)}
	}
}

func computeCacheBreakpoints(anthropicReq map[string]any, includeTools bool) ([]CacheBreakpoint, int, string) {
	breakpoints := make([]CacheBreakpoint, 0, 4)
	hasher := sha256.New()
	cumulativeTokens := 0
	ctx := &cacheCtx{toolUseIdMap: make(map[string]string), nextToolRef: 1}

	if includeTools {
		tools, _ := anthropicReq["tools"].([]any)
		toolObjs := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			if m, ok := t.(map[string]any); ok && m != nil {
				toolObjs = append(toolObjs, m)
			}
		}
		sort.Slice(toolObjs, func(i, j int) bool {
			return strings.Compare(asString(toolObjs[i]["name"]), asString(toolObjs[j]["name"])) < 0
		})
		toolCacheTtl := CacheTtlMs(0)
		toolCacheTtlSet := false
		for _, tool := range toolObjs {
			normalized := normalizeToolForCache(tool)
			if normalized == "" {
				continue
			}
			_, _ = hasher.Write([]byte(normalized))
			cumulativeTokens += countToolTokensForUsage(tool)
			if cc, ok := tool["cache_control"]; ok && cc != nil {
				toolCacheTtl = ttlMsForCacheControl(cc)
				toolCacheTtlSet = true
			}
		}
		if toolCacheTtlSet {
			breakpoints = append(breakpoints, CacheBreakpoint{
				Hash:   hex.EncodeToString(hasher.Sum(nil)),
				Tokens: cumulativeTokens,
				TTL:    toolCacheTtl,
			})
		}
	}

	system := anthropicReq["system"]
	switch s := system.(type) {
	case string:
		_, _ = hasher.Write([]byte(normalizeSystemTextForPromptCacheHash(s)))
		cumulativeTokens += countTokensText(normalizeSystemTextForPromptCache(s))
	case []any:
		for _, msgAny := range s {
			m, ok := msgAny.(map[string]any)
			textRaw := ""
			if ok && m != nil {
				textRaw = asString(m["text"])
			}
			_, _ = hasher.Write([]byte(normalizeSystemTextForPromptCacheHash(textRaw)))
			cumulativeTokens += countTokensText(normalizeSystemTextForPromptCache(textRaw))
			if ok && m != nil && m["cache_control"] != nil {
				breakpoints = append(breakpoints, CacheBreakpoint{
					Hash:   hex.EncodeToString(hasher.Sum(nil)),
					Tokens: cumulativeTokens,
					TTL:    ttlMsForCacheControl(m["cache_control"]),
				})
			}
		}
	case map[string]any:
		textRaw := asString(s["text"])
		_, _ = hasher.Write([]byte(normalizeSystemTextForPromptCacheHash(textRaw)))
		cumulativeTokens += countTokensText(normalizeSystemTextForPromptCache(textRaw))
		if s["cache_control"] != nil {
			breakpoints = append(breakpoints, CacheBreakpoint{
				Hash:   hex.EncodeToString(hasher.Sum(nil)),
				Tokens: cumulativeTokens,
				TTL:    ttlMsForCacheControl(s["cache_control"]),
			})
		}
	}

	messages, _ := anthropicReq["messages"].([]any)
	for _, msgAny := range messages {
		msg, ok := msgAny.(map[string]any)
		if !ok || msg == nil {
			continue
		}
		content := msg["content"]
		switch c := content.(type) {
		case []any:
			for _, blockAny := range c {
				if s, ok := blockAny.(string); ok {
					canonical := canonicalizeContentBlockForCache(map[string]any{"type": "text", "text": s}, ctx)
					_, _ = hasher.Write([]byte(stableJsonStringify(canonical)))
					cumulativeTokens += countTokensText(s)
					continue
				}
				block, ok := blockAny.(map[string]any)
				if !ok || block == nil {
					continue
				}
				canonical := canonicalizeContentBlockForCache(block, ctx)
				_, _ = hasher.Write([]byte(stableJsonStringify(canonical)))
				cumulativeTokens += countMessageBlockTokensForUsage(block)
				if block["cache_control"] != nil {
					breakpoints = append(breakpoints, CacheBreakpoint{
						Hash:   hex.EncodeToString(hasher.Sum(nil)),
						Tokens: cumulativeTokens,
						TTL:    ttlMsForCacheControl(block["cache_control"]),
					})
				}
			}
		case string:
			canonical := canonicalizeContentBlockForCache(map[string]any{"type": "text", "text": c}, ctx)
			_, _ = hasher.Write([]byte(stableJsonStringify(canonical)))
			cumulativeTokens += countTokensText(c)
		}
	}

	finalHash := hex.EncodeToString(hasher.Sum(nil))
	return breakpoints, cumulativeTokens, finalHash
}

func computeToolCacheUsage(promptCache *PromptCache, anthropicReq map[string]any, dryRun bool) (readTokens int, creationTokens int, creation5mTokens int, creation1hTokens int) {
	if promptCache == nil {
		return 0, 0, 0, 0
	}
	tools, _ := anthropicReq["tools"].([]any)
	if len(tools) == 0 {
		return 0, 0, 0, 0
	}

	for _, toolAny := range tools {
		tool, ok := toolAny.(map[string]any)
		if !ok || tool == nil {
			continue
		}
		normalized := normalizeToolForCache(tool)
		if normalized == "" {
			continue
		}

		sum := sha256.Sum256([]byte(normalized))
		hashHex := hex.EncodeToString(sum[:])
		key := "tool:" + hashHex
		ttl := ttlMsForCacheControl(tool["cache_control"])
		ttlDur := time.Duration(ttl) * time.Millisecond

		if hit, ok := promptCache.Get(key); ok && hit >= 0 {
			readTokens += hit
			if !dryRun {
				promptCache.Set(key, hit, ttlDur)
			}
			continue
		}

		tokens := countToolTokensForUsage(tool)
		if tokens < 0 {
			tokens = 0
		}
		creationTokens += tokens
		if ttl == cacheTtl5m {
			creation5mTokens += tokens
		}
		if ttl == cacheTtl1h {
			creation1hTokens += tokens
		}
		if !dryRun {
			promptCache.Set(key, tokens, ttlDur)
		}
	}

	return readTokens, creationTokens, creation5mTokens, creation1hTokens
}

func computePromptCacheUsage(promptCache *PromptCache, anthropicReq map[string]any, dryRun bool) UsageCache {
	usage := buildDefaultUsageCache()
	if promptCache == nil || anthropicReq == nil {
		return usage
	}

	toolRead, toolCreate, toolCreate5m, toolCreate1h := computeToolCacheUsage(promptCache, anthropicReq, dryRun)

	breakpoints, _, _ := computeCacheBreakpoints(anthropicReq, false)
	if len(breakpoints) == 0 {
		usage.CacheReadInputTokens = toolRead
		usage.CacheCreationInputTokens = toolCreate
		usage.CacheCreation = UsageCacheCreationBreakdown{Ephemeral5mInputTokens: toolCreate5m, Ephemeral1hInputTokens: toolCreate1h}
		usage.ClaudeCacheCreation5m = toolCreate5m
		usage.ClaudeCacheCreation1h = toolCreate1h
		return usage
	}

	readTokens := toolRead
	creationTokens := toolCreate
	creation5mTokens := toolCreate5m
	creation1hTokens := toolCreate1h

	for i := len(breakpoints) - 1; i >= 0; i-- {
		bp := breakpoints[i]
		key := "cache:" + bp.Hash

		hit, ok := promptCache.Get(key)
		if !ok {
			hashText := strings.TrimSpace(bp.Hash)
			if hashText != "" && !strings.HasPrefix(hashText, "full:") {
				if legacy, ok2 := promptCache.Get("cache:full:" + hashText); ok2 {
					hit = legacy
					ok = true
					if !dryRun {
						promptCache.Set(key, maxInt(0, bp.Tokens), time.Duration(bp.TTL)*time.Millisecond)
					}
				}
			}
		}

		if ok && hit >= 0 {
			nonToolReadTokens := maxInt(0, bp.Tokens)
			readTokens += nonToolReadTokens
			if !dryRun {
				promptCache.Set(key, nonToolReadTokens, time.Duration(bp.TTL)*time.Millisecond)
			}

			cachedTokens := nonToolReadTokens
			for j := i + 1; j < len(breakpoints); j++ {
				later := breakpoints[j]
				laterKey := "cache:" + later.Hash
				if !dryRun {
					promptCache.Set(laterKey, later.Tokens, time.Duration(later.TTL)*time.Millisecond)
				}
				additional := later.Tokens - cachedTokens
				if additional < 0 {
					additional = 0
				}
				creationTokens += additional
				if later.TTL == cacheTtl5m {
					creation5mTokens += additional
				}
				if later.TTL == cacheTtl1h {
					creation1hTokens += additional
				}
				if later.Tokens > cachedTokens {
					cachedTokens = later.Tokens
				}
			}

			usage.CacheReadInputTokens = readTokens
			usage.CacheCreationInputTokens = creationTokens
			usage.CacheCreation = UsageCacheCreationBreakdown{Ephemeral5mInputTokens: creation5mTokens, Ephemeral1hInputTokens: creation1hTokens}
			usage.ClaudeCacheCreation5m = creation5mTokens
			usage.ClaudeCacheCreation1h = creation1hTokens
			return usage
		}
	}

	last := breakpoints[len(breakpoints)-1]
	creationTokens += last.Tokens
	if last.TTL == cacheTtl5m {
		creation5mTokens += last.Tokens
	}
	if last.TTL == cacheTtl1h {
		creation1hTokens += last.Tokens
	}

	if !dryRun {
		for _, bp := range breakpoints {
			promptCache.Set("cache:"+bp.Hash, bp.Tokens, time.Duration(bp.TTL)*time.Millisecond)
		}
	}

	usage.CacheReadInputTokens = readTokens
	usage.CacheCreationInputTokens = creationTokens
	usage.CacheCreation = UsageCacheCreationBreakdown{Ephemeral5mInputTokens: creation5mTokens, Ephemeral1hInputTokens: creation1hTokens}
	usage.ClaudeCacheCreation5m = creation5mTokens
	usage.ClaudeCacheCreation1h = creation1hTokens
	return usage
}

func buildUsageForTotal(localTotalInputTokens int, totalInputTokens int, usageCacheLocal UsageCache, sessionState *PromptCacheSessionState) (inputTokens int, usageCache UsageCache) {
	usageCache = reconcileCacheUsage(usageCacheLocal, localTotalInputTokens, totalInputTokens)
	applyCacheSmoothing(sessionState, totalInputTokens, &usageCache)

	uncached := maxInt(0, toNonNegativeInt(totalInputTokens)-toNonNegativeInt(usageCache.CacheCreationInputTokens)-toNonNegativeInt(usageCache.CacheReadInputTokens))
	return uncached, usageCache
}

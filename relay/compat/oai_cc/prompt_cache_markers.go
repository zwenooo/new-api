package oai_cc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

const promptCacheRebuildIdle = 5 * time.Minute

type stableCacheMarkerState struct {
	LastRequestAtMs int64
	PrevEndMarker   *messageMarkerPlanItem
}

type messageMarkerPlanItem struct {
	MessageIndex int
	Kind         string
	BlockIndex   int
}

var reBillingHeaderLine = regexp.MustCompile(`(?im)^\s*x-anthropic-billing-header:[^\n]*(?:\n|$)`)
var reMultiNewlines = regexp.MustCompile(`\n{3,}`)

func StripAnthropicBillingHeaderFromSystemValue(system any) (any, int) {
	stripText := func(text any) (string, int) {
		raw := asString(text)
		if raw == "" {
			return raw, 0
		}
		matches := reBillingHeaderLine.FindAllStringIndex(raw, -1)
		if len(matches) == 0 {
			return raw, 0
		}
		cleaned := reBillingHeaderLine.ReplaceAllString(raw, "")
		cleaned = reMultiNewlines.ReplaceAllString(cleaned, "\n\n")
		return cleaned, len(matches)
	}

	switch s := system.(type) {
	case string:
		cleaned, removed := stripText(s)
		return cleaned, removed
	case []any:
		removed := 0
		out := make([]any, len(s))
		for i, item := range s {
			if str, ok := item.(string); ok {
				cleaned, r := stripText(str)
				out[i] = cleaned
				removed += r
				continue
			}
			m, ok := item.(map[string]any)
			if !ok || m == nil {
				out[i] = item
				continue
			}
			cleaned, r := stripText(m["text"])
			m2 := make(map[string]any, len(m))
			for k, v := range m {
				m2[k] = v
			}
			m2["text"] = cleaned
			out[i] = m2
			removed += r
		}
		return out, removed
	case map[string]any:
		cleaned, removed := stripText(s["text"])
		out := make(map[string]any, len(s))
		for k, v := range s {
			out[k] = v
		}
		out["text"] = cleaned
		return out, removed
	default:
		return system, 0
	}
}

func deepCopyJSONMap(body map[string]any) map[string]any {
	if body == nil {
		return map[string]any{}
	}
	b, err := json.Marshal(body)
	if err != nil {
		out := make(map[string]any, len(body))
		for k, v := range body {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil || out == nil {
		out = make(map[string]any, len(body))
		for k, v := range body {
			out[k] = v
		}
	}
	return out
}

func extractSystemTextForScopeSeed(system any) string {
	switch s := system.(type) {
	case string:
		return s
	case []any:
		parts := make([]string, 0, len(s))
		for _, item := range s {
			m, ok := item.(map[string]any)
			if !ok || m == nil {
				continue
			}
			t := asString(m["text"])
			if t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		return asString(s["text"])
	default:
		return ""
	}
}

func computeStableCacheMarkerScopeKey(body map[string]any) string {
	if body == nil {
		return ""
	}

	model := strings.ToLower(strings.TrimSpace(asString(body["model"])))
	systemRaw := extractSystemTextForScopeSeed(body["system"])
	system := normalizeSystemTextForPromptCacheHash(systemRaw)

	toolNames := make([]string, 0, 8)
	if tools, ok := body["tools"].([]any); ok {
		for _, t := range tools {
			m, ok := t.(map[string]any)
			if !ok || m == nil {
				continue
			}
			name := strings.TrimSpace(asString(m["name"]))
			if name != "" {
				toolNames = append(toolNames, name)
			}
		}
	}
	sort.Strings(toolNames)
	if len(toolNames) > 128 {
		toolNames = toolNames[:128]
	}

	var modelVal any
	if model != "" {
		modelVal = model
	} else {
		modelVal = nil
	}

	sysSeed := system
	if len(sysSeed) > 2048 {
		sysSeed = sysSeed[:2048]
	}

	seed := map[string]any{
		"v":     1,
		"model": modelVal,
		"system": sysSeed,
		"tools": toolNames,
	}

	payload := stableJsonStringify(seed)
	if payload == "" {
		return ""
	}

	sum := sha256.Sum256([]byte("stable_cache_marker_scope\x00" + payload))
	return hex.EncodeToString(sum[:])[:32]
}

func getLastUserMessageIndex(messages []any) int {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok || msg == nil {
			continue
		}
		if strings.TrimSpace(asString(msg["role"])) == "user" {
			return i
		}
	}
	return -1
}

func normalizeMessageMarkerPlanItem(item *messageMarkerPlanItem) *messageMarkerPlanItem {
	if item == nil {
		return nil
	}
	if item.MessageIndex < 0 {
		return nil
	}
	blockIndex := item.BlockIndex
	if blockIndex < 0 {
		blockIndex = 0
	}
	return &messageMarkerPlanItem{MessageIndex: item.MessageIndex, Kind: "array", BlockIndex: blockIndex}
}

func isSameMessageMarker(a *messageMarkerPlanItem, b *messageMarkerPlanItem) bool {
	if a == nil || b == nil {
		return false
	}
	return a.MessageIndex == b.MessageIndex && a.Kind == b.Kind && a.BlockIndex == b.BlockIndex
}

func isMessageMarkerValid(messages []any, marker *messageMarkerPlanItem) bool {
	if len(messages) == 0 || marker == nil {
		return false
	}
	m := normalizeMessageMarkerPlanItem(marker)
	if m == nil {
		return false
	}
	if m.MessageIndex < 0 || m.MessageIndex >= len(messages) {
		return false
	}
	msg, ok := messages[m.MessageIndex].(map[string]any)
	if !ok || msg == nil {
		return false
	}
	if strings.TrimSpace(asString(msg["role"])) != "user" {
		return false
	}
	content := msg["content"]
	if s, ok := content.(string); ok {
		return m.BlockIndex == 0 && strings.TrimSpace(s) != ""
	}
	arr, ok := content.([]any)
	if !ok || len(arr) == 0 {
		return false
	}
	return m.BlockIndex >= 0 && m.BlockIndex < len(arr)
}

func selectRollingEndMessageMarker(messages []any, maxMessageIndexExclusive int) *messageMarkerPlanItem {
	start := len(messages) - 1
	if maxMessageIndexExclusive >= 0 {
		start = minInt(start, maxMessageIndexExclusive-1)
	}

	for mi := start; mi >= 0; mi-- {
		msg, ok := messages[mi].(map[string]any)
		if !ok || msg == nil {
			continue
		}
		if strings.TrimSpace(asString(msg["role"])) != "user" {
			continue
		}
		content := msg["content"]
		if s, ok := content.(string); ok {
			if strings.TrimSpace(s) == "" {
				continue
			}
			return &messageMarkerPlanItem{MessageIndex: mi, Kind: "array", BlockIndex: 0}
		}
		arr, ok := content.([]any)
		if !ok || len(arr) == 0 {
			continue
		}

		chosen := -1
		for bi := len(arr) - 1; bi >= 0; bi-- {
			b := arr[bi]
			if s, ok := b.(string); ok {
				if strings.TrimSpace(s) == "" {
					continue
				}
				chosen = bi
				break
			}
			m, ok := b.(map[string]any)
			if !ok || m == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(asString(m["type"]))) == "tool_use" {
				continue
			}
			chosen = bi
			break
		}
		if chosen == -1 {
			continue
		}
		return &messageMarkerPlanItem{MessageIndex: mi, Kind: "array", BlockIndex: chosen}
	}

	return nil
}

func stripAllCacheControlMarkers(body map[string]any) {
	if body == nil {
		return
	}

	if tools, ok := body["tools"].([]any); ok {
		for _, t := range tools {
			m, ok := t.(map[string]any)
			if !ok || m == nil {
				continue
			}
			delete(m, "cache_control")
		}
	}

	system := body["system"]
	switch s := system.(type) {
	case []any:
		for _, item := range s {
			m, ok := item.(map[string]any)
			if !ok || m == nil {
				continue
			}
			delete(m, "cache_control")
		}
	case map[string]any:
		delete(s, "cache_control")
	}

	if messages, ok := body["messages"].([]any); ok {
		for _, msgAny := range messages {
			msg, ok := msgAny.(map[string]any)
			if !ok || msg == nil {
				continue
			}
			content, ok := msg["content"].([]any)
			if !ok {
				continue
			}
			for _, blockAny := range content {
				m, ok := blockAny.(map[string]any)
				if !ok || m == nil {
					continue
				}
				delete(m, "cache_control")
			}
		}
	}
}

func isLikelyMediaContentBlock(block any) bool {
	m, ok := block.(map[string]any)
	if !ok || m == nil {
		return false
	}

	typ := strings.ToLower(strings.TrimSpace(asString(m["type"])))
	switch typ {
	case "image", "input_image", "audio", "input_audio", "video", "file", "document":
		return true
	}

	mediaType := strings.ToLower(strings.TrimSpace(asString(firstNonNil(m["media_type"], m["mime_type"], getNested(m, "source", "media_type")))))
	if strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/") {
		return true
	}

	for _, k := range []string{"image_url", "file", "document", "audio"} {
		if m[k] != nil {
			return true
		}
	}
	return false
}

func computeAutoMessageCacheMarkerPlan(messages []any) []*messageMarkerPlanItem {
	plan := make([]*messageMarkerPlanItem, 0, 2)
	if len(messages) <= 2 {
		return plan
	}

	msgCount := len(messages)
	lastMsgIdx := msgCount - 1
	candidateSet := make(map[int]struct{})

	userIndices := make([]int, 0, 4)
	for i := 0; i < msgCount; i++ {
		msg, ok := messages[i].(map[string]any)
		if !ok || msg == nil {
			continue
		}
		if msg["role"] == "user" {
			userIndices = append(userIndices, i)
		}
	}
	for i := len(userIndices) - 1; i >= 0 && len(candidateSet) < 2; i-- {
		idx := userIndices[i]
		if idx == lastMsgIdx {
			continue
		}
		candidateSet[idx] = struct{}{}
	}

	if len(candidateSet) == 0 {
		if msgCount >= 2 {
			candidateSet[msgCount-2] = struct{}{}
		}
		if msgCount >= 4 {
			candidateSet[msgCount-4] = struct{}{}
		}
	}

	chosenMessageIndices := make([]int, 0, len(candidateSet))
	for idx := range candidateSet {
		chosenMessageIndices = append(chosenMessageIndices, idx)
	}
	sort.Ints(chosenMessageIndices)

	for _, idx := range chosenMessageIndices {
		msg, ok := messages[idx].(map[string]any)
		if !ok || msg == nil {
			continue
		}

		if s, ok := msg["content"].(string); ok {
			if strings.TrimSpace(s) == "" {
				continue
			}
			plan = append(plan, &messageMarkerPlanItem{MessageIndex: idx, Kind: "string", BlockIndex: 0})
			continue
		}

		content, ok := msg["content"].([]any)
		if !ok || len(content) == 0 {
			continue
		}

		hasMedia := false
		for _, b := range content {
			if isLikelyMediaContentBlock(b) {
				hasMedia = true
				break
			}
		}

		chosenBlockIdx := -1
		if hasMedia {
			for i := len(content) - 1; i >= 0; i-- {
				b := content[i]
				if _, ok := b.(string); ok {
					chosenBlockIdx = i
					break
				}
				m, ok := b.(map[string]any)
				if !ok || m == nil {
					continue
				}
				if strings.ToLower(strings.TrimSpace(asString(m["type"]))) == "tool_use" {
					continue
				}
				chosenBlockIdx = i
				break
			}
		} else {
			for i := len(content) - 1; i >= 0; i-- {
				b := content[i]
				if _, ok := b.(string); ok {
					chosenBlockIdx = i
					break
				}
				m, ok := b.(map[string]any)
				if !ok || m == nil {
					continue
				}
				if strings.TrimSpace(asString(m["type"])) == "text" {
					chosenBlockIdx = i
					break
				}
			}
			if chosenBlockIdx == -1 {
				for i := len(content) - 1; i >= 0; i-- {
					m, ok := content[i].(map[string]any)
					if !ok || m == nil {
						continue
					}
					if strings.TrimSpace(asString(m["type"])) == "tool_result" {
						chosenBlockIdx = i
						break
					}
				}
			}
		}

		if chosenBlockIdx == -1 {
			chosenBlockIdx = len(content) - 1
		}

		if m, ok := content[chosenBlockIdx].(map[string]any); ok && m != nil {
			if strings.TrimSpace(asString(m["type"])) == "tool_use" {
				continue
			}
		}

		plan = append(plan, &messageMarkerPlanItem{MessageIndex: idx, Kind: "array", BlockIndex: chosenBlockIdx})
	}

	if len(plan) > 2 {
		plan = plan[:2]
	}
	return plan
}

func applyAutoMessageCacheMarkerPlan(messages []any, plan []*messageMarkerPlanItem) bool {
	if len(messages) == 0 || len(plan) == 0 {
		return false
	}

	appliedAny := false
	for _, item := range plan {
		if item == nil {
			continue
		}
		idx := item.MessageIndex
		if idx < 0 || idx >= len(messages) {
			continue
		}
		msg, ok := messages[idx].(map[string]any)
		if !ok || msg == nil {
			continue
		}
		if msg["role"] != "user" {
			continue
		}

		if item.Kind == "string" {
			contentStr, ok := msg["content"].(string)
			if !ok {
				continue
			}
			msg["content"] = []any{
				map[string]any{
					"type":          "text",
					"text":          contentStr,
					"cache_control": map[string]any{"type": "ephemeral"},
				},
			}
			appliedAny = true
			continue
		}

		blockIndex := item.BlockIndex
		if contentStr, ok := msg["content"].(string); ok {
			msg["content"] = []any{map[string]any{"type": "text", "text": contentStr}}
		}

		content, ok := msg["content"].([]any)
		if !ok || len(content) == 0 {
			continue
		}
		if blockIndex < 0 || blockIndex >= len(content) {
			continue
		}

		chosen := content[blockIndex]
		if m, ok := chosen.(map[string]any); ok && m != nil && strings.TrimSpace(asString(m["type"])) == "tool_use" {
			continue
		}

		if s, ok := chosen.(string); ok {
			content[blockIndex] = map[string]any{
				"type":          "text",
				"text":          s,
				"cache_control": map[string]any{"type": "ephemeral"},
			}
			msg["content"] = content
			appliedAny = true
			continue
		}

		if m, ok := chosen.(map[string]any); ok && m != nil {
			m2 := make(map[string]any, len(m)+1)
			for k, v := range m {
				m2[k] = v
			}
			m2["cache_control"] = map[string]any{"type": "ephemeral"}
			content[blockIndex] = m2
			msg["content"] = content
			appliedAny = true
		}
	}

	return appliedAny
}

func addAutoCacheControlMarkers(body map[string]any, mode string, messageMarkerPlan []*messageMarkerPlanItem) (map[string]any, []*messageMarkerPlanItem, bool) {
	if body == nil {
		return body, messageMarkerPlan, false
	}

	newBody := deepCopyJSONMap(body)

	if mode == "force" {
		stripAllCacheControlMarkers(newBody)
	}

	if tools, ok := newBody["tools"].([]any); ok && len(tools) > 0 {
		lastToolIdx := len(tools) - 1
		for i, t := range tools {
			m, ok := t.(map[string]any)
			if !ok || m == nil {
				continue
			}
			if m["cache_control"] != nil {
				delete(m, "cache_control")
				tools[i] = m
			}
		}
		if last, ok := tools[lastToolIdx].(map[string]any); ok && last != nil {
			last2 := make(map[string]any, len(last)+1)
			for k, v := range last {
				last2[k] = v
			}
			last2["cache_control"] = map[string]any{"type": "ephemeral"}
			tools[lastToolIdx] = last2
		}
		newBody["tools"] = tools
	}

	if system := newBody["system"]; system != nil {
		switch s := system.(type) {
		case string:
			newBody["system"] = []any{
				map[string]any{
					"type":          "text",
					"text":          s,
					"cache_control": map[string]any{"type": "ephemeral"},
				},
			}
		case []any:
			hasSystemCache := false
			for _, item := range s {
				if m, ok := item.(map[string]any); ok && m != nil && m["cache_control"] != nil {
					hasSystemCache = true
					break
				}
			}
			if (!hasSystemCache || mode == "force") && len(s) > 0 {
				lastIdx := len(s) - 1
				if m, ok := s[lastIdx].(map[string]any); ok && m != nil {
					m2 := make(map[string]any, len(m)+1)
					for k, v := range m {
						m2[k] = v
					}
					m2["cache_control"] = map[string]any{"type": "ephemeral"}
					s[lastIdx] = m2
				}
			}
			newBody["system"] = s
		case map[string]any:
			obj := make(map[string]any, len(s)+1)
			for k, v := range s {
				obj[k] = v
			}
			obj["cache_control"] = map[string]any{"type": "ephemeral"}
			newBody["system"] = []any{obj}
		}
	}

	resolvedPlan := messageMarkerPlan
	messageMarkerPlanApplied := false

	if messages, ok := newBody["messages"].([]any); ok && len(messages) > 2 {
		hasMessageCache := false
		for _, msgAny := range messages {
			msg, ok := msgAny.(map[string]any)
			if !ok || msg == nil {
				continue
			}
			content, ok := msg["content"].([]any)
			if !ok {
				continue
			}
			for _, blockAny := range content {
				if m, ok := blockAny.(map[string]any); ok && m != nil && m["cache_control"] != nil {
					hasMessageCache = true
					break
				}
			}
			if hasMessageCache {
				break
			}
		}

		if !hasMessageCache || mode == "force" {
			if resolvedPlan == nil {
				resolvedPlan = computeAutoMessageCacheMarkerPlan(messages)
			}
			messageMarkerPlanApplied = applyAutoMessageCacheMarkerPlan(messages, resolvedPlan)
			newBody["messages"] = messages
		}
	}

	return newBody, resolvedPlan, messageMarkerPlanApplied
}

func getOrInitStableCacheMarkerState(sessionState *PromptCacheSessionState, accountId string, scopeKey string) *stableCacheMarkerState {
	if sessionState == nil {
		return nil
	}
	if strings.TrimSpace(accountId) == "" {
		accountId = "default"
	}
	key := accountId
	if strings.TrimSpace(scopeKey) != "" {
		key = accountId + ":" + strings.TrimSpace(scopeKey)
	}

	sessionState.mu.Lock()
	defer sessionState.mu.Unlock()

	if sessionState.StableCacheMarkers == nil {
		sessionState.StableCacheMarkers = make(map[string]*stableCacheMarkerState)
	}
	if existing := sessionState.StableCacheMarkers[key]; existing != nil {
		return existing
	}
	created := &stableCacheMarkerState{LastRequestAtMs: 0, PrevEndMarker: nil}
	sessionState.StableCacheMarkers[key] = created
	return created
}

func addStableAutoCacheControlMarkers(body map[string]any, sessionState *PromptCacheSessionState, accountId string, idleRebuild time.Duration) map[string]any {
	if body == nil || sessionState == nil {
		return body
	}
	if idleRebuild <= 0 {
		idleRebuild = promptCacheRebuildIdle
	}

	scopeKey := computeStableCacheMarkerScopeKey(body)
	markerState := getOrInitStableCacheMarkerState(sessionState, accountId, scopeKey)
	nowMs := time.Now().UnixMilli()

	lastAt := int64(0)
	prevMarker := (*messageMarkerPlanItem)(nil)

	if markerState != nil {
		sessionState.mu.Lock()
		lastAt = markerState.LastRequestAtMs
		prevMarker = markerState.PrevEndMarker
		markerState.LastRequestAtMs = nowMs
		sessionState.mu.Unlock()
	}

	idleMs := int64(0)
	idleRebuildTriggered := false
	if lastAt > 0 {
		idleMs = nowMs - lastAt
		idleRebuildTriggered = idleMs >= idleRebuild.Milliseconds()
	}

	messages, _ := body["messages"].([]any)
	var prev *messageMarkerPlanItem
	if !idleRebuildTriggered && isMessageMarkerValid(messages, prevMarker) {
		prev = normalizeMessageMarkerPlanItem(prevMarker)
	}

	lastUserIdx := getLastUserMessageIndex(messages)
	var current *messageMarkerPlanItem
	if lastUserIdx > 0 {
		current = normalizeMessageMarkerPlanItem(selectRollingEndMessageMarker(messages, lastUserIdx))
	}

	plan := make([]*messageMarkerPlanItem, 0, 2)
	if prev != nil {
		plan = append(plan, prev)
	} else if current != nil {
		fallbackPrev := normalizeMessageMarkerPlanItem(selectRollingEndMessageMarker(messages, current.MessageIndex))
		if fallbackPrev != nil && !isSameMessageMarker(fallbackPrev, current) {
			plan = append(plan, fallbackPrev)
		}
	}
	if current != nil && !isSameMessageMarker(current, func() *messageMarkerPlanItem {
		if len(plan) == 0 {
			return nil
		}
		return plan[len(plan)-1]
	}()) {
		plan = append(plan, current)
	}

	var planArg []*messageMarkerPlanItem
	if len(plan) > 0 {
		planArg = plan
	} else {
		planArg = nil
	}

	result, _, _ := addAutoCacheControlMarkers(body, "force", planArg)

	if markerState != nil {
		sessionState.mu.Lock()
		markerState.PrevEndMarker = current
		sessionState.mu.Unlock()
	}

	_ = idleMs // kept for parity; currently unused.
	return result
}

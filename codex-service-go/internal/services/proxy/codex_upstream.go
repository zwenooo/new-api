package proxy

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var codexOfficialClientUserAgentPrefixes = []string{
	"codex_cli_rs/",
	"codex_vscode/",
	"codex_app/",
	"codex_chatgpt_desktop/",
	"codex_atlas/",
	"codex_exec/",
	"codex_sdk_ts/",
	"codex ",
}

var codexOfficialClientOriginatorPrefixes = []string{
	"codex_",
	"codex ",
}

var codexResponsesAllowedHeaders = map[string]struct{}{
	"accept-language":                       {},
	"content-type":                          {},
	"conversation_id":                       {},
	"openai-beta":                           {},
	"originator":                            {},
	"session_id":                            {},
	"user-agent":                            {},
	"version":                               {},
	"x-codex-beta-features":                 {},
	"x-codex-turn-metadata":                 {},
	"x-codex-turn-state":                    {},
	"x-responsesapi-include-timing-metrics": {},
}

func filterHeadersByAllowlist(src http.Header, allowed map[string]struct{}) http.Header {
	dst := http.Header{}
	for key, vals := range src {
		lower := strings.ToLower(strings.TrimSpace(key))
		if _, ok := allowed[lower]; !ok {
			continue
		}
		for _, v := range vals {
			dst.Add(http.CanonicalHeaderKey(key), v)
		}
	}
	return dst
}

func normalizeCodexClientHeader(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchCodexClientHeaderPrefixes(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		normalizedPrefix := normalizeCodexClientHeader(prefix)
		if normalizedPrefix == "" {
			continue
		}
		if strings.HasPrefix(value, normalizedPrefix) || strings.Contains(value, normalizedPrefix) {
			return true
		}
	}
	return false
}

func isCodexOfficialClientRequest(userAgent string) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(ua, codexOfficialClientUserAgentPrefixes)
}

func isCodexOfficialClientOriginator(originator string) bool {
	v := normalizeCodexClientHeader(originator)
	if v == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(v, codexOfficialClientOriginatorPrefixes)
}

func isCodexOfficialClientByHeaders(userAgent, originator string) bool {
	return isCodexOfficialClientRequest(userAgent) || isCodexOfficialClientOriginator(originator)
}

func resolveOpenAIUpstreamOriginator(userAgent, originator, fallback string) string {
	if normalizedOriginator := strings.TrimSpace(originator); normalizedOriginator != "" {
		return normalizedOriginator
	}
	if isCodexOfficialClientByHeaders(userAgent, originator) {
		return "codex_cli_rs"
	}
	normalizedFallback := strings.TrimSpace(fallback)
	if normalizedFallback != "" && !strings.EqualFold(normalizedFallback, "codex_cli_rs") {
		return normalizedFallback
	}
	return "opencode"
}

func extractCodexClientVersion(userAgent string) string {
	trimmed := strings.TrimSpace(userAgent)
	lower := strings.ToLower(trimmed)
	for _, prefix := range codexOfficialClientUserAgentPrefixes {
		normalizedPrefix := normalizeCodexClientHeader(prefix)
		if normalizedPrefix == "" || !strings.HasSuffix(normalizedPrefix, "/") {
			continue
		}
		idx := strings.Index(lower, normalizedPrefix)
		if idx < 0 {
			continue
		}
		remainder := strings.TrimSpace(trimmed[idx+len(normalizedPrefix):])
		if remainder == "" {
			continue
		}
		version := remainder
		if cut := strings.IndexAny(version, " ();"); cut >= 0 {
			version = version[:cut]
		}
		version = strings.TrimSpace(version)
		if version != "" {
			return version
		}
	}
	return ""
}

func resolveCodexRequestVersion(userAgent, fallbackUserAgent string) string {
	if version := extractCodexClientVersion(userAgent); version != "" {
		return version
	}
	if version := extractCodexClientVersion(fallbackUserAgent); version != "" {
		return version
	}
	return defaultCodexClientVersion
}

func mergeOpenAIBetaHeader(existing string, requiredTokens ...string) string {
	tokens := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)

	appendToken := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		key := strings.ToLower(token)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		tokens = append(tokens, token)
	}

	hasFamily := func(token string) bool {
		token = strings.TrimSpace(token)
		if token == "" {
			return false
		}
		idx := strings.Index(token, "=")
		if idx < 0 {
			_, ok := seen[strings.ToLower(token)]
			return ok
		}
		family := strings.ToLower(strings.TrimSpace(token[:idx]))
		if family == "" {
			return false
		}
		for _, existingToken := range tokens {
			lower := strings.ToLower(strings.TrimSpace(existingToken))
			if lower == family || strings.HasPrefix(lower, family+"=") {
				return true
			}
		}
		return false
	}

	for _, part := range strings.Split(existing, ",") {
		appendToken(part)
	}
	for _, token := range requiredTokens {
		if hasFamily(token) {
			continue
		}
		appendToken(token)
	}
	return strings.Join(tokens, ",")
}

func normalizeResponsesWebSocketBetaHeader(existing string) string {
	tokens := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	hasWebSocketBeta := false

	appendToken := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		lower := strings.ToLower(token)
		if lower == "responses=experimental" || strings.HasPrefix(lower, "responses=") {
			return
		}
		if _, ok := seen[lower]; ok {
			return
		}
		if strings.HasPrefix(lower, "responses_websockets=") {
			hasWebSocketBeta = true
		}
		seen[lower] = struct{}{}
		tokens = append(tokens, token)
	}

	for _, part := range strings.Split(existing, ",") {
		appendToken(part)
	}
	if !hasWebSocketBeta {
		appendToken(openAIResponsesWSBetaV2)
	}
	return strings.Join(tokens, ",")
}

func resolveOpenAICompactSessionID(headers http.Header, promptCacheKey string) string {
	if sessionID := strings.TrimSpace(headerFirst(headers, "session_id")); sessionID != "" {
		return sessionID
	}
	if conversationID := strings.TrimSpace(headerFirst(headers, "conversation_id")); conversationID != "" {
		return conversationID
	}
	if promptCacheKey = strings.TrimSpace(promptCacheKey); promptCacheKey != "" {
		return promptCacheKey
	}
	return uuid.NewString()
}

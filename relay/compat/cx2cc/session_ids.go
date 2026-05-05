package cx2cc

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	appcommon "one-api/common"
	"one-api/dto"

	"github.com/gin-gonic/gin"
)

type sessionIDs struct {
	ThreadID       string
	SessionKey     string
	SessionSource  string
	ConversationID string
	SessionID      string
}

func resolveSessionIDs(c *gin.Context, anthropicReq *dto.ClaudeRequest) sessionIDs {
	sessionKey, source := resolvePromptCacheSessionKey(c, anthropicReq)
	derivedConversationID := buildDeterministicConversationID(sessionKey)

	meta := parseClaudeMetadata(anthropicReq)
	metaSessionID := firstNonEmptyString(meta["session_id"], meta["sessionId"], meta["session"])
	metaConversationID := firstNonEmptyString(meta["conversation_id"], meta["conversationId"])

	upstreamConversationIDCandidate := metaConversationID
	if upstreamConversationIDCandidate == "" {
		upstreamConversationIDCandidate = metaSessionID
	}

	upstreamConversationID := derivedConversationID
	if upstreamConversationIDCandidate != "" && upstreamConversationIDCandidate != sessionKey {
		upstreamConversationID = upstreamConversationIDCandidate
	}

	upstreamSessionIDCandidate := metaSessionID
	upstreamSessionID := upstreamConversationID
	if upstreamSessionIDCandidate != "" && upstreamSessionIDCandidate != sessionKey {
		upstreamSessionID = upstreamSessionIDCandidate
	}

	threadID := upstreamSessionID
	if threadID == "" {
		threadID = upstreamConversationID
	}

	return sessionIDs{
		ThreadID:       threadID,
		SessionKey:     sessionKey,
		SessionSource:  source,
		ConversationID: upstreamConversationID,
		SessionID:      upstreamSessionID,
	}
}

func parseClaudeMetadata(req *dto.ClaudeRequest) map[string]any {
	if req == nil || len(req.Metadata) == 0 {
		return map[string]any{}
	}
	var meta map[string]any
	_ = appcommon.Unmarshal(req.Metadata, &meta)
	if meta == nil {
		meta = map[string]any{}
	}
	return meta
}

func resolvePromptCacheSessionKey(c *gin.Context, req *dto.ClaudeRequest) (key string, source string) {
	meta := parseClaudeMetadata(req)

	if v := firstNonEmptyString(meta["session_key"], meta["sessionKey"]); v != "" {
		return v, "metadata_session_key"
	}

	if userID := firstNonEmptyString(meta["user_id"], meta["userId"]); userID != "" {
		if sid := extractSessionIDFromMetadataUserID(userID); sid != "" {
			return sid, "metadata_user_id"
		}
	}

	if v := firstNonEmptyString(meta["session_id"], meta["sessionId"], meta["session"], meta["conversation_id"], meta["conversationId"]); v != "" {
		return deriveStableSessionKeyFromProvidedID("metadata", v), "metadata_session"
	}

	for _, k := range []string{"x-session-key", "session-key", "session_key"} {
		if v := strings.TrimSpace(c.GetHeader(k)); v != "" {
			return v, "header"
		}
	}

	for _, k := range []string{
		"x-session-id",
		"session-id",
		"session_id",
		"x-conversation-id",
		"conversation-id",
		"conversation_id",
		"x-claude-session-id",
		"x-claude-code-session-id",
	} {
		if v := strings.TrimSpace(c.GetHeader(k)); v != "" {
			return deriveStableSessionKeyFromProvidedID("header", v), "header"
		}
	}

	if inferred := inferSessionKeyFromHistory(c, req); inferred != "" {
		return inferred, "auto_seed"
	}

	if ip := strings.TrimSpace(c.ClientIP()); ip != "" {
		return "ip:" + ip, "ip"
	}

	if c.Request != nil {
		if remote := strings.TrimSpace(c.Request.RemoteAddr); remote != "" {
			return "ip:" + remote, "ip"
		}
	}

	return "global", "global"
}

func inferSessionKeyFromHistory(c *gin.Context, req *dto.ClaudeRequest) string {
	ua := normalizeUserAgentForSessionSeed(c.GetHeader("User-Agent"))
	clientIP := extractClientIPForSessionSeed(c)
	accessKey := readClientAccessKey(c)
	accessKeyHash := ""
	if accessKey != "" {
		h := sha256.Sum256([]byte("access_key\x00" + accessKey))
		accessKeyHash = hex.EncodeToString(h[:])[:16]
	}

	clientHint := strings.TrimSpace(firstNonEmptyString(
		c.GetHeader("x-client-fingerprint"),
		c.GetHeader("x-client-session"),
		c.GetHeader("x-client-id"),
	))

	meta := parseClaudeMetadata(req)
	metaHint := strings.TrimSpace(firstNonEmptyString(
		meta["client_fingerprint"],
		meta["clientFingerprint"],
		meta["client_id"],
		meta["clientId"],
	))

	hint := clientHint
	if hint == "" {
		hint = metaHint
	}

	if ua == "" && clientIP == "" && hint == "" && accessKeyHash == "" {
		return ""
	}

	// If the client provides an access key, prefer it over IP so sessions remain stable across proxies.
	ipForFingerprint := clientIP
	if accessKeyHash != "" {
		ipForFingerprint = ""
	}

	sum := sha256.Sum256([]byte("auto_session_fp\x00" + accessKeyHash + "\x00" + ua + "\x00" + ipForFingerprint + "\x00" + hint))
	fingerprintHex := hex.EncodeToString(sum[:])[:32]
	return "auto_" + fingerprintHex
}

func extractClientIPForSessionSeed(c *gin.Context) string {
	if c == nil {
		return ""
	}
	for _, k := range []string{"cf-connecting-ip", "true-client-ip", "x-real-ip"} {
		if v := strings.TrimSpace(c.GetHeader(k)); v != "" {
			if len(v) > 128 {
				return v[:128]
			}
			return v
		}
	}

	if xff := strings.TrimSpace(c.GetHeader("x-forwarded-for")); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			if len(first) > 128 {
				return first[:128]
			}
			return first
		}
	}

	ip := strings.TrimSpace(c.ClientIP())
	if len(ip) > 128 {
		ip = ip[:128]
	}
	return ip
}

func normalizeUserAgentForSessionSeed(userAgent string) string {
	s := strings.TrimSpace(userAgent)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func readClientAccessKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if xAPIKey := strings.TrimSpace(c.GetHeader("x-api-key")); xAPIKey != "" {
		return xAPIKey
	}
	if auth := strings.TrimSpace(c.GetHeader("authorization")); auth != "" {
		if token := parseBearerToken(auth); token != "" {
			return token
		}
	}
	return ""
}

func parseBearerToken(authorization string) string {
	const prefix = "bearer "
	if len(authorization) < len(prefix) {
		return ""
	}
	if strings.ToLower(authorization[:len(prefix)]) != prefix {
		return ""
	}
	return strings.TrimSpace(authorization[len(prefix):])
}

var reSessionUUID = regexp.MustCompile(`session_([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

func extractSessionIDFromMetadataUserID(userID string) string {
	normalized := strings.ReplaceAll(userID, " ", "")
	m := reSessionUUID.FindStringSubmatch(normalized)
	if len(m) != 2 {
		return ""
	}
	return m[1]
}

func deriveStableSessionKeyFromProvidedID(prefix string, providedID string) string {
	s := strings.TrimSpace(providedID)
	sum := sha256.Sum256([]byte("provided_session_key\x00" + prefix + "\x00" + s))
	return "provided_" + hex.EncodeToString(sum[:])[:32]
}

func firstNonEmptyString(values ...any) string {
	for _, v := range values {
		switch t := v.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s != "" {
				return s
			}
		case []any:
			for _, item := range t {
				if s := firstNonEmptyString(item); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func buildDeterministicConversationID(sessionKey string) string {
	s := strings.TrimSpace(sessionKey)
	if s == "" {
		return ""
	}
	return buildDeterministicUUID("conversation\x00" + s)
}

func buildDeterministicUUID(seed string) string {
	sum := md5.Sum([]byte(seed))
	hexStr := hex.EncodeToString(sum[:]) // 32 chars
	if len(hexStr) != 32 {
		return ""
	}
	// Stable UUID-ish format for non-UUID seeds.
	// Uses md5 and sets version/variant bits for consistent formatting.
	return hexStr[:8] + "-" + hexStr[8:12] + "-4" + hexStr[13:16] + "-a" + hexStr[17:20] + "-" + hexStr[20:32]
}

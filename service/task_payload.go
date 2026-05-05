package service

import (
	"encoding/base64"
	"sort"
	"strings"

	"one-api/common"
)

func SanitizeTaskPayload(body []byte, publicTaskID string, upstreamTaskID string) []byte {
	publicTaskID = strings.TrimSpace(publicTaskID)
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	if len(body) == 0 || publicTaskID == "" || upstreamTaskID == "" {
		return body
	}

	sensitiveIDs := map[string]struct{}{
		upstreamTaskID: {},
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(upstreamTaskID); err == nil {
		if decodedID := strings.TrimSpace(string(decoded)); decodedID != "" {
			sensitiveIDs[decodedID] = struct{}{}
		}
	}
	sensitiveList := make([]string, 0, len(sensitiveIDs))
	for sensitiveID := range sensitiveIDs {
		sensitiveList = append(sensitiveList, sensitiveID)
	}
	sort.Slice(sensitiveList, func(i, j int) bool {
		return len(sensitiveList[i]) > len(sensitiveList[j])
	})

	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return body
	}

	sanitized, ok := sanitizeTaskPayloadValue(payload, publicTaskID, sensitiveIDs, sensitiveList)
	if !ok {
		return body
	}
	result, err := common.Marshal(sanitized)
	if err != nil {
		return body
	}
	return result
}

func sanitizeTaskPayloadValue(value any, publicTaskID string, sensitiveIDs map[string]struct{}, sensitiveList []string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			sanitized, ok := sanitizeTaskPayloadValue(item, publicTaskID, sensitiveIDs, sensitiveList)
			if !ok {
				return nil, false
			}
			typed[key] = sanitized
		}
		return typed, true
	case []any:
		for idx, item := range typed {
			sanitized, ok := sanitizeTaskPayloadValue(item, publicTaskID, sensitiveIDs, sensitiveList)
			if !ok {
				return nil, false
			}
			typed[idx] = sanitized
		}
		return typed, true
	case string:
		if _, ok := sensitiveIDs[typed]; ok {
			return publicTaskID, true
		}
		sanitized := typed
		for _, sensitiveID := range sensitiveList {
			if sensitiveID == "" || !strings.Contains(sanitized, sensitiveID) {
				continue
			}
			sanitized = strings.ReplaceAll(sanitized, sensitiveID, publicTaskID)
		}
		return sanitized, true
	default:
		return value, true
	}
}

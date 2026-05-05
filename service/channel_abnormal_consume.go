package service

import (
	"encoding/json"
	"one-api/common"
	"one-api/constant"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"one-api/types"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	ChannelAbnormalConsumeMaxRecords  = 200
	channelAbnormalConsumeStringLimit = 100
)

type ChannelAbnormalConsumeRecord struct {
	ID          int64  `json:"id"`
	Time        int64  `json:"time"` // unix seconds
	DurationMs  int64  `json:"duration_ms"`
	RequestID   string `json:"request_id"`
	RequestIP   string `json:"request_ip"`
	RequestPath string `json:"request_path"`
	Group       string `json:"group"`

	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelType int    `json:"channel_type"`

	ErrorType   string `json:"error_type"`
	ErrorCode   string `json:"error_code"`
	StatusCode  int    `json:"status_code"`
	RequestBody string `json:"request_body"`
	ResponseBody string `json:"response_body"`

	MatchedKeywords []string `json:"matched_keywords,omitempty"`
}

type channelAbnormalConsumeCollector struct {
	mu      sync.RWMutex
	enabled map[int]bool
	records map[int][]ChannelAbnormalConsumeRecord
	nextID  int64
}

var channelAbnormalConsumeStore = &channelAbnormalConsumeCollector{
	enabled: make(map[int]bool),
	records: make(map[int][]ChannelAbnormalConsumeRecord),
}

func SetChannelAbnormalConsumeEnabled(channelId int, enabled bool) {
	if channelId <= 0 {
		return
	}
	channelAbnormalConsumeStore.mu.Lock()
	channelAbnormalConsumeStore.enabled[channelId] = enabled
	channelAbnormalConsumeStore.mu.Unlock()
}

func IsChannelAbnormalConsumeEnabled(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	channelAbnormalConsumeStore.mu.RLock()
	enabled := channelAbnormalConsumeStore.enabled[channelId]
	channelAbnormalConsumeStore.mu.RUnlock()
	return enabled
}

func ListChannelAbnormalConsumeRecords(channelId int, limit int) []ChannelAbnormalConsumeRecord {
	if channelId <= 0 {
		return []ChannelAbnormalConsumeRecord{}
	}
	if limit <= 0 || limit > ChannelAbnormalConsumeMaxRecords {
		limit = ChannelAbnormalConsumeMaxRecords
	}

	channelAbnormalConsumeStore.mu.RLock()
	src := channelAbnormalConsumeStore.records[channelId]
	if len(src) == 0 {
		channelAbnormalConsumeStore.mu.RUnlock()
		return []ChannelAbnormalConsumeRecord{}
	}
	copied := make([]ChannelAbnormalConsumeRecord, len(src))
	copy(copied, src)
	channelAbnormalConsumeStore.mu.RUnlock()

	if limit > len(copied) {
		limit = len(copied)
	}
	out := make([]ChannelAbnormalConsumeRecord, 0, limit)
	for i := len(copied) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, copied[i])
	}
	return out
}

func ClearChannelAbnormalConsumeRecords(channelId int) {
	if channelId <= 0 {
		return
	}
	channelAbnormalConsumeStore.mu.Lock()
	delete(channelAbnormalConsumeStore.records, channelId)
	channelAbnormalConsumeStore.mu.Unlock()
}

func GetChannelAbnormalConsumeEnabledMapCopy() map[int]bool {
	channelAbnormalConsumeStore.mu.RLock()
	defer channelAbnormalConsumeStore.mu.RUnlock()

	copied := make(map[int]bool, len(channelAbnormalConsumeStore.enabled))
	for k, v := range channelAbnormalConsumeStore.enabled {
		copied[k] = v
	}
	return copied
}

func RecordChannelAbnormalConsume(
	c *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	channelError types.ChannelError,
	err *types.NewAPIError,
) {
	if c == nil || relayInfo == nil || err == nil {
		return
	}

	channelId := channelError.ChannelId
	if channelId <= 0 {
		return
	}

	if !IsChannelAbnormalConsumeEnabled(channelId) {
		return
	}

	requestBodyBytes, _ := common.GetRequestBody(c)
	requestBody := maskLongStringValuesJSON(requestBodyBytes)

	responseAny := buildAbnormalConsumeResponseAny(err)
	responseAny = maskLongStringValuesAny(responseAny)
	responseBytes, marshalErr := json.MarshalIndent(responseAny, "", "  ")
	responseBody := ""
	if marshalErr == nil {
		responseBody = string(responseBytes)
	}

	matchedKeywords := matchAutomaticDisableKeywords(err.Error())

	attemptStart := common.GetContextKeyTime(c, constant.ContextKeyChannelAttemptStartTime)
	if attemptStart.IsZero() {
		attemptStart = relayInfo.StartTime
	}
	durationMs := time.Since(attemptStart).Milliseconds()

	usingGroupID := relayInfo.UsingGroupId
	if usingGroupID <= 0 {
		usingGroupID = common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	}
	group := ""
	if usingGroupID > 0 {
		group = strconv.Itoa(usingGroupID)
	}

	record := ChannelAbnormalConsumeRecord{
		Time:         time.Now().Unix(),
		DurationMs:   durationMs,
		RequestID:    c.GetString(common.RequestIdKey),
		RequestIP:    c.ClientIP(),
		RequestPath:  c.Request.URL.Path,
		ChannelID:    channelError.ChannelId,
		ChannelName:  channelError.ChannelName,
		ChannelType:  channelError.ChannelType,
		Group:        group,
		ErrorType:    string(err.GetErrorType()),
		ErrorCode:    string(err.GetErrorCode()),
		StatusCode:   err.StatusCode,
		RequestBody:  requestBody,
		ResponseBody: responseBody,
	}
	if len(matchedKeywords) > 0 {
		record.MatchedKeywords = matchedKeywords
	}

	channelAbnormalConsumeStore.mu.Lock()
	channelAbnormalConsumeStore.nextID++
	record.ID = channelAbnormalConsumeStore.nextID
	records := append(channelAbnormalConsumeStore.records[channelId], record)
	if len(records) > ChannelAbnormalConsumeMaxRecords {
		records = records[len(records)-ChannelAbnormalConsumeMaxRecords:]
	}
	channelAbnormalConsumeStore.records[channelId] = records
	channelAbnormalConsumeStore.mu.Unlock()
}

func buildAbnormalConsumeResponseAny(err *types.NewAPIError) any {
	if err == nil {
		return map[string]any{}
	}
	switch err.GetErrorType() {
	case types.ErrorTypeClaudeError:
		claudeErr := err.ToClaudeError()
		return map[string]any{
			"type":  "error",
			"error": map[string]any{
				"type":    claudeErr.Type,
				"message": claudeErr.Message,
			},
		}
	default:
		openaiErr := err.ToOpenAIError()
		return map[string]any{
			"error": map[string]any{
				"message": openaiErr.Message,
				"type":    openaiErr.Type,
				"param":   openaiErr.Param,
				"code":    openaiErr.Code,
			},
		}
	}
}

func matchAutomaticDisableKeywords(message string) []string {
	lowerMessage := strings.ToLower(message)
	found, words := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, false)
	if !found || len(words) == 0 {
		return nil
	}
	dedup := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		keyword := strings.TrimSpace(w)
		if keyword == "" {
			continue
		}
		if _, ok := dedup[keyword]; ok {
			continue
		}
		dedup[keyword] = struct{}{}
		out = append(out, keyword)
	}
	return out
}

func maskLongStringValuesJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		s := strings.TrimSpace(string(raw))
		if utf8.RuneCountInString(s) > channelAbnormalConsumeStringLimit {
			return "***"
		}
		return s
	}
	v = maskLongStringValuesAny(v)
	maskedBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(maskedBytes)
}

func maskLongStringValuesAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = maskLongStringValuesAny(val)
		}
		return x
	case []any:
		for i, val := range x {
			x[i] = maskLongStringValuesAny(val)
		}
		return x
	case string:
		if utf8.RuneCountInString(x) > channelAbnormalConsumeStringLimit {
			return "***"
		}
		return x
	default:
		return v
	}
}

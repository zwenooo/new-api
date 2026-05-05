package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	filePath string
	expireAt string

	mu             sync.RWMutex
	cache          runtimeCache
	overrideExpire int64
	onChange       func()
}

type runtimeCache struct {
	modTime time.Time
	data    map[string]interface{}
}

type BlockResult struct {
	Blocked    bool
	Reason     string
	RetryAfter int
}

type UsageLimitInfo struct {
	ResetsInSeconds int
	PlanType        string
	Message         string
}

func New(filePath, expireAt string, onChange func()) *Manager {
	filePath = strings.TrimSpace(filePath)
	expireAt = strings.TrimSpace(expireAt)
	m := &Manager{filePath: filePath, expireAt: expireAt, onChange: onChange}
	if ts, ok := parseTimestamp(expireAt); ok {
		m.overrideExpire = ts
	}
	return m
}

func (m *Manager) ShouldBlock(now time.Time) (BlockResult, error) {
	if m == nil {
		return BlockResult{Blocked: false}, nil
	}
	data, err := m.readRuntimeData()
	if err != nil {
		return BlockResult{}, err
	}
	status := extractStatus(data)

	// Priority: member_expired > expired > transport_quarantine > channel_backoff > cooldown.
	if status != nil {
		switch strings.ToLower(getString(status, "state")) {
		case "payment_required":
			reason := strings.TrimSpace(getString(status, "reason"))
			if reason == "" {
				reason = "payment_required"
			}
			return BlockResult{Blocked: true, Reason: reason}, nil
		case "expired":
			reason := strings.TrimSpace(getString(status, "reason"))
			if reason == "" {
				reason = "expired"
			}
			return BlockResult{Blocked: true, Reason: reason}, nil
		}
	}

	if m.overrideExpire > 0 && m.overrideExpire <= now.UnixMilli() {
		return BlockResult{Blocked: true, Reason: "expired"}, nil
	}
	if expire := resolveExpireTimestamp(data); expire > 0 && expire <= now.UnixMilli() {
		return BlockResult{Blocked: true, Reason: "expired"}, nil
	}

	if status == nil {
		return BlockResult{Blocked: false}, nil
	}
	switch strings.ToLower(getString(status, "state")) {
	case "transport_quarantine":
		return m.temporaryBlockResult(now, status, "transport_quarantine", "transportUntil", "transportStartedAt")
	case "channel_backoff":
		return m.temporaryBlockResult(now, status, "channel_backoff", "backoffUntil", "backoffStartedAt")
	case "sleep":
		return m.temporaryBlockResult(now, status, "sleep", "sleepUntil", "sleepStartedAt")
	default:
		return BlockResult{Blocked: false}, nil
	}
}

func (m *Manager) RecordUsageLimit(info UsageLimitInfo) error {
	if m == nil || info.ResetsInSeconds <= 0 {
		return nil
	}
	return m.mutateRuntime(func(data map[string]interface{}) (bool, error) {
		status := map[string]interface{}{
			"state":           "sleep",
			"reason":          "usage_limit",
			"sleepUntil":      time.Now().Add(time.Duration(info.ResetsInSeconds) * time.Second).UTC().Format(time.RFC3339),
			"resetsInSeconds": info.ResetsInSeconds,
			"sleepStartedAt":  time.Now().UTC().Format(time.RFC3339),
			"updatedAt":       time.Now().UTC().Format(time.RFC3339),
		}
		if info.PlanType != "" {
			status["planType"] = info.PlanType
		}
		if info.Message != "" {
			status["message"] = info.Message
		}
		data["status"] = status
		return true, nil
	})
}

func (m *Manager) RecordChannelBackoff(seconds int, message string) error {
	if m == nil || seconds <= 0 {
		return nil
	}
	message = strings.TrimSpace(message)
	return m.mutateRuntime(func(data map[string]interface{}) (bool, error) {
		status := map[string]interface{}{
			"state":            "channel_backoff",
			"reason":           "channel_backoff",
			"backoffUntil":     time.Now().Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339),
			"resetsInSeconds":  seconds,
			"backoffStartedAt": time.Now().UTC().Format(time.RFC3339),
			"updatedAt":        time.Now().UTC().Format(time.RFC3339),
		}
		if message != "" {
			status["message"] = message
		}
		data["status"] = status
		return true, nil
	})
}

func (m *Manager) RecordTransportQuarantine(seconds int, message string) error {
	if m == nil || seconds <= 0 {
		return nil
	}
	message = strings.TrimSpace(message)
	return m.mutateRuntime(func(data map[string]interface{}) (bool, error) {
		status := map[string]interface{}{
			"state":              "transport_quarantine",
			"reason":             "transport_quarantine",
			"transportUntil":     time.Now().Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339),
			"resetsInSeconds":    seconds,
			"transportStartedAt": time.Now().UTC().Format(time.RFC3339),
			"updatedAt":          time.Now().UTC().Format(time.RFC3339),
		}
		if message != "" {
			status["message"] = message
		}
		data["status"] = status
		return true, nil
	})
}

func (m *Manager) ClearSleepStatus() error {
	if m == nil {
		return nil
	}
	return m.clearStatusState("sleep")
}

func (m *Manager) ClearChannelBackoff() error {
	if m == nil {
		return nil
	}
	return m.clearStatusState("channel_backoff")
}

func (m *Manager) ClearTransportQuarantine() error {
	if m == nil {
		return nil
	}
	return m.clearStatusState("transport_quarantine")
}

func (m *Manager) ClearExpiredStatus() error {
	if m == nil {
		return nil
	}
	return m.clearStatusState("expired")
}

func (m *Manager) ClearBlockingStatus() error {
	if m == nil {
		return nil
	}
	return m.clearStatusState("sleep", "channel_backoff", "transport_quarantine", "expired", "payment_required")
}

func (m *Manager) clearStatusState(states ...string) error {
	return m.mutateRuntime(func(data map[string]interface{}) (bool, error) {
		status := extractStatus(data)
		if status == nil {
			return false, nil
		}
		currentState := strings.ToLower(strings.TrimSpace(getString(status, "state")))
		if currentState == "" {
			return false, nil
		}
		matched := false
		for _, state := range states {
			if strings.EqualFold(currentState, strings.TrimSpace(state)) {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
		delete(data, "status")
		data["statusClearedAt"] = time.Now().UTC().Format(time.RFC3339)
		return true, nil
	})
}

func (m *Manager) RecordPaymentRequired(code string) error {
	if m == nil {
		return nil
	}
	code = strings.TrimSpace(code)
	if code == "" {
		code = "payment_required"
	}
	return m.mutateRuntime(func(data map[string]interface{}) (bool, error) {
		data["status"] = map[string]interface{}{
			"state":     "payment_required",
			"reason":    code,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		return true, nil
	})
}

func (m *Manager) RecordExpired(code string) error {
	if m == nil {
		return nil
	}
	code = strings.TrimSpace(code)
	if code == "" {
		code = "expired"
	}
	return m.mutateRuntime(func(data map[string]interface{}) (bool, error) {
		data["status"] = map[string]interface{}{
			"state":     "expired",
			"reason":    code,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		return true, nil
	})
}

func (m *Manager) SyncFromPayload(respStatus int, contentType string, body []byte) error {
	if m == nil {
		return nil
	}
	if respStatus >= 400 {
		if respStatus == 402 {
			code := ""
			if strings.Contains(strings.ToLower(contentType), "json") && len(body) > 0 {
				var payload map[string]interface{}
				if json.Unmarshal(body, &payload) == nil {
					code = extractPaymentRequiredCode(payload)
				}
			}
			return m.RecordPaymentRequired(code)
		}
		if !strings.Contains(strings.ToLower(contentType), "json") {
			return nil
		}
		var payload map[string]interface{}
		if len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil
		}
		if info := extractUsageLimitInfo(payload); info != nil {
			return m.RecordUsageLimit(*info)
		}
		if strings.EqualFold(extractErrorCode(payload), "token_revoked") {
			return m.RecordExpired("token_revoked")
		}
		return nil
	}
	return m.ClearSleepStatus()
}

func (m *Manager) CurrentState() (string, error) {
	if m == nil {
		return "", nil
	}
	data, err := m.readRuntimeData()
	if err != nil || data == nil {
		return "", err
	}
	status := extractStatus(data)
	if status == nil {
		return "", nil
	}
	return strings.ToLower(strings.TrimSpace(getString(status, "state"))), nil
}

func (m *Manager) mutateRuntime(mutator func(map[string]interface{}) (bool, error)) error {
	if strings.TrimSpace(m.filePath) == "" {
		return nil
	}
	m.mu.Lock()
	data := make(map[string]interface{})
	raw, err := os.ReadFile(m.filePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			m.mu.Unlock()
			return err
		}
	} else if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			m.mu.Unlock()
			return err
		}
	}

	changed, err := mutator(data)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if !changed {
		m.mu.Unlock()
		return nil
	}
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if dir := filepath.Dir(m.filePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.mu.Unlock()
			return err
		}
	}
	if err := os.WriteFile(m.filePath, append(buf, '\n'), 0o600); err != nil {
		m.mu.Unlock()
		return err
	}
	m.cache = runtimeCache{}
	onChange := m.onChange
	m.mu.Unlock()
	if onChange != nil {
		onChange()
	}
	return nil
}

func (m *Manager) readRuntimeData() (map[string]interface{}, error) {
	path := strings.TrimSpace(m.filePath)
	if path == "" {
		return nil, nil
	}
	m.mu.RLock()
	cache := m.cache
	m.mu.RUnlock()
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if cache.data != nil && info.ModTime().Equal(cache.modTime) {
		return cache.data, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
	}
	m.mu.Lock()
	m.cache = runtimeCache{modTime: info.ModTime(), data: data}
	m.mu.Unlock()
	return data, nil
}

func extractStatus(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	if raw, ok := data["status"]; ok {
		if m, ok := raw.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func computeTimedStatusInfo(status map[string]interface{}, now time.Time, untilKey string, startedKey string) (int64, int) {
	until := int64(0)
	if ts, ok := parseTimestamp(status[untilKey]); ok {
		until = ts
	}
	if until == 0 {
		if seconds, ok := parseSeconds(status["resetsInSeconds"]); ok && seconds > 0 {
			base := now.UnixMilli()
			if ts, ok := parseTimestamp(status[startedKey]); ok {
				base = ts
			}
			until = base + int64(seconds*1000)
		}
	}
	if until == 0 {
		return 0, 0
	}
	remaining := int(math.Ceil(float64(until-now.UnixMilli()) / 1000))
	if remaining < 0 {
		remaining = 0
	}
	return until, remaining
}

func computeSleepInfo(status map[string]interface{}, now time.Time) (int64, int) {
	return computeTimedStatusInfo(status, now, "sleepUntil", "sleepStartedAt")
}

func computeChannelBackoffInfo(status map[string]interface{}, now time.Time) (int64, int) {
	return computeTimedStatusInfo(status, now, "backoffUntil", "backoffStartedAt")
}

func computeTransportQuarantineInfo(status map[string]interface{}, now time.Time) (int64, int) {
	return computeTimedStatusInfo(status, now, "transportUntil", "transportStartedAt")
}

func formatTemporaryStatusReason(state string, status map[string]interface{}) string {
	reason := strings.TrimSpace(state)
	if msg := strings.TrimSpace(getString(status, "message")); msg != "" {
		return reason + ": " + msg
	}
	if rawReason := strings.TrimSpace(getString(status, "reason")); rawReason != "" && !strings.EqualFold(rawReason, state) {
		return reason + ": " + rawReason
	}
	return reason
}

func (m *Manager) temporaryBlockResult(now time.Time, status map[string]interface{}, state string, untilKey string, startedKey string) (BlockResult, error) {
	var until int64
	var remaining int
	switch state {
	case "sleep":
		until, remaining = computeSleepInfo(status, now)
	case "channel_backoff":
		until, remaining = computeChannelBackoffInfo(status, now)
	case "transport_quarantine":
		until, remaining = computeTransportQuarantineInfo(status, now)
	default:
		until, remaining = computeTimedStatusInfo(status, now, untilKey, startedKey)
	}
	if until <= 0 {
		if err := m.clearStatusState(state); err != nil {
			return BlockResult{}, err
		}
		return BlockResult{Blocked: false}, nil
	}
	if remaining > 0 {
		return BlockResult{Blocked: true, Reason: formatTemporaryStatusReason(state, status), RetryAfter: remaining}, nil
	}
	if err := m.clearStatusState(state); err != nil {
		return BlockResult{}, err
	}
	return BlockResult{Blocked: false}, nil
}

func resolveExpireTimestamp(data map[string]interface{}) int64 {
	if ts, ok := parseTimestamp(data["expireAt"]); ok {
		return ts
	}
	if meta, ok := data["meta"].(map[string]interface{}); ok {
		if ts, ok := parseTimestamp(meta["expireAt"]); ok {
			return ts
		}
	}
	if env, ok := data["env"].(map[string]interface{}); ok {
		if ts, ok := parseTimestamp(env["PROXY_EXPIRE_AT"]); ok {
			return ts
		}
	}
	return 0
}

func extractUsageLimitInfo(payload map[string]interface{}) *UsageLimitInfo {
	if payload == nil {
		return nil
	}
	errObj, _ := payload["error"].(map[string]interface{})
	if errObj == nil {
		return nil
	}
	typeStr := strings.ToLower(strings.TrimSpace(getString(errObj, "type")))
	if typeStr == "" {
		typeStr = strings.ToLower(strings.TrimSpace(getString(errObj, "error_type")))
	}
	if typeStr != "usage_limit_reached" {
		return nil
	}
	seconds, ok := parseSeconds(errObj["resets_in_seconds"])
	if !ok || seconds <= 0 {
		seconds, ok = parseSeconds(errObj["resetsInSeconds"])
		if !ok || seconds <= 0 {
			seconds, ok = parseSeconds(errObj["resets_in_secs"])
			if !ok || seconds <= 0 {
				seconds, ok = parseSeconds(errObj["resetsInSecs"])
			}
		}
	}
	if !ok || seconds <= 0 {
		return nil
	}
	info := UsageLimitInfo{ResetsInSeconds: seconds}
	if plan := getString(errObj, "plan_type"); plan != "" {
		info.PlanType = plan
	} else if plan = getString(errObj, "planType"); plan != "" {
		info.PlanType = plan
	}
	if msg := strings.TrimSpace(getString(errObj, "message")); msg != "" {
		if typeStr != "" {
			info.Message = typeStr + ": " + msg
		} else {
			info.Message = msg
		}
	} else if typeStr != "" {
		info.Message = typeStr
	}
	return &info
}

func extractPaymentRequiredCode(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if detail, ok := payload["detail"].(map[string]interface{}); ok {
		if code := getString(detail, "code"); code != "" {
			return code
		}
	}
	return ""
}

func extractErrorCode(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	errObj, _ := payload["error"].(map[string]interface{})
	if errObj == nil {
		return ""
	}
	if code := strings.TrimSpace(getString(errObj, "code")); code != "" {
		return code
	}
	if code := strings.TrimSpace(getString(errObj, "error_code")); code != "" {
		return code
	}
	return ""
}

func parseTimestamp(raw interface{}) (int64, bool) {
	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		if num, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			if len(trimmed) <= 10 {
				return num * 1000, true
			}
			return num, true
		}
		if !strings.Contains(trimmed, "T") && strings.Contains(trimmed, " ") {
			trimmed = strings.ReplaceAll(trimmed, " ", "T")
		}
		if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return ts.UnixMilli(), true
		}
		if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return ts.UnixMilli(), true
		}
		if ts, err := time.Parse("2006-01-02T15:04:05", trimmed); err == nil {
			return ts.UnixMilli(), true
		}
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, true
		}
	}
	return 0, false
}

func parseSeconds(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return int(math.Round(num)), true
		}
	}
	return 0, false
}

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		switch vv := v.(type) {
		case string:
			return vv
		case json.Number:
			return vv.String()
		case float64:
			return strconv.FormatFloat(vv, 'f', -1, 64)
		case int:
			return strconv.Itoa(vv)
		case int64:
			return strconv.FormatInt(vv, 10)
		}
	}
	return ""
}

func (m *Manager) Dump() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	data, err := m.readRuntimeData()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	buf := bytes.Buffer{}
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

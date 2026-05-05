package proxy

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	transportHealthProxyBurstWindow        = 2 * time.Minute
	transportHealthProxyResetCooldown      = 30 * time.Second
	defaultTransportHealthStalledThreshold = 45 * time.Second
)

type TransportHealthSnapshot struct {
	InstanceID          int64  `json:"instance_id"`
	State               string `json:"state"`
	Inflight            int    `json:"inflight,omitempty"`
	ConsecutiveErrors   int    `json:"consecutive_errors,omitempty"`
	ConsecutiveTimeouts int    `json:"consecutive_timeouts,omitempty"`
	ResetCount          int    `json:"reset_count,omitempty"`
	LastStartedAt       int64  `json:"last_started_at,omitempty"`
	LastSuccessAt       int64  `json:"last_success_at,omitempty"`
	LastErrorAt         int64  `json:"last_error_at,omitempty"`
	LastTimeoutAt       int64  `json:"last_timeout_at,omitempty"`
	LastResetAt         int64  `json:"last_reset_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	LastResetReason     string `json:"last_reset_reason,omitempty"`
}

type transportHealthStore struct {
	mu         sync.Mutex
	byInstance map[int64]*transportHealthRecord
	byProxy    map[string]*transportHealthProxyRecord
}

type transportHealthRecord struct {
	instanceID          int64
	proxyKey            string
	inflight            int
	consecutiveErrors   int
	consecutiveTimeouts int
	resetCount          int
	lastStartedAt       time.Time
	lastSuccessAt       time.Time
	lastErrorAt         time.Time
	lastTimeoutAt       time.Time
	lastResetAt         time.Time
	lastError           string
	lastResetReason     string
}

type transportHealthProxyRecord struct {
	recentTimeouts []transportHealthTimeoutEvent
	lastResetAt    time.Time
}

type transportHealthTimeoutEvent struct {
	instanceID int64
	at         time.Time
}

type transportHealthDecision struct {
	resetProxy bool
	reason     string
}

func newTransportHealthStore() *transportHealthStore {
	return &transportHealthStore{
		byInstance: make(map[int64]*transportHealthRecord),
		byProxy:    make(map[string]*transportHealthProxyRecord),
	}
}

func transportHealthStalledThreshold() time.Duration {
	threshold := upstreamResponseHeaderTimeout()
	if threshold <= 0 {
		return defaultTransportHealthStalledThreshold
	}
	if threshold <= 10*time.Second {
		return threshold
	}
	return threshold - 5*time.Second
}

func (s *transportHealthStore) ensureInstanceLocked(instanceID int64, proxyKey string) *transportHealthRecord {
	if instanceID <= 0 {
		return nil
	}
	rec := s.byInstance[instanceID]
	if rec == nil {
		rec = &transportHealthRecord{instanceID: instanceID}
		s.byInstance[instanceID] = rec
	}
	if strings.TrimSpace(proxyKey) != "" {
		rec.proxyKey = strings.TrimSpace(proxyKey)
	}
	return rec
}

func (s *transportHealthStore) ensureProxyLocked(proxyKey string) *transportHealthProxyRecord {
	proxyKey = strings.TrimSpace(proxyKey)
	if proxyKey == "" {
		proxyKey = "direct"
	}
	rec := s.byProxy[proxyKey]
	if rec == nil {
		rec = &transportHealthProxyRecord{}
		s.byProxy[proxyKey] = rec
	}
	return rec
}

func (s *transportHealthStore) onStart(instanceID int64, proxyKey string, now time.Time) {
	if s == nil || instanceID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.ensureInstanceLocked(instanceID, proxyKey)
	if rec == nil {
		return
	}
	rec.inflight++
	rec.lastStartedAt = now
}

func (s *transportHealthStore) onResult(instanceID int64, proxyKey string, now time.Time, err error) transportHealthDecision {
	if s == nil || instanceID <= 0 {
		return transportHealthDecision{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.ensureInstanceLocked(instanceID, proxyKey)
	if rec == nil {
		return transportHealthDecision{}
	}
	if rec.inflight > 0 {
		rec.inflight--
	}
	if err == nil {
		rec.lastSuccessAt = now
		rec.consecutiveErrors = 0
		rec.consecutiveTimeouts = 0
		return transportHealthDecision{}
	}

	rec.lastErrorAt = now
	rec.lastError = shortenTransportError(err)
	rec.consecutiveErrors++
	if !isLikelyTransportTimeout(err) {
		rec.consecutiveTimeouts = 0
		return transportHealthDecision{}
	}

	rec.lastTimeoutAt = now
	rec.consecutiveTimeouts++
	proxyRec := s.ensureProxyLocked(rec.proxyKey)
	cutoff := now.Add(-transportHealthProxyBurstWindow)
	filtered := proxyRec.recentTimeouts[:0]
	for _, event := range proxyRec.recentTimeouts {
		if event.at.Before(cutoff) {
			continue
		}
		filtered = append(filtered, event)
	}
	proxyRec.recentTimeouts = append(filtered, transportHealthTimeoutEvent{instanceID: instanceID, at: now})
	if !proxyRec.lastResetAt.IsZero() && now.Sub(proxyRec.lastResetAt) < transportHealthProxyResetCooldown {
		return transportHealthDecision{}
	}
	uniqueInstances := make(map[int64]struct{}, len(proxyRec.recentTimeouts))
	for _, event := range proxyRec.recentTimeouts {
		if event.instanceID > 0 {
			uniqueInstances[event.instanceID] = struct{}{}
		}
	}
	if len(uniqueInstances) < 2 && len(proxyRec.recentTimeouts) < 3 {
		return transportHealthDecision{}
	}
	ids := make([]int64, 0, len(uniqueInstances))
	for id := range uniqueInstances {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return transportHealthDecision{
		resetProxy: true,
		reason:     fmt.Sprintf("proxy burst timeout detected: proxy=%s instances=%v events=%d", rec.proxyKey, ids, len(proxyRec.recentTimeouts)),
	}
}

func (s *transportHealthStore) noteInstanceReset(instanceIDs []int64, proxyKey string, now time.Time, reason string) {
	if s == nil {
		return
	}
	proxyKey = strings.TrimSpace(proxyKey)
	if proxyKey == "" {
		proxyKey = "direct"
	}
	reason = strings.TrimSpace(reason)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, instanceID := range instanceIDs {
		if instanceID <= 0 {
			continue
		}
		rec := s.ensureInstanceLocked(instanceID, proxyKey)
		if rec == nil {
			continue
		}
		rec.lastResetAt = now
		rec.lastResetReason = reason
		rec.resetCount++
	}
}

func (s *transportHealthStore) noteProxyReset(instanceIDs []int64, proxyKey string, now time.Time, reason string) {
	if s == nil {
		return
	}
	proxyKey = strings.TrimSpace(proxyKey)
	if proxyKey == "" {
		proxyKey = "direct"
	}
	reason = strings.TrimSpace(reason)
	s.mu.Lock()
	defer s.mu.Unlock()
	proxyRec := s.ensureProxyLocked(proxyKey)
	proxyRec.lastResetAt = now
	proxyRec.recentTimeouts = nil
	for _, instanceID := range instanceIDs {
		if instanceID <= 0 {
			continue
		}
		rec := s.ensureInstanceLocked(instanceID, proxyKey)
		if rec == nil {
			continue
		}
		rec.lastResetAt = now
		rec.lastResetReason = reason
		rec.resetCount++
	}
}

func (s *transportHealthStore) snapshot(instanceID int64, now time.Time) (TransportHealthSnapshot, bool) {
	if s == nil || instanceID <= 0 {
		return TransportHealthSnapshot{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.byInstance[instanceID]
	if rec == nil {
		return TransportHealthSnapshot{}, false
	}
	return buildTransportHealthSnapshot(rec, now), true
}

func (s *transportHealthStore) snapshots(instanceIDs []int64, now time.Time) map[int64]TransportHealthSnapshot {
	out := make(map[int64]TransportHealthSnapshot)
	if s == nil || len(instanceIDs) == 0 {
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, instanceID := range instanceIDs {
		if instanceID <= 0 {
			continue
		}
		rec := s.byInstance[instanceID]
		if rec == nil {
			continue
		}
		out[instanceID] = buildTransportHealthSnapshot(rec, now)
	}
	return out
}

func buildTransportHealthSnapshot(rec *transportHealthRecord, now time.Time) TransportHealthSnapshot {
	snap := TransportHealthSnapshot{
		InstanceID:          rec.instanceID,
		State:               transportHealthState(rec, now),
		Inflight:            rec.inflight,
		ConsecutiveErrors:   rec.consecutiveErrors,
		ConsecutiveTimeouts: rec.consecutiveTimeouts,
		ResetCount:          rec.resetCount,
		LastError:           rec.lastError,
		LastResetReason:     rec.lastResetReason,
	}
	if !rec.lastStartedAt.IsZero() {
		snap.LastStartedAt = rec.lastStartedAt.Unix()
	}
	if !rec.lastSuccessAt.IsZero() {
		snap.LastSuccessAt = rec.lastSuccessAt.Unix()
	}
	if !rec.lastErrorAt.IsZero() {
		snap.LastErrorAt = rec.lastErrorAt.Unix()
	}
	if !rec.lastTimeoutAt.IsZero() {
		snap.LastTimeoutAt = rec.lastTimeoutAt.Unix()
	}
	if !rec.lastResetAt.IsZero() {
		snap.LastResetAt = rec.lastResetAt.Unix()
	}
	return snap
}

func transportHealthState(rec *transportHealthRecord, now time.Time) string {
	if rec == nil {
		return "healthy"
	}
	if rec.inflight > 0 && !rec.lastStartedAt.IsZero() && now.Sub(rec.lastStartedAt) >= transportHealthStalledThreshold() {
		return "stalled"
	}
	if rec.consecutiveTimeouts > 0 {
		return "timeout"
	}
	if rec.consecutiveErrors > 0 {
		return "error"
	}
	if !rec.lastResetAt.IsZero() && (rec.lastSuccessAt.IsZero() || rec.lastResetAt.After(rec.lastSuccessAt)) {
		return "recovering"
	}
	return "healthy"
}

func (s *Service) GetTransportHealthSnapshot(instanceID int64) (TransportHealthSnapshot, bool) {
	if s == nil || s.transportHealth == nil {
		return TransportHealthSnapshot{}, false
	}
	return s.transportHealth.snapshot(instanceID, time.Now())
}

func (s *Service) GetTransportHealthSnapshots(instanceIDs []int64) map[int64]TransportHealthSnapshot {
	if s == nil || s.transportHealth == nil {
		return map[int64]TransportHealthSnapshot{}
	}
	return s.transportHealth.snapshots(instanceIDs, time.Now())
}

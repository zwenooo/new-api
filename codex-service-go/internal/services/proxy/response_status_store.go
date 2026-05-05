package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultResponseStatusMaxPerInstance = 2000
	defaultResponseStatusMaxTotal       = 2000
	defaultResponseStatusBodyLimit      = 32 << 10 // 32KB
)

type ResponseStatusItem struct {
	Status    int    `json:"status"`
	Body      string `json:"body"`
	Truncated bool   `json:"truncated,omitempty"`
	Count     int    `json:"count"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
}

type ResponseStatusDelta struct {
	InstanceID int64  `json:"instance_id"`
	Status     int    `json:"status"`
	BodyHash   string `json:"body_hash"`
	Body       string `json:"body"`
	Truncated  bool   `json:"truncated,omitempty"`
	CountDelta int    `json:"count_delta"`
	FirstSeen  int64  `json:"first_seen"`
	LastSeen   int64  `json:"last_seen"`
}

type responseStatusRecord struct {
	status    int
	body      string
	truncated bool
	count     int
	firstSeen time.Time
	lastSeen  time.Time
}

type responseStatusDeltaRecord struct {
	status    int
	body      string
	truncated bool
	count     int
	firstSeen time.Time
	lastSeen  time.Time
}

type responseStatusStore struct {
	mu             sync.RWMutex
	maxPerInstance int
	maxTotal       int
	bodyLimit      int
	byInst         map[int64]map[string]*responseStatusRecord
	pendingByInst  map[int64]map[string]*responseStatusDeltaRecord
}

func newResponseStatusStore() *responseStatusStore {
	return &responseStatusStore{
		maxPerInstance: defaultResponseStatusMaxPerInstance,
		maxTotal:       defaultResponseStatusMaxTotal,
		bodyLimit:      defaultResponseStatusBodyLimit,
		byInst:         make(map[int64]map[string]*responseStatusRecord),
		pendingByInst:  make(map[int64]map[string]*responseStatusDeltaRecord),
	}
}

func (s *responseStatusStore) record(instanceID int64, status int, body []byte, truncated bool) {
	if s == nil || instanceID < 0 || status <= 0 {
		return
	}

	bodyText, clipped := normalizeResponseStatusBody(body, s.bodyLimit)
	if clipped {
		truncated = true
	}
	key := responseStatusKey(status, bodyText)
	now := time.Now()

	s.mu.Lock()
	perInst := s.byInst[instanceID]
	if perInst == nil {
		perInst = make(map[string]*responseStatusRecord)
		s.byInst[instanceID] = perInst
	}
	rec := perInst[key]
	if rec == nil {
		rec = &responseStatusRecord{
			status:    status,
			body:      bodyText,
			truncated: truncated,
			count:     0,
			firstSeen: now,
			lastSeen:  now,
		}
		perInst[key] = rec
	}
	rec.count++
	rec.lastSeen = now
	if truncated {
		rec.truncated = true
	}

	pending := s.pendingByInst[instanceID]
	if pending == nil {
		pending = make(map[string]*responseStatusDeltaRecord)
		s.pendingByInst[instanceID] = pending
	}
	prec := pending[key]
	if prec == nil {
		prec = &responseStatusDeltaRecord{
			status:    status,
			body:      bodyText,
			truncated: truncated,
			count:     0,
			firstSeen: now,
			lastSeen:  now,
		}
		pending[key] = prec
	}
	prec.count++
	prec.lastSeen = now
	if truncated {
		prec.truncated = true
	}

	s.enforceLimitsLocked()
	s.mu.Unlock()
}

func (s *responseStatusStore) list(instanceID int64) []ResponseStatusItem {
	if s == nil || instanceID < 0 {
		return nil
	}
	s.mu.RLock()
	perInst := s.byInst[instanceID]
	if len(perInst) == 0 {
		s.mu.RUnlock()
		return nil
	}
	out := make([]ResponseStatusItem, 0, len(perInst))
	for _, rec := range perInst {
		if rec == nil {
			continue
		}
		out = append(out, ResponseStatusItem{
			Status:    rec.status,
			Body:      rec.body,
			Truncated: rec.truncated,
			Count:     rec.count,
			FirstSeen: rec.firstSeen.Unix(),
			LastSeen:  rec.lastSeen.Unix(),
		})
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeen == out[j].LastSeen {
			if out[i].Status == out[j].Status {
				return out[i].Body < out[j].Body
			}
			return out[i].Status < out[j].Status
		}
		return out[i].LastSeen > out[j].LastSeen
	})
	return out
}

func (s *responseStatusStore) drain(instanceID int64) []ResponseStatusDelta {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if instanceID >= 0 {
		perInst := s.pendingByInst[instanceID]
		if len(perInst) == 0 {
			delete(s.pendingByInst, instanceID)
			return nil
		}
		out := make([]ResponseStatusDelta, 0, len(perInst))
		for key, rec := range perInst {
			if rec == nil || rec.count <= 0 {
				continue
			}
			out = append(out, ResponseStatusDelta{
				InstanceID: instanceID,
				Status:     rec.status,
				BodyHash:   responseStatusHashFromKey(key),
				Body:       rec.body,
				Truncated:  rec.truncated,
				CountDelta: rec.count,
				FirstSeen:  rec.firstSeen.Unix(),
				LastSeen:   rec.lastSeen.Unix(),
			})
		}
		delete(s.pendingByInst, instanceID)
		return out
	}

	total := 0
	for instID, perInst := range s.pendingByInst {
		if instID < 0 || len(perInst) == 0 {
			continue
		}
		total += len(perInst)
	}
	if total == 0 {
		s.pendingByInst = make(map[int64]map[string]*responseStatusDeltaRecord)
		return nil
	}

	out := make([]ResponseStatusDelta, 0, total)
	for instID, perInst := range s.pendingByInst {
		if instID < 0 || len(perInst) == 0 {
			continue
		}
		for key, rec := range perInst {
			if rec == nil || rec.count <= 0 {
				continue
			}
			out = append(out, ResponseStatusDelta{
				InstanceID: instID,
				Status:     rec.status,
				BodyHash:   responseStatusHashFromKey(key),
				Body:       rec.body,
				Truncated:  rec.truncated,
				CountDelta: rec.count,
				FirstSeen:  rec.firstSeen.Unix(),
				LastSeen:   rec.lastSeen.Unix(),
			})
		}
	}
	s.pendingByInst = make(map[int64]map[string]*responseStatusDeltaRecord)
	return out
}

type responseStatusRef struct {
	instanceID int64
	key        string
	lastSeen   time.Time
}

func (s *responseStatusStore) enforceLimitsLocked() {
	if s == nil {
		return
	}
	maxPerInstance := sanitizePositiveLimit(s.maxPerInstance, defaultResponseStatusMaxPerInstance)
	maxTotal := sanitizePositiveLimit(s.maxTotal, defaultResponseStatusMaxTotal)
	if maxTotal < maxPerInstance {
		maxTotal = maxPerInstance
	}

	total := 0
	for instanceID, perInst := range s.byInst {
		if len(perInst) == 0 {
			delete(s.byInst, instanceID)
			continue
		}
		if len(perInst) > maxPerInstance {
			refs := make([]responseStatusRef, 0, len(perInst))
			for key, rec := range perInst {
				if rec == nil {
					delete(perInst, key)
					continue
				}
				refs = append(refs, responseStatusRef{instanceID: instanceID, key: key, lastSeen: rec.lastSeen})
			}
			if len(refs) > maxPerInstance {
				sort.Slice(refs, func(i, j int) bool { return refs[i].lastSeen.Before(refs[j].lastSeen) })
				overflow := len(refs) - maxPerInstance
				for i := 0; i < overflow; i++ {
					delete(perInst, refs[i].key)
				}
			}
		}
		if len(perInst) == 0 {
			delete(s.byInst, instanceID)
			continue
		}
		total += len(perInst)
	}
	if total <= maxTotal {
		return
	}

	all := make([]responseStatusRef, 0, total)
	for instanceID, perInst := range s.byInst {
		for key, rec := range perInst {
			if rec == nil {
				delete(perInst, key)
				continue
			}
			all = append(all, responseStatusRef{instanceID: instanceID, key: key, lastSeen: rec.lastSeen})
		}
		if len(perInst) == 0 {
			delete(s.byInst, instanceID)
		}
	}
	if len(all) <= maxTotal {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].lastSeen.Before(all[j].lastSeen) })
	overflow := len(all) - maxTotal
	for i := 0; i < overflow; i++ {
		ref := all[i]
		perInst := s.byInst[ref.instanceID]
		if perInst == nil {
			continue
		}
		delete(perInst, ref.key)
		if len(perInst) == 0 {
			delete(s.byInst, ref.instanceID)
		}
	}
}

func responseStatusKey(status int, body string) string {
	sum := sha256.Sum256([]byte(body))
	return fmt.Sprintf("%d:%s", status, hex.EncodeToString(sum[:]))
}

func responseStatusHashFromKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if idx := strings.IndexByte(key, ':'); idx >= 0 && idx+1 < len(key) {
		return key[idx+1:]
	}
	return ""
}

var (
	responseStatusRequestIDJSONRe = regexp.MustCompile(`(?i)"request[_-]?id"\s*:\s*"[^"]*"`)
	responseStatusRequestIDEqRe   = regexp.MustCompile(`(?i)request[_-]?id=([a-z0-9_-]+)`)
)

func normalizeResponseStatusBody(body []byte, limit int) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	if limit <= 0 {
		limit = defaultResponseStatusBodyLimit
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", false
	}
	clipped := false
	if len(text) > limit {
		text = text[:limit]
		clipped = true
	}

	// Ignore request ID for de-duplication by normalizing it in the stored body.
	text = responseStatusRequestIDJSONRe.ReplaceAllString(text, `"request_id":"***"`)
	text = responseStatusRequestIDEqRe.ReplaceAllString(text, `request_id=***`)

	return strings.TrimSpace(text), clipped
}

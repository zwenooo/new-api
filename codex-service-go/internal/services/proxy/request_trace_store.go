package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultRequestTraceRetention      = 15 * time.Minute
	defaultRequestTraceBodyLimit      = 512 << 10 // 512KB
	requestTraceHeaderLimitBytes      = 32 << 10  // 32KB
	requestTraceHeaderValueLimitBytes = 2048      // 2KB
	requestTraceFieldDepth            = 2
	defaultRequestTraceMaxPerInstance = 1000
	defaultRequestTraceMaxTotal       = 10000
	defaultRequestTraceMaxEvents      = 200
)

type RequestTraceSummary struct {
	RequestID        string   `json:"request_id"`
	RequestMethod    string   `json:"request_method,omitempty"`
	RequestTarget    string   `json:"request_target,omitempty"`
	RequestPath      string   `json:"request_path,omitempty"`
	RequestUserAgent string   `json:"request_user_agent,omitempty"`
	RequestHeaders   http.Header `json:"request_headers,omitempty"`
	ResponseHeaders  http.Header `json:"response_headers,omitempty"`
	RequestFields    []string `json:"request_fields,omitempty"`
	ResponseFields   []string `json:"response_fields,omitempty"`
	ResponseEvents   []string `json:"response_events,omitempty"`
	ResponseStatus   int      `json:"response_status"`
	IsStream         bool     `json:"is_stream"`
	DeltaSuppressed  int      `json:"delta_suppressed,omitempty"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

type RequestTraceEvent struct {
	Event string `json:"event"`
	Data  string `json:"data,omitempty"`
	At    int64  `json:"at"`
}

type RequestTraceDetail struct {
	Summary               *RequestTraceSummary `json:"summary"`
	RequestBody           string               `json:"request_body,omitempty"`
	RequestBodyTruncated  bool                 `json:"request_body_truncated,omitempty"`
	ResponseBody          string               `json:"response_body,omitempty"`
	ResponseBodyTruncated bool                 `json:"response_body_truncated,omitempty"`
	StreamEvents          []RequestTraceEvent  `json:"stream_events,omitempty"`
	StreamEventsTruncated bool                 `json:"stream_events_truncated,omitempty"`
}

type requestTraceRecord struct {
	requestID        string
	requestMethod    string
	requestTarget    string
	requestPath      string
	requestUserAgent string
	requestHeaders   http.Header
	responseHeaders  http.Header
	responseStatus   int
	isStream         bool
	createdAt        time.Time
	updatedAt        time.Time
	requestFields    map[string]struct{}
	responseFields   map[string]struct{}
	responseEvents   map[string]struct{}

	requestBody          []byte
	requestBodyTruncated bool

	responseBody          []byte
	responseBodyTruncated bool

	streamEvents          []RequestTraceEvent
	streamEventsTruncated bool

	deltaSuppressed int
}

type requestTraceStore struct {
	mu             sync.RWMutex
	retention      time.Duration
	maxPerInstance int
	maxTotal       int
	byInst         map[int64]map[string]*requestTraceRecord
}

func newRequestTraceStore(retention time.Duration) *requestTraceStore {
	if retention <= 0 {
		retention = defaultRequestTraceRetention
	}
	return &requestTraceStore{
		retention:      retention,
		maxPerInstance: defaultRequestTraceMaxPerInstance,
		maxTotal:       defaultRequestTraceMaxTotal,
		byInst:         make(map[int64]map[string]*requestTraceRecord),
	}
}

func cloneLimitedHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}

	out := http.Header{}
	used := 0

	for k, vals := range src {
		key := http.CanonicalHeaderKey(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		for _, raw := range vals {
			value := raw
			if strings.TrimSpace(value) == "" {
				continue
			}
			if limit := requestTraceHeaderValueLimitBytes; limit > 0 && len(value) > limit {
				if limit <= 3 {
					value = "..."
				} else {
					value = value[:limit-3] + "..."
				}
			}

			bytesNeeded := len(key) + len(value)
			if used+bytesNeeded > requestTraceHeaderLimitBytes {
				return out
			}
			out.Add(key, value)
			used += bytesNeeded
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *requestTraceStore) setRetention(retention time.Duration) {
	if s == nil {
		return
	}
	if retention <= 0 {
		retention = defaultRequestTraceRetention
	}
	now := time.Now()
	s.mu.Lock()
	s.retention = retention
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) setLimits(maxPerInstance int, maxTotal int) {
	if s == nil {
		return
	}
	maxPerInstance = sanitizePositiveLimit(maxPerInstance, defaultRequestTraceMaxPerInstance)
	maxTotal = sanitizePositiveLimit(maxTotal, defaultRequestTraceMaxTotal)
	if maxTotal < maxPerInstance {
		maxTotal = maxPerInstance
	}
	now := time.Now()
	s.mu.Lock()
	s.maxPerInstance = maxPerInstance
	s.maxTotal = maxTotal
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) start(instanceID int64, requestID, method, target string, headers http.Header, requestBody []byte, requestBodyTruncated bool) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" {
		return
	}
	now := time.Now()
	path := requestPathFromTarget(target)
	requestFields := extractJSONFieldPaths(requestBody)

	s.mu.Lock()
	rec := s.ensureRecordLocked(instanceID, requestID, now)
	rec.requestMethod = strings.ToUpper(strings.TrimSpace(method))
	if strings.TrimSpace(target) != "" {
		rec.requestTarget = strings.TrimSpace(target)
	}
	if path != "" {
		rec.requestPath = path
	}
	if headers != nil {
		rec.requestHeaders = cloneLimitedHeader(headers)
	}
	if ua := strings.TrimSpace(headerFirst(headers, "User-Agent")); ua != "" {
		rec.requestUserAgent = ua
	}
	if len(requestBody) > 0 {
		rec.requestBody = append(rec.requestBody[:0], requestBody...)
		rec.requestBodyTruncated = requestBodyTruncated
	}
	mergeFieldSet(rec.requestFields, requestFields)
	rec.updatedAt = now
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) updateResponseMeta(instanceID int64, requestID string, status int, isStream bool, headers http.Header) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	rec := s.ensureRecordLocked(instanceID, requestID, now)
	rec.responseStatus = status
	rec.isStream = isStream
	if headers != nil {
		rec.responseHeaders = cloneLimitedHeader(headers)
	}
	rec.updatedAt = now
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) mergeResponseData(instanceID int64, requestID string, payload []byte, fallbackEvent string, truncated bool) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" || len(payload) == 0 {
		return
	}
	fields, events := extractResponseFieldsAndEvents(payload, fallbackEvent)
	now := time.Now()
	s.mu.Lock()
	rec := s.ensureRecordLocked(instanceID, requestID, now)
	mergeFieldSet(rec.responseFields, fields)
	mergeFieldSet(rec.responseEvents, events)
	rec.responseBody = append(rec.responseBody[:0], payload...)
	rec.responseBodyTruncated = truncated
	rec.updatedAt = now
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) mergeResponseSummary(instanceID int64, requestID string, fields []string, events []string) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	rec := s.ensureRecordLocked(instanceID, requestID, now)
	mergeFieldSet(rec.responseFields, fields)
	mergeFieldSet(rec.responseEvents, events)
	rec.updatedAt = now
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) finishStream(instanceID int64, requestID string, fields []string, events []string, streamEvents []RequestTraceEvent, streamEventsTruncated bool, deltaSuppressed int) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	rec := s.ensureRecordLocked(instanceID, requestID, now)
	mergeFieldSet(rec.responseFields, fields)
	mergeFieldSet(rec.responseEvents, events)
	if len(streamEvents) > 0 {
		rec.streamEvents = append(rec.streamEvents[:0], streamEvents...)
		rec.streamEventsTruncated = streamEventsTruncated
	}
	rec.deltaSuppressed = deltaSuppressed
	rec.updatedAt = now
	s.pruneLocked(now)
	s.mu.Unlock()
}

func (s *requestTraceStore) getSummary(instanceID int64, requestID string) (*RequestTraceSummary, bool) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" {
		return nil, false
	}
	now := time.Now()

	s.mu.Lock()
	s.pruneLocked(now)
	instRecords, ok := s.byInst[instanceID]
	if !ok || instRecords == nil {
		s.mu.Unlock()
		return nil, false
	}
	rec, ok := instRecords[requestID]
	if !ok || rec == nil {
		s.mu.Unlock()
		return nil, false
	}

	summary := &RequestTraceSummary{
		RequestID:        requestID,
		RequestMethod:    rec.requestMethod,
		RequestTarget:    rec.requestTarget,
		RequestPath:      rec.requestPath,
		RequestUserAgent: rec.requestUserAgent,
		RequestFields:    sortedFieldSet(rec.requestFields),
		ResponseFields:   sortedFieldSet(rec.responseFields),
		ResponseEvents:   sortedFieldSet(rec.responseEvents),
		ResponseStatus:   rec.responseStatus,
		IsStream:         rec.isStream,
		DeltaSuppressed:  rec.deltaSuppressed,
		CreatedAt:        rec.createdAt.Unix(),
		UpdatedAt:        rec.updatedAt.Unix(),
	}
	s.mu.Unlock()
	return summary, true
}

func (s *requestTraceStore) getDetail(instanceID int64, requestID string) (*RequestTraceDetail, bool) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || instanceID <= 0 || requestID == "" {
		return nil, false
	}
	now := time.Now()

	s.mu.Lock()
	s.pruneLocked(now)
	instRecords, ok := s.byInst[instanceID]
	if !ok || instRecords == nil {
		s.mu.Unlock()
		return nil, false
	}
	rec, ok := instRecords[requestID]
	if !ok || rec == nil {
		s.mu.Unlock()
		return nil, false
	}

	summary := &RequestTraceSummary{
		RequestID:        requestID,
		RequestMethod:    rec.requestMethod,
		RequestTarget:    rec.requestTarget,
		RequestPath:      rec.requestPath,
		RequestUserAgent: rec.requestUserAgent,
		RequestHeaders:   cloneLimitedHeader(rec.requestHeaders),
		ResponseHeaders:  cloneLimitedHeader(rec.responseHeaders),
		RequestFields:    sortedFieldSet(rec.requestFields),
		ResponseFields:   sortedFieldSet(rec.responseFields),
		ResponseEvents:   sortedFieldSet(rec.responseEvents),
		ResponseStatus:   rec.responseStatus,
		IsStream:         rec.isStream,
		DeltaSuppressed:  rec.deltaSuppressed,
		CreatedAt:        rec.createdAt.Unix(),
		UpdatedAt:        rec.updatedAt.Unix(),
	}

	detail := &RequestTraceDetail{
		Summary:               summary,
		RequestBody:           strings.TrimSpace(string(rec.requestBody)),
		RequestBodyTruncated:  rec.requestBodyTruncated,
		ResponseBody:          strings.TrimSpace(string(rec.responseBody)),
		ResponseBodyTruncated: rec.responseBodyTruncated,
		StreamEvents:          append([]RequestTraceEvent(nil), rec.streamEvents...),
		StreamEventsTruncated: rec.streamEventsTruncated,
	}
	s.mu.Unlock()
	return detail, true
}

func (s *requestTraceStore) ensureRecordLocked(instanceID int64, requestID string, now time.Time) *requestTraceRecord {
	instRecords := s.byInst[instanceID]
	if instRecords == nil {
		instRecords = make(map[string]*requestTraceRecord)
		s.byInst[instanceID] = instRecords
	}
	rec := instRecords[requestID]
	if rec != nil {
		return rec
	}
	rec = &requestTraceRecord{
		requestID:      requestID,
		createdAt:      now,
		updatedAt:      now,
		requestFields:  make(map[string]struct{}),
		responseFields: make(map[string]struct{}),
		responseEvents: make(map[string]struct{}),
	}
	instRecords[requestID] = rec
	return rec
}

func (s *requestTraceStore) pruneLocked(now time.Time) {
	if s == nil {
		return
	}
	retention := s.retention
	if retention <= 0 {
		retention = defaultRequestTraceRetention
	}
	cutoff := now.Add(-retention)
	for instanceID, records := range s.byInst {
		for requestID, rec := range records {
			if rec == nil || rec.updatedAt.Before(cutoff) {
				delete(records, requestID)
			}
		}
		if len(records) == 0 {
			delete(s.byInst, instanceID)
		}
	}
	s.enforceCountLimitLocked()
}

type traceRecordRef struct {
	instanceID int64
	requestID  string
	updatedAt  time.Time
}

func (s *requestTraceStore) enforceCountLimitLocked() {
	if s == nil {
		return
	}
	maxPerInstance := sanitizePositiveLimit(s.maxPerInstance, defaultRequestTraceMaxPerInstance)
	maxTotal := sanitizePositiveLimit(s.maxTotal, defaultRequestTraceMaxTotal)
	if maxTotal < maxPerInstance {
		maxTotal = maxPerInstance
	}

	totalCount := 0
	for instanceID, records := range s.byInst {
		if len(records) == 0 {
			delete(s.byInst, instanceID)
			continue
		}

		if len(records) > maxPerInstance {
			refs := make([]traceRecordRef, 0, len(records))
			for requestID, rec := range records {
				if rec == nil {
					delete(records, requestID)
					continue
				}
				refs = append(refs, traceRecordRef{instanceID: instanceID, requestID: requestID, updatedAt: rec.updatedAt})
			}
			if len(refs) > maxPerInstance {
				sortTraceRecordRefs(refs)
				overflow := len(refs) - maxPerInstance
				for i := 0; i < overflow; i++ {
					delete(records, refs[i].requestID)
				}
			}
		}

		if len(records) == 0 {
			delete(s.byInst, instanceID)
			continue
		}
		totalCount += len(records)
	}

	if totalCount <= maxTotal {
		return
	}

	all := make([]traceRecordRef, 0, totalCount)
	for instanceID, records := range s.byInst {
		for requestID, rec := range records {
			if rec == nil {
				delete(records, requestID)
				continue
			}
			all = append(all, traceRecordRef{instanceID: instanceID, requestID: requestID, updatedAt: rec.updatedAt})
		}
		if len(records) == 0 {
			delete(s.byInst, instanceID)
		}
	}
	if len(all) <= maxTotal {
		return
	}

	sortTraceRecordRefs(all)
	overflow := len(all) - maxTotal
	for i := 0; i < overflow; i++ {
		ref := all[i]
		records := s.byInst[ref.instanceID]
		if records == nil {
			continue
		}
		delete(records, ref.requestID)
		if len(records) == 0 {
			delete(s.byInst, ref.instanceID)
		}
	}
}

func sortTraceRecordRefs(refs []traceRecordRef) {
	if len(refs) < 2 {
		return
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].updatedAt.Equal(refs[j].updatedAt) {
			if refs[i].instanceID == refs[j].instanceID {
				return refs[i].requestID < refs[j].requestID
			}
			return refs[i].instanceID < refs[j].instanceID
		}
		return refs[i].updatedAt.Before(refs[j].updatedAt)
	})
}

func sanitizePositiveLimit(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func requestPathFromTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	if p := strings.TrimSpace(u.Path); p != "" {
		return p
	}
	return target
}

func extractJSONFieldPaths(payload []byte) []string {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil
	}
	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil
	}
	return collectJSONFieldPaths(raw)
}

func extractResponseFieldsAndEvents(payload []byte, fallbackEvent string) ([]string, []string) {
	fields, eventType := extractResponseFieldsAndEventType(payload, fallbackEvent)
	if eventType == "response.output_text.delta" {
		return nil, nil
	}
	if eventType == "" {
		return fields, nil
	}
	return fields, []string{eventType}
}

func extractResponseFieldsAndEventType(payload []byte, fallbackEvent string) ([]string, string) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return nil, ""
	}
	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, ""
	}

	fields := collectJSONFieldPaths(raw)
	eventType := normalizeRequestTraceEventType(fallbackEvent)
	if eventType == "" {
		if m, ok := raw.(map[string]any); ok {
			if v, ok := m["type"].(string); ok {
				eventType = normalizeRequestTraceEventType(v)
			}
		}
	}
	return fields, eventType
}

func collectJSONFieldPaths(raw any) []string {
	set := make(map[string]struct{})
	collectJSONFieldPathsRecursive(raw, "", 0, requestTraceFieldDepth, set)
	return sortedFieldSet(set)
}

func collectJSONFieldPathsRecursive(raw any, prefix string, depth int, maxDepth int, set map[string]struct{}) {
	if set == nil || depth > maxDepth {
		return
	}
	switch v := raw.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			if key = strings.TrimSpace(key); key != "" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			set[path] = struct{}{}
			if depth < maxDepth {
				collectJSONFieldPathsRecursive(v[key], path, depth+1, maxDepth, set)
			}
		}
	case []any:
		if len(v) == 0 {
			return
		}
		marker := "[]"
		if prefix != "" {
			marker = prefix + "[]"
		}
		set[marker] = struct{}{}
		if depth >= maxDepth {
			return
		}
		limit := len(v)
		if limit > 3 {
			limit = 3
		}
		for i := 0; i < limit; i++ {
			collectJSONFieldPathsRecursive(v[i], marker, depth+1, maxDepth, set)
		}
	}
}

func mergeFieldSet(target map[string]struct{}, values []string) {
	if target == nil || len(values) == 0 {
		return
	}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			target[value] = struct{}{}
		}
	}
}

func sortedFieldSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	arr := make([]string, 0, len(values))
	for value := range values {
		if value = strings.TrimSpace(value); value != "" {
			arr = append(arr, value)
		}
	}
	sort.Strings(arr)
	return arr
}

func normalizeRequestTraceEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if strings.HasPrefix(strings.ToLower(eventType), "event:") {
		eventType = strings.TrimSpace(eventType[len("event:"):])
	}
	return eventType
}

type requestTraceJSONReadCloser struct {
	rc        io.ReadCloser
	onPayload func([]byte, bool)
	limit     int
	buf       bytes.Buffer
	captured  int
	truncated bool
}

func newRequestTraceJSONReadCloser(rc io.ReadCloser, limit int, onPayload func([]byte, bool)) io.ReadCloser {
	if rc == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultRequestTraceBodyLimit
	}
	return &requestTraceJSONReadCloser{
		rc:        rc,
		onPayload: onPayload,
		limit:     limit,
	}
}

func (t *requestTraceJSONReadCloser) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 && t.captured < t.limit {
		remaining := t.limit - t.captured
		if remaining > n {
			remaining = n
		}
		_, _ = t.buf.Write(p[:remaining])
		t.captured += remaining
		if remaining < n {
			t.truncated = true
		}
	} else if n > 0 && t.captured >= t.limit {
		t.truncated = true
	}
	return n, err
}

func (t *requestTraceJSONReadCloser) Close() error {
	if t.onPayload != nil && t.buf.Len() > 0 {
		t.onPayload(t.buf.Bytes(), t.truncated)
	}
	return t.rc.Close()
}

type requestTraceSSEReadCloser struct {
	rc       io.ReadCloser
	onDetail func(fields []string, events []string, records []RequestTraceEvent, recordsTruncated bool, deltaSuppressed int)
	pending  []byte

	currentEvent string
	fieldSet     map[string]struct{}
	eventSet     map[string]struct{}

	records          []RequestTraceEvent
	recordsCaptured  int
	recordsLimit     int
	recordsTruncated bool

	deltaSuppressed      int
	deltaPlaceholderDone bool
}

func newRequestTraceSSEReadCloser(rc io.ReadCloser, limit int, onDetail func(fields []string, events []string, records []RequestTraceEvent, recordsTruncated bool, deltaSuppressed int)) io.ReadCloser {
	if rc == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultRequestTraceBodyLimit
	}
	return &requestTraceSSEReadCloser{
		rc:           rc,
		onDetail:     onDetail,
		fieldSet:     make(map[string]struct{}),
		eventSet:     make(map[string]struct{}),
		recordsLimit: limit,
	}
}

func (t *requestTraceSSEReadCloser) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 {
		t.consume(p[:n])
	}
	return n, err
}

func (t *requestTraceSSEReadCloser) Close() error {
	if len(t.pending) > 0 {
		t.processLine(t.pending)
		t.pending = nil
	}
	if t.onDetail != nil {
		t.onDetail(sortedFieldSet(t.fieldSet), sortedFieldSet(t.eventSet), t.records, t.recordsTruncated, t.deltaSuppressed)
	}
	return t.rc.Close()
}

func (t *requestTraceSSEReadCloser) consume(chunk []byte) {
	t.pending = append(t.pending, chunk...)
	for {
		idx := bytes.IndexByte(t.pending, '\n')
		if idx < 0 {
			return
		}
		line := t.pending[:idx+1]
		t.pending = t.pending[idx+1:]
		t.processLine(line)
	}
}

func (t *requestTraceSSEReadCloser) processLine(line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		t.currentEvent = ""
		return
	}
	if bytes.HasPrefix(trimmed, []byte("event:")) {
		t.currentEvent = normalizeRequestTraceEventType(string(trimmed))
		return
	}
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return
	}
	payload := bytes.TrimSpace(trimmed[len("data:"):])
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}

	fields, eventType := extractResponseFieldsAndEventType(payload, t.currentEvent)
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		mergeFieldSet(t.fieldSet, fields)
		return
	}

	if eventType == "response.output_text.delta" {
		t.deltaSuppressed++
		if !t.deltaPlaceholderDone {
			t.deltaPlaceholderDone = true
			t.addRecord(RequestTraceEvent{Event: eventType, Data: "...", At: time.Now().Unix()})
		}
		return
	}

	mergeFieldSet(t.fieldSet, fields)
	t.eventSet[eventType] = struct{}{}
	t.addRecord(RequestTraceEvent{Event: eventType, Data: string(payload), At: time.Now().Unix()})
}

func (t *requestTraceSSEReadCloser) addRecord(record RequestTraceEvent) {
	if t == nil {
		return
	}
	if record.Event = strings.TrimSpace(record.Event); record.Event == "" {
		return
	}
	if t.recordsTruncated {
		return
	}
	if len(t.records) >= defaultRequestTraceMaxEvents {
		t.recordsTruncated = true
		return
	}
	if record.Data != "" {
		remaining := t.recordsLimit - t.recordsCaptured
		if remaining <= 0 {
			t.recordsTruncated = true
			return
		}
		if len(record.Data) > remaining {
			if remaining <= 3 {
				t.recordsTruncated = true
				return
			}
			record.Data = record.Data[:remaining-3] + "..."
			t.recordsTruncated = true
		}
		t.recordsCaptured += len(record.Data)
	}
	t.records = append(t.records, record)
}

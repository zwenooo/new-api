package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"one-api/common"
	"one-api/logger"
	"one-api/model"
)

type requestTraceCtxKey struct{}

type requestTraceManager struct {
	enabled atomic.Bool

	spoolDir string

	lastSyncErr atomic.Value // string
}

var globalRequestTraceManager requestTraceManager

const (
	requestTraceEnabledOpt          = "request_trace.enabled"
	requestTraceRetentionMinutesOpt = "request_trace.retention_minutes"
	// Legacy days-based retention option (deprecated; kept for backward compatibility).
	requestTraceRetentionDaysOpt = "request_trace.retention_days"
)

func InitRequestTrace() {
	m := &globalRequestTraceManager

	m.spoolDir = strings.TrimSpace(common.RequestTraceSpoolDir)

	if m.spoolDir == "" {
		// This should never happen because common.initRequestTraceEnv sets a default.
		logger.LogError(context.Background(), "[request_trace] REQUEST_TRACE_SPOOL_DIR is empty")
	}
	if m.spoolDir != "" {
		if err := os.MkdirAll(m.spoolDir, 0o755); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("[request_trace] mkdir spool dir failed: dir=%s err=%v", m.spoolDir, err))
		}
	}

	// Apply admin-configured option (takes effect immediately on this node and
	// will be synced by the existing option sync loop across nodes).
	if err := SyncRequestTraceFromOptions(); err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("[request_trace] sync from options failed: %v", err))
	}

	// Lightweight watcher: options are updated in-process immediately, and
	// across nodes via SyncOptions. Keep request_trace.enabled hot-reloadable.
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_ = SyncRequestTraceFromOptions()
		}
	}()

	StartRequestTraceCleanup()
}

func requestTraceEnabled() bool {
	return globalRequestTraceManager.enabled.Load()
}

func requestTraceSessionFromContext(ctx context.Context) (*requestTraceSession, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(requestTraceCtxKey{})
	if v == nil {
		return nil, false
	}
	sess, ok := v.(*requestTraceSession)
	return sess, ok && sess != nil
}

func WithRequestTraceSession(ctx context.Context, sess *requestTraceSession) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sess == nil {
		return ctx
	}
	return context.WithValue(ctx, requestTraceCtxKey{}, sess)
}

func requestTraceDesiredEnabled() (bool, error) {
	raw := strings.TrimSpace(readOption(requestTraceEnabledOpt))
	if raw == "" {
		return common.RequestTraceEnabled, nil
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true/false", requestTraceEnabledOpt)
	}
	return enabled, nil
}

func requestTraceDesiredRetentionMinutes() (int, error) {
	// New canonical option: minutes.
	raw := strings.TrimSpace(readOption(requestTraceRetentionMinutesOpt))
	if raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes < 0 {
			return 0, fmt.Errorf("%s must be a non-negative integer", requestTraceRetentionMinutesOpt)
		}
		return minutes, nil
	}

	// Backward compatibility: days-based option.
	rawDays := strings.TrimSpace(readOption(requestTraceRetentionDaysOpt))
	if rawDays != "" {
		days, err := strconv.Atoi(rawDays)
		if err != nil || days < 0 {
			return 0, fmt.Errorf("%s must be a non-negative integer", requestTraceRetentionDaysOpt)
		}
		if days == 0 {
			return 0, nil
		}
		return days * 24 * 60, nil
	}

	// Env default.
	return common.RequestTraceRetentionMinutes, nil
}

func (m *requestTraceManager) setLastSyncErr(err error) {
	if m == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = strings.TrimSpace(err.Error())
	}
	m.lastSyncErr.Store(msg)
}

func (m *requestTraceManager) ensureEnabled(enabled bool) error {
	if m == nil {
		return nil
	}
	if enabled {
		spoolDir := strings.TrimSpace(m.spoolDir)
		if spoolDir == "" {
			spoolDir = strings.TrimSpace(common.RequestTraceSpoolDir)
			m.spoolDir = spoolDir
		}
		if spoolDir == "" {
			return errors.New("REQUEST_TRACE_SPOOL_DIR is empty")
		}
		if err := os.MkdirAll(spoolDir, 0o755); err != nil {
			return fmt.Errorf("mkdir spool dir failed: dir=%s err=%w", spoolDir, err)
		}
		m.enabled.Store(true)
		return nil
	}
	m.enabled.Store(false)
	return nil
}

func SyncRequestTraceFromOptions() error {
	m := &globalRequestTraceManager
	desired, err := requestTraceDesiredEnabled()
	if err != nil {
		m.setLastSyncErr(err)
		return err
	}

	current := m.enabled.Load()
	if desired == current {
		m.setLastSyncErr(nil)
		return nil
	}

	if err := m.ensureEnabled(desired); err != nil {
		m.setLastSyncErr(err)
		return err
	}
	m.setLastSyncErr(nil)
	return nil
}

type requestTraceHandle struct {
	sess *requestTraceSession
	node *requestTraceNodeBuilder

	reqBodySpool  *spoolFile
	respBodySpool *spoolFile
}

func shouldCaptureRequestBody(req *http.Request) bool {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return false
	}
	if req.ContentLength > 0 {
		return true
	}
	if strings.TrimSpace(req.Header.Get("Transfer-Encoding")) != "" {
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(req.Method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

type teeGinResponseWriter struct {
	gin.ResponseWriter
	spool *spoolFile
}

func (w *teeGinResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 && w.spool != nil {
		_, _ = w.spool.Write(p[:n])
	}
	return n, err
}

func (w *teeGinResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 && w.spool != nil {
		_, _ = w.spool.Write([]byte(s[:n]))
	}
	return n, err
}

func BeginGinRequestTrace(c *gin.Context) *requestTraceHandle {
	if c == nil || c.Request == nil {
		return nil
	}
	if !requestTraceEnabled() {
		return nil
	}

	requestID := strings.TrimSpace(c.GetString(common.RequestIdKey))
	if requestID == "" {
		return nil
	}

	sess := newRequestTraceSession(requestID)
	if sess == nil {
		return nil
	}

	// Attach to request context so outbound RoundTrippers can see it.
	c.Request = c.Request.WithContext(WithRequestTraceSession(c.Request.Context(), sess))

	nodeBase := sess.prefix + "/new-api/http"
	node := &requestTraceNodeBuilder{
		service:     "new-api",
		kind:        "http",
		seq:         0,
		startedAtMs: sess.startMs,
		reqMethod:   strings.TrimSpace(c.Request.Method),
		reqPath: func() string {
			if c.Request.URL == nil {
				return ""
			}
			return strings.TrimSpace(c.Request.URL.Path)
		}(),
		reqURL: func() string {
			if c.Request.URL == nil {
				return ""
			}
			return c.Request.URL.String()
		}(),
	}

	// Request headers
	{
		key := nodeBase + "/request_headers.json"
		size, err := writeJSONSpool(key, c.Request.Header)
		node.reqHeadersKey = key
		node.reqHeadersSize = size
		if err != nil {
			node.errText = coalesceErr(node.errText, fmt.Sprintf("write request headers failed: %v", err))
		}
	}

	// Request body (streaming tee)
	var reqSpool *spoolFile
	if shouldCaptureRequestBody(c.Request) {
		key := nodeBase + "/request_body.bin"
		spool, err := newSpoolFile(key)
		if err != nil {
			node.errText = coalesceErr(node.errText, fmt.Sprintf("open request body spool failed: %v", err))
		} else {
			reqSpool = spool
			node.reqBodyKey = key
			node.reqBodySpool = spool
			c.Request.Body = &teeReadCloser{rc: c.Request.Body, spool: spool}
		}
	}

	// Response body (streaming tee)
	var respSpool *spoolFile
	{
		key := nodeBase + "/response_body.bin"
		spool, err := newSpoolFile(key)
		if err != nil {
			node.errText = coalesceErr(node.errText, fmt.Sprintf("open response body spool failed: %v", err))
		} else {
			respSpool = spool
			node.respBodyKey = key
			node.respBodySpool = spool
			c.Writer = &teeGinResponseWriter{ResponseWriter: c.Writer, spool: spool}
		}
	}

	sess.httpNode = node

	return &requestTraceHandle{
		sess:          sess,
		node:          node,
		reqBodySpool:  reqSpool,
		respBodySpool: respSpool,
	}
}

func EndGinRequestTrace(handle *requestTraceHandle, c *gin.Context) {
	if handle == nil || handle.sess == nil || handle.node == nil || c == nil {
		return
	}
	status := 0
	if c.Writer != nil {
		status = c.Writer.Status()
	}

	handle.node.endedAtMs = time.Now().UnixMilli()

	// Response headers
	{
		key := handle.sess.prefix + "/new-api/http/response_headers.json"
		size, err := writeJSONSpool(key, c.Writer.Header())
		handle.node.respHeadersKey = key
		handle.node.respHeadersSize = size
		if err != nil {
			handle.node.errText = coalesceErr(handle.node.errText, fmt.Sprintf("write response headers failed: %v", err))
		}
	}

	// Finalize body spools (idempotent).
	if handle.reqBodySpool != nil {
		_ = handle.reqBodySpool.Close()
	}
	if handle.respBodySpool != nil {
		_ = handle.respBodySpool.Close()
	}

	// Also finalize any upstream spools to avoid leaving *.partial behind.
	handle.sess.mu.Lock()
	upstreams := append([]*requestTraceNodeBuilder(nil), handle.sess.upstreams...)
	handle.sess.mu.Unlock()
	for _, u := range upstreams {
		if u == nil {
			continue
		}
		if u.reqBodySpool != nil {
			_ = u.reqBodySpool.Close()
		}
		if u.respBodySpool != nil {
			_ = u.respBodySpool.Close()
		}
	}

	handle.sess.persist(c.Request.Context(), c.Request, status)
}

type spoolFile struct {
	key         string
	partialPath string
	finalPath   string
	f           *os.File
	written     int64
	closed      atomic.Bool
}

func (s *spoolFile) Write(p []byte) (int, error) {
	if s == nil || s.f == nil {
		return 0, errors.New("spool writer is nil")
	}
	n, err := s.f.Write(p)
	if n > 0 {
		atomic.AddInt64(&s.written, int64(n))
	}
	return n, err
}

func (s *spoolFile) BytesWritten() int64 {
	if s == nil {
		return 0
	}
	return atomic.LoadInt64(&s.written)
}

func (s *spoolFile) Close() error {
	if s == nil {
		return nil
	}
	if s.closed.Swap(true) {
		return nil
	}
	var err1 error
	if s.f != nil {
		err1 = s.f.Close()
	}
	if err1 != nil {
		return err1
	}
	if s.partialPath == "" || s.finalPath == "" {
		return errors.New("spool writer paths are empty")
	}
	if err := os.Rename(s.partialPath, s.finalPath); err != nil {
		return err
	}
	return nil
}

func newSpoolFile(key string) (*spoolFile, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("spool key is empty")
	}
	if strings.Contains(key, "..") {
		return nil, fmt.Errorf("invalid spool key: %q", key)
	}
	root := strings.TrimSpace(globalRequestTraceManager.spoolDir)
	if root == "" {
		root = strings.TrimSpace(common.RequestTraceSpoolDir)
	}
	if root == "" {
		return nil, errors.New("spool root is empty")
	}

	finalPath := filepath.Join(root, filepath.FromSlash(key))
	partialPath := finalPath + ".partial"

	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(partialPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	return &spoolFile{
		key:         key,
		partialPath: partialPath,
		finalPath:   finalPath,
		f:           f,
	}, nil
}

func writeJSONSpool(key string, v any) (size int64, err error) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	w, err := newSpoolFile(key)
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}

type teeReadCloser struct {
	rc        io.ReadCloser
	spool     *spoolFile
	closeOnce sync.Once
}

func (t *teeReadCloser) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 && t.spool != nil {
		_, _ = t.spool.Write(p[:n])
	}
	// Close the spool on EOF to finalize the file even when callers forget to Close().
	if errors.Is(err, io.EOF) {
		t.closeOnce.Do(func() { _ = t.spool.Close() })
	}
	return n, err
}

func (t *teeReadCloser) Close() error {
	t.closeOnce.Do(func() { _ = t.spool.Close() })
	if t.rc == nil {
		return nil
	}
	return t.rc.Close()
}

type requestTraceNodeBuilder struct {
	service string
	kind    string
	seq     int

	startedAtMs int64
	endedAtMs   int64

	reqMethod string
	reqURL    string
	reqPath   string

	reqHeadersKey  string
	reqHeadersSize int64
	reqBodyKey     string
	reqBodySpool   *spoolFile

	respStatus      int
	respHeadersKey  string
	respHeadersSize int64
	respBodyKey     string
	respBodySpool   *spoolFile

	errText string
	meta    map[string]any
}

func (b *requestTraceNodeBuilder) requestBodySize() int64 {
	if b == nil || b.reqBodySpool == nil {
		return 0
	}
	return b.reqBodySpool.BytesWritten()
}

func (b *requestTraceNodeBuilder) responseBodySize() int64 {
	if b == nil || b.respBodySpool == nil {
		return 0
	}
	return b.respBodySpool.BytesWritten()
}

type requestTraceSession struct {
	requestID string
	prefix    string
	startMs   int64

	httpNode *requestTraceNodeBuilder

	mu          sync.Mutex
	upstreamSeq int
	spanSeq     int
	upstreams   []*requestTraceNodeBuilder
}

func newRequestTraceSession(requestID string) *requestTraceSession {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	now := time.Now()
	prefix := fmt.Sprintf("request_traces/%04d/%02d/%02d/%s", now.Year(), int(now.Month()), now.Day(), requestID)
	return &requestTraceSession{
		requestID: requestID,
		prefix:    prefix,
		startMs:   now.UnixMilli(),
	}
}

func (s *requestTraceSession) addUpstream(node *requestTraceNodeBuilder) {
	if s == nil || node == nil {
		return
	}
	s.mu.Lock()
	s.upstreams = append(s.upstreams, node)
	s.mu.Unlock()
}

func (s *requestTraceSession) nextUpstreamSequence() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upstreamSeq++
	return s.upstreamSeq
}

func (s *requestTraceSession) nextSpanSequence() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spanSeq++
	return s.spanSeq
}

// RecordRequestTraceSpan appends a lightweight in-process span into the current request trace session.
// It does nothing when request tracing is disabled or no session is attached to the context.
//
// Notes:
// - "kind" is a free-form category (e.g. "db", "cache", "internal").
// - "method" and "path" are displayed by the admin UI as "request_method" and "request_path".
func RecordRequestTraceSpan(
	ctx context.Context,
	kind string,
	method string,
	path string,
	startedAt time.Time,
	endedAt time.Time,
	status int,
	err error,
	meta map[string]any,
) {
	RecordRequestTraceServiceSpan(ctx, "new-api", kind, method, path, startedAt, endedAt, status, err, meta)
}

// RecordRequestTraceServiceSpan appends a lightweight in-process span for the named service.
func RecordRequestTraceServiceSpan(
	ctx context.Context,
	serviceName string,
	kind string,
	method string,
	path string,
	startedAt time.Time,
	endedAt time.Time,
	status int,
	err error,
	meta map[string]any,
) {
	sess, ok := requestTraceSessionFromContext(ctx)
	if !ok || sess == nil {
		return
	}

	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	if endedAt.IsZero() {
		endedAt = time.Now()
	}
	if endedAt.Before(startedAt) {
		endedAt = startedAt
	}

	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "new-api"
	}

	node := &requestTraceNodeBuilder{
		service:     serviceName,
		kind:        strings.TrimSpace(kind),
		seq:         sess.nextSpanSequence(),
		startedAtMs: startedAt.UnixMilli(),
		endedAtMs:   endedAt.UnixMilli(),
		reqMethod:   strings.TrimSpace(method),
		reqPath:     strings.TrimSpace(path),
		respStatus:  status,
		meta:        meta,
	}
	if err != nil {
		node.errText = err.Error()
	}
	sess.addUpstream(node)
}

func (s *requestTraceSession) persist(ctx context.Context, c *http.Request, status int) {
	if s == nil {
		return
	}
	nowSec := common.GetTimestamp()
	createdSec := s.startMs / 1000
	if createdSec <= 0 {
		createdSec = nowSec
	}
	method := ""
	path := ""
	if c != nil {
		method = strings.TrimSpace(c.Method)
		if c.URL != nil {
			path = strings.TrimSpace(c.URL.Path)
		}
	}

	sessionRow := &model.RequestTraceSession{
		RequestId:     s.requestID,
		CreatedAt:     createdSec,
		UpdatedAt:     nowSec,
		RequestMethod: method,
		RequestPath:   path,
	}
	if err := model.DB.Save(sessionRow).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("[request_trace] persist session failed: request_id=%s err=%v", s.requestID, err))
	}

	nodes := make([]*model.RequestTraceNode, 0, 1+len(s.upstreams))

	if s.httpNode != nil {
		metaJSON := ""
		if len(s.httpNode.meta) > 0 {
			if data, err := json.Marshal(s.httpNode.meta); err == nil {
				metaJSON = string(data)
			}
		}
		nodes = append(nodes, &model.RequestTraceNode{
			RequestId:           s.requestID,
			Service:             s.httpNode.service,
			Kind:                s.httpNode.kind,
			Seq:                 s.httpNode.seq,
			StartedAt:           s.httpNode.startedAtMs,
			EndedAt:             s.httpNode.endedAtMs,
			RequestMethod:       s.httpNode.reqMethod,
			RequestURL:          s.httpNode.reqURL,
			RequestPath:         s.httpNode.reqPath,
			RequestHeadersKey:   s.httpNode.reqHeadersKey,
			RequestHeadersSize:  s.httpNode.reqHeadersSize,
			RequestBodyKey:      s.httpNode.reqBodyKey,
			RequestBodySize:     s.httpNode.requestBodySize(),
			ResponseStatus:      status,
			ResponseHeadersKey:  s.httpNode.respHeadersKey,
			ResponseHeadersSize: s.httpNode.respHeadersSize,
			ResponseBodyKey:     s.httpNode.respBodyKey,
			ResponseBodySize:    s.httpNode.responseBodySize(),
			Error:               s.httpNode.errText,
			Meta:                metaJSON,
			CreatedAt:           s.httpNode.startedAtMs / 1000,
			UpdatedAt:           nowSec,
		})
	}

	s.mu.Lock()
	upstreams := append([]*requestTraceNodeBuilder(nil), s.upstreams...)
	s.mu.Unlock()

	for _, u := range upstreams {
		if u == nil {
			continue
		}
		if u.endedAtMs == 0 {
			u.endedAtMs = time.Now().UnixMilli()
		}
		metaJSON := ""
		if len(u.meta) > 0 {
			if data, err := json.Marshal(u.meta); err == nil {
				metaJSON = string(data)
			}
		}
		nodes = append(nodes, &model.RequestTraceNode{
			RequestId:           s.requestID,
			Service:             u.service,
			Kind:                u.kind,
			Seq:                 u.seq,
			StartedAt:           u.startedAtMs,
			EndedAt:             u.endedAtMs,
			RequestMethod:       u.reqMethod,
			RequestURL:          u.reqURL,
			RequestPath:         u.reqPath,
			RequestHeadersKey:   u.reqHeadersKey,
			RequestHeadersSize:  u.reqHeadersSize,
			RequestBodyKey:      u.reqBodyKey,
			RequestBodySize:     u.requestBodySize(),
			ResponseStatus:      u.respStatus,
			ResponseHeadersKey:  u.respHeadersKey,
			ResponseHeadersSize: u.respHeadersSize,
			ResponseBodyKey:     u.respBodyKey,
			ResponseBodySize:    u.responseBodySize(),
			Error:               u.errText,
			Meta:                metaJSON,
			CreatedAt:           u.startedAtMs / 1000,
			UpdatedAt:           nowSec,
		})
	}

	if len(nodes) > 0 {
		if err := model.DB.Create(&nodes).Error; err != nil {
			logger.LogError(ctx, fmt.Sprintf("[request_trace] persist nodes failed: request_id=%s err=%v", s.requestID, err))
		}
	}
}

type traceRoundTripper struct {
	base http.RoundTripper
}

func (t *traceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	sess, ok := requestTraceSessionFromContext(req.Context())
	if !ok || sess == nil {
		return base.RoundTrip(req)
	}

	seq := sess.nextUpstreamSequence()
	nodeBase := fmt.Sprintf("%s/new-api/upstream/%03d", sess.prefix, seq)
	node := &requestTraceNodeBuilder{
		service:     "new-api",
		kind:        "upstream",
		seq:         seq,
		startedAtMs: time.Now().UnixMilli(),
		reqMethod:   strings.TrimSpace(req.Method),
	}
	if req.URL != nil {
		node.reqURL = req.URL.String()
		node.reqPath = strings.TrimSpace(req.URL.Path)
	}

	if key := nodeBase + "/request_headers.json"; true {
		size, err := writeJSONSpool(key, req.Header)
		node.reqHeadersKey = key
		node.reqHeadersSize = size
		if err != nil {
			node.errText = fmt.Sprintf("write upstream request headers failed: %v", err)
		}
	}

	if req.Body != nil {
		key := nodeBase + "/request_body.bin"
		spool, err := newSpoolFile(key)
		if err != nil {
			node.errText = coalesceErr(node.errText, fmt.Sprintf("open upstream request body spool failed: %v", err))
		} else {
			node.reqBodyKey = key
			node.reqBodySpool = spool
			req.Body = &teeReadCloser{rc: req.Body, spool: spool}
		}
	}

	sess.addUpstream(node)

	resp, err := base.RoundTrip(req)
	if err != nil {
		node.errText = coalesceErr(node.errText, err.Error())
		node.endedAtMs = time.Now().UnixMilli()
		if node.reqBodySpool != nil {
			_ = node.reqBodySpool.Close()
		}
		return nil, err
	}
	if resp == nil {
		node.errText = coalesceErr(node.errText, "upstream response is nil")
		node.endedAtMs = time.Now().UnixMilli()
		return nil, errors.New("upstream response is nil")
	}

	node.respStatus = resp.StatusCode
	if key := nodeBase + "/response_headers.json"; true {
		size, werr := writeJSONSpool(key, resp.Header)
		node.respHeadersKey = key
		node.respHeadersSize = size
		if werr != nil {
			node.errText = coalesceErr(node.errText, fmt.Sprintf("write upstream response headers failed: %v", werr))
		}
	}

	if resp.Body != nil {
		key := nodeBase + "/response_body.bin"
		spool, werr := newSpoolFile(key)
		if werr != nil {
			node.errText = coalesceErr(node.errText, fmt.Sprintf("open upstream response body spool failed: %v", werr))
		} else {
			node.respBodyKey = key
			node.respBodySpool = spool
			resp.Body = &teeReadCloser{rc: resp.Body, spool: spool}
		}
	}
	return resp, nil
}

func wrapTransportForRequestTrace(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	// Always wrap: RoundTrip is a cheap context check when tracing is disabled.
	return &traceRoundTripper{base: base}
}

func coalesceErr(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" {
		return existing
	}
	return existing + "; " + next
}

// ---- Read helpers (API) ----

func RequestTraceReadObject(ctx context.Context, key string) (rc io.ReadCloser, size int64, contentType string, err error) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return nil, 0, "", errors.New("key is empty")
	}
	if strings.Contains(key, "..") {
		return nil, 0, "", fmt.Errorf("invalid key: %q", key)
	}

	// Prefer local spool for freshest data.
	localPath := filepath.Join(globalRequestTraceManager.spoolDir, filepath.FromSlash(key))
	if st, err := os.Stat(localPath); err == nil && st != nil && !st.IsDir() {
		f, err := os.Open(localPath)
		if err != nil {
			return nil, 0, "", err
		}
		return f, st.Size(), sniffContentTypeFromKey(key), nil
	}

	return nil, 0, "", os.ErrNotExist
}

func sniffContentTypeFromKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.HasSuffix(key, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(key, ".txt"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func RequestTraceGuessContentType(key string) string {
	return sniffContentTypeFromKey(key)
}

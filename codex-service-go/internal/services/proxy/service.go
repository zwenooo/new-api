package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pruntime "codex-service-go/internal/proxy/runtime"
	instsvc "codex-service-go/internal/services/instances"
	"codex-service-go/pkg/proxyurl"
	promptdef "codex-service-go/prompt"

	cpatranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	cpabuiltin "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
	xproxy "golang.org/x/net/proxy"
)

type RequestStage struct {
	Service   string
	Kind      string
	Method    string
	Path      string
	StartedAt time.Time
	EndedAt   time.Time
	Status    int
	Error     error
	Meta      map[string]any
}

type RequestStageRecorder func(ctx context.Context, stage RequestStage)

type Options struct {
	Debug                  bool
	AccessLog              bool
	Originator             string
	UserAgent              string
	RuntimeFile            string
	RuntimeExpireAt        string
	ChatGPTClientID        string
	ChatGPTAccountID       string
	DefaultAuthFile        string
	DefaultAuthMode        string
	DefaultUpstreamBaseURL string
	DefaultUpstreamAPIKey  string
	ResponsesBaseURL       string
	OnRuntimeChange        func(instanceID int64)
	OnRequestStage         RequestStageRecorder
	AuthStore              AuthStore
}

type Service struct {
	opts                  Options
	runtimeBaseFile       string
	runtimeExpireAt       string
	onRuntimeChange       func(instanceID int64)
	requestTraces         *requestTraceStore
	requestTraceBodyLimit atomic.Int64
	responseStatuses      *responseStatusStore
	transportHealth       *transportHealthStore

	cliProxyAPIMode     atomic.Int32
	openCodeCompat      openCodeCompatConfig
	responsesCompat     responsesCompatConfig
	responsesUpstreamWS atomic.Bool
	cxPoolStateKeywords atomic.Value // cxPoolStateKeywordsConfig

	clientMu          sync.Mutex
	clientsByInstance map[int64]*clientCacheEntry
	runtimeMu         sync.Mutex
	runtimeByInstance map[int64]*pruntime.Manager
}

type openCodeCompatConfig struct {
	enabled      atomic.Bool
	instructions atomic.Value // string
	cpaExpected  atomic.Value // string
}

type responsesCompatConfig struct {
	codexCLIUserAgentContains atomic.Value // string
	cliProxyUserAgentContains atomic.Value // string
	overrideInstructions      atomic.Bool
	bodyPatch                 atomic.Value // *responsesBodyPatchConfig
}

type responsesBodyPatchConfig struct {
	patch map[string]interface{}
}

type cxPoolStateKeywordsConfig struct {
	MemberExpired []string `json:"member_expired"`
	Expired       []string `json:"expired"`
	Cooldown      []string `json:"cooldown"`
}

const (
	cliProxyAPIModeOff int32 = iota
	cliProxyAPIModeOpenCodeUA
	cliProxyAPIModeAll
	cliProxyAPIModeAllForceOpenCode
)

const defaultCodexClientVersion = "0.104.0"
const defaultCompatResponsesInstructions = "You are a helpful coding assistant."

var codexModelMap = map[string]string{
	"gpt-5.4":                    "gpt-5.4",
	"gpt-5.4-none":               "gpt-5.4",
	"gpt-5.4-low":                "gpt-5.4",
	"gpt-5.4-medium":             "gpt-5.4",
	"gpt-5.4-high":               "gpt-5.4",
	"gpt-5.4-xhigh":              "gpt-5.4",
	"gpt-5.4-chat-latest":        "gpt-5.4",
	"gpt-5.3":                    "gpt-5.3-codex",
	"gpt-5.3-none":               "gpt-5.3-codex",
	"gpt-5.3-low":                "gpt-5.3-codex",
	"gpt-5.3-medium":             "gpt-5.3-codex",
	"gpt-5.3-high":               "gpt-5.3-codex",
	"gpt-5.3-xhigh":              "gpt-5.3-codex",
	"gpt-5.3-codex":              "gpt-5.3-codex",
	"gpt-5.3-codex-spark":        "gpt-5.3-codex",
	"gpt-5.3-codex-spark-low":    "gpt-5.3-codex",
	"gpt-5.3-codex-spark-medium": "gpt-5.3-codex",
	"gpt-5.3-codex-spark-high":   "gpt-5.3-codex",
	"gpt-5.3-codex-spark-xhigh":  "gpt-5.3-codex",
	"gpt-5.3-codex-low":          "gpt-5.3-codex",
	"gpt-5.3-codex-medium":       "gpt-5.3-codex",
	"gpt-5.3-codex-high":         "gpt-5.3-codex",
	"gpt-5.3-codex-xhigh":        "gpt-5.3-codex",
	"gpt-5.1-codex":              "gpt-5.1-codex",
	"gpt-5.1-codex-low":          "gpt-5.1-codex",
	"gpt-5.1-codex-medium":       "gpt-5.1-codex",
	"gpt-5.1-codex-high":         "gpt-5.1-codex",
	"gpt-5.1-codex-max":          "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-low":      "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-medium":   "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-high":     "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-xhigh":    "gpt-5.1-codex-max",
	"gpt-5.2":                    "gpt-5.2",
	"gpt-5.2-none":               "gpt-5.2",
	"gpt-5.2-low":                "gpt-5.2",
	"gpt-5.2-medium":             "gpt-5.2",
	"gpt-5.2-high":               "gpt-5.2",
	"gpt-5.2-xhigh":              "gpt-5.2",
	"gpt-5.2-codex":              "gpt-5.2-codex",
	"gpt-5.2-codex-low":          "gpt-5.2-codex",
	"gpt-5.2-codex-medium":       "gpt-5.2-codex",
	"gpt-5.2-codex-high":         "gpt-5.2-codex",
	"gpt-5.2-codex-xhigh":        "gpt-5.2-codex",
	"gpt-5.1-codex-mini":         "gpt-5.1-codex-mini",
	"gpt-5.1-codex-mini-medium":  "gpt-5.1-codex-mini",
	"gpt-5.1-codex-mini-high":    "gpt-5.1-codex-mini",
	"gpt-5.1":                    "gpt-5.1",
	"gpt-5.1-none":               "gpt-5.1",
	"gpt-5.1-low":                "gpt-5.1",
	"gpt-5.1-medium":             "gpt-5.1",
	"gpt-5.1-high":               "gpt-5.1",
	"gpt-5.1-chat-latest":        "gpt-5.1",
	"gpt-5-codex":                "gpt-5.1-codex",
	"codex-mini-latest":          "gpt-5.1-codex-mini",
	"gpt-5-codex-mini":           "gpt-5.1-codex-mini",
	"gpt-5-codex-mini-medium":    "gpt-5.1-codex-mini",
	"gpt-5-codex-mini-high":      "gpt-5.1-codex-mini",
	"gpt-5":                      "gpt-5.1",
	"gpt-5-mini":                 "gpt-5.1",
	"gpt-5-nano":                 "gpt-5.1",
}

const (
	defaultUpstreamDialTimeoutSeconds           = 10
	defaultUpstreamTLSHandshakeTimeoutSeconds   = 10
	defaultUpstreamResponseHeaderTimeoutSeconds = 50
	defaultUpstreamIdleConnTimeoutSeconds       = 60
	defaultUpstreamTransportQuarantineSeconds   = 45
	defaultUpstreamMaxIdleConns                 = 128
	defaultUpstreamMaxIdleConnsPerHost          = 32
)

type closeIdler interface {
	CloseIdleConnections()
}

type clientCacheEntry struct {
	cacheKey string
	client   *http.Client
}

func envIntSeconds(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func envDurationSeconds(name string, fallbackSeconds int) time.Duration {
	seconds := envIntSeconds(name, fallbackSeconds)
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func upstreamDialTimeout() time.Duration {
	return envDurationSeconds("CX_POOL_UPSTREAM_DIAL_TIMEOUT_SECONDS", defaultUpstreamDialTimeoutSeconds)
}

func upstreamTLSHandshakeTimeout() time.Duration {
	return envDurationSeconds("CX_POOL_UPSTREAM_TLS_HANDSHAKE_TIMEOUT_SECONDS", defaultUpstreamTLSHandshakeTimeoutSeconds)
}

func upstreamResponseHeaderTimeout() time.Duration {
	return envDurationSeconds("CX_POOL_UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS", defaultUpstreamResponseHeaderTimeoutSeconds)
}

func upstreamIdleConnTimeout() time.Duration {
	return envDurationSeconds("CX_POOL_UPSTREAM_IDLE_CONN_TIMEOUT_SECONDS", defaultUpstreamIdleConnTimeoutSeconds)
}

func upstreamTransportQuarantineSeconds() int {
	return envIntSeconds("CX_POOL_UPSTREAM_TIMEOUT_COOLDOWN_SECONDS", defaultUpstreamTransportQuarantineSeconds)
}

func newUpstreamNetDialer() *net.Dialer {
	dialer := &net.Dialer{KeepAlive: 30 * time.Second}
	if timeout := upstreamDialTimeout(); timeout > 0 {
		dialer.Timeout = timeout
	}
	return dialer
}

func newUpstreamTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = newUpstreamNetDialer().DialContext
	if timeout := upstreamTLSHandshakeTimeout(); timeout > 0 {
		transport.TLSHandshakeTimeout = timeout
	}
	if timeout := upstreamResponseHeaderTimeout(); timeout > 0 {
		transport.ResponseHeaderTimeout = timeout
	}
	if timeout := upstreamIdleConnTimeout(); timeout > 0 {
		transport.IdleConnTimeout = timeout
	}
	if transport.MaxIdleConns < defaultUpstreamMaxIdleConns {
		transport.MaxIdleConns = defaultUpstreamMaxIdleConns
	}
	if transport.MaxIdleConnsPerHost < defaultUpstreamMaxIdleConnsPerHost {
		transport.MaxIdleConnsPerHost = defaultUpstreamMaxIdleConnsPerHost
	}
	return transport
}

func closeHTTPClientIdleConnections(client *http.Client) {
	if client == nil || client.Transport == nil {
		return
	}
	if closer, ok := client.Transport.(closeIdler); ok {
		closer.CloseIdleConnections()
	}
}

func isLikelyTransportTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "timeout awaiting response headers") ||
		strings.Contains(message, "client.timeout exceeded while awaiting headers") ||
		strings.Contains(message, "i/o timeout")
}

func shortenTransportError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	runes := []rune(message)
	if len(runes) > 180 {
		message = string(runes[:180])
	}
	return message
}

func NewService(opts Options) *Service {
	if strings.TrimSpace(opts.Originator) == "" {
		opts.Originator = "codex_cli_rs"
	}
	if strings.TrimSpace(opts.UserAgent) == "" {
		// 近似对齐 Codex CLI UA 格式：codex_cli_rs/<ver> (<os>; <arch>) codex-service-go
		opts.UserAgent = defaultUserAgent()
	}
	if strings.TrimSpace(opts.DefaultAuthMode) == "" {
		opts.DefaultAuthMode = "chatgpt"
	}
	svc := &Service{
		opts:              opts,
		runtimeBaseFile:   strings.TrimSpace(opts.RuntimeFile),
		runtimeExpireAt:   strings.TrimSpace(opts.RuntimeExpireAt),
		onRuntimeChange:   opts.OnRuntimeChange,
		requestTraces:     newRequestTraceStore(defaultRequestTraceRetention),
		responseStatuses:  newResponseStatusStore(),
		transportHealth:   newTransportHealthStore(),
		clientsByInstance: make(map[int64]*clientCacheEntry),
		runtimeByInstance: make(map[int64]*pruntime.Manager),
	}
	svc.openCodeCompat.instructions.Store("")
	svc.openCodeCompat.cpaExpected.Store("")
	svc.responsesCompat.codexCLIUserAgentContains.Store("codex_vscode,codex_exec,Codex Desktop,codex_cli_rs")
	svc.responsesCompat.cliProxyUserAgentContains.Store("ai-sdk/openai,opencode/,openai/js")
	svc.responsesCompat.overrideInstructions.Store(false)
	svc.responsesCompat.bodyPatch.Store(&responsesBodyPatchConfig{patch: nil})
	svc.cxPoolStateKeywords.Store(cxPoolStateKeywordsConfig{})
	svc.requestTraceBodyLimit.Store(int64(defaultRequestTraceBodyLimit))
	return svc
}

func (s *Service) SetResponsesCompatCodexCLIUserAgentContains(contains string) {
	if s == nil {
		return
	}
	s.responsesCompat.codexCLIUserAgentContains.Store(strings.TrimSpace(contains))
}

func (s *Service) SetResponsesCompatCLIProxyUserAgentContains(contains string) {
	if s == nil {
		return
	}
	s.responsesCompat.cliProxyUserAgentContains.Store(strings.TrimSpace(contains))
}

func (s *Service) SetResponsesCompatOverrideInstructions(override bool) {
	if s == nil {
		return
	}
	s.responsesCompat.overrideInstructions.Store(override)
}

func (s *Service) SetResponsesCompatBodyPatchJSON(bodyPatchJSON string) error {
	if s == nil {
		return nil
	}
	bodyPatchJSON = strings.TrimSpace(bodyPatchJSON)
	if bodyPatchJSON == "" || bodyPatchJSON == "null" || bodyPatchJSON == "<nil>" {
		s.responsesCompat.bodyPatch.Store(&responsesBodyPatchConfig{patch: nil})
		return nil
	}

	var patch map[string]interface{}
	if err := json.Unmarshal([]byte(bodyPatchJSON), &patch); err != nil {
		return fmt.Errorf("cx_compat.responses.body_patch_json: %w", err)
	}
	if len(patch) == 0 {
		s.responsesCompat.bodyPatch.Store(&responsesBodyPatchConfig{patch: nil})
		return nil
	}

	s.responsesCompat.bodyPatch.Store(&responsesBodyPatchConfig{patch: patch})
	return nil
}

func (s *Service) SetCxPoolStateKeywordsJSON(stateKeywordsJSON string) error {
	if s == nil {
		return nil
	}
	stateKeywordsJSON = strings.TrimSpace(stateKeywordsJSON)
	if stateKeywordsJSON == "" || stateKeywordsJSON == "null" || stateKeywordsJSON == "<nil>" {
		s.cxPoolStateKeywords.Store(cxPoolStateKeywordsConfig{})
		return nil
	}
	var cfg cxPoolStateKeywordsConfig
	if err := json.Unmarshal([]byte(stateKeywordsJSON), &cfg); err != nil {
		return fmt.Errorf("cx_pool.state_keywords: %w", err)
	}
	normalized, err := normalizeCxPoolStateKeywordsConfig(cfg)
	if err != nil {
		return err
	}
	s.cxPoolStateKeywords.Store(normalized)
	return nil
}

func (s *Service) SetRequestTraceRetention(retention time.Duration) {
	if s == nil {
		return
	}
	if s.requestTraces == nil {
		s.requestTraces = newRequestTraceStore(retention)
		return
	}
	s.requestTraces.setRetention(retention)
}

func (s *Service) SetRequestTraceLimits(maxPerInstance int, maxTotal int) {
	if s == nil {
		return
	}
	if s.requestTraces == nil {
		s.requestTraces = newRequestTraceStore(defaultRequestTraceRetention)
	}
	s.requestTraces.setLimits(maxPerInstance, maxTotal)
}

func (s *Service) SetRequestTraceBodyLimit(limit int) {
	if s == nil {
		return
	}
	if limit <= 0 {
		limit = defaultRequestTraceBodyLimit
	}
	s.requestTraceBodyLimit.Store(int64(limit))
}

func (s *Service) requestTraceBodyCaptureLimit() int {
	if s == nil {
		return defaultRequestTraceBodyLimit
	}
	v := int(s.requestTraceBodyLimit.Load())
	if v <= 0 {
		return defaultRequestTraceBodyLimit
	}
	return v
}

func limitBytes(payload []byte, limit int) ([]byte, bool) {
	if len(payload) == 0 || limit <= 0 {
		return payload, false
	}
	if len(payload) <= limit {
		return payload, false
	}
	return payload[:limit], true
}

func (s *Service) GetRequestTraceSummary(instanceID int64, requestID string) (*RequestTraceSummary, bool) {
	if s == nil || s.requestTraces == nil {
		return nil, false
	}
	return s.requestTraces.getSummary(instanceID, requestID)
}

func (s *Service) GetRequestTraceDetail(instanceID int64, requestID string) (*RequestTraceDetail, bool) {
	if s == nil || s.requestTraces == nil {
		return nil, false
	}
	return s.requestTraces.getDetail(instanceID, requestID)
}

func (s *Service) GetResponseStatusSet(instanceID int64) []ResponseStatusItem {
	if s == nil || s.responseStatuses == nil {
		return nil
	}
	return s.responseStatuses.list(instanceID)
}

func (s *Service) DrainResponseStatusDeltas(instanceID int64) []ResponseStatusDelta {
	if s == nil || s.responseStatuses == nil {
		return nil
	}
	return s.responseStatuses.drain(instanceID)
}

func (s *Service) beginRequestStage(ctx context.Context, kind, method, path string) func(status int, err error, meta map[string]any) {
	startedAt := time.Now()
	return func(status int, err error, meta map[string]any) {
		if s == nil || s.opts.OnRequestStage == nil {
			return
		}
		endedAt := time.Now()
		if status <= 0 {
			if err != nil {
				status = http.StatusInternalServerError
			} else {
				status = http.StatusOK
			}
		}
		s.opts.OnRequestStage(ctx, RequestStage{
			Service:   "codex-service-go",
			Kind:      strings.TrimSpace(kind),
			Method:    strings.TrimSpace(method),
			Path:      strings.TrimSpace(path),
			StartedAt: startedAt,
			EndedAt:   endedAt,
			Status:    status,
			Error:     err,
			Meta:      meta,
		})
	}
}

func stageTarget(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return strings.TrimSpace(req.URL.String())
}

func (s *Service) doUpstreamRequest(ctx context.Context, inst instsvc.InstanceWithPaths, httpClient *http.Client, req *http.Request, kind string) (*http.Response, error) {
	finish := s.beginRequestStage(ctx, kind, strings.TrimSpace(req.Method), stageTarget(req))
	proxyKey, _, _ := proxyCacheKey(inst.Proxy)
	if s != nil && s.transportHealth != nil {
		s.transportHealth.onStart(inst.ID, proxyKey, time.Now())
	}
	resp, err := httpClient.Do(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	finish(status, err, nil)
	if s != nil && s.transportHealth != nil {
		decision := s.transportHealth.onResult(inst.ID, proxyKey, time.Now(), err)
		if decision.resetProxy {
			resetIDs := s.dropClientsByCacheKey(proxyKey)
			s.transportHealth.noteProxyReset(resetIDs, proxyKey, time.Now(), decision.reason)
			if inst.LogPath != "" {
				s.appendLog(inst.LogPath, fmt.Sprintf("[%s] transport auto-recovery reset proxy clients: %s", time.Now().Format(time.RFC3339), decision.reason))
			}
		}
	}
	return resp, err
}

func proxyCacheKey(rawProxyURL string) (string, *url.URL, error) {
	rawProxyURL = strings.TrimSpace(rawProxyURL)
	if rawProxyURL == "" {
		return "direct", nil, nil
	}
	parsedURL, err := proxyurl.Parse(rawProxyURL)
	if err != nil {
		return "", nil, err
	}
	return parsedURL.String(), parsedURL, nil
}

func (s *Service) clientForInstance(inst instsvc.InstanceWithPaths) (*http.Client, error) {
	cacheKey, parsedURL, err := proxyCacheKey(inst.Proxy)
	if err != nil {
		return nil, err
	}
	if inst.ID <= 0 {
		if parsedURL == nil {
			return &http.Client{Transport: newUpstreamTransport(), Timeout: 0}, nil
		}
		return newProxyHTTPClientFromURL(parsedURL)
	}

	s.clientMu.Lock()
	entry := s.clientsByInstance[inst.ID]
	if entry != nil && entry.cacheKey == cacheKey && entry.client != nil {
		client := entry.client
		s.clientMu.Unlock()
		return client, nil
	}
	s.clientMu.Unlock()

	var client *http.Client
	if parsedURL == nil {
		client = &http.Client{Transport: newUpstreamTransport(), Timeout: 0}
	} else {
		client, err = newProxyHTTPClientFromURL(parsedURL)
		if err != nil {
			return nil, err
		}
	}

	var stale *http.Client
	s.clientMu.Lock()
	entry = s.clientsByInstance[inst.ID]
	if entry != nil && entry.cacheKey == cacheKey && entry.client != nil {
		existing := entry.client
		s.clientMu.Unlock()
		closeHTTPClientIdleConnections(client)
		return existing, nil
	}
	if entry != nil {
		stale = entry.client
	}
	s.clientsByInstance[inst.ID] = &clientCacheEntry{cacheKey: cacheKey, client: client}
	s.clientMu.Unlock()
	if stale != nil && stale != client {
		closeHTTPClientIdleConnections(stale)
	}
	return client, nil
}

func (s *Service) dropInstanceClient(inst instsvc.InstanceWithPaths, client *http.Client) {
	if s == nil {
		closeHTTPClientIdleConnections(client)
		return
	}
	if inst.ID <= 0 {
		closeHTTPClientIdleConnections(client)
		return
	}
	var stale *http.Client
	s.clientMu.Lock()
	if entry := s.clientsByInstance[inst.ID]; entry != nil {
		if client == nil || entry.client == client {
			stale = entry.client
			delete(s.clientsByInstance, inst.ID)
		}
	}
	s.clientMu.Unlock()
	if stale == nil {
		stale = client
	}
	closeHTTPClientIdleConnections(stale)
}

func (s *Service) dropClientsByCacheKey(cacheKey string) []int64 {
	if s == nil {
		return nil
	}
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		cacheKey = "direct"
	}
	staleClients := make([]*http.Client, 0)
	instanceIDs := make([]int64, 0)
	s.clientMu.Lock()
	for instanceID, entry := range s.clientsByInstance {
		if entry == nil || entry.client == nil || strings.TrimSpace(entry.cacheKey) != cacheKey {
			continue
		}
		staleClients = append(staleClients, entry.client)
		instanceIDs = append(instanceIDs, instanceID)
		delete(s.clientsByInstance, instanceID)
	}
	s.clientMu.Unlock()
	for _, client := range staleClients {
		closeHTTPClientIdleConnections(client)
	}
	return instanceIDs
}

func newProxyHTTPClientFromURL(parsedURL *url.URL) (*http.Client, error) {
	if parsedURL == nil {
		return nil, errors.New("proxy url is required")
	}

	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "http", "https":
		transport := newUpstreamTransport()
		transport.Proxy = http.ProxyURL(parsedURL)
		return &http.Client{Transport: transport, Timeout: 0}, nil
	case "socks5", "socks5h":
		var auth *xproxy.Auth
		if parsedURL.User != nil {
			auth = &xproxy.Auth{
				User:     parsedURL.User.Username(),
				Password: "",
			}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}
		dialer, err := xproxy.SOCKS5("tcp", parsedURL.Host, auth, newUpstreamNetDialer())
		if err != nil {
			return nil, err
		}
		transport := newUpstreamTransport()
		transport.Proxy = nil
		if contextDialer, ok := dialer.(xproxy.ContextDialer); ok {
			transport.DialContext = contextDialer.DialContext
		} else {
			transport.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
		return &http.Client{Transport: transport, Timeout: 0}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
	}
}

func defaultUserAgent() string {
	ver := defaultCodexClientVersion
	osName := runtime.GOOS
	arch := runtime.GOARCH
	// 简化：不尝试读取内核/发行版版本
	return fmt.Sprintf("codex_cli_rs/%s (%s; %s) codex-service-go", ver, osName, arch)
}

func (s *Service) ShouldBlock(ctx context.Context, instanceID int64) (pruntime.BlockResult, error) {
	_ = ctx
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return pruntime.BlockResult{Blocked: false}, nil
	}
	return runtime.ShouldBlock(time.Now())
}

func (s *Service) RecoverInstanceForLocalTestSuccess(instanceID int64) error {
	if s == nil || instanceID <= 0 {
		return nil
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return nil
	}
	return runtime.ClearBlockingStatus()
}

// RecordInstanceChannelBackoff stores a temporary local backoff for a managed channel instance
// without mutating the instance config itself. The backoff is automatically cleared by runtime
// when it expires.
func (s *Service) RecordInstanceChannelBackoff(instanceID int64, seconds int, message string) error {
	if s == nil || instanceID <= 0 || seconds <= 0 {
		return nil
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return nil
	}
	return runtime.RecordChannelBackoff(seconds, strings.TrimSpace(message))
}

// ClearInstanceChannelBackoff clears the runtime "channel_backoff" status for the given instance,
// if present, without touching other blocking states.
func (s *Service) ClearInstanceChannelBackoff(instanceID int64) error {
	if s == nil || instanceID <= 0 {
		return nil
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return nil
	}
	state, err := runtime.CurrentState()
	if err != nil {
		return err
	}
	if state != "channel_backoff" {
		return nil
	}
	return runtime.ClearChannelBackoff()
}

// RecordInstanceTransportQuarantine stores a temporary local transport isolation state so a broken
// client / proxy chain can be taken out of rotation without being mislabeled as upstream cooldown.
func (s *Service) RecordInstanceTransportQuarantine(instanceID int64, seconds int, message string) error {
	if s == nil || instanceID <= 0 || seconds <= 0 {
		return nil
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return nil
	}
	return runtime.RecordTransportQuarantine(seconds, strings.TrimSpace(message))
}

// ClearInstanceTransportQuarantine clears the runtime "transport_quarantine" status for the given
// instance, if present, without touching other blocking states.
func (s *Service) ClearInstanceTransportQuarantine(instanceID int64) error {
	if s == nil || instanceID <= 0 {
		return nil
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return nil
	}
	state, err := runtime.CurrentState()
	if err != nil {
		return err
	}
	if state != "transport_quarantine" {
		return nil
	}
	return runtime.ClearTransportQuarantine()
}

func (s *Service) InvalidateInstanceAuth(instanceID int64) {
	if s == nil {
		return
	}
	clientID := strings.TrimSpace(s.opts.ChatGPTClientID)
	if instanceID <= 0 || clientID == "" {
		return
	}
	chatGPTAuthCache.Delete(fmt.Sprintf("db:%d|%s", instanceID, clientID))
}

func (s *Service) RefreshInstanceAuth(ctx context.Context, instanceID int64) error {
	if s == nil {
		return errors.New("proxy service not configured")
	}
	if s.opts.AuthStore == nil {
		return errors.New("auth store not configured")
	}
	auth := NewChatGPTAuthForInstance(s.opts.AuthStore, instanceID, s.opts.ChatGPTClientID)
	if auth == nil {
		return errors.New("auth manager not initialized")
	}
	if err := auth.refresh(ctx); err != nil {
		return err
	}
	if runtime := s.runtimeForInstance(instanceID); runtime != nil {
		if err := runtime.ClearExpiredStatus(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SetCLIProxyAPIMode(mode string) error {
	if s == nil {
		return nil
	}

	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "off":
		s.cliProxyAPIMode.Store(cliProxyAPIModeOff)
	case "opencode_ua":
		s.cliProxyAPIMode.Store(cliProxyAPIModeOpenCodeUA)
	case "all":
		s.cliProxyAPIMode.Store(cliProxyAPIModeAll)
	case "all_force_opencode":
		s.cliProxyAPIMode.Store(cliProxyAPIModeAllForceOpenCode)
	default:
		return fmt.Errorf("invalid cli_proxy_api mode: %q", mode)
	}
	return nil
}

func (s *Service) SetOpenCodeCompatEnabled(enabled bool) {
	if s == nil {
		return
	}
	s.openCodeCompat.enabled.Store(enabled)
}

func (s *Service) SetOpenCodeCompatInstructions(instructions string) {
	if s == nil {
		return
	}
	s.openCodeCompat.instructions.Store(strings.TrimSpace(instructions))
}

func (s *Service) SetResponsesUpstreamWebSocketAllEnabled(enabled bool) {
	if s == nil {
		return
	}
	s.responsesUpstreamWS.Store(enabled)
}

func (s *Service) responsesUpstreamWebSocketAllEnabled() bool {
	return s != nil && s.responsesUpstreamWS.Load()
}

func (s *Service) UseResponsesUpstreamWebSocket(headers http.Header) bool {
	if s == nil {
		return false
	}
	if s.responsesUpstreamWebSocketAllEnabled() {
		return true
	}
	return headerTruthy(headers, internalCx2ccUpstreamResponsesWSHeader)
}

func (s *Service) ForwardResponses(ctx context.Context, inst instsvc.InstanceWithPaths, req *http.Request) (*http.Response, error) {
	finishReadBody := s.beginRequestStage(ctx, "internal", "READ", "request_body")
	body, err := io.ReadAll(req.Body)
	if err != nil {
		finishReadBody(http.StatusInternalServerError, err, nil)
		return nil, fmt.Errorf("read request body: %w", err)
	}
	finishReadBody(http.StatusOK, nil, map[string]any{"bytes": len(body)})
	_ = req.Body.Close()

	accept := headerFirst(req.Header, "Accept")
	if strings.TrimSpace(accept) == "" {
		accept = "text/event-stream"
	}

	instForResp := s.asChatGPTResponses(inst)
	if s.UseResponsesUpstreamWebSocket(req.Header) {
		return s.OpenResponsesWebSocketEventStream(ctx, instForResp, req.Header, req.URL.RawQuery, body)
	}

	opts := requestOptions{
		Accept:           accept,
		PrepareResponses: true,
	}
	return s.send(ctx, instForResp, req.Method, "/responses", req.URL.RawQuery, body, req.Header, opts)
}

func (s *Service) CallResponsesJSON(ctx context.Context, inst instsvc.InstanceWithPaths, body []byte) (*http.Response, error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	// Even when downstream wants a non-stream chat-completions response, Codex upstream still expects
	// the normal /responses SSE contract. We aggregate it back to JSON locally after the response.
	headers.Set("Accept", "text/event-stream")
	instCGPT := s.asChatGPTResponses(inst)
	if s.responsesUpstreamWebSocketAllEnabled() {
		return s.OpenResponsesWebSocketEventStream(ctx, instCGPT, headers, "", body)
	}
	options := requestOptions{Accept: "text/event-stream", PrepareResponses: true, FromChatCompat: true}
	return s.send(ctx, instCGPT, http.MethodPost, "/responses", "", body, headers, options)
}

func (s *Service) CallResponsesStream(ctx context.Context, inst instsvc.InstanceWithPaths, body []byte) (*http.Response, error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "text/event-stream")
	instCGPT := s.asChatGPTResponses(inst)
	if s.responsesUpstreamWebSocketAllEnabled() {
		return s.OpenResponsesWebSocketEventStream(ctx, instCGPT, headers, "", body)
	}
	options := requestOptions{Accept: "text/event-stream", PrepareResponses: true, FromChatCompat: true}
	return s.send(ctx, instCGPT, http.MethodPost, "/responses", "", body, headers, options)
}

func (s *Service) ForwardAny(ctx context.Context, inst instsvc.InstanceWithPaths, req *http.Request) (*http.Response, error) {
	var body []byte
	var err error
	if shouldReadBody(req.Method) {
		finishReadBody := s.beginRequestStage(ctx, "internal", "READ", "request_body")
		body, err = io.ReadAll(req.Body)
		if err != nil {
			finishReadBody(http.StatusInternalServerError, err, nil)
			return nil, fmt.Errorf("read request body: %w", err)
		}
		finishReadBody(http.StatusOK, nil, map[string]any{"bytes": len(body)})
	}
	_ = req.Body.Close()
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	// 修正路径：去掉实例前缀（如 "/2/..."），避免上游地址变成 ".../2/v1/..."
	if strings.HasPrefix(path, inst.BasePath) {
		path = strings.TrimPrefix(path, inst.BasePath)
		if path == "" {
			path = "/"
		}
	}
	opts := requestOptions{Accept: headerFirst(req.Header, "Accept")}
	// /v1/responses/compact is a non-streaming endpoint and must not include `stream` in the body.
	// Mark it so we can adjust the Responses compat logic accordingly.
	if strings.EqualFold(path, "/v1/responses/compact") || strings.EqualFold(path, "/responses/compact") {
		opts.ResponsesCompact = true
	}
	// 若是 /responses（外部标准路径），仅标记需要进行 Responses 兼容处理；
	// 不改动对外路径，由内部构建目标 URL 时选择正确的上游路径。
	if strings.Contains(path, "/responses") && strings.EqualFold(req.Method, http.MethodPost) {
		opts.PrepareResponses = true
	}
	resp, err := s.send(ctx, inst, req.Method, path, req.URL.RawQuery, body, req.Header, opts)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Service) ForwardModels(ctx context.Context, inst instsvc.InstanceWithPaths) (*http.Response, error) {
	mode := s.resolveAuthMode(inst.AuthMode)
	if mode == "api_key" {
		headers := http.Header{}
		headers.Set("Accept", "application/json")
		resp, err := s.send(ctx, inst, http.MethodGet, "/models", "", nil, headers, requestOptions{Accept: "application/json"})
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	// ChatGPT OAuth mode: return empty OpenAI-style list.
	payload := []byte(`{"object":"list","data":[]}`)
	response := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)),
	}
	response.Header.Set("Content-Type", "application/json; charset=utf-8")
	if runtime := s.runtimeForInstance(inst.ID); runtime != nil {
		_ = runtime.ClearSleepStatus()
	}
	return response, nil
}

func (s *Service) send(ctx context.Context, inst instsvc.InstanceWithPaths, method, path, rawQuery string, body []byte, original http.Header, opts requestOptions) (*http.Response, error) {
	started := time.Now()
	target, err := s.buildTargetURL(inst, path, rawQuery)
	if err != nil {
		return nil, err
	}
	mode := s.resolveAuthMode(inst.AuthMode)
	requestID := requestIDFromHeaders(original)
	preparedHeaders := sanitizeHeaders(original)
	stripInternalRequestIDHeaders(preparedHeaders)
	requestBody := body
	var promptCacheKey string
	if opts.PrepareResponses {
		finishPrepare := s.beginRequestStage(ctx, "internal", "PREPARE", "responses_body")
		var err error
		requestBody, promptCacheKey, err = s.prepareResponsesBody(requestBody, preparedHeaders, mode, opts)
		if err != nil {
			finishPrepare(http.StatusInternalServerError, err, nil)
			return nil, err
		}
		finishPrepare(http.StatusOK, nil, map[string]any{"bytes": len(requestBody)})
	}
	finishAuth := s.beginRequestStage(ctx, "auth", "AUTH", "get_auth_context")
	ctxAuth, err := s.getAuthContext(ctx, inst, mode)
	if err != nil {
		finishAuth(http.StatusInternalServerError, err, nil)
		return nil, err
	}
	finishAuth(http.StatusOK, nil, map[string]any{"mode": ctxAuth.mode})
	// 判断目标是否为 OpenAI 平台（而非 ChatGPT 后端），以对齐 header 行为
	isOpenAIPlatform := strings.Contains(target, "api.openai.com") || strings.Contains(target, "openai.azure.com")
	finalHeaders := s.applyOverrides(preparedHeaders, ctxAuth, promptCacheKey, opts, isOpenAIPlatform)
	var bodyReader io.Reader
	if requestBody != nil {
		bodyReader = bytes.NewReader(requestBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		return nil, err
	}
	if ctxAuth.mode == "chatgpt" && strings.Contains(strings.ToLower(target), "chatgpt.com/backend-api/codex") {
		req.Host = "chatgpt.com"
	}
	applyHeaders(req.Header, finalHeaders)
	// 与 codex 保持一致：使用 Authorization: Bearer <token>
	req.Header.Set("Authorization", "Bearer "+ctxAuth.token)
	// 对齐 codex：使用小写 chatgpt-account-id；仅当下游未提供同义头时补齐。
	if ctxAuth.accountID != "" {
		if req.Header.Get("ChatGPT-Account-Id") == "" && req.Header.Get("chatgpt-account-id") == "" {
			req.Header.Set("chatgpt-account-id", ctxAuth.accountID)
		}
	}

	s.logDebug("-> %s %s", method, target)
	if requestID == "" {
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] -> %s %s", started.Format(time.RFC3339), method, target))
	} else {
		limitedBody, truncated := limitBytes(requestBody, s.requestTraceBodyCaptureLimit())
		s.requestTraces.start(inst.ID, requestID, method, target, finalHeaders, limitedBody, truncated)
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] -> %s %s (request_id=%s)", started.Format(time.RFC3339), method, target, requestID))
	}
	// Debug: 记录请求头与请求体（屏蔽敏感头）
	if inst.DebugEnabled {
		s.pruneLogOneHour(inst.LogPath)
		s.logRequestDebug(inst.LogPath, requestID, method, target, finalHeaders, requestBody, inst)
	}
	finishClient := s.beginRequestStage(ctx, "internal", "BUILD", "client_for_instance")
	httpClient, err := s.clientForInstance(inst)
	if err != nil {
		finishClient(http.StatusInternalServerError, err, nil)
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] proxy error: %v", time.Now().Format(time.RFC3339), err))
		return nil, err
	}
	finishClient(http.StatusOK, nil, nil)
	resp, err := s.doUpstreamRequest(ctx, inst, httpClient, req, "upstream")
	if err != nil {
		s.handleUpstreamTransportError(inst, httpClient, err)
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] upstream error: %v", time.Now().Format(time.RFC3339), err))
		return nil, err
	}
	// Cloudflare may return a transient 403 HTML page when the instance is cold (e.g. first request after idle).
	// In practice, a quick retry usually succeeds, so do an in-place warm retry to avoid bubbling 403 to callers.
	if resp.StatusCode == http.StatusForbidden {
		contentType := strings.ToLower(strings.TrimSpace(headerFirst(resp.Header, "Content-Type")))
		if contentType == "" || !strings.Contains(contentType, "json") {
			data, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr == nil {
				resp.Body = io.NopCloser(bytes.NewReader(data))
				if looksLikeCloudflareBlockPage(data) {
					_ = resp.Body.Close()
					time.Sleep(200 * time.Millisecond)

					var retryBody io.Reader
					if requestBody != nil {
						retryBody = bytes.NewReader(requestBody)
					}
					retryReq, rerr := http.NewRequestWithContext(ctx, method, target, retryBody)
					if rerr == nil {
						if ctxAuth.mode == "chatgpt" && strings.Contains(strings.ToLower(target), "chatgpt.com/backend-api/codex") {
							retryReq.Host = "chatgpt.com"
						}
						applyHeaders(retryReq.Header, finalHeaders)
						retryReq.Header.Set("Authorization", "Bearer "+ctxAuth.token)
						if ctxAuth.accountID != "" {
							if retryReq.Header.Get("ChatGPT-Account-Id") == "" && retryReq.Header.Get("chatgpt-account-id") == "" {
								retryReq.Header.Set("chatgpt-account-id", ctxAuth.accountID)
							}
						}
						s.appendLog(inst.LogPath, fmt.Sprintf("[%s] warm retry triggered by upstream 403 (cloudflare)", time.Now().Format(time.RFC3339)))
						s.logDebug("-> warm retry %s %s", method, target)
						resp, err = s.doUpstreamRequest(ctx, inst, httpClient, retryReq, "upstream_retry")
						if err != nil {
							s.handleUpstreamTransportError(inst, httpClient, err)
							s.appendLog(inst.LogPath, fmt.Sprintf("[%s] upstream error after warm retry: %v", time.Now().Format(time.RFC3339), err))
							return nil, err
						}
					}
				}
			} else {
				resp.Body = io.NopCloser(bytes.NewReader(data))
			}
		}
	}
	// ChatGPT backend may return 403 for an expired/invalid bearer instead of 401.
	if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && ctxAuth.mode == "chatgpt" && ctxAuth.client != nil && TokenRefreshEnabled() {
		if requestID == "" {
			s.appendLog(inst.LogPath, fmt.Sprintf("[%s] auth refresh triggered by upstream %d", time.Now().Format(time.RFC3339), resp.StatusCode))
		} else {
			s.appendLog(inst.LogPath, fmt.Sprintf("[%s] auth refresh triggered by upstream %d (request_id=%s)", time.Now().Format(time.RFC3339), resp.StatusCode, requestID))
		}
		finishRefresh := s.beginRequestStage(ctx, "auth", "REFRESH", "access_token")
		if err := ctxAuth.client.refresh(ctx); err == nil {
			finishRefresh(http.StatusOK, nil, nil)
			freshToken, ferr := ctxAuth.client.getBearer(ctx)
			if ferr == nil {
				ctxAuth.token = freshToken
				if acct, aerr := ctxAuth.client.getAccountID(ctx); aerr == nil && acct != "" {
					ctxAuth.accountID = acct
				}

				_ = resp.Body.Close()

				var retryBody io.Reader
				if requestBody != nil {
					retryBody = bytes.NewReader(requestBody)
				}
				retryReq, rerr := http.NewRequestWithContext(ctx, method, target, retryBody)
				if rerr == nil {
					if ctxAuth.mode == "chatgpt" && strings.Contains(strings.ToLower(target), "chatgpt.com/backend-api/codex") {
						retryReq.Host = "chatgpt.com"
					}
					applyHeaders(retryReq.Header, finalHeaders)
					retryReq.Header.Set("Authorization", "Bearer "+ctxAuth.token)
					// 与首次请求相同：仅在下游未提供此头时设置，且统一为 chatgpt-account-id
					if ctxAuth.accountID != "" {
						if retryReq.Header.Get("ChatGPT-Account-Id") == "" && retryReq.Header.Get("chatgpt-account-id") == "" {
							retryReq.Header.Set("chatgpt-account-id", ctxAuth.accountID)
						}
					}
					s.logDebug("-> retry %s %s", method, target)
					resp, err = s.doUpstreamRequest(ctx, inst, httpClient, retryReq, "upstream_retry")
					if err != nil {
						s.handleUpstreamTransportError(inst, httpClient, err)
						s.appendLog(inst.LogPath, fmt.Sprintf("[%s] upstream error after refresh: %v", time.Now().Format(time.RFC3339), err))
						return nil, err
					}
				}
			} else {
				s.markAuthRuntimeError(inst.ID, ferr)
			}
		} else {
			finishRefresh(http.StatusInternalServerError, err, nil)
			s.markAuthRuntimeError(inst.ID, err)
		}
	}

	if resp != nil {
		s.persistCodexUsageSnapshot(ctx, inst.ID, resp.Header)
	}

	isStreamResp := resp != nil && strings.Contains(strings.ToLower(headerFirst(resp.Header, "Content-Type")), "text/event-stream")
	if requestID != "" && resp != nil {
		s.requestTraces.updateResponseMeta(inst.ID, requestID, resp.StatusCode, isStreamResp, resp.Header)
		if isStreamResp {
			resp.Body = newRequestTraceSSEReadCloser(resp.Body, s.requestTraceBodyCaptureLimit(), func(fields []string, events []string, records []RequestTraceEvent, recordsTruncated bool, deltaSuppressed int) {
				s.requestTraces.finishStream(inst.ID, requestID, fields, events, records, recordsTruncated, deltaSuppressed)
			})
		}
	}

	// Debug：根据响应类型选择记录方式
	if inst.DebugEnabled && resp != nil {
		ct := strings.ToLower(headerFirst(resp.Header, "Content-Type"))
		if strings.Contains(ct, "text/event-stream") {
			// 流式：边转发边写入日志文件，避免破坏流
			s.pruneLogOneHour(inst.LogPath)
			if f, err := os.OpenFile(inst.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				if requestID != "" {
					_, _ = fmt.Fprintf(f, "[%s] REQUEST_ID: %s\n", time.Now().Format(time.RFC3339), requestID)
				}
				// 写入响应头
				_ = writeDebugHeaders(f, "RESP", resp.StatusCode, resp.Header, inst.DebugLogRedactHeaders)
				if inst.DebugDetailEnabled {
					resp.Body = wrapSSEBodyForLogging(resp.Body, f, inst)
				} else {
					ts := time.Now().Format(time.RFC3339)
					_, _ = fmt.Fprintf(f, "[%s] RESP SSE BODY: (hidden by debug settings)\n", ts)
					_ = f.Close()
				}
			}
		} else {
			respBodyBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				respBodyBytes = nil
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
			s.pruneLogOneHour(inst.LogPath)
			s.logResponseDebug(inst.LogPath, requestID, resp.StatusCode, resp.Header, respBodyBytes, inst.DebugLogRedactHeaders)
			if requestID != "" {
				limitedBody, truncated := limitBytes(respBodyBytes, s.requestTraceBodyCaptureLimit())
				s.requestTraces.mergeResponseData(inst.ID, requestID, limitedBody, "", truncated)
			}
		}
	} else if resp != nil && resp.StatusCode >= 400 {
		// 非调试模式下，保留原有错误摘要
		respBodyBytes, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			respBodyBytes = nil
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
		if requestID != "" {
			limitedBody, truncated := limitBytes(respBodyBytes, s.requestTraceBodyCaptureLimit())
			s.requestTraces.mergeResponseData(inst.ID, requestID, limitedBody, "", truncated)
		}
		if len(respBodyBytes) > 0 {
			snippet := respBodyBytes
			if len(snippet) > 2048 {
				snippet = snippet[:2048]
			}
			s.appendLog(inst.LogPath, fmt.Sprintf("[%s] upstream error body: %s", time.Now().Format(time.RFC3339), string(snippet)))
			if s.opts.AccessLog {
				log.Printf("[proxy] upstream error body: %s", string(snippet))
			}
		}
	}

	if resp != nil && !isStreamResp {
		status := resp.StatusCode
		instanceID := inst.ID
		resp.Body = newRequestTraceJSONReadCloser(resp.Body, s.requestTraceBodyCaptureLimit(), func(payload []byte, truncated bool) {
			if requestID != "" {
				s.requestTraces.mergeResponseData(instanceID, requestID, payload, "", truncated)
			}
			if s.responseStatuses != nil {
				// Record only non-200 responses; aggregate globally across instances.
				if status != http.StatusOK {
					s.responseStatuses.record(0, status, payload, truncated)
				}
			}
		})
	}

	resp, err = s.postProcessResponse(resp, inst.ID)
	if err != nil {
		return nil, err
	}
	s.logDebug("<- status %d", resp.StatusCode)
	if s.opts.AccessLog {
		log.Printf("[proxy] %s %s -> %d %dms", method, target, resp.StatusCode, time.Since(started)/time.Millisecond)
	}
	if requestID == "" {
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] <- %d (%dms)", time.Now().Format(time.RFC3339), resp.StatusCode, time.Since(started)/time.Millisecond))
	} else {
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] <- %d (%dms) (request_id=%s)", time.Now().Format(time.RFC3339), resp.StatusCode, time.Since(started)/time.Millisecond, requestID))
	}
	return resp, nil
}

// --- Debug helpers ---
type teeReadCloser struct {
	rc io.ReadCloser
	w  io.WriteCloser
}

func (t *teeReadCloser) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 {
		_, _ = t.w.Write(p[:n])
	}
	return n, err
}

func (t *teeReadCloser) Close() error {
	err1 := t.rc.Close()
	err2 := t.w.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func redactHeaders(h http.Header) http.Header {
	out := http.Header{}
	for k, vals := range h {
		key := http.CanonicalHeaderKey(k)
		for _, v := range vals {
			if strings.EqualFold(key, "Authorization") {
				out.Add(key, "Bearer ***")
			} else if strings.EqualFold(key, "ChatGPT-Account-Id") || strings.EqualFold(key, "chatgpt-account-id") {
				out.Add(key, "***")
			} else {
				out.Add(key, v)
			}
		}
	}
	return out
}

func writeDebugHeaders(w io.Writer, kind string, status int, headers http.Header, redact bool) error {
	ts := time.Now().Format(time.RFC3339)
	if status > 0 {
		_, _ = fmt.Fprintf(w, "[%s] %s STATUS: %d\n", ts, kind, status)
	}
	toWrite := headers
	if redact {
		toWrite = redactHeaders(headers)
	}
	for k, vals := range toWrite {
		for _, v := range vals {
			_, _ = fmt.Fprintf(w, "[%s] %s HDR %s: %s\n", ts, kind, k, v)
		}
	}
	return nil
}

func (s *Service) logRequestDebug(logPath, requestID, method, target string, headers http.Header, body []byte, inst instsvc.InstanceWithPaths) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format(time.RFC3339)
	if requestID != "" {
		_, _ = fmt.Fprintf(f, "[%s] REQUEST_ID: %s\n", ts, requestID)
	}
	_, _ = fmt.Fprintf(f, "[%s] REQ %s %s\n", ts, method, target)
	if inst.DebugLogReqHeaders {
		_ = writeDebugHeaders(f, "REQ", 0, headers, inst.DebugLogRedactHeaders)
	}

	switch effectiveReqBodyMode(inst) {
	case reqBodyModeOff:
		return
	case reqBodyModeSummary:
		_, _ = fmt.Fprintf(f, "[%s] REQ BODY: ***\n", ts)
	case reqBodyModeFull:
		if len(body) > 0 {
			// 尽量写完整体（可能为 JSON）
			_, _ = fmt.Fprintf(f, "[%s] REQ BODY: %s\n", ts, string(body))
		} else {
			_, _ = fmt.Fprintf(f, "[%s] REQ BODY:\n", ts)
		}
	}
}

func (s *Service) logResponseDebug(logPath, requestID string, status int, headers http.Header, body []byte, redact bool) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if requestID != "" {
		_, _ = fmt.Fprintf(f, "[%s] REQUEST_ID: %s\n", time.Now().Format(time.RFC3339), requestID)
	}
	_ = writeDebugHeaders(f, "RESP", status, headers, redact)
	if len(body) > 0 {
		ts := time.Now().Format(time.RFC3339)
		_, _ = fmt.Fprintf(f, "[%s] RESP BODY: %s\n", ts, string(body))
	}
}

func (s *Service) pruneLogOneHour(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	lines := strings.Split(string(data), "\n")
	cutoff := time.Now().Add(-1 * time.Hour)
	keepFrom := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if len(line) < 3 {
			continue
		}
		if line[0] == '[' {
			if idx := strings.IndexRune(line, ']'); idx > 1 {
				ts := line[1:idx]
				if t, e := time.Parse(time.RFC3339, ts); e == nil {
					if t.Before(cutoff) {
						break
					}
					keepFrom = i
				}
			}
		}
	}
	if keepFrom <= 0 {
		return
	}
	trimmed := strings.Join(lines[keepFrom:], "\n")
	_ = os.WriteFile(path, []byte(trimmed), 0o644)
}

func (s *Service) postProcessResponse(resp *http.Response, instanceID int64) (*http.Response, error) {
	runtime := s.runtimeForInstance(instanceID)
	if resp == nil || runtime == nil {
		return resp, nil
	}
	if resp.StatusCode >= 400 {
		contentType := resp.Header.Get("Content-Type")
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(data))
		if err := runtime.SyncFromPayload(resp.StatusCode, contentType, data); err != nil {
			s.logDebug("runtime sync error: %v", err)
		}
		s.syncRuntimeFromCxPoolStateKeywords(runtime, resp.Header, data)
		return resp, nil
	}
	if err := runtime.ClearSleepStatus(); err != nil {
		s.logDebug("runtime clear error: %v", err)
	}
	return resp, nil
}

func normalizeCxPoolStateKeywordsConfig(cfg cxPoolStateKeywordsConfig) (cxPoolStateKeywordsConfig, error) {
	memberExpired := normalizeCxPoolKeywordList(cfg.MemberExpired)
	expired := normalizeCxPoolKeywordList(cfg.Expired)
	cooldown := normalizeCxPoolKeywordList(cfg.Cooldown)

	seen := make(map[string]string, len(memberExpired)+len(expired)+len(cooldown))
	add := func(group string, keywords []string) error {
		for _, kw := range keywords {
			key := strings.ToLower(strings.TrimSpace(kw))
			if key == "" {
				continue
			}
			if prev, ok := seen[key]; ok && prev != group {
				return fmt.Errorf("cx_pool.state_keywords duplicated across groups: %q in %s and %s", kw, prev, group)
			}
			seen[key] = group
		}
		return nil
	}
	if err := add("member_expired", memberExpired); err != nil {
		return cxPoolStateKeywordsConfig{}, err
	}
	if err := add("expired", expired); err != nil {
		return cxPoolStateKeywordsConfig{}, err
	}
	if err := add("cooldown", cooldown); err != nil {
		return cxPoolStateKeywordsConfig{}, err
	}

	return cxPoolStateKeywordsConfig{
		MemberExpired: memberExpired,
		Expired:       expired,
		Cooldown:      cooldown,
	}, nil
}

func normalizeCxPoolKeywordList(list []string) []string {
	if len(list) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, raw := range list {
		kw := strings.ToLower(strings.TrimSpace(raw))
		if kw == "" {
			continue
		}
		if _, ok := seen[kw]; ok {
			continue
		}
		seen[kw] = struct{}{}
		out = append(out, kw)
	}
	return out
}

func (s *Service) cxPoolStateKeywordsConfig() cxPoolStateKeywordsConfig {
	if s == nil {
		return cxPoolStateKeywordsConfig{}
	}
	if v := s.cxPoolStateKeywords.Load(); v != nil {
		if cfg, ok := v.(cxPoolStateKeywordsConfig); ok {
			return cfg
		}
	}
	return cxPoolStateKeywordsConfig{}
}

func (s *Service) syncRuntimeFromCxPoolStateKeywords(runtime *pruntime.Manager, headers http.Header, body []byte) {
	if runtime == nil {
		return
	}
	cfg := s.cxPoolStateKeywordsConfig()
	if len(cfg.MemberExpired) == 0 && len(cfg.Expired) == 0 && len(cfg.Cooldown) == 0 {
		return
	}

	textLower := strings.ToLower(strings.TrimSpace(string(body)))
	if textLower == "" {
		return
	}

	currentState, err := runtime.CurrentState()
	if err != nil {
		s.logDebug("runtime keyword sync state read error: %v", err)
		currentState = ""
	}

	if kw := firstMatchedKeyword(textLower, cfg.MemberExpired); kw != "" {
		if err := runtime.RecordPaymentRequired(kw); err != nil {
			s.logDebug("runtime keyword sync (member_expired) error: %v", err)
		}
		return
	}
	if kw := firstMatchedKeyword(textLower, cfg.Expired); kw != "" {
		if currentState == "payment_required" {
			return
		}
		if err := runtime.RecordExpired(kw); err != nil {
			s.logDebug("runtime keyword sync (expired) error: %v", err)
		}
		return
	}

	if kw := firstMatchedKeyword(textLower, cfg.Cooldown); kw != "" {
		if currentState == "payment_required" || currentState == "expired" {
			return
		}
		now := time.Now()
		retryAfter := parseRetryAfterSeconds(headers, now)
		if retryAfter <= 0 {
			retryAfter = parseRetryAfterSecondsFromBody(body, now)
		}
		if retryAfter <= 0 {
			return
		}
		if err := runtime.RecordUsageLimit(pruntime.UsageLimitInfo{ResetsInSeconds: retryAfter, Message: coalesce(kw, "retry_after")}); err != nil {
			s.logDebug("runtime keyword sync (cooldown) error: %v", err)
		}
		return
	}
}

func firstMatchedKeyword(haystackLower string, keywords []string) string {
	if haystackLower == "" || len(keywords) == 0 {
		return ""
	}
	for _, kw := range keywords {
		key := strings.ToLower(strings.TrimSpace(kw))
		if key == "" {
			continue
		}
		if strings.Contains(haystackLower, key) {
			return key
		}
	}
	return ""
}

func parseRetryAfterSeconds(headers http.Header, now time.Time) int {
	raw := strings.TrimSpace(headerFirst(headers, "Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds < 0 {
			return 0
		}
		return seconds
	}
	if when, err := http.ParseTime(raw); err == nil {
		d := when.Sub(now)
		if d <= 0 {
			return 0
		}
		seconds := int(d.Seconds())
		if seconds <= 0 {
			return 1
		}
		return seconds
	}
	return 0
}

func parseRetryAfterSecondsFromBody(body []byte, now time.Time) int {
	if len(body) == 0 {
		return 0
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return 0
	}

	candidates := make([]map[string]interface{}, 0, 2)
	if errObj, ok := payload["error"].(map[string]interface{}); ok && errObj != nil {
		candidates = append(candidates, errObj)
	}
	candidates = append(candidates, payload)

	for _, m := range candidates {
		if m == nil {
			continue
		}
		if seconds := firstPositiveSeconds(m, []string{
			"resets_in_seconds",
			"resetsInSeconds",
			"resets_in_secs",
			"resetsInSecs",
			"retry_after",
			"retryAfter",
			"retry_after_seconds",
			"retryAfterSeconds",
			"retry_after_secs",
			"retryAfterSecs",
		}); seconds > 0 {
			return seconds
		}

		if when := firstPositiveUnixTimestampSeconds(m, []string{
			"resets_at",
			"resetsAt",
			"reset_at",
			"resetAt",
			"retry_after_at",
			"retryAfterAt",
		}); when > 0 {
			if when <= now.Unix() {
				continue
			}
			if diff := int(when - now.Unix()); diff > 0 {
				return diff
			}
		}
	}

	return 0
}

func firstPositiveSeconds(m map[string]interface{}, keys []string) int {
	if m == nil || len(keys) == 0 {
		return 0
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if v, ok := parsePositiveIntSeconds(m[key]); ok && v > 0 {
			return v
		}
	}
	return 0
}

func parsePositiveIntSeconds(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return 0, false
		}
		return int(v), true
	case int:
		if v <= 0 {
			return 0, false
		}
		return v, true
	case int64:
		if v <= 0 {
			return 0, false
		}
		return int(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func firstPositiveUnixTimestampSeconds(m map[string]interface{}, keys []string) int64 {
	if m == nil || len(keys) == 0 {
		return 0
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if v, ok := parseUnixTimestampSeconds(m[key]); ok && v > 0 {
			return v
		}
	}
	return 0
}

func parseUnixTimestampSeconds(raw interface{}) (int64, bool) {
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return 0, false
		}
		n := int64(v)
		if n > 1_000_000_000_000 {
			return n / 1000, true
		}
		return n, true
	case int:
		if v <= 0 {
			return 0, false
		}
		n := int64(v)
		if n > 1_000_000_000_000 {
			return n / 1000, true
		}
		return n, true
	case int64:
		if v <= 0 {
			return 0, false
		}
		if v > 1_000_000_000_000 {
			return v / 1000, true
		}
		return v, true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		if n > 1_000_000_000_000 {
			return n / 1000, true
		}
		return n, true
	default:
		return 0, false
	}
}

func looksLikeCloudflareBlockPage(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "cloudflare") ||
		strings.Contains(lower, "cf-ray") ||
		strings.Contains(lower, "/cdn-cgi/") ||
		strings.Contains(lower, "cf-chl") ||
		strings.Contains(lower, "attention required") ||
		strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "verify you are human") ||
		strings.Contains(lower, "checking your browser") ||
		strings.Contains(lower, "error 1020")
}

func (s *Service) runtimeForInstance(instanceID int64) *pruntime.Manager {
	if strings.TrimSpace(s.runtimeBaseFile) == "" {
		return nil
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if runtime, ok := s.runtimeByInstance[instanceID]; ok {
		return runtime
	}
	path := runtimeFileForInstance(s.runtimeBaseFile, instanceID)
	runtime := pruntime.New(path, s.runtimeExpireAt, func() {
		if s.onRuntimeChange != nil {
			s.onRuntimeChange(instanceID)
		}
	})
	s.runtimeByInstance[instanceID] = runtime
	return runtime
}

func classifyAuthRuntimeReason(err error) string {
	if err == nil {
		return ""
	}
	var refreshErr *RefreshError
	if errors.As(err, &refreshErr) {
		code := strings.ToLower(strings.TrimSpace(refreshErr.BackendCode))
		switch code {
		case "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated":
			return "expired: " + code
		}
		if refreshErr.StatusCode == http.StatusUnauthorized {
			return "expired"
		}
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "auth not found"),
		strings.Contains(message, "auth missing"),
		strings.Contains(message, "refresh_token missing"):
		return "auth_missing"
	case strings.Contains(message, "refresh token has expired"),
		strings.Contains(message, "refresh token was revoked"),
		strings.Contains(message, "refresh token was already used"):
		return "expired"
	default:
		return ""
	}
}

func (s *Service) markAuthRuntimeError(instanceID int64, err error) {
	if s == nil || instanceID <= 0 || err == nil {
		return
	}
	reason := classifyAuthRuntimeReason(err)
	if reason == "" {
		return
	}
	runtime := s.runtimeForInstance(instanceID)
	if runtime == nil {
		return
	}
	_ = runtime.RecordExpired(reason)
}

func (s *Service) handleUpstreamTransportError(inst instsvc.InstanceWithPaths, client *http.Client, err error) {
	if s == nil || err == nil {
		closeHTTPClientIdleConnections(client)
		return
	}
	proxyKey, _, _ := proxyCacheKey(inst.Proxy)
	s.dropInstanceClient(inst, client)
	if s.transportHealth != nil && inst.ID > 0 {
		s.transportHealth.noteInstanceReset([]int64{inst.ID}, proxyKey, time.Now(), shortenTransportError(err))
	}
	if !isLikelyTransportTimeout(err) {
		return
	}
	seconds := upstreamTransportQuarantineSeconds()
	if seconds <= 0 || inst.ID <= 0 {
		return
	}
	message := shortenTransportError(err)
	if message == "" {
		message = "upstream transport timeout"
	}
	if quarantineErr := s.RecordInstanceTransportQuarantine(inst.ID, seconds, message); quarantineErr == nil {
		s.appendLog(inst.LogPath, fmt.Sprintf("[%s] runtime transport quarantine %ds after transport timeout: %s", time.Now().Format(time.RFC3339), seconds, message))
	}
}

func runtimeFileForInstance(baseFile string, instanceID int64) string {
	ext := filepath.Ext(baseFile)
	if ext != "" {
		base := strings.TrimSuffix(baseFile, ext)
		return fmt.Sprintf("%s.%d%s", base, instanceID, ext)
	}
	return fmt.Sprintf("%s.%d.json", baseFile, instanceID)
}

func (s *Service) buildTargetURL(inst instsvc.InstanceWithPaths, path, rawQuery string) (string, error) {
	base := s.resolveUpstreamBaseURL(inst.UpstreamBaseURL)
	if strings.TrimSpace(base) == "" {
		return "", errors.New("upstream base url not configured")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base = strings.TrimRight(base, "/")
	// 若上游为 ChatGPT 后端（/backend-api/codex），将对外标准路径映射到内部真实路径：
	// - /v1/responses → /responses
	// - /v1/responses/compact → /responses/compact
	// 其余路径不在此强行改写，由各入口（如 chat compat）决定是否走 Responses 聚合。
	normalizedPath := path
	lowerBase := strings.ToLower(base)
	if strings.Contains(lowerBase, "chatgpt.com/backend-api/codex") {
		if strings.EqualFold(path, "/v1/responses") {
			normalizedPath = "/responses"
		} else if strings.EqualFold(path, "/v1/responses/compact") {
			normalizedPath = "/responses/compact"
		}
	}
	target := base + normalizedPath
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return target, nil
}

func (s *Service) getAuthContext(ctx context.Context, inst instsvc.InstanceWithPaths, mode string) (*authContext, error) {
	ctxAuth := &authContext{mode: mode}
	switch mode {
	case "api_key":
		token := strings.TrimSpace(inst.UpstreamAPIKey)
		if token == "" {
			token = strings.TrimSpace(s.opts.DefaultUpstreamAPIKey)
		}
		if token == "" {
			return nil, errors.New("upstream api key not configured")
		}
		ctxAuth.token = token
	case "chatgpt":
		var auth *chatGPTAuth
		if s.opts.AuthStore != nil {
			auth = NewChatGPTAuthForInstance(s.opts.AuthStore, inst.ID, s.opts.ChatGPTClientID)
		} else {
			authFile := strings.TrimSpace(inst.AuthPath)
			if authFile == "" {
				authFile = strings.TrimSpace(s.opts.DefaultAuthFile)
			}
			if authFile == "" {
				return nil, errors.New("auth not configured")
			}
			auth = NewChatGPTAuth(authFile, s.opts.ChatGPTClientID)
		}
		if auth == nil {
			return nil, errors.New("auth manager not initialized")
		}
		token, err := auth.getBearer(ctx)
		if err != nil {
			s.markAuthRuntimeError(inst.ID, err)
			return nil, err
		}
		ctxAuth.token = token
		ctxAuth.client = auth
		ctxAuth.accountID, _ = auth.getAccountID(ctx)
		if ctxAuth.accountID == "" {
			ctxAuth.accountID = strings.TrimSpace(s.opts.ChatGPTAccountID)
		}
	default:
		return nil, fmt.Errorf("unsupported auth mode %s", mode)
	}
	return ctxAuth, nil
}

func (s *Service) applyOverrides(headers http.Header, auth *authContext, promptCacheKey string, opts requestOptions, isOpenAIPlatform bool) http.Header {
	final := cloneHeader(headers)
	isChatGPTResponses := auth != nil && auth.mode == "chatgpt" && opts.PrepareResponses
	if isChatGPTResponses {
		final = filterHeadersByAllowlist(headers, codexResponsesAllowedHeaders)
	}

	if strings.TrimSpace(opts.Accept) != "" && !isChatGPTResponses {
		final.Set("Accept", opts.Accept)
	}
	if opts.PrepareResponses {
		setResponsesBetaHeader := func() {
			existing := headerFirst(final, "OpenAI-Beta")
			if opts.ResponsesWebSocket {
				final.Set("OpenAI-Beta", normalizeResponsesWebSocketBetaHeader(existing))
				return
			}
			final.Set("OpenAI-Beta", mergeOpenAIBetaHeader(existing, "responses=experimental"))
		}
		if isChatGPTResponses {
			setResponsesBetaHeader()
			final.Set("Originator", resolveOpenAIUpstreamOriginator(
				headerFirst(headers, "User-Agent"),
				headerFirst(headers, "Originator"),
				s.opts.Originator,
			))
			if headerFirst(final, "Version") == "" {
				final.Set("Version", resolveCodexRequestVersion(headerFirst(final, "User-Agent"), s.opts.UserAgent))
			}
			if opts.ResponsesCompact {
				final.Set("Accept", "application/json")
				final.Set("session_id", resolveOpenAICompactSessionID(final, promptCacheKey))
			} else {
				final.Set("Accept", strings.TrimSpace(coalesce(opts.Accept, "text/event-stream")))
			}
		} else {
			setResponsesBetaHeader()
			if headerFirst(final, "Accept") == "" {
				final.Set("Accept", strings.TrimSpace(coalesce(opts.Accept, "text/event-stream")))
			}
		}
	}
	if !isChatGPTResponses && final.Get("Originator") == "" {
		final.Set("Originator", s.opts.Originator)
	}
	// Go 的 http.Client 会在请求缺失 UA 时自动添加 "Go-http-client/1.1"；
	// 该值并非调用方显式意图，且会导致上游识别偏差，因此视作“缺失”并用 codex 风格 UA 覆盖。
	ua := strings.TrimSpace(final.Get("User-Agent"))
	shouldForceCodexUA := ua == "" || strings.HasPrefix(ua, "Go-http-client/")
	if isChatGPTResponses && !shouldForceCodexUA && !isCodexOfficialClientByHeaders(ua, headerFirst(final, "Originator")) {
		// ChatGPT Codex upstream is stricter about request fingerprinting than the public OpenAI API.
		// For non-official callers, pin the upstream UA to a Codex-like default while still preserving
		// genuine Codex-family fingerprints from official clients.
		shouldForceCodexUA = true
	}
	if shouldForceCodexUA {
		final.Set("User-Agent", s.opts.UserAgent)
	}
	// 避免透传下游的 Accept-Encoding（Go Transport 在显式设置时不会自动解压 gzip），
	// 让上游请求维持与 codex CLI 更一致的行为。
	if isChatGPTResponses || !isOpenAIPlatform {
		final.Del("Accept-Encoding")
	}
	if promptCacheKey != "" {
		if isChatGPTResponses {
			final.Set("conversation_id", promptCacheKey)
			final.Set("session_id", promptCacheKey)
		} else {
			// 与 codex-rs 一致：仅设置下划线写法，且仅在缺失时补齐
			if final.Get("conversation_id") == "" {
				final.Set("conversation_id", promptCacheKey)
			}
			if final.Get("session_id") == "" {
				final.Set("session_id", promptCacheKey)
			}
		}
	}
	if isChatGPTResponses && headerFirst(final, "session_id") == "" {
		final.Set("session_id", resolveOpenAICompactSessionID(final, promptCacheKey))
	}
	if isChatGPTResponses && headerFirst(final, "Content-Type") == "" {
		final.Set("Content-Type", "application/json")
	}
	return final
}

func (s *Service) prepareResponsesBody(body []byte, headers http.Header, mode string, opts requestOptions) ([]byte, string, error) {
	var raw map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return body, "", nil
		}
	}
	if raw == nil {
		raw = make(map[string]interface{})
	}
	var promptKey string
	if v, ok := raw["prompt_cache_key"].(string); ok {
		promptKey = strings.TrimSpace(v)
	}
	matchesUserAgentContains := func(uaLower, pattern string) bool {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			return false
		}
		items := strings.FieldsFunc(pattern, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == ';'
		})
		for _, it := range items {
			it = strings.ToLower(strings.TrimSpace(it))
			if it == "" {
				continue
			}
			if strings.Contains(uaLower, it) {
				return true
			}
		}
		return false
	}
	shouldApplyCLIProxyUA := func(uaLower string) bool {
		patternAny := s.responsesCompat.cliProxyUserAgentContains.Load()
		pattern, _ := patternAny.(string)
		return matchesUserAgentContains(uaLower, pattern)
	}
	reqUserAgent := strings.TrimSpace(headerFirst(headers, "User-Agent"))
	reqOriginator := strings.TrimSpace(headerFirst(headers, "Originator"))
	reqUserAgentLower := strings.ToLower(reqUserAgent)
	isInternalCompatBridge := strings.EqualFold(reqUserAgent, "undici")
	// The internal undici bridge already builds a structurally valid /responses payload for cx2cc.
	// Still, Codex upstream requires a small set of fixed fields (for example stream/store and
	// a few unsupported parameters must be locked/stripped), so we apply only that narrow
	// compatibility normalization instead of the full codex-pool request rewrite.
	isCompatPassthrough := mode == "chatgpt" && isInternalCompatBridge
	isOfficialCodexClient := isCodexOfficialClientByHeaders(reqUserAgent, reqOriginator)
	resolveDefaultCodexInstructions := func() (string, error) {
		instructions := strings.TrimSpace(os.Getenv("ONEAPI_CODEX_PROMPT_CHAT_COMPLETIONS_INSTRUCTIONS"))
		if instructions == "" {
			// Fallback to the built-in Codex CLI prompt.
			instructions = promptdef.GPT5Codex
		}
		instructions = strings.TrimSpace(instructions)
		if instructions == "" {
			return "", errors.New("codex instructions not configured")
		}
		return instructions, nil
	}
	ensureResponsesInstructions := func(raw map[string]interface{}, uaLower string) error {
		if raw == nil {
			return nil
		}
		// If override is enabled, always force our base instructions.
		if s.responsesCompat.overrideInstructions.Load() {
			instructions, err := resolveDefaultCodexInstructions()
			if err != nil {
				return err
			}
			raw["instructions"] = instructions
			return nil
		}
		if _, ok := readTopLevelInstructions(raw); ok {
			return nil
		}
		raw["instructions"] = defaultCompatResponsesInstructions
		return nil
	}
	bodyPatchAny := s.responsesCompat.bodyPatch.Load()
	bodyPatchCfg, _ := bodyPatchAny.(*responsesBodyPatchConfig)
	var bodyPatch map[string]interface{}
	// Note: /responses/compact must be as close to passthrough as possible; in particular it must not
	// receive any `stream` injection/patching. Skip body patches entirely for compact requests.
	if mode == "chatgpt" && bodyPatchCfg != nil && !opts.ResponsesCompact && isCompatPassthrough {
		bodyPatch = bodyPatchCfg.patch
	}
	hasBodyPatch := len(bodyPatch) > 0

	bodyNormalized := false
	if mode == "chatgpt" {
		keepPreviousResponseID := opts.ResponsesWebSocket || needsCodexToolContinuation(raw)
		if !isCompatPassthrough {
			if applyCodexPoolResponsesTransform(raw, opts.ResponsesCompact) {
				bodyNormalized = true
			}
			if normalizeCodexPoolHTTPOnlyFields(raw, isOfficialCodexClient, keepPreviousResponseID) {
				bodyNormalized = true
			}
		} else if opts.ResponsesCompact {
			if _, ok := raw["stream"]; ok {
				delete(raw, "stream")
				bodyNormalized = true
			}
			if _, ok := raw["store"]; ok {
				delete(raw, "store")
				bodyNormalized = true
			}
		} else if normalizeCompatBridgeCodexResponsesPayload(raw, isOfficialCodexClient) {
			bodyNormalized = true
		}
	}

	if opts.ResponsesWebSocket {
		eventType, _ := raw["type"].(string)
		eventType = strings.TrimSpace(eventType)
		switch eventType {
		case "", "response.create":
			if eventType == "" {
				raw["type"] = "response.create"
				bodyNormalized = true
			}
		default:
			return nil, promptKey, fmt.Errorf("unsupported websocket request type: %s", eventType)
		}
		if _, ok := raw["background"]; ok {
			delete(raw, "background")
			bodyNormalized = true
		}
		if previousResponseID, _ := raw["previous_response_id"].(string); strings.HasPrefix(strings.TrimSpace(previousResponseID), "msg_") {
			return nil, promptKey, fmt.Errorf("previous_response_id must be a response.id (resp_*), not a message id")
		}
	}

	if mode == "chatgpt" {
		// ChatGPT backend "/backend-api/codex/responses" is not a 1:1 mirror of the OpenAI platform
		// Responses API, and it rejects some otherwise-valid parameters (e.g. max_output_tokens).
		// Keep passthrough as much as possible, but deterministically drop known-bad keys to avoid hard failures.
		deny := strings.TrimSpace(os.Getenv("ONEAPI_CODEX_CHATGPT_RESPONSES_DENY_PARAMS"))
		if deny == "" {
			deny = "max_output_tokens,max_completion_tokens,temperature,top_p,frequency_penalty,presence_penalty"
		}
		items := strings.FieldsFunc(deny, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == ';'
		})
		for _, it := range items {
			key := strings.TrimSpace(it)
			if key == "" {
				continue
			}
			if _, exists := raw[key]; exists {
				delete(raw, key)
				bodyNormalized = true
			}
		}
	}

	if isCompatPassthrough {
		if hasBodyPatch {
			mergeJSONObject(raw, bodyPatch)
			bodyNormalized = true
		}
		if bodyNormalized && normalizeCompatBridgeCodexResponsesPayload(raw, isOfficialCodexClient) {
			bodyNormalized = true
		}
		if !bodyNormalized {
			return body, promptKey, nil
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			return body, promptKey, nil
		}
		return encoded, promptKey, nil
	}

	// /responses/compact does not support `stream` / `store`. Do not inject them, and ensure they are dropped
	// even when callers include them.

	if mode == "chatgpt" && !opts.ResponsesCompact {
		if stream, ok := raw["stream"].(bool); !ok || !stream {
			raw["stream"] = true
		}
		if store, ok := raw["store"].(bool); !ok || store {
			raw["store"] = false
		}

		stream := true
		if v, ok := raw["stream"].(bool); ok {
			stream = v
		}
		if stream && !opts.FromChatCompat {
			cliProxyMode := s.cliProxyAPIMode.Load()
			if cliProxyMode != cliProxyAPIModeOff {
				userAgent := strings.TrimSpace(headerFirst(headers, "User-Agent"))
				uaLower := strings.ToLower(userAgent)
				// OpenCode / Vercel AI SDK 的 UA 可能包含：
				// - "ai-sdk/openai/"（或无尾部 "/" 的变体）
				// - "opencode/<ver>"（新版 OpenCode 已不再包含 ai-sdk/openai）
				// 默认匹配串包含 "ai-sdk/openai"、"opencode/"、"openai/js"（大小写不敏感，可在配置中修改）。
				// 这里统一识别并把传给 CLIProxyAPI 的 UA 规范化为 "ai-sdk/openai/"，
				// 以便其内部按 OpenCode 指令分支处理。
				hasOpenAISDKSlash := strings.Contains(uaLower, "ai-sdk/openai/")
				hasOpenCodeToken := strings.Contains(uaLower, "opencode/")
				isCLIProxyUA := shouldApplyCLIProxyUA(uaLower)

				apply := false
				userAgentForCPA := userAgent
				if isCLIProxyUA && !hasOpenAISDKSlash {
					userAgentForCPA = "ai-sdk/openai/"
				}
				switch cliProxyMode {
				case cliProxyAPIModeOpenCodeUA:
					apply = isCLIProxyUA
				case cliProxyAPIModeAll:
					apply = true
				case cliProxyAPIModeAllForceOpenCode:
					apply = true
					if !isCLIProxyUA {
						userAgentForCPA = "ai-sdk/openai/"
					}
				}

				if apply {
					raw["__cpa_user_agent"] = userAgentForCPA
					modelName := ""
					if v, ok := raw["model"].(string); ok {
						modelName = strings.TrimSpace(v)
					}

					// OpenCode baseurl+apikey 场景通常把系统提示词放在 system messages 中；
					// CLIProxyAPI 在指令不匹配时会把 system prompt 作为 user 消息注入（EXECUTE...），
					// 这会改变语义并导致兼容性问题。这里在服务端做一次规范化：
					// - system messages -> instructions
					// - input 去掉 system messages
					// - 仍使用 CLIProxyAPI 做结构转换，但在转换前喂给它“期望的 OpenCode 指令头”
					//   以避免其执行 EXECUTE... 注入；转换后再把 instructions 覆盖为我们拼好的指令。
					if hasOpenCodeToken && s.openCodeCompat.enabled.Load() {
						desiredInstructions, err := s.normalizeOpenCodeInstructions(raw)
						if err != nil {
							return nil, promptKey, err
						}
						cpaExpected, err := s.cliProxyAPIOpenCodeExpectedInstructions(modelName, stream)
						if err != nil {
							return nil, promptKey, err
						}
						raw["instructions"] = cpaExpected

						encoded, err := json.Marshal(raw)
						if err != nil {
							return body, promptKey, nil
						}
						translated := cpabuiltin.Registry().TranslateRequest(
							cpatranslator.FormatOpenAIResponse,
							cpatranslator.FormatCodex,
							modelName,
							encoded,
							stream,
						)
						patched, err := setTopLevelJSONStringField(translated, "instructions", desiredInstructions)
						if err != nil {
							return nil, promptKey, err
						}
						if hasBodyPatch {
							patched, err = applyJSONBodyPatchBytes(patched, bodyPatch)
							if err != nil {
								return nil, promptKey, err
							}
						}
						return patched, promptKey, nil
					}

					if err := ensureResponsesInstructions(raw, reqUserAgentLower); err != nil {
						return nil, promptKey, err
					}
					desiredInstructions, _ := raw["instructions"].(string)
					desiredInstructions = strings.TrimSpace(desiredInstructions)

					encoded, err := json.Marshal(raw)
					if err != nil {
						return body, promptKey, nil
					}
					translated := cpabuiltin.Registry().TranslateRequest(
						cpatranslator.FormatOpenAIResponse,
						cpatranslator.FormatCodex,
						modelName,
						encoded,
						stream,
					)
					if desiredInstructions != "" {
						// Ensure we don't accidentally drop required instructions after translation.
						var out map[string]interface{}
						if json.Unmarshal(translated, &out) == nil {
							translatedInstructions, _ := out["instructions"].(string)
							if strings.TrimSpace(translatedInstructions) == "" {
								patched, err := setTopLevelJSONStringField(translated, "instructions", desiredInstructions)
								if err != nil {
									return nil, promptKey, err
								}
								translated = patched
							}
						}
					}
					if hasBodyPatch {
						patched, err := applyJSONBodyPatchBytes(translated, bodyPatch)
						if err != nil {
							return nil, promptKey, err
						}
						translated = patched
					}
					return translated, promptKey, nil
				}
			}
		}
	}

	if mode == "chatgpt" {
		if err := ensureResponsesInstructions(raw, reqUserAgentLower); err != nil {
			return nil, promptKey, err
		}
	}
	if hasBodyPatch {
		mergeJSONObject(raw, bodyPatch)
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return body, promptKey, nil
	}
	return encoded, promptKey, nil
}

func (s *Service) normalizeOpenCodeInstructions(raw map[string]interface{}) (string, error) {
	if s == nil {
		return "", errors.New("proxy service not configured")
	}

	baseAny := s.openCodeCompat.instructions.Load()
	base, _ := baseAny.(string)
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("OpenCode instructions 未配置，请先在「cx模型兼容性配置」中同步")
	}

	inputAny, ok := raw["input"]
	if !ok || inputAny == nil {
		return "", errors.New("OpenCode request missing input")
	}

	originalInstructions, _ := readTopLevelInstructions(raw)

	systemText, strippedInput := extractAndStripSystemMessages(inputAny)
	raw["input"] = strippedInput

	desired := strings.TrimSpace(base)
	originalInstructions = strings.TrimSpace(originalInstructions)
	systemText = strings.TrimSpace(systemText)

	// Prefer preserving any extra instructions the client already sent, while ensuring our base header is present.
	if originalInstructions != "" && strings.HasPrefix(originalInstructions, desired) {
		desired = originalInstructions
		originalInstructions = ""
	}

	// Append any remaining instructions content (avoid duplicating the base header if present).
	if originalInstructions != "" {
		seg := originalInstructions
		if strings.HasPrefix(seg, base) {
			seg = strings.TrimSpace(strings.TrimPrefix(seg, base))
		}
		if seg != "" {
			desired = strings.TrimSpace(desired + "\n\n" + seg)
		}
	}

	// Append system/developer prompts moved out of input (avoid duplicating the base header if present).
	if systemText != "" {
		seg := systemText
		if originalInstructions != "" {
			if seg == originalInstructions {
				seg = ""
			} else if strings.HasPrefix(seg, originalInstructions+"\n\n") {
				seg = strings.TrimSpace(strings.TrimPrefix(seg, originalInstructions))
			}
		}
		if strings.HasPrefix(seg, base) {
			seg = strings.TrimSpace(strings.TrimPrefix(seg, base))
		}
		if seg != "" {
			desired = strings.TrimSpace(desired + "\n\n" + seg)
		}
	}

	return strings.TrimSpace(desired), nil
}

func extractAndStripSystemMessages(input any) (systemText string, stripped any) {
	items, ok := input.([]interface{})
	if !ok {
		return "", input
	}
	if len(items) == 0 {
		return "", items
	}

	var systemParts []string
	strippedItems := make([]interface{}, 0, len(items))
	for _, item := range items {
		msg, ok := item.(map[string]interface{})
		if !ok {
			strippedItems = append(strippedItems, item)
			continue
		}
		role, _ := msg["role"].(string)
		role = strings.TrimSpace(role)
		if strings.EqualFold(role, "system") || strings.EqualFold(role, "developer") {
			text := strings.TrimSpace(extractTextFromContent(msg["content"]))
			if text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		strippedItems = append(strippedItems, item)
	}
	return strings.TrimSpace(strings.Join(systemParts, "\n\n")), strippedItems
}

func extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var b strings.Builder
		for _, part := range v {
			m, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			text, _ := m["text"].(string)
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
		return b.String()
	case map[string]interface{}:
		text, _ := v["text"].(string)
		return text
	default:
		return ""
	}
}

func readTopLevelInstructions(raw map[string]interface{}) (string, bool) {
	if raw == nil {
		return "", false
	}
	value, ok := raw["instructions"]
	if !ok || value == nil {
		return "", false
	}
	instructions, ok := value.(string)
	if !ok {
		return "", false
	}
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return "", false
	}
	return instructions, true
}

func hasTopLevelInstructionsString(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}
	value, ok := raw["instructions"]
	if !ok || value == nil {
		return false
	}
	_, ok = value.(string)
	return ok
}

func ensureReasoningEncryptedContentInclude(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}

	const target = "reasoning.encrypted_content"
	value, exists := raw["include"]
	if !exists || value == nil {
		raw["include"] = []interface{}{target}
		return true
	}

	switch items := value.(type) {
	case []interface{}:
		for _, item := range items {
			if str, ok := item.(string); ok && strings.TrimSpace(str) == target {
				return false
			}
		}
		raw["include"] = append(items, target)
		return true
	case []string:
		for _, item := range items {
			if strings.TrimSpace(item) == target {
				return false
			}
		}
		raw["include"] = append(items, target)
		return true
	case string:
		if strings.TrimSpace(items) == target {
			return false
		}
		raw["include"] = []interface{}{items, target}
		return true
	default:
		raw["include"] = []interface{}{target}
		return true
	}
}

func normalizeCompatBridgeCodexResponsesPayload(raw map[string]interface{}, isOfficialCodexClient bool) bool {
	if raw == nil {
		return false
	}

	modified := false
	needsContinuation := needsCodexToolContinuation(raw)

	if stream, ok := raw["stream"].(bool); !ok || !stream {
		raw["stream"] = true
		modified = true
	}
	if store, ok := raw["store"].(bool); !ok || store {
		raw["store"] = false
		modified = true
	}
	if parallelToolCalls, ok := raw["parallel_tool_calls"].(bool); !ok || !parallelToolCalls {
		raw["parallel_tool_calls"] = true
		modified = true
	}
	if ensureReasoningEncryptedContentInclude(raw) {
		modified = true
	}
	if _, ok := readTopLevelInstructions(raw); !ok {
		raw["instructions"] = defaultCompatResponsesInstructions
		modified = true
	}
	if input, ok := raw["input"].([]interface{}); ok {
		raw["input"] = filterCodexPoolInput(input, needsContinuation)
		modified = true
	}

	if reasoning, ok := raw["reasoning"].(map[string]interface{}); ok {
		if effort, ok := reasoning["effort"].(string); ok && strings.EqualFold(strings.TrimSpace(effort), "minimal") {
			reasoning["effort"] = "none"
			modified = true
		}
	}

	if model, ok := raw["model"].(string); ok && !supportsCodexPoolVerbosity(model) {
		if text, ok := raw["text"].(map[string]interface{}); ok {
			if _, exists := text["verbosity"]; exists {
				delete(text, "verbosity")
				modified = true
			}
		}
	}

	if value, ok := raw["service_tier"]; ok {
		tier, _ := value.(string)
		if strings.TrimSpace(tier) != "priority" {
			delete(raw, "service_tier")
			modified = true
		}
	}

	for _, key := range []string{
		"max_output_tokens",
		"max_completion_tokens",
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		"truncation",
		"user",
		"context_management",
	} {
		if _, ok := raw[key]; ok {
			delete(raw, key)
			modified = true
		}
	}

	if normalizeCodexPoolHTTPOnlyFields(raw, isOfficialCodexClient, needsContinuation) {
		modified = true
	}

	return modified
}

func applyCodexPoolResponsesTransform(raw map[string]interface{}, isCompact bool) bool {
	if raw == nil {
		return false
	}

	modified := false
	needsContinuation := needsCodexToolContinuation(raw)

	model, _ := raw["model"].(string)
	normalizedModel := normalizeCodexPoolModel(model)
	if normalizedModel != "" && strings.TrimSpace(model) != normalizedModel {
		raw["model"] = normalizedModel
		modified = true
	}

	if isCompact {
		if _, ok := raw["store"]; ok {
			delete(raw, "store")
			modified = true
		}
		if _, ok := raw["stream"]; ok {
			delete(raw, "stream")
			modified = true
		}
	} else {
		if v, ok := raw["store"].(bool); !ok || v {
			raw["store"] = false
			modified = true
		}
		if v, ok := raw["stream"].(bool); !ok || !v {
			raw["stream"] = true
			modified = true
		}
	}

	if functionsRaw, ok := raw["functions"]; ok {
		if functions, ok := functionsRaw.([]interface{}); ok {
			tools := make([]interface{}, 0, len(functions))
			for _, f := range functions {
				tools = append(tools, map[string]interface{}{
					"type":     "function",
					"function": f,
				})
			}
			raw["tools"] = tools
		}
		delete(raw, "functions")
		modified = true
	}

	if functionCallRaw, ok := raw["function_call"]; ok {
		if functionCall, ok := functionCallRaw.(string); ok {
			raw["tool_choice"] = functionCall
		} else if functionCallObj, ok := functionCallRaw.(map[string]interface{}); ok {
			if name, ok := functionCallObj["name"].(string); ok && strings.TrimSpace(name) != "" {
				raw["tool_choice"] = map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name": name,
					},
				}
			}
		}
		delete(raw, "function_call")
		modified = true
	}

	if normalizeCodexPoolTools(raw) {
		modified = true
	}

	if input, ok := raw["input"].([]interface{}); ok {
		raw["input"] = filterCodexPoolInput(input, needsContinuation)
		modified = true
	} else if inputStr, ok := raw["input"].(string); ok {
		trimmed := strings.TrimSpace(inputStr)
		if trimmed != "" {
			raw["input"] = []interface{}{
				map[string]interface{}{
					"type":    "message",
					"role":    "user",
					"content": inputStr,
				},
			}
		} else {
			raw["input"] = []interface{}{}
		}
		modified = true
	}

	if isCompact {
		compact := make(map[string]interface{}, 4)
		for _, key := range []string{"model", "input", "instructions", "previous_response_id"} {
			value, ok := raw[key]
			if ok {
				compact[key] = value
			}
		}
		for key := range raw {
			delete(raw, key)
		}
		for key, value := range compact {
			raw[key] = value
		}
		modified = true
	}

	return modified
}

func normalizeCodexPoolHTTPOnlyFields(raw map[string]interface{}, isOfficialCodexClient bool, keepPreviousResponseID bool) bool {
	if raw == nil {
		return false
	}

	modified := false
	if reasoning, ok := raw["reasoning"].(map[string]interface{}); ok {
		if effort, ok := reasoning["effort"].(string); ok && strings.TrimSpace(strings.ToLower(effort)) == "minimal" {
			reasoning["effort"] = "none"
			modified = true
		}
	}

	if model, ok := raw["model"].(string); ok && !supportsCodexPoolVerbosity(model) {
		if text, ok := raw["text"].(map[string]interface{}); ok {
			if _, exists := text["verbosity"]; exists {
				delete(text, "verbosity")
				modified = true
			}
		}
	}

	if !isOfficialCodexClient {
		for _, key := range []string{"prompt_cache_retention", "safety_identifier"} {
			if _, ok := raw[key]; ok {
				delete(raw, key)
				modified = true
			}
		}
	}

	if !keepPreviousResponseID {
		if _, ok := raw["previous_response_id"]; ok {
			delete(raw, "previous_response_id")
			modified = true
		}
	}

	return modified
}

func normalizeCodexPoolModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "gpt-5.1"
	}

	modelID := model
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}
	if mapped, ok := codexModelMap[modelID]; ok {
		return mapped
	}

	normalized := strings.ToLower(modelID)
	for key, value := range codexModelMap {
		if strings.ToLower(key) == normalized {
			return value
		}
	}

	if strings.Contains(normalized, "gpt-5.4") || strings.Contains(normalized, "gpt 5.4") {
		return "gpt-5.4"
	}
	if strings.Contains(normalized, "gpt-5.2-codex") || strings.Contains(normalized, "gpt 5.2 codex") {
		return "gpt-5.2-codex"
	}
	if strings.Contains(normalized, "gpt-5.2") || strings.Contains(normalized, "gpt 5.2") {
		return "gpt-5.2"
	}
	if strings.Contains(normalized, "gpt-5.3-codex") || strings.Contains(normalized, "gpt 5.3 codex") {
		return "gpt-5.3-codex"
	}
	if strings.Contains(normalized, "gpt-5.3") || strings.Contains(normalized, "gpt 5.3") {
		return "gpt-5.3-codex"
	}
	if strings.Contains(normalized, "gpt-5.1-codex-max") || strings.Contains(normalized, "gpt 5.1 codex max") {
		return "gpt-5.1-codex-max"
	}
	if strings.Contains(normalized, "gpt-5.1-codex-mini") || strings.Contains(normalized, "gpt 5.1 codex mini") {
		return "gpt-5.1-codex-mini"
	}
	if strings.Contains(normalized, "codex-mini-latest") || strings.Contains(normalized, "gpt-5-codex-mini") || strings.Contains(normalized, "gpt 5 codex mini") {
		return "codex-mini-latest"
	}
	if strings.Contains(normalized, "gpt-5.1-codex") || strings.Contains(normalized, "gpt 5.1 codex") {
		return "gpt-5.1-codex"
	}
	if strings.Contains(normalized, "gpt-5.1") || strings.Contains(normalized, "gpt 5.1") {
		return "gpt-5.1"
	}
	if strings.Contains(normalized, "codex") {
		return "gpt-5.1-codex"
	}
	if strings.Contains(normalized, "gpt-5") || strings.Contains(normalized, "gpt 5") {
		return "gpt-5.1"
	}
	return "gpt-5.1"
}

func supportsCodexPoolVerbosity(model string) bool {
	if !strings.HasPrefix(model, "gpt-") {
		return true
	}

	var major, minor int
	n, _ := fmt.Sscanf(model, "gpt-%d.%d", &major, &minor)
	if major > 5 {
		return true
	}
	if major < 5 {
		return false
	}
	if n == 1 {
		return true
	}
	return minor >= 3
}

func needsCodexToolContinuation(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}
	if hasNonEmptyString(raw["previous_response_id"]) {
		return true
	}
	if hasToolsSignal(raw) || hasToolChoiceSignal(raw) {
		return true
	}
	input, ok := raw["input"].([]interface{})
	if !ok {
		return false
	}
	for _, item := range input {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		if itemType == "function_call_output" || itemType == "item_reference" {
			return true
		}
	}
	return false
}

func hasNonEmptyString(value interface{}) bool {
	str, ok := value.(string)
	return ok && strings.TrimSpace(str) != ""
}

func hasToolsSignal(raw map[string]interface{}) bool {
	tools, ok := raw["tools"].([]interface{})
	return ok && len(tools) > 0
}

func hasToolChoiceSignal(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}
	value, ok := raw["tool_choice"]
	if !ok || value == nil {
		return false
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str) != ""
	}
	if obj, ok := value.(map[string]interface{}); ok {
		return len(obj) > 0
	}
	return false
}

func filterCodexPoolInput(input []interface{}, preserveReferences bool) []interface{} {
	filtered := make([]interface{}, 0, len(input))
	for _, item := range input {
		msg, ok := item.(map[string]interface{})
		if !ok {
			filtered = append(filtered, item)
			continue
		}

		itemType, _ := msg["type"].(string)
		fixCallIDPrefix := func(id string) string {
			if id == "" || strings.HasPrefix(id, "fc") {
				return id
			}
			if strings.HasPrefix(id, "call_") {
				return "fc" + strings.TrimPrefix(id, "call_")
			}
			return "fc_" + id
		}

		if itemType == "item_reference" {
			if !preserveReferences {
				continue
			}
			next := make(map[string]interface{}, len(msg))
			for key, value := range msg {
				next[key] = value
			}
			if id, ok := next["id"].(string); ok && strings.HasPrefix(id, "call_") {
				next["id"] = fixCallIDPrefix(id)
			}
			filtered = append(filtered, next)
			continue
		}

		next := msg
		copied := false
		ensureCopy := func() {
			if copied {
				return
			}
			next = make(map[string]interface{}, len(msg))
			for key, value := range msg {
				next[key] = value
			}
			copied = true
		}

		if isCodexPoolToolCallItemType(itemType) {
			callID, ok := msg["call_id"].(string)
			if !ok || strings.TrimSpace(callID) == "" {
				if id, ok := msg["id"].(string); ok && strings.TrimSpace(id) != "" {
					callID = id
					ensureCopy()
					next["call_id"] = callID
				}
			}
			if callID != "" {
				fixedCallID := fixCallIDPrefix(callID)
				if fixedCallID != callID {
					ensureCopy()
					next["call_id"] = fixedCallID
				}
			}
		}

		if !preserveReferences {
			ensureCopy()
			delete(next, "id")
			if !isCodexPoolToolCallItemType(itemType) {
				delete(next, "call_id")
			}
		}

		filtered = append(filtered, next)
	}
	return filtered
}

func isCodexPoolToolCallItemType(itemType string) bool {
	if itemType == "" {
		return false
	}
	return strings.HasSuffix(itemType, "_call") || strings.HasSuffix(itemType, "_call_output")
}

func normalizeCodexPoolTools(raw map[string]interface{}) bool {
	toolsRaw, ok := raw["tools"]
	if !ok || toolsRaw == nil {
		return false
	}
	tools, ok := toolsRaw.([]interface{})
	if !ok {
		return false
	}

	modified := false
	validTools := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			validTools = append(validTools, tool)
			continue
		}

		toolType, _ := toolMap["type"].(string)
		toolType = strings.TrimSpace(toolType)
		if toolType != "function" {
			validTools = append(validTools, toolMap)
			continue
		}
		if name, ok := toolMap["name"].(string); ok && strings.TrimSpace(name) != "" {
			validTools = append(validTools, toolMap)
			continue
		}

		functionValue, hasFunction := toolMap["function"]
		function, ok := functionValue.(map[string]interface{})
		if !hasFunction || functionValue == nil || !ok || function == nil {
			modified = true
			continue
		}

		if _, ok := toolMap["name"]; !ok {
			if name, ok := function["name"].(string); ok && strings.TrimSpace(name) != "" {
				toolMap["name"] = name
				modified = true
			}
		}
		if _, ok := toolMap["description"]; !ok {
			if desc, ok := function["description"].(string); ok && strings.TrimSpace(desc) != "" {
				toolMap["description"] = desc
				modified = true
			}
		}
		if _, ok := toolMap["parameters"]; !ok {
			if params, ok := function["parameters"]; ok {
				toolMap["parameters"] = params
				modified = true
			}
		}
		if _, ok := toolMap["strict"]; !ok {
			if strict, ok := function["strict"]; ok {
				toolMap["strict"] = strict
				modified = true
			}
		}

		validTools = append(validTools, toolMap)
	}

	if modified {
		raw["tools"] = validTools
	}
	return modified
}

func (s *Service) cliProxyAPIOpenCodeExpectedInstructions(modelName string, stream bool) (string, error) {
	if s == nil {
		return "", errors.New("proxy service not configured")
	}
	if v := s.openCodeCompat.cpaExpected.Load(); v != nil {
		if cached, ok := v.(string); ok && strings.TrimSpace(cached) != "" {
			return cached, nil
		}
	}

	// 通过 CLIProxyAPI 自身的转换结果反推出其“期望的 OpenCode 指令头”，避免硬编码依赖。
	sample := map[string]interface{}{
		"model":            strings.TrimSpace(coalesce(modelName, "gpt-5-codex")),
		"stream":           true,
		"store":            false,
		"__cpa_user_agent": "ai-sdk/openai/",
		"input": []interface{}{
			map[string]interface{}{
				"type": "message",
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "input_text",
						"text": "ping",
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(sample)
	if err != nil {
		return "", fmt.Errorf("marshal cpa sample: %w", err)
	}
	translated := cpabuiltin.Registry().TranslateRequest(
		cpatranslator.FormatOpenAIResponse,
		cpatranslator.FormatCodex,
		modelName,
		encoded,
		stream,
	)
	var out map[string]interface{}
	if err := json.Unmarshal(translated, &out); err != nil {
		return "", fmt.Errorf("unmarshal cpa sample: %w", err)
	}
	instructions, _ := out["instructions"].(string)
	if strings.TrimSpace(instructions) == "" {
		return "", errors.New("failed to resolve CLIProxyAPI OpenCode instructions")
	}
	s.openCodeCompat.cpaExpected.Store(instructions)
	return instructions, nil
}

func setTopLevelJSONStringField(raw []byte, key string, value string) ([]byte, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unmarshal translated body: %w", err)
	}
	obj[key] = value
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal translated body: %w", err)
	}
	return encoded, nil
}

func applyJSONBodyPatchBytes(raw []byte, patch map[string]interface{}) ([]byte, error) {
	if len(patch) == 0 {
		return raw, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unmarshal body for patch: %w", err)
	}
	if obj == nil {
		obj = make(map[string]interface{})
	}
	mergeJSONObject(obj, patch)
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal body after patch: %w", err)
	}
	return encoded, nil
}

func mergeJSONObject(dst map[string]interface{}, patch map[string]interface{}) {
	if dst == nil || len(patch) == 0 {
		return
	}
	for key, patchVal := range patch {
		if patchObj, ok := patchVal.(map[string]interface{}); ok {
			if existingVal, ok := dst[key]; ok {
				if existingObj, ok := existingVal.(map[string]interface{}); ok {
					mergeJSONObject(existingObj, patchObj)
					continue
				}
			}
			dst[key] = deepCopyJSONValue(patchObj)
			continue
		}
		dst[key] = deepCopyJSONValue(patchVal)
	}
}

func deepCopyJSONValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, vv := range t {
			out[k] = deepCopyJSONValue(vv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, vv := range t {
			out[i] = deepCopyJSONValue(vv)
		}
		return out
	default:
		return v
	}
}

func (s *Service) resolveUpstreamBaseURL(candidate string) string {
	base := strings.TrimSpace(candidate)
	if base != "" {
		return base
	}
	return strings.TrimSpace(s.opts.DefaultUpstreamBaseURL)
}

// asChatGPTResponses 返回一个用于调用 ChatGPT 后端 /responses 的实例拷贝：
// - 强制 AuthMode 为 chatgpt；
// - 上游基址优先取 ResponsesBaseURL（若其指向 api.openai.com 将被忽略）；
// - 否则固定为 https://chatgpt.com/backend-api/codex。
func (s *Service) asChatGPTResponses(inst instsvc.InstanceWithPaths) instsvc.InstanceWithPaths {
	inst2 := inst
	inst2.AuthMode = "chatgpt"
	base := strings.TrimSpace(s.opts.ResponsesBaseURL)
	if base == "" || strings.Contains(strings.ToLower(base), "api.openai.com") {
		inst2.UpstreamBaseURL = "https://chatgpt.com/backend-api/codex"
	} else {
		inst2.UpstreamBaseURL = base
	}
	return inst2
}

func (s *Service) resolveAuthMode(candidate string) string {
	mode := strings.TrimSpace(strings.ToLower(candidate))
	if mode != "" {
		return mode
	}
	return strings.TrimSpace(strings.ToLower(s.opts.DefaultAuthMode))
}

func (s *Service) logDebug(format string, args ...interface{}) {
	if !s.opts.Debug {
		return
	}
	log.Printf("[proxy:debug] "+format, args...)
}

func (s *Service) appendLog(path string, line string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}

package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	common2 "one-api/common"
	globalconst "one-api/constant"
	"one-api/logger"
	"one-api/relay/common"
	"one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/operation_setting"
	"one-api/types"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	xnetproxy "golang.org/x/net/proxy"
)

var hopByHopRequestHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Content-Length":      {},
	"Host":                {},
}

type replayableRequestBody interface {
	io.Reader
	ContentLength() int64
	GetBody() (io.ReadCloser, error)
}

func parseConnectionHeaderTokens(values []string) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			token := http.CanonicalHeaderKey(strings.TrimSpace(part))
			if token == "" {
				continue
			}
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

func SetupPassThroughRequestHeader(c *gin.Context, req *http.Header, strippedKeys ...string) {
	if c == nil || c.Request == nil || req == nil {
		return
	}

	blockedHeaders := make(map[string]struct{}, len(hopByHopRequestHeaders))
	for key := range hopByHopRequestHeaders {
		blockedHeaders[key] = struct{}{}
	}
	for key := range parseConnectionHeaderTokens(c.Request.Header.Values("Connection")) {
		blockedHeaders[key] = struct{}{}
	}

	clientStrippedHeaders := make(map[string]struct{}, len(strippedKeys))
	for _, key := range strippedKeys {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(key))
		if canonical == "" {
			continue
		}
		clientStrippedHeaders[canonical] = struct{}{}
	}

	existingHeaders := http.Header{}
	if *req != nil {
		existingHeaders = (*req).Clone()
	}
	outHeaders := make(http.Header, len(c.Request.Header)+len(existingHeaders))

	for key, values := range c.Request.Header {
		canonical := http.CanonicalHeaderKey(key)
		if _, blocked := blockedHeaders[canonical]; blocked {
			continue
		}
		if _, stripped := clientStrippedHeaders[canonical]; stripped {
			continue
		}
		if len(values) == 0 {
			continue
		}
		outHeaders[canonical] = append([]string(nil), values...)
	}

	for key, values := range existingHeaders {
		canonical := http.CanonicalHeaderKey(key)
		if _, blocked := blockedHeaders[canonical]; blocked {
			continue
		}
		if len(values) == 0 {
			continue
		}
		outHeaders[canonical] = append([]string(nil), values...)
	}

	if *req == nil {
		*req = make(http.Header, len(outHeaders))
	} else {
		for key := range *req {
			delete(*req, key)
		}
	}
	for key, values := range outHeaders {
		(*req)[key] = values
	}
}

func SetupApiRequestHeader(info *common.RelayInfo, c *gin.Context, req *http.Header) {
	if info.RelayMode == constant.RelayModeAudioTranscription || info.RelayMode == constant.RelayModeAudioTranslation {
		// multipart/form-data
	} else if info.RelayMode == constant.RelayModeRealtime {
		// websocket
	} else {
		channelMessagesCompat := common2.GetContextKeyBool(c, globalconst.ContextKeyChannelMessagesToResponsesCompat)
		forceResponsesStream := common2.GetContextKeyBool(c, globalconst.ContextKeyResponsesForceUpstreamStream)
		isChannelCx2ccResponses := channelMessagesCompat &&
			info.RelayMode == constant.RelayModeResponses &&
			info.RelayFormat == types.RelayFormatClaude

		if isChannelCx2ccResponses {
			if info.IsStream || forceResponsesStream {
				req.Set("Accept", "text/event-stream")
			} else {
				req.Set("Accept", "application/json")
			}
			// Keep the upstream leg on exact JSON media type. Generic passthrough may leave it
			// empty or add charset parameters, but the codex-style responses backends are stricter.
			req.Set("Content-Type", "application/json")
			// Channel-level cx2cc already normalizes the body into codex-compatible Responses
			// semantics, so the matching minimal Responses headers should not remain managed-only.
			if req.Get("OpenAI-Beta") == "" {
				req.Set("OpenAI-Beta", "responses=experimental")
			}
			if req.Get("originator") == "" {
				req.Set("originator", "codex_cli_rs")
			}
		}

		if isChannelCx2ccResponses {
			if rid := strings.TrimSpace(c.GetString(common2.RequestIdKey)); rid != "" {
				req.Set(common2.RequestIdKey, rid)
			}
			if ua := c.Request.Header.Get("User-Agent"); ua != "" {
				req.Set("User-Agent", ua)
			}
			for _, key := range []string{
				"ChatGPT-Account-ID",
				"session_id",
				"conversation_id",
				"originator",
				"openai-beta",
				"accept-language",
				"x-codex-beta-features",
				"x-codex-turn-state",
				"x-codex-turn-metadata",
				"x-oai-web-search-eligible",
				"x-openai-subagent",
			} {
				if v := c.Request.Header.Get(key); v != "" {
					req.Set(key, v)
				}
			}
			return
		}

		// 用于链路追踪：将 Transfer API 的 request_id 透传给上游（codex-service-go 会用它关联实例日志）。
		if rid := strings.TrimSpace(c.GetString(common2.RequestIdKey)); rid != "" {
			req.Set(common2.RequestIdKey, rid)
		}
		// 透传客户端 UA，避免 Go 默认的 "Go-http-client/1.1" 污染上游请求特征（对 codex-service-go 特别关键）。
		if ua := c.Request.Header.Get("User-Agent"); ua != "" {
			req.Set("User-Agent", ua)
		}
		req.Set("Content-Type", c.Request.Header.Get("Content-Type"))
		req.Set("Accept", c.Request.Header.Get("Accept"))
		if info.RelayMode == constant.RelayModeResponses && forceResponsesStream && info.ClientWs == nil {
			req.Set("Accept", "text/event-stream")
		}
		acceptLower := strings.ToLower(strings.TrimSpace(req.Get("Accept")))
		if acceptLower == "" {
			if info.IsStream {
				req.Set("Accept", "text/event-stream")
			} else if info.RelayMode == constant.RelayModeResponses {
				// Responses API non-stream response is JSON; some upstreams (e.g. codex-service-go /v1/responses)
				// may default to SSE when Accept is empty/weak (like */*). Be explicit to avoid receiving SSE
				// and then attempting to decode it as JSON.
				req.Set("Accept", "application/json")
			} else {
				// Missing Accept means "accept anything" per HTTP semantics; use */* explicitly.
				// This also avoids upstreams that treat an empty Accept as a streaming hint (e.g. codex-service-go /v1/responses).
				req.Set("Accept", "*/*")
			}
		} else if info.IsStream && !strings.Contains(acceptLower, "text/event-stream") {
			// For streaming requests, be explicit about SSE even when callers send broad accepts (like */*).
			// This avoids upstreams that decide streaming vs JSON based on Accept.
			req.Set("Accept", "text/event-stream")
		} else if info.RelayMode == constant.RelayModeResponses && !info.IsStream {
			// Browsers/clients commonly send "Accept: */*" by default; for non-stream /v1/responses we need JSON.
			// codex-service-go/ChatGPT backend may return SSE when Accept is too broad.
			if strings.HasPrefix(acceptLower, "*/*") &&
				!strings.Contains(acceptLower, "text/event-stream") &&
				!strings.Contains(acceptLower, "application/json") {
				req.Set("Accept", "application/json")
			}
		}
		for _, key := range []string{
			"ChatGPT-Account-ID",
			"session_id",
			"x-codex-beta-features",
			"x-codex-turn-state",
			"x-oai-web-search-eligible",
			"x-openai-subagent",
		} {
			if v := c.Request.Header.Get(key); v != "" {
				req.Set(key, v)
			}
		}
	}
}

func DoApiRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if common2.DebugEnabled {
		println("fullRequestURL:", fullRequestURL)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	if replayableBody, ok := requestBody.(replayableRequestBody); ok {
		req.ContentLength = replayableBody.ContentLength()
		req.GetBody = replayableBody.GetBody
	}
	headers := req.Header
	headerOverride, err := common.ResolveHeaderOverride(c, info, info.HeadersOverride)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid)
	}
	for key, value := range headerOverride {
		headers.Set(key, value)
	}
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	req.Header = headers
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

func DoFormRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	if common2.DebugEnabled {
		println("fullRequestURL:", fullRequestURL)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	if replayableBody, ok := requestBody.(replayableRequestBody); ok {
		req.ContentLength = replayableBody.ContentLength()
		req.GetBody = replayableBody.GetBody
	}
	// set form data
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	headers := req.Header
	headerOverride, err := common.ResolveHeaderOverride(c, info, info.HeadersOverride)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid)
	}
	for key, value := range headerOverride {
		headers.Set(key, value)
	}
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	req.Header = headers
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

func DoWssRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*websocket.Conn, error) {
	targetConn, _, err := DoWssRequestWithResponse(a, c, info, requestBody)
	if err != nil {
		return nil, err
	}
	return targetConn, nil
}

func DoWssRequestWithResponse(a Adaptor, c *gin.Context, info *common.RelayInfo, _ io.Reader) (*websocket.Conn, *http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, nil, fmt.Errorf("get request url failed: %w", err)
	}
	targetHeader := http.Header{}
	err = a.SetupRequestHeader(c, &targetHeader, info)
	if err != nil {
		return nil, nil, fmt.Errorf("setup request header failed: %w", err)
	}
	targetHeader.Set("Content-Type", c.Request.Header.Get("Content-Type"))

	// Build websocket dialer honoring channel proxy settings
	dialer := *websocket.DefaultDialer // copy to avoid mutating global default
	if info.ChannelSetting.Proxy != "" {
		purl, perr := url.Parse(info.ChannelSetting.Proxy)
		if perr != nil {
			return nil, nil, fmt.Errorf("invalid proxy url: %w", perr)
		}
		switch purl.Scheme {
		case "http", "https":
			dialer.Proxy = http.ProxyURL(purl)
		case "socks5", "socks5h":
			var auth *xnetproxy.Auth
			if purl.User != nil {
				user := purl.User.Username()
				pass, _ := purl.User.Password()
				auth = &xnetproxy.Auth{User: user, Password: pass}
			}
			sd, derr := xnetproxy.SOCKS5("tcp", purl.Host, auth, xnetproxy.Direct)
			if derr != nil {
				return nil, nil, fmt.Errorf("create socks5 dialer failed: %w", derr)
			}
			dialer.NetDial = sd.Dial
		default:
			return nil, nil, fmt.Errorf("unsupported proxy scheme for websocket: %s", purl.Scheme)
		}
	}
	if common2.RelayTimeout > 0 {
		dialer.HandshakeTimeout = time.Duration(common2.RelayTimeout) * time.Second
	}

	targetConn, resp, err := dialer.DialContext(c.Request.Context(), fullRequestURL, targetHeader)
	if err != nil {
		return nil, resp, fmt.Errorf("dial failed to %s: %w", fullRequestURL, err)
	}
	// send request body
	//all, err := io.ReadAll(requestBody)
	//err = service.WssString(c, targetConn, string(all))
	return targetConn, resp, nil
}

func startPingKeepAlive(c *gin.Context, pingInterval time.Duration) context.CancelFunc {
	pingerCtx, stopPinger := context.WithCancel(context.Background())

	gopool.Go(func() {
		defer func() {
			// 增加panic恢复处理
			if r := recover(); r != nil {
				if common2.DebugEnabled {
					println("SSE ping goroutine panic recovered:", fmt.Sprintf("%v", r))
				}
			}
			if common2.DebugEnabled {
				println("SSE ping goroutine stopped.")
			}
		}()

		if pingInterval <= 0 {
			pingInterval = helper.DefaultPingInterval
		}

		ticker := time.NewTicker(pingInterval)
		// 确保在任何情况下都清理ticker
		defer func() {
			ticker.Stop()
			if common2.DebugEnabled {
				println("SSE ping ticker stopped")
			}
		}()

		var pingMutex sync.Mutex
		if common2.DebugEnabled {
			println("SSE ping goroutine started")
		}

		// 增加超时控制，防止goroutine长时间运行
		maxPingDuration := 120 * time.Minute // 最大ping持续时间
		pingTimeout := time.NewTimer(maxPingDuration)
		defer pingTimeout.Stop()

		for {
			select {
			// 发送 ping 数据
			case <-ticker.C:
				if err := sendPingData(c, &pingMutex); err != nil {
					if common2.DebugEnabled {
						println("SSE ping error, stopping goroutine:", err.Error())
					}
					return
				}
			// 收到退出信号
			case <-pingerCtx.Done():
				return
			// request 结束
			case <-c.Request.Context().Done():
				return
			// 超时保护，防止goroutine无限运行
			case <-pingTimeout.C:
				if common2.DebugEnabled {
					println("SSE ping goroutine timeout, stopping")
				}
				return
			}
		}
	})

	return stopPinger
}

func sendPingData(c *gin.Context, mutex *sync.Mutex) error {
	// 增加超时控制，防止锁死等待
	done := make(chan error, 1)
	go func() {
		mutex.Lock()
		defer mutex.Unlock()

		err := helper.PingData(c)
		if err != nil {
			logger.LogError(c, "SSE ping error: "+err.Error())
			done <- err
			return
		}

		if common2.DebugEnabled {
			println("SSE ping data sent.")
		}
		done <- nil
	}()

	// 设置发送ping数据的超时时间
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("SSE ping data send timeout")
	case <-c.Request.Context().Done():
		return errors.New("request context cancelled during ping")
	}
}

func DoRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	return doRequest(c, req, info)
}
func doRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	client, err := service.NewProxyHttpClient(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}

	var stopPinger context.CancelFunc
	if info.IsStream {
		helper.SetEventStreamHeaders(c)
		// 处理流式请求的 ping 保活
		generalSettings := operation_setting.GetGeneralSetting()
		if generalSettings.PingIntervalEnabled && !info.DisablePing {
			pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
			stopPinger = startPingKeepAlive(c, pingInterval)
			// 使用defer确保在任何情况下都能停止ping goroutine
			defer func() {
				if stopPinger != nil {
					stopPinger()
					if common2.DebugEnabled {
						println("SSE ping goroutine stopped by defer")
					}
				}
			}()
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	cost := time.Since(start)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("upstream request failed: channel=%d method=%s url=%s cost=%s err=%v",
			c.GetInt("channel_id"), req.Method, req.URL.String(), cost.String(), err))
		if errors.Is(err, context.Canceled) {
			// The request context was canceled (client disconnected / upstream chain canceled).
			// Retrying is pointless because the same context will be canceled for subsequent attempts.
			return nil, types.NewError(err, types.ErrorCodeDoRequestFailed,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithHideErrMsg("request canceled"),
			)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			// The request context hit its deadline. Avoid retry storms under slow/overloaded conditions.
			return nil, types.NewError(err, types.ErrorCodeDoRequestFailed,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithHideErrMsg("request timeout"),
			)
		}
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithHideErrMsg("upstream error: do request failed"))
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}
	if cost >= 10*time.Second {
		logger.LogWarn(c, fmt.Sprintf("upstream response headers slow: channel=%d method=%s url=%s status=%d cost=%s",
			c.GetInt("channel_id"), req.Method, req.URL.String(), resp.StatusCode, cost.String()))
	}

	_ = req.Body.Close()
	_ = c.Request.Body.Close()
	return resp, nil
}

func DoTaskApiRequest(a TaskAdaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(requestBody), nil
	}

	err = a.BuildRequestHeader(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

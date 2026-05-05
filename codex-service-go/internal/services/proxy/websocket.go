package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	instsvc "codex-service-go/internal/services/instances"
	"codex-service-go/pkg/proxyurl"

	"github.com/gorilla/websocket"
	xproxy "golang.org/x/net/proxy"
)

const openAIResponsesWSBetaV2 = "responses_websockets=2026-02-06"

func (s *Service) OpenResponsesWebSocket(ctx context.Context, inst instsvc.InstanceWithPaths, original http.Header, rawQuery string, firstPayload []byte) (*websocket.Conn, []byte, error) {
	conn, resp, normalizedPayload, err := s.openResponsesWebSocketWithHandshake(ctx, inst, original, rawQuery, firstPayload)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, nil, err
	}
	return conn, normalizedPayload, nil
}

func (s *Service) openResponsesWebSocketWithHandshake(ctx context.Context, inst instsvc.InstanceWithPaths, original http.Header, rawQuery string, firstPayload []byte) (*websocket.Conn, *http.Response, []byte, error) {
	trimmed := bytes.TrimSpace(firstPayload)
	if len(trimmed) == 0 {
		return nil, nil, nil, fmt.Errorf("empty websocket request payload")
	}
	if !json.Valid(trimmed) {
		return nil, nil, nil, fmt.Errorf("invalid websocket request payload")
	}

	mode := s.resolveAuthMode(inst.AuthMode)
	preparedHeaders := sanitizeHeaders(original)
	stripInternalRequestIDHeaders(preparedHeaders)
	if headerFirst(preparedHeaders, "Content-Type") == "" {
		preparedHeaders.Set("Content-Type", "application/json")
	}

	opts := requestOptions{
		Accept:             "text/event-stream",
		PrepareResponses:   true,
		ResponsesWebSocket: true,
	}
	normalizedPayload, promptCacheKey, err := s.prepareResponsesBody(trimmed, preparedHeaders, mode, opts)
	if err != nil {
		return nil, nil, nil, err
	}

	target, err := s.buildWebSocketTargetURL(inst, "/responses", rawQuery)
	if err != nil {
		return nil, nil, nil, err
	}

	ctxAuth, err := s.getAuthContext(ctx, inst, mode)
	if err != nil {
		return nil, nil, nil, err
	}
	isOpenAIPlatform := strings.Contains(target, "api.openai.com") || strings.Contains(target, "openai.azure.com")
	finalHeaders := s.applyOverrides(preparedHeaders, ctxAuth, promptCacheKey, opts, isOpenAIPlatform)
	finalHeaders.Set("Authorization", "Bearer "+ctxAuth.token)
	finalHeaders.Set("OpenAI-Beta", mergeOpenAIBetaHeader(headerFirst(finalHeaders, "OpenAI-Beta"), openAIResponsesWSBetaV2))
	if ctxAuth.accountID != "" {
		if finalHeaders.Get("ChatGPT-Account-Id") == "" && finalHeaders.Get("chatgpt-account-id") == "" {
			finalHeaders.Set("chatgpt-account-id", ctxAuth.accountID)
		}
	}
	if ctxAuth.mode == "chatgpt" && !isCodexOfficialClientByHeaders(headerFirst(finalHeaders, "User-Agent"), headerFirst(finalHeaders, "Originator")) {
		finalHeaders.Set("User-Agent", s.opts.UserAgent)
	}

	dialer, err := websocketDialerForInstance(inst)
	if err != nil {
		return nil, nil, nil, err
	}
	conn, resp, err := dialer.DialContext(ctx, target, finalHeaders)
	if err != nil {
		return nil, resp, nil, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	conn.EnableWriteCompression(false)
	return conn, nil, normalizedPayload, nil
}

func (s *Service) PrepareResponsesWebSocketPayload(inst instsvc.InstanceWithPaths, original http.Header, payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty websocket request payload")
	}
	if !json.Valid(trimmed) {
		return nil, fmt.Errorf("invalid websocket request payload")
	}

	headers := sanitizeHeaders(original)
	if headerFirst(headers, "Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}
	normalizedPayload, _, err := s.prepareResponsesBody(trimmed, headers, s.resolveAuthMode(inst.AuthMode), requestOptions{
		Accept:             "text/event-stream",
		PrepareResponses:   true,
		ResponsesWebSocket: true,
	})
	if err != nil {
		return nil, err
	}
	return normalizedPayload, nil
}

func (s *Service) buildWebSocketTargetURL(inst instsvc.InstanceWithPaths, path, rawQuery string) (string, error) {
	target, err := s.buildTargetURL(inst, path, rawQuery)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
	default:
		return "", fmt.Errorf("unsupported scheme for websocket: %s", parsed.Scheme)
	}
	return parsed.String(), nil
}

func websocketDialerForInstance(inst instsvc.InstanceWithPaths) (*websocket.Dialer, error) {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = upstreamResponseHeaderTimeout()
	dialer.EnableCompression = true
	dialer.NetDialContext = newUpstreamNetDialer().DialContext

	cacheKey, parsedURL, err := proxyCacheKey(inst.Proxy)
	_ = cacheKey
	if err != nil {
		return nil, err
	}
	if parsedURL == nil {
		return &dialer, nil
	}

	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "http", "https":
		dialer.Proxy = http.ProxyURL(parsedURL)
		return &dialer, nil
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
		socksDialer, err := xproxy.SOCKS5("tcp", parsedURL.Host, auth, newUpstreamNetDialer())
		if err != nil {
			return nil, err
		}
		if contextDialer, ok := socksDialer.(xproxy.ContextDialer); ok {
			dialer.NetDialContext = contextDialer.DialContext
		} else {
			dialer.NetDialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
				return socksDialer.Dial(network, addr)
			}
		}
		dialer.Proxy = nil
		return &dialer, nil
	default:
		validatedURL, err := proxyurl.Parse(inst.Proxy)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unsupported proxy scheme: %s", validatedURL.Scheme)
	}
}

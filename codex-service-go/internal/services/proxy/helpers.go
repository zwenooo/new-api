package proxy

import (
	"net/http"
	"strings"
)

const internalCx2ccUpstreamResponsesWSHeader = "X-Oneapi-Cx2cc-Upstream-Responses-Websocket"

type requestOptions struct {
	Accept             string
	PrepareResponses   bool
	FromChatCompat     bool
	ResponsesCompact   bool
	ResponsesWebSocket bool
}

type authContext struct {
	mode      string
	token     string
	accountID string
	client    *chatGPTAuth
}

func cloneHeader(h http.Header) http.Header {
	dup := make(http.Header, len(h))
	for k, vals := range h {
		for _, v := range vals {
			dup.Add(k, v)
		}
	}
	return dup
}

func sanitizeHeaders(src http.Header) http.Header {
	dst := http.Header{}
	for key, vals := range src {
		lower := strings.ToLower(key)
		if lower == "host" || lower == "content-length" {
			continue
		}
		if _, banned := hopByHopHeaders[lower]; banned {
			continue
		}
		if lower == "cookie" || lower == "referer" || lower == "x-api-key" || lower == "authorization" {
			continue
		}
		if lower == "x-cxpool-test" || lower == strings.ToLower(internalCx2ccUpstreamResponsesWSHeader) {
			continue
		}
		if strings.HasPrefix(lower, "sec-ch-ua") {
			continue
		}
		for _, v := range vals {
			dst.Add(http.CanonicalHeaderKey(key), v)
		}
	}
	dst.Del("Content-Length")
	return dst
}

func applyHeaders(dst http.Header, src http.Header) {
	for k := range dst {
		dst.Del(k)
	}
	for key, vals := range src {
		for _, v := range vals {
			dst.Add(key, v)
		}
	}
}

func headerFirst(h http.Header, key string) string {
	if h == nil {
		return ""
	}
	vals := h[http.CanonicalHeaderKey(key)]
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func shouldReadBody(method string) bool {
	if method == "" {
		return false
	}
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead:
		return false
	default:
		return true
	}
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func headerTruthy(h http.Header, key string) bool {
	switch strings.ToLower(strings.TrimSpace(headerFirst(h, key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

var hopByHopHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

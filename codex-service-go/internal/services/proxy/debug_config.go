package proxy

import (
	"net/http"
	"strings"

	instsvc "codex-service-go/internal/services/instances"
)

type reqBodyMode int

const (
	reqBodyModeOff reqBodyMode = iota
	reqBodyModeSummary
	reqBodyModeFull
)

func effectiveReqBodyMode(inst instsvc.InstanceWithPaths) reqBodyMode {
	switch inst.DebugLogReqBodyMode {
	case 1:
		return reqBodyModeOff
	case 2:
		return reqBodyModeSummary
	case 3:
		return reqBodyModeFull
	default:
		if inst.DebugLogReqBody {
			return reqBodyModeFull
		}
		return reqBodyModeOff
	}
}

func requestIDFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	return strings.TrimSpace(coalesce(
		headerFirst(h, "X-Oneapi-Request-Id"),
		headerFirst(h, "X-Request-Id"),
		headerFirst(h, "X-Oai-Request-Id"),
	))
}

func stripInternalRequestIDHeaders(h http.Header) {
	if h == nil {
		return
	}
	h.Del("X-Oneapi-Request-Id")
	h.Del("X-Request-Id")
	h.Del("X-Oai-Request-Id")
}


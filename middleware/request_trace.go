package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"one-api/service"
)

// RequestTrace captures full request/response headers and bodies for point debugging.
// It is opt-in and can be toggled at runtime via admin option `request_trace.enabled`
// (env REQUEST_TRACE_ENABLED provides the default).
//
// Scope: only traces upstream-style API requests (e.g. /v1/*, /v1beta/*) to avoid
// persisting admin console traffic by default.
func RequestTrace() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil || c.Request == nil || c.Request.URL == nil {
			return
		}

		path := strings.TrimSpace(c.Request.URL.Path)
		if !(strings.HasPrefix(path, "/v1/") ||
			strings.HasPrefix(path, "/v1beta/") ||
			path == "/v1" ||
			path == "/v1beta") {
			c.Next()
			return
		}

		handle := service.BeginGinRequestTrace(c)
		if handle != nil {
			defer service.EndGinRequestTrace(handle, c)
		}

		c.Next()
	}
}

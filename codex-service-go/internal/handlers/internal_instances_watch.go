package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type internalInstancesWatchResponse struct {
	Service   string `json:"service"`
	Revision  int64  `json:"revision"`
	Changed   bool   `json:"changed"`
	Reset     bool   `json:"reset"`
	Timestamp int64  `json:"timestamp"`
}

func (h *APIHandler) HandleInternalInstancesWatch(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		c.String(http.StatusUnauthorized, "missing bearer token")
		return
	}
	expected := strings.TrimSpace(h.cfg.ServiceToken)
	if token := strings.TrimPrefix(authHeader, "Bearer "); token != expected {
		c.String(http.StatusUnauthorized, "invalid token")
		return
	}

	since := int64(0)
	if v := strings.TrimSpace(c.Query("since")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid since")
			return
		}
		since = n
	}

	timeoutSec := 30
	if v := strings.TrimSpace(c.Query("timeout")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			c.String(http.StatusBadRequest, "invalid timeout")
			return
		}
		timeoutSec = n
	}

	cur := h.instances.CurrentRevision()
	if since > cur {
		c.JSON(http.StatusOK, internalInstancesWatchResponse{
			Service:   "codex-service-go",
			Revision:  cur,
			Changed:   true,
			Reset:     true,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	if cur > since {
		c.JSON(http.StatusOK, internalInstancesWatchResponse{
			Service:   "codex-service-go",
			Revision:  cur,
			Changed:   true,
			Reset:     false,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	rev, changed := h.instances.WaitForRevision(c.Request.Context(), since, time.Duration(timeoutSec)*time.Second)
	c.JSON(http.StatusOK, internalInstancesWatchResponse{
		Service:   "codex-service-go",
		Revision:  rev,
		Changed:   changed,
		Reset:     false,
		Timestamp: time.Now().Unix(),
	})
}

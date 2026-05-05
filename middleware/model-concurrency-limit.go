package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/setting"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type streamMeta struct {
	Stream bool `json:"stream"`
}

type userConcurrencyLimiter struct {
	userID   int
	mu       sync.Mutex
	inFlight int
	notify   chan struct{}
}

func (l *userConcurrencyLimiter) Acquire(ctx context.Context, limit int, wait time.Duration) bool {
	if limit <= 0 {
		return true
	}

	var deadline time.Time
	if wait > 0 {
		deadline = time.Now().Add(wait)
	}

	for {
		l.mu.Lock()
		if l.inFlight < limit {
			l.inFlight++
			l.mu.Unlock()
			return true
		}
		ch := l.notify
		if ch == nil {
			ch = make(chan struct{})
			l.notify = ch
		}
		l.mu.Unlock()

		if wait <= 0 {
			return false
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}

		timer := time.NewTimer(remaining)
		select {
		case <-ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false
		case <-timer.C:
			return false
		}
	}
}

func (l *userConcurrencyLimiter) Release() {
	l.mu.Lock()
	if l.inFlight <= 0 {
		common.SysError(fmt.Sprintf("concurrency limiter release underflow: user=%d", l.userID))
	} else {
		l.inFlight--
	}
	if l.notify != nil {
		close(l.notify)
		l.notify = nil
	}
	l.mu.Unlock()
}

var userConcurrencyLimiters sync.Map

func getUserConcurrencyLimiter(userID int) *userConcurrencyLimiter {
	if v, ok := userConcurrencyLimiters.Load(userID); ok {
		return v.(*userConcurrencyLimiter)
	}
	limiter := &userConcurrencyLimiter{userID: userID}
	actual, _ := userConcurrencyLimiters.LoadOrStore(userID, limiter)
	return actual.(*userConcurrencyLimiter)
}

func isStreamingRequest(c *gin.Context) bool {
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return true
	}
	if v := strings.ToLower(strings.TrimSpace(c.Query("stream"))); v == "true" || v == "1" {
		return true
	}
	if strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/event-stream") {
		return true
	}

	contentType := c.Request.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return false
	}
	body, err := common.GetRequestBody(c)
	if err != nil || len(body) == 0 {
		return false
	}
	// common.GetRequestBody consumes and closes the request body. Reset it so downstream
	// handlers that use ShouldBindJSON/ShouldBind still work.
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	meta := streamMeta{}
	if err := common.Unmarshal(body, &meta); err != nil {
		return false
	}
	return meta.Stream
}

// ModelRequestConcurrencyLimit enforces per-user in-flight streaming request limits for relay routes.
// It only applies to streaming requests (SSE/WebSocket). When the limit is reached, it either:
// - fails fast with 429 (wait=0), or
// - waits up to ModelRequestConcurrencyLimitWaitSeconds for a slot then returns 429.
func ModelRequestConcurrencyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !setting.ModelRequestConcurrencyLimitEnabled {
			c.Next()
			return
		}

		if !isStreamingRequest(c) {
			c.Next()
			return
		}

		userID := c.GetInt("id")
		if userID <= 0 {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "missing user id for concurrency limit", "concurrency_limit_failed")
			return
		}

		limit := setting.ModelRequestConcurrencyLimit
		groupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
		if groupID <= 0 {
			groupID = common.GetContextKeyInt(c, constant.ContextKeyDefaultModelGroupId)
		}
		if groupID > 0 {
			if groupLimit, found := setting.GetGroupConcurrencyLimit(groupID); found {
				limit = groupLimit
			}
		}
		if limit <= 0 {
			c.Next()
			return
		}

		wait := time.Duration(setting.ModelRequestConcurrencyLimitWaitSeconds) * time.Second
		limiter := getUserConcurrencyLimiter(userID)
		if !limiter.Acquire(c.Request.Context(), limit, wait) {
			retryAfter := "1"
			if setting.ModelRequestConcurrencyLimitWaitSeconds > 0 {
				retryAfter = strconv.Itoa(setting.ModelRequestConcurrencyLimitWaitSeconds)
			}
			c.Header("Retry-After", retryAfter)
			abortWithOpenAiMessage(
				c,
				http.StatusTooManyRequests,
				fmt.Sprintf("并发连接已达上限：同一用户最多同时%d个流式请求", limit),
				"concurrency_limit_exceeded",
			)
			return
		}
		defer limiter.Release()

		c.Next()
	}
}

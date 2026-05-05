package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"codex-service-go/internal/config"
	instsvc "codex-service-go/internal/services/instances"
	proxysvc "codex-service-go/internal/services/proxy"
)

type compatSettings struct {
	EnableCompat            bool
	EnableStreamCompat      bool
	EnableAggregation       bool
	ForceStreamCompat       bool
	ReasoningCompat         string
	ReasoningEffort         string
	DisableBaseInstructions bool
	BaseInstructions        string
}

type APIHandler struct {
	cfg       *config.Config
	instances *instsvc.Service
	proxy     *proxysvc.Service
	compat    compatSettings
}

func NewAPIHandler(cfg *config.Config, instances *instsvc.Service, proxy *proxysvc.Service) *APIHandler {
	return &APIHandler{
		cfg:       cfg,
		instances: instances,
		proxy:     proxy,
		compat: compatSettings{
			EnableCompat:            cfg.ProxyEnableCompatOpenAI,
			EnableStreamCompat:      cfg.ProxyEnableStreamCompat,
			EnableAggregation:       cfg.ProxyEnableAggregation,
			ForceStreamCompat:       cfg.ProxyForceStreamCompat,
			ReasoningCompat:         cfg.ProxyReasoningCompat,
			ReasoningEffort:         cfg.ProxyReasoningEffort,
			DisableBaseInstructions: cfg.ProxyDisableInstructions,
			BaseInstructions:        cfg.ProxyBaseInstructions,
		},
	}
}

func (h *APIHandler) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *APIHandler) HandleResponses(c *gin.Context) {
	inst, ok := h.authorize(c)
	if !ok {
		return
	}
	if h.shouldBlock(c, inst) {
		return
	}
	resp, err := h.proxy.ForwardResponses(c.Request.Context(), *inst, c.Request)
	if err != nil {
		h.respondGatewayError(c, err)
		return
	}
	defer resp.Body.Close()
	h.clearBlockingStatusOnLocalTestSuccess(c, inst, resp)
	h.flushUpstreamResponse(c, resp)
	h.appendLog(inst.LogPath, fmt.Sprintf("[%s] %s %s -> %d", time.Now().Format(time.RFC3339), c.Request.Method, c.Request.URL.Path, resp.StatusCode))
}

func (h *APIHandler) HandleChatCompletions(c *gin.Context) {
	inst, ok := h.authorize(c)
	if !ok {
		return
	}
	if h.shouldBlock(c, inst) {
		return
	}
	if !h.compat.EnableCompat || strings.EqualFold(inst.AuthMode, "api_key") {
		h.forwardAny(c, inst)
		return
	}
	h.handleChatCompat(c, inst)
}

func (h *APIHandler) HandleModels(c *gin.Context) {
	inst, ok := h.authorize(c)
	if !ok {
		return
	}
	if h.shouldBlock(c, inst) {
		return
	}
	resp, err := h.proxy.ForwardModels(c.Request.Context(), *inst)
	if err != nil {
		h.respondGatewayError(c, err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
		h.clearBlockingStatusOnLocalTestSuccess(c, inst, resp)
		h.flushUpstreamResponse(c, resp)
	}
}

func (h *APIHandler) HandleEmbeddings(c *gin.Context) {
	inst, ok := h.authorize(c)
	if !ok {
		return
	}
	if h.shouldBlock(c, inst) {
		return
	}
	h.forwardAny(c, inst)
}

func (h *APIHandler) HandleForward(c *gin.Context) {
	inst, ok := h.authorize(c)
	if !ok {
		return
	}
	if h.shouldBlock(c, inst) {
		return
	}
	h.forwardAny(c, inst)
}

func (h *APIHandler) HandleAny(c *gin.Context) {
	path := strings.TrimPrefix(c.Param("path"), "/")
	if path == "" {
		h.HandleForward(c)
		return
	}
	lower := strings.ToLower(path)
	switch {
	case lower == "v1/responses" && strings.EqualFold(c.Request.Method, http.MethodGet) && websocket.IsWebSocketUpgrade(c.Request):
		h.HandleResponsesWebSocket(c)
	case lower == "v1/responses" && strings.EqualFold(c.Request.Method, http.MethodPost):
		h.HandleResponses(c)
	case lower == "v1/chat/completions" && strings.EqualFold(c.Request.Method, http.MethodPost):
		h.HandleChatCompletions(c)
	case lower == "chat/completions" && strings.EqualFold(c.Request.Method, http.MethodPost):
		h.HandleChatCompletions(c)
	case lower == "v1/models" && (strings.EqualFold(c.Request.Method, http.MethodGet) || strings.EqualFold(c.Request.Method, http.MethodHead)):
		h.HandleModels(c)
	case lower == "v1/embeddings" && strings.EqualFold(c.Request.Method, http.MethodPost):
		h.HandleEmbeddings(c)
	case strings.HasPrefix(lower, "v1/"):
		h.HandleForward(c)
	default:
		h.HandleForward(c)
	}
}

func (h *APIHandler) forwardAny(c *gin.Context, inst *instsvc.InstanceWithPaths) {
	resp, err := h.proxy.ForwardAny(c.Request.Context(), *inst, c.Request)
	if err != nil {
		h.respondGatewayError(c, err)
		return
	}
	defer resp.Body.Close()
	h.clearBlockingStatusOnLocalTestSuccess(c, inst, resp)
	h.flushUpstreamResponse(c, resp)
	h.appendLog(inst.LogPath, fmt.Sprintf("[%s] %s %s -> %d", time.Now().Format(time.RFC3339), c.Request.Method, c.Request.URL.Path, resp.StatusCode))
}

func (h *APIHandler) shouldBlock(c *gin.Context, inst *instsvc.InstanceWithPaths) bool {
	result, err := h.proxy.ShouldBlock(c.Request.Context(), inst.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "runtime status check failed")
		return true
	}
	if !result.Blocked {
		return false
	}
	if isLocalCxPoolTestRequest(c) {
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] bypass: allow blocked instance for test (reason=%s retry_after=%d)", time.Now().Format(time.RFC3339), strings.TrimSpace(result.Reason), result.RetryAfter))
		return false
	}
	if result.RetryAfter > 0 {
		c.Writer.Header().Set("Retry-After", strconv.Itoa(result.RetryAfter))
	}
	c.Data(http.StatusServiceUnavailable, "text/plain; charset=utf-8", []byte("Service is unavailable"))
	return true
}

func (h *APIHandler) clearBlockingStatusOnLocalTestSuccess(c *gin.Context, inst *instsvc.InstanceWithPaths, resp *http.Response) {
	if c == nil || inst == nil || resp == nil || resp.StatusCode >= http.StatusBadRequest {
		return
	}
	if !isLocalCxPoolTestRequest(c) || h.proxy == nil {
		return
	}
	if err := h.proxy.RecoverInstanceForLocalTestSuccess(inst.ID); err != nil {
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] test recovery failed: %v", time.Now().Format(time.RFC3339), err))
		return
	}
	h.appendLog(inst.LogPath, fmt.Sprintf("[%s] test recovery cleared runtime blocking status", time.Now().Format(time.RFC3339)))
}

func (h *APIHandler) authorize(c *gin.Context) (*instsvc.InstanceWithPaths, bool) {
	idStr := strings.TrimPrefix(c.Param("instanceID"), "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		// 路径段不是有效数字时按资源不存在处理，避免把静态路径（如 /css/...）误判为 400
		c.String(http.StatusNotFound, "instance not found")
		return nil, false
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if inst == nil {
		// 记录不存在实例的访问，便于排查 ID 错误
		h.appendLog("", fmt.Sprintf("[%s] 404 not found: instance %s", time.Now().Format(time.RFC3339), idStr))
		c.String(http.StatusNotFound, "instance not found")
		return nil, false
	}
	if !inst.Enabled {
		if !isLocalCxPoolTestRequest(c) {
			h.appendLog(inst.LogPath, fmt.Sprintf("[%s] 503 blocked: instance stopped", time.Now().Format(time.RFC3339)))
			c.Data(http.StatusServiceUnavailable, "text/plain; charset=utf-8", []byte("Service is unavailable (instance stopped)"))
			return nil, false
		}
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] bypass: allow stopped instance for test", time.Now().Format(time.RFC3339)))
	}
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] 401 unauthorized: missing bearer", time.Now().Format(time.RFC3339)))
		c.String(http.StatusUnauthorized, "missing bearer token")
		return nil, false
	}
	expected := strings.TrimSpace(inst.InternalToken)
	if expected == "" {
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] 401 unauthorized: instance token missing", time.Now().Format(time.RFC3339)))
		c.String(http.StatusUnauthorized, "instance token not configured")
		return nil, false
	}
	if token := strings.TrimPrefix(authHeader, "Bearer "); token != expected {
		tail := token
		if len(tail) > 6 {
			tail = tail[len(tail)-6:]
		}
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] 401 unauthorized: invalid token (*%s)", time.Now().Format(time.RFC3339), tail))
		c.String(http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	return inst, true
}

func isLocalCxPoolTestRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	raw := strings.TrimSpace(c.GetHeader("X-CxPool-Test"))
	switch strings.ToLower(raw) {
	case "1", "true", "yes":
	default:
		return false
	}
	ip := strings.TrimSpace(c.ClientIP())
	return ip == "127.0.0.1" || ip == "::1"
}

func (h *APIHandler) flushUpstreamResponse(c *gin.Context, resp *http.Response) {
	for key, vals := range resp.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, v := range vals {
			c.Writer.Header().Add(key, v)
		}
	}
	c.Status(resp.StatusCode)
	if resp.Body != nil {
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if strings.Contains(ct, "text/event-stream") {
			h.flushUpstreamEventStream(c, resp.Body)
			return
		}
		if _, err := io.Copy(c.Writer, resp.Body); err != nil {
			c.Error(err)
		}
	}
}

func (h *APIHandler) respondGatewayError(c *gin.Context, err error) {
	c.String(http.StatusBadGateway, "bad gateway: %v", err)
}

func (h *APIHandler) flushUpstreamEventStream(c *gin.Context, body io.Reader) {
	flusher, _ := c.Writer.(http.Flusher)
	reader := bufio.NewReader(body)
	lastEvent := ""
	uaLower := strings.ToLower(strings.TrimSpace(c.GetHeader("User-Agent")))
	compatOpenCode := strings.Contains(uaLower, "opencode/") ||
		strings.Contains(uaLower, "ai-sdk/openai") ||
		strings.Contains(uaLower, "openai/js")

	for {
		if c.Request != nil && c.Request.Context().Err() != nil {
			return
		}

		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if bytes.HasPrefix(trimmed, []byte("event:")) {
				lastEvent = strings.TrimSpace(string(trimmed[len("event:"):]))
				if _, werr := c.Writer.Write(line); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
				continue
			}

			if len(trimmed) == 0 {
				if _, werr := c.Writer.Write(line); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
				continue
			}

			if bytes.Equal(trimmed, []byte("[DONE]")) || bytes.Equal(trimmed, []byte("DONE")) {
				if _, werr := c.Writer.Write(line); werr != nil {
					return
				}
				_, _ = c.Writer.Write([]byte("\n"))
				if flusher != nil {
					flusher.Flush()
				}
				return
			}

			if !bytes.HasPrefix(trimmed, []byte("data:")) {
				if _, werr := c.Writer.Write(line); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
				continue
			}

			payload := bytes.TrimSpace(trimmed[len("data:"):])

			if bytes.Equal(payload, []byte("[DONE]")) || bytes.Equal(payload, []byte("DONE")) {
				if _, werr := c.Writer.Write(line); werr != nil {
					return
				}
				_, _ = c.Writer.Write([]byte("\n"))
				if flusher != nil {
					flusher.Flush()
				}
				return
			}

			var envelope struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(payload, &envelope) != nil || strings.TrimSpace(envelope.Type) == "" {
				envelope.Type = lastEvent
			}

			eventType := strings.TrimSpace(envelope.Type)
			lineToWrite := line
			if compatOpenCode && eventType == "error" {
				var m map[string]interface{}
				if json.Unmarshal(payload, &m) == nil && m != nil {
					if nested, ok := m["error"].(map[string]interface{}); ok && nested != nil {
						code := strings.TrimSpace(fmt.Sprint(nested["code"]))
						if code == "<nil>" {
							code = ""
						}
						message := strings.TrimSpace(fmt.Sprint(nested["message"]))
						if message == "<nil>" {
							message = ""
						}
						param := strings.TrimSpace(fmt.Sprint(nested["param"]))
						if param == "<nil>" {
							param = ""
						}

						if code != "" || message != "" {
							flat := map[string]any{
								"type":    "error",
								"code":    code,
								"message": message,
								"param": func() any {
									if param == "" {
										return nil
									}
									return param
								}(),
							}
							if seq, ok := m["sequence_number"]; ok {
								flat["sequence_number"] = seq
							}
							if encoded, merr := json.Marshal(flat); merr == nil {
								out := make([]byte, 0, len(encoded)+7+1)
								out = append(out, "data: "...)
								out = append(out, encoded...)
								out = append(out, '\n')
								lineToWrite = out
							}
						}
					}
				}
			}

			if _, werr := c.Writer.Write(lineToWrite); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}

			switch eventType {
			case "response.completed", "response.done", "response.incomplete", "response.failed":
				_, _ = c.Writer.Write([]byte("\n"))
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}

		if err != nil {
			return
		}
	}
}

func (h *APIHandler) appendLog(path string, line string) {
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

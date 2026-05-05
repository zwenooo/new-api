package controller

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"one-api/common"
	"one-api/model"
	"one-api/service"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type requestTraceObjectView struct {
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
}

type requestTraceNodeView struct {
	Id        int64  `json:"id"`
	RequestId string `json:"request_id"`
	Service   string `json:"service"`
	Kind      string `json:"kind"`
	Seq       int    `json:"seq"`

	StartedAt int64 `json:"started_at"`
	EndedAt   int64 `json:"ended_at"`

	RequestMethod string `json:"request_method"`
	RequestURL    string `json:"request_url"`
	RequestPath   string `json:"request_path"`

	ResponseStatus int `json:"response_status"`

	RequestHeaders  *requestTraceObjectView `json:"request_headers,omitempty"`
	RequestBody     *requestTraceObjectView `json:"request_body,omitempty"`
	ResponseHeaders *requestTraceObjectView `json:"response_headers,omitempty"`
	ResponseBody    *requestTraceObjectView `json:"response_body,omitempty"`

	Error string `json:"error,omitempty"`
	Meta  string `json:"meta,omitempty"`
}

type requestTraceView struct {
	RequestId string                     `json:"request_id"`
	Session   *model.RequestTraceSession `json:"session,omitempty"`
	Nodes     []*requestTraceNodeView    `json:"nodes"`
}

func buildRequestTraceObjectView(requestID string, key string, size int64) *requestTraceObjectView {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return nil
	}
	qs := url.Values{}
	qs.Set("request_id", requestID)
	qs.Set("key", key)
	return &requestTraceObjectView{
		Key:         key,
		Size:        size,
		ContentType: service.RequestTraceGuessContentType(key),
		DownloadURL: "/api/request_trace/object?" + qs.Encode(),
	}
}

func GetRequestTrace(c *gin.Context) {
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		common.ApiErrorMsg(c, "request_id is required")
		return
	}

	var session model.RequestTraceSession
	hasSession := false
	if err := model.DB.Where("request_id = ?", requestID).First(&session).Error; err == nil {
		hasSession = true
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}

	var nodes []*model.RequestTraceNode
	if err := model.DB.Where("request_id = ?", requestID).
		Order("started_at asc, id asc").
		Find(&nodes).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	outNodes := make([]*requestTraceNodeView, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		view := &requestTraceNodeView{
			Id:             n.Id,
			RequestId:      n.RequestId,
			Service:        n.Service,
			Kind:           n.Kind,
			Seq:            n.Seq,
			StartedAt:      n.StartedAt,
			EndedAt:        n.EndedAt,
			RequestMethod:  n.RequestMethod,
			RequestURL:     n.RequestURL,
			RequestPath:    n.RequestPath,
			ResponseStatus: n.ResponseStatus,
			Error:          n.Error,
			Meta:           n.Meta,
		}
		view.RequestHeaders = buildRequestTraceObjectView(requestID, n.RequestHeadersKey, n.RequestHeadersSize)
		view.RequestBody = buildRequestTraceObjectView(requestID, n.RequestBodyKey, n.RequestBodySize)
		view.ResponseHeaders = buildRequestTraceObjectView(requestID, n.ResponseHeadersKey, n.ResponseHeadersSize)
		view.ResponseBody = buildRequestTraceObjectView(requestID, n.ResponseBodyKey, n.ResponseBodySize)
		outNodes = append(outNodes, view)
	}

	resp := &requestTraceView{
		RequestId: requestID,
		Nodes:     outNodes,
	}
	if hasSession {
		resp.Session = &session
	}
	common.ApiSuccess(c, resp)
}

func GetRequestTraceObject(c *gin.Context) {
	requestID := strings.TrimSpace(c.Query("request_id"))
	key := strings.TrimPrefix(strings.TrimSpace(c.Query("key")), "/")
	if requestID == "" {
		common.ApiErrorMsg(c, "request_id is required")
		return
	}
	if key == "" {
		common.ApiErrorMsg(c, "key is required")
		return
	}
	// Hard fence: only allow our trace object namespace.
	if !strings.HasPrefix(key, "request_traces/") {
		common.ApiErrorMsg(c, "invalid key")
		return
	}
	// Ensure the requested key belongs to the request_id (basic guard).
	if !strings.Contains(key, "/"+requestID+"/") && !strings.HasSuffix(key, "/"+requestID) {
		// Some keys include request_id as the last segment.
		common.ApiErrorMsg(c, "key does not match request_id")
		return
	}

	rc, size, contentType, err := service.RequestTraceReadObject(c.Request.Context(), key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			common.ApiErrorMsg(c, "not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	defer rc.Close()

	if strings.TrimSpace(contentType) != "" {
		c.Writer.Header().Set("Content-Type", contentType)
	}
	if size > 0 {
		c.Writer.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

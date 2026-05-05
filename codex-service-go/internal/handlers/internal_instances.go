package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type internalInstanceItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Group         string `json:"group"`
	BasePath      string `json:"base_path"`
	Enabled       bool   `json:"enabled"`
	Available     bool   `json:"available"`
	State         string `json:"state"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	RetryAfter    int    `json:"retry_after,omitempty"`
	AuthMode      string `json:"auth_mode"`
	InternalToken string `json:"internal_token"`
}

type internalInstancesResponse struct {
	Service   string                 `json:"service"`
	Revision  int64                  `json:"revision"`
	Instances []internalInstanceItem `json:"instances"`
	Timestamp int64                  `json:"timestamp"`
}

func (h *APIHandler) HandleInternalInstances(c *gin.Context) {
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

	list, err := h.instances.List(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	authMetaList, err := h.instances.ListAuthMeta(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	type authMetaSnapshot struct {
		expiresAt int64
	}
	authMetaByID := make(map[int64]authMetaSnapshot, len(authMetaList))
	for _, meta := range authMetaList {
		authMetaByID[meta.InstanceID] = authMetaSnapshot{expiresAt: meta.AccessTokenExpiresAt}
	}

	candidates := make(map[string]int, len(list))
	for _, it := range list {
		if g, ok := instanceGroupCandidate(it.Name); ok {
			candidates[g]++
		}
	}

	items := make([]internalInstanceItem, 0, len(list))
	nowUnix := time.Now().Unix()
	for _, it := range list {
		groupName := strings.TrimSpace(it.Name)
		if g, ok := instanceGroupCandidate(it.Name); ok && candidates[g] > 1 {
			groupName = g
		}

		state := "normal"
		available := it.Enabled
		blockedReason := ""
		retryAfter := 0

		if it.Enabled {
			blockedState := "normal"
			if res, err := h.proxy.ShouldBlock(c.Request.Context(), it.ID); err == nil && res.Blocked {
				available = false
				blockedReason = strings.TrimSpace(res.Reason)
				retryAfter = res.RetryAfter
				blockedState = runtimeBlockedState(res.Reason)
				if blockedState != "normal" {
					state = blockedState
				}
			}

			if !strings.EqualFold(it.AuthMode, "api_key") {
				meta, ok := authMetaByID[it.ID]
				if !ok {
					available = false
					if state != "cooldown" && state != "member_expired" {
						state = "expired"
					}
					if blockedReason == "" || blockedState == "channel_backoff" || blockedState == "transport_quarantine" {
						blockedReason = "auth_missing"
					}
				} else if meta.expiresAt > 0 && meta.expiresAt <= nowUnix {
					available = false
					if state != "cooldown" && state != "member_expired" {
						state = "expired"
					}
					if blockedReason == "" || blockedState == "channel_backoff" || blockedState == "transport_quarantine" {
						blockedReason = "auth_expired"
					}
				}
			}
		} else {
			state = "stopped"
		}

		items = append(items, internalInstanceItem{
			ID:            it.ID,
			Name:          it.Name,
			Group:         groupName,
			BasePath:      it.BasePath,
			Enabled:       it.Enabled,
			Available:     available,
			State:         state,
			BlockedReason: blockedReason,
			RetryAfter:    retryAfter,
			AuthMode:      it.AuthMode,
			InternalToken: it.InternalToken,
		})
	}

	c.JSON(http.StatusOK, internalInstancesResponse{
		Service:   "codex-service-go",
		Revision:  h.instances.CurrentRevision(),
		Instances: items,
		Timestamp: time.Now().Unix(),
	})
}

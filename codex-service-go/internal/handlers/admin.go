package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"codex-service-go/internal/config"
	instsvc "codex-service-go/internal/services/instances"
	loginsvc "codex-service-go/internal/services/login"
	proxysvc "codex-service-go/internal/services/proxy"
	systemsvc "codex-service-go/internal/services/system"
)

type AdminHandler struct {
	instances *instsvc.Service
	proxy     *proxysvc.Service
	login     *loginsvc.Service
	system    *systemsvc.Service
	cfg       *config.Config
	auth      *AuthManager
	refresher interface {
		IsEnabled() bool
		SetEnabled(bool)
	}
	restart func()
}

func (h *AdminHandler) renderPage(c *gin.Context, status int, tmpl string, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	user := CurrentUser(c)
	if user == "" && h.auth != nil {
		if resolved, ok := h.auth.CurrentUserFromRequest(c.Request); ok {
			user = resolved
		}
	}
	data["CurrentUser"] = user
	data["AuthEnabled"] = h.auth != nil && h.auth.IsEnabled()
	// 不再展示凭据文件路径，仅使用 .env 管理账号密码
	c.HTML(status, tmpl, data)
}

func NewAdminHandler(cfg *config.Config, instances *instsvc.Service, proxy *proxysvc.Service, login *loginsvc.Service, system *systemsvc.Service, auth *AuthManager) *AdminHandler {
	return &AdminHandler{cfg: cfg, instances: instances, proxy: proxy, login: login, system: system, auth: auth}
}

func (h *AdminHandler) AttachRefresher(r interface {
	IsEnabled() bool
	SetEnabled(bool)
}) {
	h.refresher = r
}

func (h *AdminHandler) AttachRestart(fn func()) {
	h.restart = fn
}

func (h *AdminHandler) mount(p string) string {
	base := ""
	if h != nil && h.cfg != nil {
		base = strings.TrimRight(strings.TrimSpace(h.cfg.WebBasePath), "/")
		if base == "/" {
			base = ""
		}
	}
	p = strings.TrimSpace(p)
	if p == "" {
		return base
	}
	if base == "" {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if strings.HasPrefix(p, base+"/") || p == base {
		return p
	}
	return base + p
}

func (h *AdminHandler) requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c != nil && c.Request != nil && c.Request.TLS != nil {
		scheme = "https"
	}

	host := ""
	if c != nil && c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		host = hostFromRequest(c)
		if h != nil && h.cfg != nil && h.cfg.ServerPort > 0 {
			host = fmt.Sprintf("%s:%d", host, h.cfg.ServerPort)
		}
	} else if !strings.Contains(host, ":") && h != nil && h.cfg != nil && h.cfg.ServerPort > 0 {
		host = fmt.Sprintf("%s:%d", host, h.cfg.ServerPort)
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, h.mount(""))
}

func (h *AdminHandler) Index(c *gin.Context) {
	base := h.requestBaseURL(c)
	token := strings.TrimSpace(h.cfg.ServiceToken)
	embedded := h.cfg != nil && h.cfg.ServerPort == 0
	envFormEnabled := !embedded
	h.renderPage(c, http.StatusOK, "admin_list.html", gin.H{
		"ServiceToken":   token,
		"BaseURL":        base,
		"SampleBase":     "POST " + base + "/<实例ID>/v1",
		"Embedded":       embedded,
		"EnvFormEnabled": envFormEnabled,
	})
}

// CompatStatus 返回与 Node 版本 /admin/api/compat 等价的开关状态
// 字段对齐：
// - auth_mode
// - upstream_base_url
// - enable_compat_openai
// - enable_stream_compat
// - enable_response_aggregation
// - force_stream_compat
// - reasoning_compat
func (h *AdminHandler) CompatStatus(c *gin.Context) {
	if h.cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config_not_loaded"})
		return
	}
	payload := gin.H{
		"auth_mode":                   strings.TrimSpace(strings.ToLower(h.cfg.DefaultProxyAuthMode)),
		"upstream_base_url":           h.cfg.DefaultUpstreamBaseURL,
		"enable_compat_openai":        h.cfg.ProxyEnableCompatOpenAI,
		"enable_stream_compat":        h.cfg.ProxyEnableStreamCompat,
		"enable_response_aggregation": h.cfg.ProxyEnableAggregation,
		"force_stream_compat":         h.cfg.ProxyForceStreamCompat,
		"reasoning_compat":            h.cfg.ProxyReasoningCompat,
	}
	c.JSON(http.StatusOK, payload)
}

func (h *AdminHandler) LoginPage(c *gin.Context) {
	enabled := h.auth != nil && h.auth.IsEnabled()
	if !enabled {
		log.Printf("[admin] login page: auth disabled (auth=nil:%v enabled:%v)", h.auth == nil, enabled)
		h.renderPage(c, http.StatusOK, "login.html", gin.H{
			"AuthConfigured": false,
		})
		return
	}
	log.Printf("[admin] login page: auth enabled for user %s", strings.TrimSpace(h.cfg.AdminUser))
	if user, ok := h.auth.CurrentUserFromRequest(c.Request); ok && strings.TrimSpace(user) != "" {
		c.Redirect(http.StatusFound, h.mount("/admin"))
		return
	}
	data := gin.H{
		"AuthConfigured": true,
	}
	if next := strings.TrimSpace(c.Query("next")); isSafeRedirect(next) {
		data["Next"] = next
	}
	h.renderPage(c, http.StatusOK, "login.html", data)
}

func (h *AdminHandler) LoginSubmit(c *gin.Context) {
	if h.auth == nil || !h.auth.IsEnabled() {
		h.renderPage(c, http.StatusServiceUnavailable, "login.html", gin.H{
			"AuthConfigured":      false,
			"AuthCredentialsPath": h.cfg.AdminCredentialsFile,
			"Error":               "管理员账号未配置，请设置 WEB_ADMIN_USER/WEB_ADMIN_PASS",
		})
		return
	}
	username := strings.TrimSpace(c.PostForm("username"))
	password := strings.TrimSpace(c.PostForm("password"))
	if username == "" || password == "" || !h.auth.Validate(username, password) {
		data := gin.H{
			"AuthConfigured": true,
			"Error":          "账号或密码错误",
			"Username":       username,
		}
		if next := strings.TrimSpace(c.PostForm("next")); isSafeRedirect(next) {
			data["Next"] = next
		}
		h.renderPage(c, http.StatusUnauthorized, "login.html", data)
		return
	}
	token, expires, err := h.auth.CreateSession(username)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	}
	if c.Request.TLS != nil {
		cookie.Secure = true
	}
	http.SetCookie(c.Writer, cookie)
	// 登录后统一进入 /admin（实例列表），避免在带有轮询片段的页面上登录后被重定向导致误请求。
	next := h.mount("/admin")
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", next)
		c.Status(http.StatusSeeOther)
		return
	}
	c.Redirect(http.StatusSeeOther, next)
}

func (h *AdminHandler) Logout(c *gin.Context) {
	if h.auth != nil && h.auth.IsEnabled() {
		if token, err := c.Cookie(sessionCookieName); err == nil {
			h.auth.Destroy(token)
		}
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
	}
	target := h.mount("/admin/login")
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", target)
		c.Status(http.StatusSeeOther)
		return
	}
	c.Redirect(http.StatusSeeOther, target)
}

func (h *AdminHandler) InstancesTable(c *gin.Context) {
	data, err := h.instancesTableData(c)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.HTML(http.StatusOK, "instances_table", data)
}

func (h *AdminHandler) instancesTableData(c *gin.Context) (gin.H, error) {
	baseURL := h.requestBaseURL(c)

	activeTab := normalizeInstancesTab(c.Query("state"))
	viewMode := normalizeInstancesView(c.Query("view"))

	list, err := h.instances.List(c.Request.Context())
	if err != nil {
		return nil, err
	}
	authMetaList, err := h.instances.ListAuthMeta(c.Request.Context())
	if err != nil {
		return nil, err
	}
	authMetaByID := make(map[int64]instsvc.AuthMeta, len(authMetaList))
	for _, meta := range authMetaList {
		authMetaByID[meta.InstanceID] = meta
	}
	type row struct {
		instsvc.InstanceWithPaths
		State             string
		AccountID         string
		CooldownMain      string
		CooldownUntilUnix int64
		AuthExpired       bool
		AuthImportedAt    string
		LastRefreshAt     string
		ATExpiresAt       string
		ATExpiresAtUnix   int64
		Status            string
		DuplicateOfID     int64
		DuplicateOfState  string
		dupKey            string
	}
	type group struct {
		Name string
		Rows []row
	}
	allRows := make([]row, 0, len(list))
	runtimeEnabled := strings.TrimSpace(h.cfg.ProxyRuntimeFile) != ""
	cnTZ := time.FixedZone("UTC+8", 8*60*60)
	const dateTimeLayout = "2006-01-02 15:04:05"
	formatAuthTime := func(raw string) string {
		ts := strings.TrimSpace(raw)
		if ts == "" {
			return ""
		}
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return t.In(cnTZ).Format(dateTimeLayout)
		}
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return t.In(cnTZ).Format(dateTimeLayout)
		}
		return ""
	}
	nowUnix := time.Now().Unix()
	for _, it := range list {
		r := row{InstanceWithPaths: it, State: "normal", CooldownMain: "正常"}
		if runtimeEnabled {
			if res, err := h.proxy.ShouldBlock(c.Request.Context(), it.ID); err == nil && res.Blocked {
				blockedState := runtimeBlockedState(res.Reason)
				if blockedState != "normal" {
					r.State = blockedState
					r.CooldownMain = runtimeStateLabel(blockedState)
					if res.RetryAfter > 0 && runtimeIsTemporaryState(blockedState) {
						r.CooldownUntilUnix = time.Now().Add(time.Duration(res.RetryAfter) * time.Second).Unix()
					}
				}
			}
		}
		// 状态：已启动/已停止（受 enabled 控制），附带凭据提示
		if it.Enabled {
			r.Status = "已启动"
		} else {
			r.Status = "已停止"
		}
		// 从 DB 读取 auth 元信息（last_refresh / access_token exp / account_id / dupKey）
		if meta, ok := authMetaByID[it.ID]; ok {
			r.dupKey = strings.TrimSpace(meta.DupKey)
			r.AccountID = strings.TrimSpace(meta.AccountID)
			r.AuthImportedAt = meta.UpdatedAt.In(cnTZ).Format(dateTimeLayout)
			r.LastRefreshAt = formatAuthTime(meta.LastRefresh)
			if meta.AccessTokenExpiresAt > 0 {
				r.ATExpiresAtUnix = meta.AccessTokenExpiresAt
				r.ATExpiresAt = time.Unix(meta.AccessTokenExpiresAt, 0).In(cnTZ).Format(dateTimeLayout)
				r.AuthExpired = meta.AccessTokenExpiresAt <= nowUnix
			}
		}
		if r.AuthExpired && r.State != "cooldown" && r.State != "member_expired" {
			r.State = "expired"
			r.CooldownMain = "过期"
		}
		allRows = append(allRows, r)
	}

	type dupInfo struct {
		ID    int64
		State string
		Count int
	}
	dupMap := make(map[string]dupInfo, len(allRows))
	for _, r := range allRows {
		if strings.TrimSpace(r.dupKey) == "" {
			continue
		}
		info := dupMap[r.dupKey]
		if info.Count == 0 || r.ID < info.ID {
			info.ID = r.ID
			info.State = r.State
		}
		info.Count++
		dupMap[r.dupKey] = info
	}
	for i := range allRows {
		key := strings.TrimSpace(allRows[i].dupKey)
		if key == "" {
			continue
		}
		info := dupMap[key]
		if info.Count <= 1 {
			continue
		}
		if allRows[i].ID == info.ID {
			continue
		}
		allRows[i].DuplicateOfID = info.ID
		allRows[i].DuplicateOfState = info.State
	}

	rows := make([]row, 0, len(allRows))
	for _, r := range allRows {
		if r.State != activeTab {
			continue
		}
		rows = append(rows, r)
	}
	var groups []group
	if len(rows) > 0 {
		if viewMode == "group" {
			candidates := make(map[string]int, len(rows))
			for _, r := range rows {
				if g, ok := instanceGroupCandidate(r.Name); ok {
					candidates[g]++
				}
			}
			index := make(map[string]int, 8)
			for _, r := range rows {
				groupName := strings.TrimSpace(r.Name)
				if g, ok := instanceGroupCandidate(r.Name); ok && candidates[g] > 1 {
					groupName = g
				}
				idx, ok := index[groupName]
				if !ok {
					groups = append(groups, group{Name: groupName})
					idx = len(groups) - 1
					index[groupName] = idx
				}
				groups[idx].Rows = append(groups[idx].Rows, r)
			}
		} else {
			groups = []group{{Rows: rows}}
		}
	}
	return gin.H{
		"Groups":         groups,
		"RefreshEnabled": proxysvc.TokenRefreshEnabled(),
		"BaseURL":        baseURL,
		"ActiveTab":      activeTab,
		"ViewMode":       viewMode,
	}, nil
}

func normalizeInstancesTab(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "normal", "正常":
		return "normal"
	case "cooldown", "sleep", "冷却", "休眠", "upstream_cooldown", "上游冷却":
		return "cooldown"
	case "channel_backoff", "渠道退避":
		return "channel_backoff"
	case "transport_quarantine", "链路隔离":
		return "transport_quarantine"
	case "member_expired", "member-expired", "会员过期":
		return "member_expired"
	case "expired", "expire", "过期":
		return "expired"
	default:
		return "normal"
	}
}

func normalizeInstancesView(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "group", "groups":
		return "group"
	case "", "instance", "instances":
		return "instance"
	default:
		return "instance"
	}
}

func instanceGroupCandidate(name string) (string, bool) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", false
	}
	dash := strings.LastIndex(n, "-")
	if dash <= 0 || dash >= len(n)-1 {
		return "", false
	}
	suffix := n[dash+1:]
	if len(suffix) < 2 || len(suffix) > 3 {
		return "", false
	}
	for _, ch := range suffix {
		if ch < '0' || ch > '9' {
			return "", false
		}
	}
	prefix := strings.TrimSpace(n[:dash])
	if prefix == "" {
		return "", false
	}
	return prefix, true
}

func authDuplicateKey(payload map[string]any) string {
	tokens, _ := payload["tokens"].(map[string]any)
	accountID := getStringAny(tokens["account_id"])
	if accountID == "" {
		accountID = getStringAny(payload["account_id"])
	}
	if accountID == "" {
		return ""
	}
	refreshToken := getStringAny(tokens["refresh_token"])
	if refreshToken == "" {
		refreshToken = getStringAny(payload["refresh_token"])
	}
	accessToken := getStringAny(tokens["access_token"])
	if accessToken == "" {
		accessToken = getStringAny(payload["access_token"])
	}
	idToken := getStringAny(tokens["id_token"])
	if idToken == "" {
		idToken = getStringAny(payload["id_token"])
	}
	if refreshToken != "" {
		return "acct:" + accountID + "|rt:" + tokenFingerprint(refreshToken)
	}
	if idToken != "" {
		return "acct:" + accountID + "|id:" + tokenFingerprint(idToken)
	}
	if accessToken != "" {
		return "acct:" + accountID + "|at:" + tokenFingerprint(accessToken)
	}
	return ""
}

func tokenFingerprint(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:8])
}

func getStringAny(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func jwtExpiryUnix(token string) int64 {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return 0
	}
	return claims.Exp
}

func jwtExpiryUTC(token string) string {
	return jwtExpiryCN(token)
}

func jwtExpiryCN(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return ""
	}
	cnTZ := time.FixedZone("UTC+8", 8*60*60)
	return time.Unix(claims.Exp, 0).In(cnTZ).Format("2006-01-02 15:04:05")
}

func (h *AdminHandler) NewInstanceForm(c *gin.Context) {
	data := gin.H{
		"Action":          "/admin/instances",
		"DefaultAuthMode": h.cfg.DefaultProxyAuthMode,
		"DefaultUpstream": h.cfg.DefaultUpstreamBaseURL,
	}
	c.HTML(http.StatusOK, "instance_form", data)
}

func (h *AdminHandler) EditInstanceForm(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	activeTab := normalizeInstancesTab(c.Query("state"))
	viewMode := normalizeInstancesView(c.Query("view"))
	data := gin.H{
		"Instance":        inst,
		"Action":          "/admin/instances/" + c.Param("id") + "?state=" + activeTab + "&view=" + viewMode,
		"DefaultAuthMode": h.cfg.DefaultProxyAuthMode,
		"DefaultUpstream": h.cfg.DefaultUpstreamBaseURL,
	}
	c.HTML(http.StatusOK, "instance_form", data)
}

func (h *AdminHandler) CreateInstance(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		c.String(http.StatusBadRequest, "name is required")
		return
	}
	params := instsvc.CreateParams{
		Name:            name,
		InternalToken:   strings.TrimSpace(c.PostForm("internal_token")),
		AuthMode:        strings.TrimSpace(c.PostForm("auth_mode")),
		UpstreamBaseURL: strings.TrimSpace(c.PostForm("upstream_base_url")),
		UpstreamAPIKey:  strings.TrimSpace(c.PostForm("upstream_api_key")),
	}
	inst, err := h.instances.Create(c.Request.Context(), params)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	list, err := h.instances.List(c.Request.Context())
	if err != nil {
		list = []instsvc.InstanceWithPaths{}
	}
	data := gin.H{"Instance": inst}
	data["Instances"] = list
	data["CurrentUser"] = CurrentUser(c)
	data["AuthEnabled"] = h.auth != nil && h.auth.IsEnabled()
	data["ActiveTab"] = normalizeInstancesTab(c.Query("state"))
	data["ViewMode"] = normalizeInstancesView(c.Query("view"))
	c.HTML(http.StatusOK, "auth_form_with_table", data)
}

func (h *AdminHandler) UpdateInstance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	params := instsvc.UpdateParams{
		ID:              id,
		Name:            strings.TrimSpace(c.PostForm("name")),
		InternalToken:   strings.TrimSpace(c.PostForm("internal_token")),
		AuthMode:        strings.TrimSpace(c.PostForm("auth_mode")),
		UpstreamBaseURL: strings.TrimSpace(c.PostForm("upstream_base_url")),
		UpstreamAPIKey:  strings.TrimSpace(c.PostForm("upstream_api_key")),
	}
	if _, err := h.instances.Update(c.Request.Context(), params); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Header("HX-Trigger", "{\"refreshInstances\":true,\"closeModal\":true}")
	h.InstancesTable(c)
}

func (h *AdminHandler) DeleteInstance(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.instances.Delete(c.Request.Context(), id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	h.InstancesTable(c)
}

func (h *AdminHandler) CloseModal(c *gin.Context) {
	c.String(http.StatusOK, "")
}

func (h *AdminHandler) AuthForm(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	content := ""
	if rec, err := h.instances.GetAuth(c.Request.Context(), id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	} else if rec != nil {
		content = rec.AuthJSON
	}
	c.HTML(http.StatusOK, "auth_form", gin.H{
		"Instance":  inst,
		"Content":   content,
		"ActiveTab": normalizeInstancesTab(c.Query("state")),
		"ViewMode":  normalizeInstancesView(c.Query("view")),
	})
}

func (h *AdminHandler) BulkAuthForm(c *gin.Context) {
	c.HTML(http.StatusOK, "auth_bulk_form", gin.H{})
}

func (h *AdminHandler) ExportAuthBulk(c *gin.Context) {
	records, err := h.instances.ListAuth(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	blocks := make([]string, 0, len(records))
	for _, rec := range records {
		if s := strings.TrimSpace(rec.AuthJSON); s != "" {
			blocks = append(blocks, s)
		}
	}
	out := strings.Join(blocks, "\n---\n")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="auth.bulk.txt"`)
	c.String(http.StatusOK, out)
}

func splitAuthBulk(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	var blocks []string
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			blocks = append(blocks, s)
		}
		cur.Reset()
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
	}
	flush()
	return blocks
}

func (h *AdminHandler) BulkAuthSubmit(c *gin.Context) {
	raw := c.PostForm("auth_bulk")
	if strings.TrimSpace(raw) == "" {
		c.String(http.StatusBadRequest, "auth_bulk is required")
		return
	}
	prefix := strings.TrimSpace(c.PostForm("name_prefix"))
	if prefix == "" {
		prefix = "Imported"
	}
	enable := c.PostForm("enable") == "true"

	blocks := splitAuthBulk(raw)
	if len(blocks) == 0 {
		c.String(http.StatusBadRequest, "no auth blocks found")
		return
	}

	existingMeta, err := h.instances.ListAuthMeta(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	existingDupKeys := make(map[string]int64, len(existingMeta))
	for _, meta := range existingMeta {
		key := strings.TrimSpace(meta.DupKey)
		if key == "" {
			continue
		}
		if prev, ok := existingDupKeys[key]; !ok || meta.InstanceID < prev {
			existingDupKeys[key] = meta.InstanceID
		}
	}

	type authBlock struct {
		Raw    string
		DupKey string
	}
	seen := make(map[[32]byte]bool, len(blocks))
	unique := make([]authBlock, 0, len(blocks))
	for i, block := range blocks {
		var tmp any
		if err := json.Unmarshal([]byte(block), &tmp); err != nil {
			c.String(http.StatusBadRequest, "invalid json in block %d: %v", i+1, err)
			return
		}
		canon, err := json.Marshal(tmp)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		sum := sha256.Sum256(canon)
		if seen[sum] {
			continue
		}
		seen[sum] = true
		dupKey := ""
		if payload, ok := tmp.(map[string]any); ok {
			dupKey = authDuplicateKey(payload)
		}
		if dupKey == "" {
			dupKey = fmt.Sprintf("sha:%x", sum)
		}
		unique = append(unique, authBlock{Raw: block, DupKey: dupKey})
	}

	width := len(strconv.Itoa(len(unique)))
	if width < 2 {
		width = 2
	}

	dupFound := false
	dupSeen := make(map[string]bool, len(unique))
	for i, item := range unique {
		block := item.Raw
		key := strings.TrimSpace(item.DupKey)
		if key != "" {
			if existingDupKeys[key] != 0 {
				dupFound = true
			}
			if dupSeen[key] {
				dupFound = true
			}
			dupSeen[key] = true
		}
		name := ""
		if len(unique) == 1 {
			name = prefix
		} else {
			name = fmt.Sprintf("%s-%0*d", prefix, width, i+1)
		}
		inst, err := h.instances.Create(c.Request.Context(), instsvc.CreateParams{Name: name})
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.instances.SaveAuth(c.Request.Context(), inst.ID, block); err != nil {
			var ve *instsvc.ValidationError
			if errors.As(err, &ve) {
				c.String(http.StatusBadRequest, ve.Error())
				return
			}
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if h.proxy != nil {
			h.proxy.InvalidateInstanceAuth(inst.ID)
		}
		if enable {
			if err := h.instances.SetEnabled(c.Request.Context(), inst.ID, true); err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
		}
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] auth imported", time.Now().Format(time.RFC3339)))
	}

	if dupFound {
		c.Header("HX-Trigger", "{\"closeModal\":true,\"toast\":\"存在重复实例，已标识\"}")
	} else {
		c.Header("HX-Trigger", "{\"closeModal\":true}")
	}
	h.InstancesTable(c)
}

func (h *AdminHandler) AuthView(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	rec, err := h.instances.GetAuth(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if rec == nil || strings.TrimSpace(rec.AuthJSON) == "" {
		c.String(http.StatusNotFound, "auth not found")
		return
	}
	c.HTML(http.StatusOK, "auth_view", gin.H{
		"Instance":  inst,
		"Content":   rec.AuthJSON,
		"ActiveTab": normalizeInstancesTab(c.Query("state")),
		"ViewMode":  normalizeInstancesView(c.Query("view")),
	})
}

func (h *AdminHandler) SaveAuth(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	authJSON := c.PostForm("auth_json")
	if strings.TrimSpace(authJSON) == "" {
		c.String(http.StatusBadRequest, "auth_json is required")
		return
	}
	enable := c.PostForm("enable") == "true"
	if err := h.instances.SaveAuth(c.Request.Context(), id, authJSON); err != nil {
		var ve *instsvc.ValidationError
		if errors.As(err, &ve) {
			c.String(http.StatusBadRequest, ve.Error())
			return
		}
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if h.proxy != nil {
		h.proxy.InvalidateInstanceAuth(id)
	}
	if enable {
		if err := h.instances.SetEnabled(c.Request.Context(), id, true); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}
	// 记录日志
	_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] auth saved", time.Now().Format(time.RFC3339)))
	c.Header("HX-Trigger", "{\"closeModal\":true}")
	h.InstancesTable(c)
}

func (h *AdminHandler) DeleteAuth(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if err := h.instances.DeleteAuth(c.Request.Context(), id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if h.proxy != nil {
		h.proxy.InvalidateInstanceAuth(id)
	}
	_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] auth deleted", time.Now().Format(time.RFC3339)))
	// 触发前端刷新实例表格
	c.Header("HX-Trigger", "{\"refreshInstances\":true}")
	c.Status(http.StatusNoContent)
}

func (h *AdminHandler) LogView(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	logContent := readLogTail(inst.LogPath, 4000)
	c.HTML(http.StatusOK, "log_view", gin.H{
		"Instance": inst,
		"Log":      logContent,
	})
}

// 新标签页查看完整日志（自动刷新）
func (h *AdminHandler) LogFullPage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil || inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	h.renderPage(c, http.StatusOK, "log_full", gin.H{"Instance": inst})
}

// 返回完整原始日志内容（供 hx 每 3s 刷新）
func (h *AdminHandler) LogRaw(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil || inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	var content string
	if strings.TrimSpace(inst.LogPath) != "" {
		b, err := os.ReadFile(inst.LogPath)
		if err == nil {
			content = string(b)
		}
	}
	// 返回只读 <pre> 片段（无任何自动刷新属性），供按钮手动刷新时替换。
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<pre id="log-pre">%s</pre>`, htmlEscapeStr(content))
}

func (h *AdminHandler) DeleteLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if strings.TrimSpace(inst.LogPath) != "" {
		if err := os.WriteFile(inst.LogPath, []byte{}, 0o644); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.InstancesTable(c)
}

func readLogTail(path string, max int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "log file not found"
		}
		return fmt.Sprintf("unable to read log: %v", err)
	}
	if len(data) <= max {
		return string(data)
	}
	return "…" + string(data[len(data)-max:])
}

func (h *AdminHandler) LoginModal(c *gin.Context) {
	inst, session, ok := h.fetchInstanceAndSession(c)
	if !ok {
		return
	}
	// 已有凭据仍允许进入引导（按你最新要求，是否打开引导由凭据判断但不影响启动/停止按钮展示）
	c.HTML(http.StatusOK, "login_modal", h.loginModalData(c, inst, session))
}

func (h *AdminHandler) StartLogin(c *gin.Context) {
	inst, _, ok := h.fetchInstanceAndSession(c)
	if !ok {
		return
	}
	session, err := h.login.Start(context.Background(), *inst)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] login started", time.Now().Format(time.RFC3339)))
	c.HTML(http.StatusOK, "login_modal", h.loginModalData(c, inst, session))
}

func (h *AdminHandler) CancelLogin(c *gin.Context) {
	inst, _, ok := h.fetchInstanceAndSession(c)
	if !ok {
		return
	}
	h.login.Cancel(inst.ID)
	c.Header("HX-Trigger", "{\"refreshInstances\":true}")
	c.HTML(http.StatusOK, "login_modal", h.loginModalData(c, inst, h.login.SessionInfo(inst.ID)))
}

func (h *AdminHandler) LoginForceCancel(c *gin.Context) {
	inst, session, ok := h.fetchInstanceAndSession(c)
	if !ok {
		return
	}
	h.login.Cancel(inst.ID)
	if h.system != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		port := 0
		if session != nil && session.Port > 0 {
			port = session.Port
		} else if h.cfg != nil {
			port = h.cfg.LoginCallbackPort
		}
		if port > 0 {
			_, _ = h.system.KillPort(ctx, port)
		}
	}
	c.Header("HX-Trigger", "{\"refreshInstances\":true}")
	c.HTML(http.StatusOK, "login_modal", h.loginModalData(c, inst, h.login.SessionInfo(inst.ID)))
}

func (h *AdminHandler) LoginStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	// 获取实例以便在成功时回填路径
	inst, _ := h.instances.Get(c.Request.Context(), id)
	session := h.login.SessionInfo(id)
	if session == nil {
		c.String(http.StatusOK, "Idle")
		return
	}
	if session.Result == nil {
		c.String(http.StatusOK, "等待授权完成…")
		return
	}
	if session.Result.Success {
		// 登录成功：标记实例为已启动
		_ = h.instances.SetEnabled(c.Request.Context(), id, true)
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] login success, enabled", time.Now().Format(time.RFC3339)))
		if h.proxy != nil {
			h.proxy.InvalidateInstanceAuth(id)
		}
		// 通知外层刷新实例列表
		c.Header("HX-Trigger", "{\"refreshInstances\":true}")
		// 成功后自动刷新整个弹窗，以便按钮状态与提示切换到“登录成功”
		// 通过插入一个带 hx-get 的占位元素在插入时自动拉取最新弹窗内容
		fetchURL := h.mount(fmt.Sprintf("/admin/instances/%d/login?state=%s", id, normalizeInstancesTab(c.Query("state"))))
		fetch := fmt.Sprintf(`<div hx-get="%s" hx-target="#modal" hx-swap="innerHTML" hx-trigger="load"></div>`, fetchURL)
		html := fmt.Sprintf(`<span class="text-emerald-600">登录成功，凭据已保存到数据库。</span>%s`, fetch)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
		return
	}
	c.String(http.StatusOK, fmt.Sprintf("登录失败：%s", session.Result.Message))
}

// 启用/停用实例（不修改凭据）
func (h *AdminHandler) SetInstanceEnabled(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	activeTab := normalizeInstancesTab(c.Query("state"))
	enable := strings.TrimSpace(c.PostForm("enable")) == "true"
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil || inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if enable {
		// ChatGPT OAuth：验证凭据存在且 JSON 合法，否则打开登录引导
		if !strings.EqualFold(inst.AuthMode, "api_key") {
			rec, err := h.instances.GetAuth(c.Request.Context(), id)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			if rec == nil || strings.TrimSpace(rec.AuthJSON) == "" || !validateAuthJSONContent(rec.AuthJSON) {
				fetchURL := h.mount(fmt.Sprintf("/admin/instances/%d/login?state=%s", id, activeTab))
				html := fmt.Sprintf(`<div hx-get="%s" hx-target="#modal" hx-swap="innerHTML" hx-trigger="load"></div>`, fetchURL)
				c.Header("Content-Type", "text/html; charset=utf-8")
				c.String(http.StatusOK, html)
				return
			}
		}
	}
	if err := h.instances.SetEnabled(c.Request.Context(), id, enable); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	// 写入启停日志
	if enable {
		// 启动时清空旧日志
		if strings.TrimSpace(inst.LogPath) != "" {
			_ = os.Remove(inst.LogPath)
		}
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] instance started", time.Now().Format(time.RFC3339)))
	} else {
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] instance stopped", time.Now().Format(time.RFC3339)))
	}
	if !enable && h.login != nil {
		h.login.Cancel(id)
	}
	// 直接返回最新表格片段，确保按钮即时切换
	h.InstancesTable(c)
}

func (h *AdminHandler) DebugForm(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.HTML(http.StatusOK, "debug_form", gin.H{
		"Instance":  inst,
		"ActiveTab": normalizeInstancesTab(c.Query("state")),
		"ViewMode":  normalizeInstancesView(c.Query("view")),
	})
}

func (h *AdminHandler) BulkDebugForm(c *gin.Context) {
	rawIDs := c.QueryArray("ids")
	if len(rawIDs) == 0 {
		c.String(http.StatusBadRequest, "ids are required")
		return
	}
	ids := make([]int64, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid id")
			return
		}
		ids = append(ids, id)
	}
	c.HTML(http.StatusOK, "debug_bulk_form", gin.H{
		"IDs":       ids,
		"Count":     len(ids),
		"ActiveTab": normalizeInstancesTab(c.Query("state")),
		"ViewMode":  normalizeInstancesView(c.Query("view")),
	})
}

func (h *AdminHandler) SetInstanceDebug(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	enable := strings.TrimSpace(c.PostForm("enable")) == "true"
	hideEventData := strings.TrimSpace(c.PostForm("hide_event_data")) == "true"
	hideReqHeaders := strings.TrimSpace(c.PostForm("hide_req_headers")) == "true"
	hideReqBody := strings.TrimSpace(c.PostForm("hide_req_body")) == "true"
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil || inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	cfg := instsvc.DebugConfig{
		Enabled:                    enable,
		DetailEnabled:              !hideEventData,
		LogReqHeaders:              !hideReqHeaders,
		LogReqBody:                 !hideReqBody,
		LogReqBodyMode:             inst.DebugLogReqBodyMode,
		LogRedactHeaders:           inst.DebugLogRedactHeaders,
		SSECompressOutputTextDelta: inst.DebugSSECompressOutputTextDelta,
		SSEKeepalive:               inst.DebugSSEKeepalive,
		SSEMaskInstructions:        inst.DebugSSEMaskInstructions,
		SSEMaskText:                inst.DebugSSEMaskText,
	}
	if err := h.instances.SetDebugConfig(c.Request.Context(), id, cfg); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	// 写入日志
	if !enable {
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] debug disabled", time.Now().Format(time.RFC3339)))
	} else {
		var hidden []string
		if hideEventData {
			hidden = append(hidden, "event/data")
		}
		if hideReqHeaders {
			hidden = append(hidden, "REQ HDR")
		}
		if hideReqBody {
			hidden = append(hidden, "REQ BODY")
		}
		line := "debug enabled"
		if len(hidden) > 0 {
			line += " (hide: " + strings.Join(hidden, ", ") + ")"
		}
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), line))
	}
	c.Header("HX-Trigger", "{\"refreshInstances\":true,\"closeModal\":true}")
	h.InstancesTable(c)
}

func (h *AdminHandler) BulkSetInstanceEnabled(c *gin.Context) {
	ids := c.PostFormArray("ids")
	if len(ids) == 0 {
		c.String(http.StatusBadRequest, "ids are required")
		return
	}
	enable := strings.TrimSpace(c.PostForm("enable")) == "true"

	instances := make([]*instsvc.InstanceWithPaths, 0, len(ids))
	for _, raw := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid id")
			return
		}
		inst, err := h.instances.Get(c.Request.Context(), id)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if inst == nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		instances = append(instances, inst)
	}

	if enable {
		for _, inst := range instances {
			if strings.EqualFold(inst.AuthMode, "api_key") {
				continue
			}
			rec, err := h.instances.GetAuth(c.Request.Context(), inst.ID)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			if rec == nil || strings.TrimSpace(rec.AuthJSON) == "" || !validateAuthJSONContent(rec.AuthJSON) {
				data, err := h.instancesTableData(c)
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
					return
				}
				data["Modal"] = h.loginModalData(c, inst, h.login.SessionInfo(inst.ID))
				c.HTML(http.StatusOK, "instances_table_with_modal", data)
				return
			}
		}
	}

	for _, inst := range instances {
		if err := h.instances.SetEnabled(c.Request.Context(), inst.ID, enable); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if enable {
			if strings.TrimSpace(inst.LogPath) != "" {
				_ = os.Remove(inst.LogPath)
			}
			_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] instance started", time.Now().Format(time.RFC3339)))
		} else {
			_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] instance stopped", time.Now().Format(time.RFC3339)))
			if h.login != nil {
				h.login.Cancel(inst.ID)
			}
		}
	}
	h.InstancesTable(c)
}

func (h *AdminHandler) BulkSetInstanceDebug(c *gin.Context) {
	ids := c.PostFormArray("ids")
	if len(ids) == 0 {
		c.String(http.StatusBadRequest, "ids are required")
		return
	}
	enable := strings.TrimSpace(c.PostForm("enable")) == "true"
	hideEventData := strings.TrimSpace(c.PostForm("hide_event_data")) == "true"
	hideReqHeaders := strings.TrimSpace(c.PostForm("hide_req_headers")) == "true"
	hideReqBody := strings.TrimSpace(c.PostForm("hide_req_body")) == "true"
	for _, raw := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid id")
			return
		}
		inst, err := h.instances.Get(c.Request.Context(), id)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if inst == nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		cfg := instsvc.DebugConfig{
			Enabled:                    enable,
			DetailEnabled:              !hideEventData,
			LogReqHeaders:              !hideReqHeaders,
			LogReqBody:                 !hideReqBody,
			LogReqBodyMode:             inst.DebugLogReqBodyMode,
			LogRedactHeaders:           inst.DebugLogRedactHeaders,
			SSECompressOutputTextDelta: inst.DebugSSECompressOutputTextDelta,
			SSEKeepalive:               inst.DebugSSEKeepalive,
			SSEMaskInstructions:        inst.DebugSSEMaskInstructions,
			SSEMaskText:                inst.DebugSSEMaskText,
		}
		if err := h.instances.SetDebugConfig(c.Request.Context(), id, cfg); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if !enable {
			_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] debug disabled", time.Now().Format(time.RFC3339)))
		} else {
			var hidden []string
			if hideEventData {
				hidden = append(hidden, "event/data")
			}
			if hideReqHeaders {
				hidden = append(hidden, "REQ HDR")
			}
			if hideReqBody {
				hidden = append(hidden, "REQ BODY")
			}
			line := "debug enabled"
			if len(hidden) > 0 {
				line += " (hide: " + strings.Join(hidden, ", ") + ")"
			}
			_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), line))
		}
	}
	c.Header("HX-Trigger", "{\"refreshInstances\":true,\"closeModal\":true}")
	h.InstancesTable(c)
}

func (h *AdminHandler) BulkDeleteInstances(c *gin.Context) {
	ids := c.PostFormArray("ids")
	if len(ids) == 0 {
		c.String(http.StatusBadRequest, "ids are required")
		return
	}
	for _, raw := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid id")
			return
		}
		inst, err := h.instances.Get(c.Request.Context(), id)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if inst == nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		if err := h.instances.Delete(c.Request.Context(), id); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.InstancesTable(c)
}

func readEnvFileValues(path string) (map[string]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return map[string]string{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make(map[string]string, 32)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}
		eq := strings.IndexRune(trimmed, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out, nil
}

func setDefaultEnvValue(values map[string]string, key string, value string) {
	if values == nil {
		return
	}
	if _, ok := values[key]; ok {
		return
	}
	values[key] = value
}

// 本文件内局部 HTML 转义（避免引入模板依赖）
func htmlEscapeStr(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")
	return replacer.Replace(s)
}

// 追加写实例日志（忽略错误）
func appendFileLine(path string, line string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	if err != nil {
		return err
	}
	// 启用 1 小时保留策略
	pruneLogOneHour(path)
	return nil
}

// 保留最近 1 小时日志（基于行首 [RFC3339] 前缀）。
func pruneLogOneHour(path string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	lines := strings.Split(string(data), "\n")
	cutoff := time.Now().Add(-1 * time.Hour)
	keepFrom := 0
	for i := len(lines) - 1; i >= 0; i-- { // 从尾部向前找第一个满足 cutoff 的位置
		line := strings.TrimSpace(lines[i])
		if len(line) < 3 {
			continue
		}
		if line[0] == '[' {
			if idx := strings.IndexRune(line, ']'); idx > 1 {
				ts := line[1:idx]
				if t, e := time.Parse(time.RFC3339, ts); e == nil {
					if t.Before(cutoff) {
						break
					}
					keepFrom = i
				}
			}
		}
	}
	if keepFrom <= 0 {
		return
	}
	trimmed := strings.Join(lines[keepFrom:], "\n")
	_ = os.WriteFile(path, []byte(trimmed), 0o644)
}

// validateAuthJSONContent 检查 auth.json 是否为 JSON 且 tokens 内至少包含 refresh_token/access_token/id_token 中之一。
func validateAuthJSONContent(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	var obj map[string]any
	if json.Unmarshal([]byte(content), &obj) != nil {
		return false
	}
	tokens, _ := obj["tokens"].(map[string]any)
	if tokens == nil {
		return false
	}
	for _, k := range []string{"refresh_token", "access_token", "id_token"} {
		if v, ok := tokens[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

func (h *AdminHandler) EnvForm(c *gin.Context) {
	if h.cfg != nil && h.cfg.ServerPort == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	envFile := strings.TrimSpace(config.LoadedEnvFile())
	if envFile == "" {
		envFile = ".env"
	}
	values, err := readEnvFileValues(envFile)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if values == nil {
		values = make(map[string]string)
	}
	if h.cfg != nil {
		setDefaultEnvValue(values, "SERVER_HOST", h.cfg.ServerHost)
		setDefaultEnvValue(values, "SERVER_PORT", strconv.Itoa(h.cfg.ServerPort))
		setDefaultEnvValue(values, "TOKEN", h.cfg.ServiceToken)
		setDefaultEnvValue(values, "DATABASE_PATH", h.cfg.DatabasePath)
		setDefaultEnvValue(values, "AUTH_DIR", h.cfg.AuthDir)
		setDefaultEnvValue(values, "LOG_DIR", h.cfg.LogDir)
		setDefaultEnvValue(values, "DEFAULT_UPSTREAM_BASE_URL", h.cfg.DefaultUpstreamBaseURL)
		setDefaultEnvValue(values, "LOGIN_CLIENT_ID", h.cfg.LoginClientID)
		setDefaultEnvValue(values, "LOGIN_ISSUER", h.cfg.LoginIssuer)
		setDefaultEnvValue(values, "LOGIN_CALLBACK_HOST", h.cfg.LoginCallbackHost)
		setDefaultEnvValue(values, "LOGIN_REDIRECT_HOST", h.cfg.LoginRedirectHost)
		setDefaultEnvValue(values, "LOGIN_CALLBACK_PORT", strconv.Itoa(h.cfg.LoginCallbackPort))
		setDefaultEnvValue(values, "WEB_ADMIN_USER", h.cfg.AdminUser)
		setDefaultEnvValue(values, "WEB_ADMIN_PASS", h.cfg.AdminPass)
	}
	c.HTML(http.StatusOK, "env_form", gin.H{
		"EnvFile": envFile,
		"Values":  values,
	})
}

func (h *AdminHandler) EnvSubmit(c *gin.Context) {
	if h.cfg != nil && h.cfg.ServerPort == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	envFile := strings.TrimSpace(config.LoadedEnvFile())
	if envFile == "" {
		envFile = ".env"
	}

	read := func(key string) string {
		return strings.TrimSpace(c.PostForm(key))
	}

	serverHost := read("SERVER_HOST")
	if serverHost == "" {
		c.String(http.StatusBadRequest, "SERVER_HOST is required")
		return
	}
	serverPortStr := read("SERVER_PORT")
	port, err := strconv.Atoi(serverPortStr)
	if err != nil || port < 1 || port > 65535 {
		c.String(http.StatusBadRequest, "invalid SERVER_PORT")
		return
	}

	callbackPortStr := read("LOGIN_CALLBACK_PORT")
	callbackPort := 0
	if callbackPortStr != "" {
		n, err := strconv.Atoi(callbackPortStr)
		if err != nil || n < 1 || n > 65535 {
			c.String(http.StatusBadRequest, "invalid LOGIN_CALLBACK_PORT")
			return
		}
		callbackPort = n
	}

	updates := map[string]string{
		"SERVER_HOST": serverHost,
		"SERVER_PORT": strconv.Itoa(port),
		"TOKEN":       read("TOKEN"),

		"DATABASE_PATH": read("DATABASE_PATH"),
		"AUTH_DIR":      read("AUTH_DIR"),
		"LOG_DIR":       read("LOG_DIR"),

		"DEFAULT_UPSTREAM_BASE_URL": read("DEFAULT_UPSTREAM_BASE_URL"),

		"LOGIN_CLIENT_ID":     read("LOGIN_CLIENT_ID"),
		"LOGIN_ISSUER":        read("LOGIN_ISSUER"),
		"LOGIN_CALLBACK_HOST": read("LOGIN_CALLBACK_HOST"),
		"LOGIN_REDIRECT_HOST": read("LOGIN_REDIRECT_HOST"),

		"WEB_ADMIN_USER": read("WEB_ADMIN_USER"),
		"WEB_ADMIN_PASS": read("WEB_ADMIN_PASS"),
	}
	if callbackPortStr != "" {
		updates["LOGIN_CALLBACK_PORT"] = strconv.Itoa(callbackPort)
	} else {
		updates["LOGIN_CALLBACK_PORT"] = ""
	}

	if err := config.UpdateEnvFileWithMarker(envFile, updates, "# Settings (managed via web admin)"); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	c.Header("HX-Trigger", "{\"closeModal\":true,\"toast\":\"已写入 .env（重启后生效）\"}")
	c.String(http.StatusOK, "")
}

func (h *AdminHandler) Restart(c *gin.Context) {
	if h.auth == nil || !h.auth.IsEnabled() {
		c.String(http.StatusForbidden, "restart requires admin auth enabled")
		return
	}
	if h.restart == nil {
		c.String(http.StatusNotImplemented, "restart not configured")
		return
	}
	c.Header("HX-Trigger", "{\"toast\":\"服务即将重启\"}")
	c.Status(http.StatusNoContent)
	go func() {
		time.Sleep(300 * time.Millisecond)
		h.restart()
	}()
}

// 刷新计划状态卡片
func (h *AdminHandler) RefreshStatus(c *gin.Context) {
	envFile := strings.TrimSpace(config.LoadedEnvFile())
	if envFile == "" {
		envFile = ".env"
	}
	c.HTML(http.StatusOK, "refresh_status", gin.H{
		"Enabled": proxysvc.TokenRefreshEnabled(),
		"MinDays": h.cfg.RefreshMinDays,
		"MaxDays": h.cfg.RefreshMaxDays,
		"EnvFile": envFile,
	})
}

func (h *AdminHandler) RefreshToggle(c *gin.Context) {
	enable := strings.TrimSpace(c.PostForm("enable")) == "true"
	h.cfg.RefreshEnabled = enable
	proxysvc.SetTokenRefreshEnabled(enable)
	if h.refresher != nil {
		h.refresher.SetEnabled(enable)
	}
	if err := h.persistRefreshConfig(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Header("HX-Trigger", "{\"refreshInstances\":true}")
	h.RefreshStatus(c)
}

func (h *AdminHandler) RefreshConfig(c *gin.Context) {
	minDays, err := strconv.Atoi(strings.TrimSpace(c.PostForm("min_days")))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid min_days")
		return
	}
	maxDays, err := strconv.Atoi(strings.TrimSpace(c.PostForm("max_days")))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid max_days")
		return
	}
	if minDays < 1 {
		minDays = 1
	}
	if maxDays < minDays {
		maxDays = minDays
	}
	h.cfg.RefreshMinDays = minDays
	h.cfg.RefreshMaxDays = maxDays
	if h.refresher != nil {
		if r, ok := h.refresher.(interface{ SetRangeDays(min, max int) }); ok {
			r.SetRangeDays(minDays, maxDays)
		}
	}
	if err := h.persistRefreshConfig(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	h.RefreshStatus(c)
}

func (h *AdminHandler) persistRefreshConfig() error {
	envFile := strings.TrimSpace(config.LoadedEnvFile())
	if envFile == "" {
		envFile = ".env"
	}
	updates := map[string]string{
		"REFRESH_ENABLED":  strconv.FormatBool(proxysvc.TokenRefreshEnabled()),
		"REFRESH_MIN_DAYS": strconv.Itoa(h.cfg.RefreshMinDays),
		"REFRESH_MAX_DAYS": strconv.Itoa(h.cfg.RefreshMaxDays),
	}
	if err := config.UpdateEnvFile(envFile, updates); err != nil {
		return err
	}
	_ = os.Setenv("REFRESH_ENABLED", updates["REFRESH_ENABLED"])
	_ = os.Setenv("REFRESH_MIN_DAYS", updates["REFRESH_MIN_DAYS"])
	_ = os.Setenv("REFRESH_MAX_DAYS", updates["REFRESH_MAX_DAYS"])
	return nil
}

// 主动刷新实例凭据
func (h *AdminHandler) RefreshInstance(c *gin.Context) {
	if !proxysvc.TokenRefreshEnabled() {
		c.String(http.StatusConflict, "refresh_disabled")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil || inst == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if strings.EqualFold(inst.AuthMode, "api_key") {
		c.String(http.StatusConflict, "auth_mode_api_key")
		return
	}
	rec, err := h.instances.GetAuth(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if rec == nil || strings.TrimSpace(rec.AuthJSON) == "" {
		c.String(http.StatusConflict, "auth_missing")
		return
	}
	if err := h.proxy.RefreshInstanceAuth(c.Request.Context(), id); err != nil {
		_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] refresh failed: %v", time.Now().Format(time.RFC3339), err))
		status := http.StatusBadGateway
		var refreshErr *proxysvc.RefreshError
		if errors.As(err, &refreshErr) {
			if refreshErr.StatusCode == http.StatusUnauthorized {
				status = http.StatusConflict
			} else if refreshErr.StatusCode >= 400 && refreshErr.StatusCode < 500 {
				status = http.StatusBadRequest
			}
		}
		c.String(status, err.Error())
		return
	}
	_ = appendFileLine(inst.LogPath, fmt.Sprintf("[%s] refresh success", time.Now().Format(time.RFC3339)))
	c.Header("HX-Trigger", "{\"refreshInstances\":true}")
	c.Status(http.StatusNoContent)
}

func (h *AdminHandler) fetchInstanceAndSession(c *gin.Context) (*instsvc.InstanceWithPaths, *loginsvc.SessionInfo, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return nil, nil, false
	}
	inst, err := h.instances.Get(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return nil, nil, false
	}
	if inst == nil {
		c.String(http.StatusNotFound, "not found")
		return nil, nil, false
	}
	var session *loginsvc.SessionInfo
	if h.login != nil {
		session = h.login.SessionInfo(id)
	}
	return inst, session, true
}

func (h *AdminHandler) loginModalData(c *gin.Context, inst *instsvc.InstanceWithPaths, session *loginsvc.SessionInfo) gin.H {
	if session != nil && session.Result != nil && !session.Result.Success && strings.EqualFold(session.Result.Message, "cancelled") {
		session = nil
	}
	host := hostFromRequest(c)
	defaultPort := 1455
	if h.cfg != nil && h.cfg.LoginCallbackPort > 0 {
		defaultPort = h.cfg.LoginCallbackPort
	}
	port := defaultPort
	if session != nil && session.Port > 0 {
		port = session.Port
	}
	authHost := strings.TrimSpace(h.cfg.LoginRedirectHost)
	if authHost == "" {
		authHost = "localhost"
	}
	defaultAuthURL := fmt.Sprintf("http://%s:%d", authHost, defaultPort)
	data := gin.H{
		"Instance":       inst,
		"SSHCommand":     buildSSHCommand(host, port, defaultPort),
		"LoginHost":      host,
		"DefaultPort":    defaultPort,
		"DefaultAuthURL": defaultAuthURL,
		"ApiBasePath":    inst.BasePath,
		"ResponsesPath":  fmt.Sprintf("%s/v1/responses", inst.BasePath),
		"InternalToken":  inst.InternalToken,
		"ActiveTab":      normalizeInstancesTab(c.Query("state")),
	}
	if session != nil {
		data["Session"] = session
		if session.Port > 0 {
			data["LocalAuthURL"] = fmt.Sprintf("http://%s:%d", authHost, session.Port)
		}
		if session.Result != nil && session.Result.Success {
			data["LoginSuccess"] = true
		}
	}
	return data
}

func (h *AdminHandler) LoginClose(c *gin.Context) {
	inst, _, ok := h.fetchInstanceAndSession(c)
	if !ok {
		return
	}
	h.login.Cancel(inst.ID)
	c.Header("HX-Trigger", "{\"refreshInstances\":true,\"closeModal\":true}")
	c.String(http.StatusOK, "")
}

func (h *AdminHandler) SSHStatus(c *gin.Context) {
	h.renderSSHStatus(c)
}

func (h *AdminHandler) SSHFirewall(c *gin.Context) {
	enable := c.PostForm("enable") == "true"
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.system.SetFirewall(ctx, enable); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	h.renderSSHStatus(c)
}

func (h *AdminHandler) SSHConfigure(c *gin.Context) {
	action := c.PostForm("action")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	var (
		result systemsvc.SSHConfigResult
		err    error
	)
	if action == "enable" {
		result, err = h.system.EnableSSH(ctx)
	} else if action == "disable" {
		result, err = h.system.DisableSSH(ctx)
	} else {
		c.String(http.StatusBadRequest, "unknown action")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	_ = result
	h.renderSSHStatus(c)
}

func (h *AdminHandler) renderSSHStatus(c *gin.Context) {
	if h.system == nil {
		c.String(http.StatusInternalServerError, "system service not configured")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	status := h.system.Status(ctx)
	c.HTML(http.StatusOK, "ssh_status", status)
}

func hostFromRequest(c *gin.Context) string {
	host := c.Request.Host
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return host
}

func isSafeRedirect(target string) bool {
	trim := strings.TrimSpace(target)
	if trim == "" {
		return false
	}
	if !strings.HasPrefix(trim, "/") {
		return false
	}
	if strings.HasPrefix(trim, "//") {
		return false
	}
	return true
}

func buildSSHCommand(host string, port int, fallback int) string {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		if fallback > 0 {
			port = fallback
		} else {
			port = 1455
		}
	}
	return fmt.Sprintf("ssh -4 -N -T -o ExitOnForwardFailure=yes -p 2222 -L 127.0.0.1:%[1]d:127.0.0.1:%[1]d -D 127.0.0.1:1080 user@%s", port, host)
}

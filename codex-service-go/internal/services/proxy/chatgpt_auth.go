package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// Align with Codex CLI default refresh cadence.
	tokenRefreshMaxAge = 8 * 24 * time.Hour

	defaultRefreshTokenURL         = "https://auth.openai.com/oauth/token"
	refreshTokenURLOverrideEnvVar  = "CODEX_REFRESH_TOKEN_URL_OVERRIDE"
	originatorOverrideEnvVar       = "CODEX_INTERNAL_ORIGINATOR_OVERRIDE"
	defaultOriginator              = "codex_cli_rs"
	defaultRefreshTokenUserAgent   = "codex_cli_rs/0.0.0 (codex-service-go)"
	refreshTokenExpiredMessage     = "Your access token could not be refreshed because your refresh token has expired. Please log out and sign in again."
	refreshTokenReusedMessage      = "Your access token could not be refreshed because your refresh token was already used. Please log out and sign in again."
	refreshTokenInvalidatedMessage = "Your access token could not be refreshed because your refresh token was revoked. Please log out and sign in again."
	refreshTokenUnknownMessage     = "Your access token could not be refreshed. Please log out and sign in again."
)

var chatGPTAuthCache sync.Map // key(string) -> *chatGPTAuth

func newRefreshHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

type chatGPTAuth struct {
	authFile   string
	authStore  AuthStore
	instanceID int64
	clientID   string
	httpClient *http.Client

	mu    sync.Mutex
	state chatGPTAuthState
}

type chatGPTAuthState struct {
	AccessToken  string
	RefreshToken string
	LastRefresh  time.Time
	AccountID    string
	IDToken      string
	fileModTime  time.Time
}

func newChatGPTAuth(authFile, clientID string) *chatGPTAuth {
	return &chatGPTAuth{
		authFile:   strings.TrimSpace(authFile),
		clientID:   strings.TrimSpace(clientID),
		httpClient: newRefreshHTTPClient(),
	}
}

// NewChatGPTAuth exposes a constructor so other services (e.g., refresher) can
// reuse the same refresh logic that persists last_refresh to auth.json.
func NewChatGPTAuth(authFile, clientID string) *chatGPTAuth {
	authFile = strings.TrimSpace(authFile)
	clientID = strings.TrimSpace(clientID)
	if authFile == "" || clientID == "" {
		return newChatGPTAuth(authFile, clientID)
	}
	key := filepath.Clean(authFile) + "|" + clientID
	if existing, ok := chatGPTAuthCache.Load(key); ok {
		return existing.(*chatGPTAuth)
	}
	auth := newChatGPTAuth(authFile, clientID)
	actual, _ := chatGPTAuthCache.LoadOrStore(key, auth)
	return actual.(*chatGPTAuth)
}

func NewChatGPTAuthForInstance(store AuthStore, instanceID int64, clientID string) *chatGPTAuth {
	clientID = strings.TrimSpace(clientID)
	if store == nil || instanceID <= 0 || clientID == "" {
		return &chatGPTAuth{
			authStore:  store,
			instanceID: instanceID,
			clientID:   clientID,
			httpClient: newRefreshHTTPClient(),
		}
	}
	key := fmt.Sprintf("db:%d|%s", instanceID, clientID)
	if existing, ok := chatGPTAuthCache.Load(key); ok {
		return existing.(*chatGPTAuth)
	}
	auth := &chatGPTAuth{
		authStore:  store,
		instanceID: instanceID,
		clientID:   clientID,
		httpClient: newRefreshHTTPClient(),
	}
	actual, _ := chatGPTAuthCache.LoadOrStore(key, auth)
	return actual.(*chatGPTAuth)
}

func (a *chatGPTAuth) isDB() bool {
	return a != nil && a.authStore != nil && a.instanceID > 0
}

func (a *chatGPTAuth) authRef() string {
	if a == nil {
		return ""
	}
	if a.isDB() {
		return fmt.Sprintf("db:%d", a.instanceID)
	}
	return a.authFile
}

// RefreshNow performs a refresh immediately and persists the updated tokens.
func (a *chatGPTAuth) RefreshNow() error { return a.refresh(context.Background()) }

func (a *chatGPTAuth) getBearer(ctx context.Context) (string, error) {
	if a == nil {
		return "", errors.New("chatgpt auth not configured")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureLoadedLocked(ctx); err != nil {
		return "", err
	}
	if a.state.AccessToken == "" {
		if a.state.RefreshToken == "" {
			return "", errors.New("access_token missing in auth")
		}
		if !TokenRefreshEnabled() {
			return "", errors.New("token refresh disabled")
		}
		if err := a.refreshLocked(ctx); err != nil {
			return "", err
		}
	}
	if a.isTokenStaleLocked() {
		if err := a.refreshLocked(ctx); err != nil {
			return "", err
		}
	}
	return a.state.AccessToken, nil
}

func (a *chatGPTAuth) refresh(ctx context.Context) error {
	if a == nil {
		return errors.New("chatgpt auth not configured")
	}
	if !TokenRefreshEnabled() {
		return errors.New("token refresh disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureLoadedLocked(ctx); err != nil {
		return err
	}
	return a.refreshLocked(ctx)
}

func (a *chatGPTAuth) getAccountID(ctx context.Context) (string, error) {
	if a == nil {
		return "", errors.New("chatgpt auth not configured")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureLoadedLocked(ctx); err != nil {
		return "", err
	}
	if a.state.AccountID != "" {
		return a.state.AccountID, nil
	}
	if acct := parseAccountIDFromIDToken(a.state.IDToken); acct != "" {
		a.state.AccountID = acct
		return acct, nil
	}
	if acct := parseAccountIDFromIDToken(readIDToken(ctx, a)); acct != "" {
		a.state.AccountID = acct
		return acct, nil
	}
	return "", nil
}

func (a *chatGPTAuth) ensureLoadedLocked(ctx context.Context) error {
	if a.isDB() {
		// DB 模式：依赖显式失效（InvalidateInstanceAuth）刷新缓存，避免每次请求查库。
		if a.state.AccessToken != "" || a.state.RefreshToken != "" || a.state.IDToken != "" {
			return nil
		}
		if a.authStore == nil || a.instanceID <= 0 {
			return errors.New("auth store not configured")
		}
		rec, err := a.authStore.GetAuth(ctx, a.instanceID)
		if err != nil {
			return fmt.Errorf("read auth: %w", err)
		}
		if rec == nil || strings.TrimSpace(rec.AuthJSON) == "" {
			return errors.New("auth not found")
		}
		raw := []byte(rec.AuthJSON)
		var payload map[string]interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("parse auth: %w", err)
		}
		tokens := extractTokens(payload)
		accessToken := tokens.accessToken
		if accessToken == "" {
			accessToken = tokens.idToken
		}
		if tokens.refreshToken == "" && accessToken == "" {
			return errors.New("auth missing access_token and refresh_token")
		}
		a.state.RefreshToken = tokens.refreshToken
		a.state.AccessToken = accessToken
		a.state.IDToken = tokens.idToken
		a.state.AccountID = tokens.accountID
		if ts, err := time.Parse(time.RFC3339, tokens.lastRefresh); err == nil {
			a.state.LastRefresh = ts
		}
		a.state.fileModTime = rec.UpdatedAt
		return nil
	}

	// 文件模式：支持外部修改，通过 mtime 自动重载
	if a.state.AccessToken != "" || a.state.RefreshToken != "" || a.state.IDToken != "" {
		if info, err := os.Stat(a.authFile); err == nil {
			if !a.state.fileModTime.IsZero() && info.ModTime().Equal(a.state.fileModTime) {
				return nil
			}
		}
	}
	if a.authFile == "" {
		return errors.New("auth file not configured")
	}
	info, err := os.Stat(a.authFile)
	if err != nil {
		return fmt.Errorf("stat auth file: %w", err)
	}
	raw, err := os.ReadFile(a.authFile)
	if err != nil {
		return fmt.Errorf("read auth file: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("parse auth file: %w", err)
	}
	tokens := extractTokens(payload)
	accessToken := tokens.accessToken
	if accessToken == "" {
		accessToken = tokens.idToken
	}
	if tokens.refreshToken == "" && accessToken == "" {
		return errors.New("auth file missing access_token and refresh_token")
	}
	a.state.RefreshToken = tokens.refreshToken
	a.state.AccessToken = accessToken
	a.state.IDToken = tokens.idToken
	a.state.AccountID = tokens.accountID
	if ts, err := time.Parse(time.RFC3339, tokens.lastRefresh); err == nil {
		a.state.LastRefresh = ts
	}
	a.state.fileModTime = info.ModTime()
	return nil
}

type RefreshError struct {
	StatusCode  int
	BackendCode string
	Message     string
}

func (e *RefreshError) Error() string { return e.Message }

func (a *chatGPTAuth) refreshLocked(ctx context.Context) error {
	if a.state.RefreshToken == "" {
		return errors.New("refresh_token missing in auth file")
	}
	payload, err := json.Marshal(map[string]string{
		"client_id":     a.clientID,
		"grant_type":    "refresh_token",
		"refresh_token": a.state.RefreshToken,
		"scope":         "openid profile email",
	})
	if err != nil {
		return err
	}
	endpoint := refreshTokenEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", refreshOriginator())
	req.Header.Set("User-Agent", defaultRefreshTokenUserAgent)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return &RefreshError{Message: err.Error()}
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		code := ""
		message := ""
		if resp.StatusCode == http.StatusUnauthorized {
			code = extractRefreshTokenErrorCode(bodyBytes)
			message = classifyRefreshTokenFailureMessage(code)
		} else {
			message = tryParseErrorMessage(bodyBytes)
		}
		if message == "" {
			message = strings.TrimSpace(string(bodyBytes))
		}
		if message == "" {
			message = fmt.Sprintf("refresh failed: %d", resp.StatusCode)
		}
		if code != "" {
			message = fmt.Sprintf("refresh failed: %d (%s): %s", resp.StatusCode, code, message)
		} else {
			message = fmt.Sprintf("refresh failed: %d: %s", resp.StatusCode, message)
		}
		return &RefreshError{StatusCode: resp.StatusCode, BackendCode: code, Message: message}
	}
	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return err
	}
	if data.AccessToken == "" {
		return errors.New("refresh succeeded without access_token")
	}
	if err := a.writeBackAuth(ctx, data.AccessToken, data.RefreshToken, data.IDToken); err != nil {
		return err
	}
	a.state.AccessToken = data.AccessToken
	if data.RefreshToken != "" {
		a.state.RefreshToken = data.RefreshToken
	}
	if data.IDToken != "" {
		a.state.IDToken = data.IDToken
		if acct := parseAccountIDFromIDToken(data.IDToken); acct != "" {
			a.state.AccountID = acct
		}
	}
	a.state.LastRefresh = time.Now().UTC()
	if a.isDB() {
		a.state.fileModTime = a.state.LastRefresh
	} else if info, err := os.Stat(a.authFile); err == nil {
		a.state.fileModTime = info.ModTime()
	}
	return nil
}

func (a *chatGPTAuth) isTokenStaleLocked() bool {
	if !TokenRefreshEnabled() {
		return false
	}
	if a.state.RefreshToken == "" {
		return false
	}
	if a.state.LastRefresh.IsZero() {
		return true
	}
	return time.Since(a.state.LastRefresh) >= tokenRefreshMaxAge
}

func refreshTokenEndpoint() string {
	if override := strings.TrimSpace(os.Getenv(refreshTokenURLOverrideEnvVar)); override != "" {
		return override
	}
	return defaultRefreshTokenURL
}

func refreshOriginator() string {
	if override := strings.TrimSpace(os.Getenv(originatorOverrideEnvVar)); override != "" {
		return override
	}
	return defaultOriginator
}

func extractRefreshTokenErrorCode(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal(body, &obj) != nil {
		return ""
	}
	if rawErr, ok := obj["error"]; ok {
		switch v := rawErr.(type) {
		case map[string]any:
			if code := getString(v, "code"); code != "" {
				return code
			}
		case string:
			return strings.TrimSpace(v)
		}
	}
	if code, ok := obj["code"].(string); ok {
		return strings.TrimSpace(code)
	}
	return ""
}

func classifyRefreshTokenFailureMessage(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "refresh_token_expired":
		return refreshTokenExpiredMessage
	case "refresh_token_reused":
		return refreshTokenReusedMessage
	case "refresh_token_invalidated":
		return refreshTokenInvalidatedMessage
	default:
		return refreshTokenUnknownMessage
	}
}

func tryParseErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal([]byte(trimmed), &obj) != nil {
		return trimmed
	}
	if rawErr, ok := obj["error"]; ok {
		if m, ok := rawErr.(map[string]any); ok {
			if msg := getString(m, "message"); msg != "" {
				return msg
			}
			if code := getString(m, "code"); code != "" {
				return code
			}
		} else if s, ok := rawErr.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	if msg, ok := obj["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if code, ok := obj["code"].(string); ok && strings.TrimSpace(code) != "" {
		return strings.TrimSpace(code)
	}
	return trimmed
}

type tokenSnapshot struct {
	accessToken  string
	refreshToken string
	idToken      string
	accountID    string
	lastRefresh  string
}

func extractTokens(payload map[string]interface{}) tokenSnapshot {
	var snap tokenSnapshot
	if payload == nil {
		return snap
	}
	if tokens, ok := payload["tokens"].(map[string]interface{}); ok {
		snap.accessToken = getString(tokens, "access_token")
		snap.refreshToken = getString(tokens, "refresh_token")
		snap.idToken = getString(tokens, "id_token")
		snap.accountID = getString(tokens, "account_id")
	}
	if snap.accessToken == "" {
		snap.accessToken = getString(payload, "access_token")
	}
	if snap.refreshToken == "" {
		snap.refreshToken = getString(payload, "refresh_token")
	}
	if snap.idToken == "" {
		snap.idToken = getString(payload, "id_token")
	}
	if snap.accountID == "" {
		snap.accountID = getString(payload, "account_id")
	}
	snap.lastRefresh = getString(payload, "last_refresh")
	return snap
}

func (a *chatGPTAuth) writeBackAuth(ctx context.Context, accessToken, refreshToken, idToken string) error {
	if a == nil {
		return errors.New("chatgpt auth not configured")
	}
	if a.isDB() {
		if a.authStore == nil || a.instanceID <= 0 {
			return errors.New("auth store not configured")
		}
		var raw []byte
		if rec, err := a.authStore.GetAuth(ctx, a.instanceID); err == nil && rec != nil {
			raw = []byte(rec.AuthJSON)
		}
		updated, err := mergeAuthPayload(raw, accessToken, refreshToken, idToken)
		if err != nil {
			return err
		}
		return a.authStore.SaveAuth(ctx, a.instanceID, string(updated))
	}
	return writeBackAuthFile(a.authFile, accessToken, refreshToken, idToken)
}

func mergeAuthPayload(raw []byte, accessToken, refreshToken, idToken string) ([]byte, error) {
	data := make(map[string]interface{})
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
	}
	tokens, _ := data["tokens"].(map[string]interface{})
	if tokens == nil {
		tokens = make(map[string]interface{})
	}
	lastRefresh := time.Now().UTC().Format(time.RFC3339)
	if accessToken != "" {
		tokens["access_token"] = accessToken
		data["access_token"] = accessToken
	}
	if refreshToken != "" {
		tokens["refresh_token"] = refreshToken
		data["refresh_token"] = refreshToken
	}
	if idToken != "" {
		tokens["id_token"] = idToken
		data["id_token"] = idToken
	}
	data["tokens"] = tokens
	data["last_refresh"] = lastRefresh
	tokens["last_refresh"] = lastRefresh
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(buf, '\n'), nil
}

func writeBackAuthFile(path, accessToken, refreshToken, idToken string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("empty auth file path")
	}
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	buf, err := mergeAuthPayload(raw, accessToken, refreshToken, idToken)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o600)
}

func parseAccountIDFromIDToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var body map[string]interface{}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	if auth, ok := body["https://api.openai.com/auth"].(map[string]interface{}); ok {
		if acct := getString(auth, "chatgpt_account_id"); acct != "" {
			return acct
		}
	}
	if auth, ok := body["auth"].(map[string]interface{}); ok {
		if acct := getString(auth, "chatgpt_account_id"); acct != "" {
			return acct
		}
	}
	return ""
}

func readIDToken(ctx context.Context, a *chatGPTAuth) string {
	if a == nil {
		return ""
	}
	if a.isDB() {
		if a.authStore == nil || a.instanceID <= 0 {
			return ""
		}
		rec, err := a.authStore.GetAuth(ctx, a.instanceID)
		if err != nil || rec == nil {
			return ""
		}
		return readIDTokenFromBytes([]byte(rec.AuthJSON))
	}
	return readIDTokenFromFile(a.authFile)
}

func readIDTokenFromBytes(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	tokens := extractTokens(payload)
	return tokens.idToken
}

func readIDTokenFromFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return readIDTokenFromBytes(raw)
}

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		case json.Number:
			return v.String()
		case float64:
			return fmt.Sprintf("%g", v)
		case int:
			return fmt.Sprintf("%d", v)
		case int64:
			return fmt.Sprintf("%d", v)
		}
	}
	return ""
}

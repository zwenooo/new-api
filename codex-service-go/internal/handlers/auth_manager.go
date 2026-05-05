package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"codex-service-go/internal/config"
)

const (
	sessionCookieName = "codex_admin_sid"
	contextUserKey    = "codex_admin_user"
	sessionTTL        = 12 * time.Hour
)

type sessionEntry struct {
	Username string
	Expires  time.Time
}

type AuthManager struct {
	enabled         bool
	username        string
	password        string
	credentialsFile string
	basePath        string
	secret          []byte
	sessions        map[string]sessionEntry
	mu              sync.Mutex
}

func NewAuthManager(cfg *config.Config) (*AuthManager, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate auth secret: %w", err)
	}
	basePath := ""
	if cfg != nil {
		basePath = strings.TrimRight(strings.TrimSpace(cfg.WebBasePath), "/")
		if basePath == "/" {
			basePath = ""
		}
	}

	mgr := &AuthManager{
		username: strings.TrimSpace(cfg.AdminUser),
		password: strings.TrimSpace(cfg.AdminPass),
		// credentialsFile 将不再使用，仅保留字段以避免大范围改动
		credentialsFile: "",
		basePath:        basePath,
		secret:          secret,
		sessions:        make(map[string]sessionEntry),
	}

	// 仅当 .env 中提供 WEB_ADMIN_USER/WEB_ADMIN_PASS 才启用后台登录
	if mgr.username != "" && mgr.password != "" {
		mgr.enabled = true
	}

	return mgr, nil
}

func (a *AuthManager) IsEnabled() bool {
	return a != nil && a.enabled
}

func (a *AuthManager) Validate(user, pass string) bool {
	if !a.IsEnabled() {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(user)), []byte(a.username)) == 1 &&
		subtle.ConstantTimeCompare([]byte(strings.TrimSpace(pass)), []byte(a.password)) == 1
}

func (a *AuthManager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.IsEnabled() {
			c.Set(contextUserKey, "")
			c.Next()
			return
		}
		if user, ok := a.currentUserFromRequest(c.Request); ok {
			c.Set(contextUserKey, user)
			c.Next()
			return
		}
		loginPath := a.mount("/admin/login")
		if c.GetHeader("HX-Request") == "true" {
			c.Header("HX-Redirect", loginPath)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Redirect(http.StatusFound, loginPath)
		c.Abort()
	}
}

func (a *AuthManager) CurrentUserFromRequest(r *http.Request) (string, bool) {
	if a == nil || !a.enabled {
		return "", false
	}
	return a.currentUserFromRequest(r)
}

func (a *AuthManager) currentUserFromRequest(r *http.Request) (string, bool) {
	if a == nil || !a.enabled {
		return "", false
	}
	token, err := r.Cookie(sessionCookieName)
	if err != nil {
		log.Printf("[auth] cookie missing: %v", err)
		return "", false
	}
	if user, ok := a.verifySession(token.Value); ok {
		return user, true
	}
	log.Printf("[auth] verify failed for token %s", token.Value)
	return "", false
}

func (a *AuthManager) CreateSession(username string) (string, time.Time, error) {
	if !a.IsEnabled() {
		return "", time.Time{}, fmt.Errorf("auth not configured")
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	sid := hex.EncodeToString(raw)
	signature := a.sign(sid)
	expires := time.Now().Add(sessionTTL)
	token := sid + "." + signature
	a.mu.Lock()
	a.sessions[sid] = sessionEntry{Username: username, Expires: expires}
	a.mu.Unlock()
	return token, expires, nil
}

func (a *AuthManager) Destroy(token string) {
	if token == "" {
		return
	}
	sid, ok := splitToken(token)
	if !ok {
		return
	}
	a.mu.Lock()
	delete(a.sessions, sid)
	a.mu.Unlock()
}

func (a *AuthManager) verifySession(token string) (string, bool) {
	sid, sig, ok := splitTokenWithSig(token)
	if !ok {
		return "", false
	}
	expected := a.sign(sid)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.sessions[sid]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.Expires) {
		delete(a.sessions, sid)
		return "", false
	}
	return entry.Username, true
}

func (a *AuthManager) sign(sid string) string {
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte(sid))
	return hex.EncodeToString(h.Sum(nil))
}

func (a *AuthManager) mount(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return a.basePath
	}
	if a.basePath == "" {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if strings.HasPrefix(p, a.basePath+"/") || p == a.basePath {
		return p
	}
	return a.basePath + p
}

func splitToken(token string) (string, bool) {
	idx := strings.LastIndex(token, ".")
	if idx <= 0 {
		return "", false
	}
	return token[:idx], true
}

func splitTokenWithSig(token string) (string, string, bool) {
	idx := strings.LastIndex(token, ".")
	if idx <= 0 || idx == len(token)-1 {
		return "", "", false
	}
	return token[:idx], token[idx+1:], true
}

type credentialsRecord struct {
	Username string
	Password string
}

func (a *AuthManager) readCredentials() (credentialsRecord, error) {
	if strings.TrimSpace(a.credentialsFile) == "" {
		return credentialsRecord{}, fmt.Errorf("credentials file not configured")
	}
	data, err := os.ReadFile(a.credentialsFile)
	if err != nil {
		return credentialsRecord{}, err
	}
	var payload struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return credentialsRecord{}, err
	}
	return credentialsRecord{
		Username: strings.TrimSpace(payload.User),
		Password: strings.TrimSpace(payload.Pass),
	}, nil
}

func (a *AuthManager) writeCredentials() error {
	// 已禁用凭据落盘，仅环境变量生效
	return nil
}

func CurrentUser(c *gin.Context) string {
	if v, ok := c.Get(contextUserKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

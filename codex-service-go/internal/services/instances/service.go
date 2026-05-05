package instances

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codex-service-go/pkg/codexauth"
	"codex-service-go/pkg/proxyurl"
)

type Service struct {
	repo            *Repository
	legacyAuthDir   string
	logDir          string
	defaultUpstream string
	defaultAPIKey   string
	defaultAuthMode string
	sharedToken     string
	tokenGenerator  func() (string, error)

	rev   atomic.Int64
	revMu sync.Mutex
	revCh chan struct{}

	batchMu    sync.Mutex
	batchDepth int
	batchDirty bool
}

type Options struct {
	AuthDir         string
	LogDir          string
	DefaultUpstream string
	DefaultAPIKey   string
	DefaultAuthMode string
	SharedToken     string
	TokenGenerator  func() (string, error)
}

func NewService(repo *Repository, opts Options) *Service {
	mode := strings.ToLower(strings.TrimSpace(opts.DefaultAuthMode))
	if mode == "" {
		mode = "chatgpt"
	}
	gen := opts.TokenGenerator
	if gen == nil {
		gen = defaultTokenGenerator
	}
	s := &Service{
		repo:            repo,
		legacyAuthDir:   strings.TrimSpace(opts.AuthDir),
		logDir:          opts.LogDir,
		defaultUpstream: opts.DefaultUpstream,
		defaultAPIKey:   strings.TrimSpace(opts.DefaultAPIKey),
		defaultAuthMode: mode,
		sharedToken:     strings.TrimSpace(opts.SharedToken),
		tokenGenerator:  gen,
	}
	s.rev.Store(time.Now().UnixNano())
	s.revCh = make(chan struct{})
	return s
}

type InstanceWithPaths struct {
	Instance
	AuthPath   string
	LogPath    string
	AuthExists bool
	LogExists  bool
	BasePath   string
}

func (s *Service) List(ctx context.Context) ([]InstanceWithPaths, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	authMeta, err := s.repo.ListAuthMeta(ctx)
	if err != nil {
		return nil, err
	}
	hasAuth := make(map[int64]bool, len(authMeta))
	for _, meta := range authMeta {
		hasAuth[meta.InstanceID] = true
	}
	out := make([]InstanceWithPaths, 0, len(items))
	for _, inst := range items {
		out = append(out, s.decorate(inst, hasAuth[inst.ID]))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id int64) (*InstanceWithPaths, error) {
	inst, err := s.repo.GetByID(ctx, id)
	if err != nil || inst == nil {
		return nil, err
	}
	meta, err := s.repo.GetAuthMeta(ctx, id)
	if err != nil {
		return nil, err
	}
	decorated := s.decorate(*inst, meta != nil)
	return &decorated, nil
}

func (s *Service) Create(ctx context.Context, arg CreateParams) (*InstanceWithPaths, error) {
	arg.Name = strings.TrimSpace(arg.Name)
	if arg.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	token, err := s.resolveToken(arg.InternalToken)
	if err != nil {
		return nil, err
	}
	arg.InternalToken = token

	upstream := strings.TrimSpace(arg.UpstreamBaseURL)
	if upstream == "" {
		upstream = s.defaultUpstream
	}
	arg.UpstreamBaseURL = upstream

	mode := sanitizeAuthMode(arg.AuthMode, s.defaultAuthMode)
	arg.AuthMode = mode

	apiKey := strings.TrimSpace(arg.UpstreamAPIKey)
	if apiKey == "" && strings.EqualFold(mode, "api_key") {
		apiKey = s.defaultAPIKey
	}
	arg.UpstreamAPIKey = apiKey

	arg.Proxy = strings.TrimSpace(arg.Proxy)
	if arg.Proxy != "" {
		normalized, err := proxyurl.Normalize(arg.Proxy)
		if err != nil {
			return nil, err
		}
		arg.Proxy = normalized
	}
	inst, err := s.repo.Create(ctx, arg)
	if err != nil {
		return nil, err
	}
	decorated := s.decorate(*inst, false)
	s.bumpRevision()
	return &decorated, nil
}

func (s *Service) Update(ctx context.Context, arg UpdateParams) (*InstanceWithPaths, error) {
	arg.Name = strings.TrimSpace(arg.Name)
	if arg.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	existing, err := s.repo.GetByID(ctx, arg.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("instance not found")
	}

	tokenCandidate := arg.InternalToken
	if strings.TrimSpace(tokenCandidate) == "" {
		tokenCandidate = s.sharedToken
	}
	if strings.TrimSpace(tokenCandidate) == "" {
		tokenCandidate = existing.InternalToken
	}
	token, err := s.resolveToken(tokenCandidate)
	if err != nil {
		return nil, err
	}
	arg.InternalToken = token

	upstream := strings.TrimSpace(arg.UpstreamBaseURL)
	if upstream == "" {
		upstream = strings.TrimSpace(existing.UpstreamBaseURL)
	}
	if upstream == "" {
		upstream = s.defaultUpstream
	}
	arg.UpstreamBaseURL = upstream

	mode := sanitizeAuthMode(arg.AuthMode, existing.AuthMode)
	mode = sanitizeAuthMode(mode, s.defaultAuthMode)
	arg.AuthMode = mode

	apiKey := strings.TrimSpace(arg.UpstreamAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(existing.UpstreamAPIKey)
	}
	if apiKey == "" && strings.EqualFold(mode, "api_key") {
		apiKey = s.defaultAPIKey
	}
	arg.UpstreamAPIKey = apiKey

	proxyCandidate := ""
	proxyProvided := arg.Proxy != nil
	if proxyProvided {
		proxyCandidate = strings.TrimSpace(*arg.Proxy)
		if proxyCandidate != "" {
			normalized, err := proxyurl.Normalize(proxyCandidate)
			if err != nil {
				return nil, err
			}
			proxyCandidate = normalized
		}
	} else {
		proxyCandidate = strings.TrimSpace(existing.Proxy)
	}
	arg.Proxy = &proxyCandidate

	inst, err := s.repo.Update(ctx, arg)
	if err != nil {
		return nil, err
	}
	meta, err := s.repo.GetAuthMeta(ctx, arg.ID)
	if err != nil {
		return nil, err
	}
	decorated := s.decorate(*inst, meta != nil)
	s.bumpRevision()
	return &decorated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.bumpRevision()
	return nil
}

func (s *Service) SetEnabled(ctx context.Context, id int64, enable bool) error {
	if err := s.repo.SetEnabled(ctx, id, enable); err != nil {
		return err
	}
	s.bumpRevision()
	return nil
}

func (s *Service) SetEnabledBatch(ctx context.Context, ids []int64, enable bool) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.repo.SetEnabledBatch(ctx, ids, enable); err != nil {
		return err
	}
	s.bumpRevision()
	return nil
}

func (s *Service) SetDebugEnabled(ctx context.Context, id int64, enable bool) error {
	if err := s.repo.SetDebugEnabled(ctx, id, enable); err != nil {
		return err
	}
	if !enable {
		if err := s.clearLogFile(id); err != nil {
			return err
		}
	}
	s.bumpRevision()
	return nil
}

func (s *Service) SetDebugConfig(ctx context.Context, id int64, cfg DebugConfig) error {
	if err := s.repo.SetDebugConfig(ctx, id, cfg); err != nil {
		return err
	}
	if !cfg.Enabled {
		if err := s.clearLogFile(id); err != nil {
			return err
		}
	}
	s.bumpRevision()
	return nil
}

func (s *Service) clearLogFile(id int64) error {
	logDir := strings.TrimSpace(s.logDir)
	if logDir == "" || id <= 0 {
		return nil
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%d.log", id))
	if err := os.Remove(logPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Service) decorate(inst Instance, hasAuth bool) InstanceWithPaths {
	logPath := filepath.Join(s.logDir, fmt.Sprintf("%d.log", inst.ID))
	return InstanceWithPaths{
		Instance:   inst,
		AuthPath:   "",
		LogPath:    logPath,
		AuthExists: hasAuth,
		LogExists:  fileExists(logPath),
		BasePath:   fmt.Sprintf("/%d", inst.ID),
	}
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (s *Service) resolveToken(candidate string) (string, error) {
	trimmed := strings.TrimSpace(candidate)
	if trimmed != "" {
		return trimmed, nil
	}
	shared := strings.TrimSpace(s.sharedToken)
	if shared != "" {
		return shared, nil
	}
	if s.tokenGenerator != nil {
		token, err := s.tokenGenerator()
		trimmed = strings.TrimSpace(token)
		if trimmed != "" {
			return trimmed, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("internal token is required")
}

func defaultTokenGenerator() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sanitizeAuthMode(mode, fallback string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "chatgpt", "api_key":
		return m
	}
	f := strings.ToLower(strings.TrimSpace(fallback))
	switch f {
	case "chatgpt", "api_key":
		return f
	}
	return "chatgpt"
}

func (s *Service) GetAuth(ctx context.Context, id int64) (*AuthRecord, error) {
	return s.repo.GetAuth(ctx, id)
}

func (s *Service) ListAuthMeta(ctx context.Context) ([]AuthMeta, error) {
	return s.repo.ListAuthMeta(ctx)
}

func (s *Service) ListAuth(ctx context.Context) ([]AuthRecord, error) {
	return s.repo.ListAuth(ctx)
}

func (s *Service) SaveAuth(ctx context.Context, id int64, authJSON string) error {
	normalized, dupKey, accountID, accountType, lastRefresh, accessTokenExpiresAt, err := normalizeAuthJSON(authJSON)
	if err != nil {
		return err
	}
	if err := s.repo.UpsertAuth(ctx, id, normalized, dupKey, accountID, accountType, lastRefresh, accessTokenExpiresAt); err != nil {
		return err
	}
	s.bumpRevision()
	return nil
}

func (s *Service) DeleteAuth(ctx context.Context, id int64) error {
	if err := s.repo.DeleteAuth(ctx, id); err != nil {
		return err
	}
	s.bumpRevision()
	return nil
}

// MigrateLegacyAuthFiles imports legacy "<AUTH_DIR>/<id>.json" files into SQLite.
// On success, the source file is removed to avoid dual sources of truth.
func (s *Service) MigrateLegacyAuthFiles(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	dir := strings.TrimSpace(s.legacyAuthDir)
	if dir == "" {
		return 0, nil
	}
	instances, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	existing, err := s.repo.ListAuthMeta(ctx)
	if err != nil {
		return 0, err
	}
	hasAuth := make(map[int64]bool, len(existing))
	for _, meta := range existing {
		hasAuth[meta.InstanceID] = true
	}
	migrated := 0
	for _, inst := range instances {
		if hasAuth[inst.ID] {
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("%d.json", inst.ID))
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return migrated, err
		}
		if err := s.SaveAuth(ctx, inst.ID, string(data)); err != nil {
			return migrated, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return migrated, err
		}
		migrated++
	}
	return migrated, nil
}

func normalizeAuthJSON(raw string) (normalized string, dupKey string, accountID string, accountType string, lastRefresh string, accessTokenExpiresAt *int64, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", "", "", nil, &ValidationError{Message: "auth_json is required"}
	}
	var payload map[string]any
	if e := json.Unmarshal([]byte(raw), &payload); e != nil {
		return "", "", "", "", "", nil, &ValidationError{Message: fmt.Sprintf("invalid auth json: %v", e)}
	}
	tokens, _ := payload["tokens"].(map[string]any)
	if tokens == nil {
		return "", "", "", "", "", nil, &ValidationError{Message: "auth json missing tokens"}
	}
	ok := false
	for _, k := range []string{"refresh_token", "access_token", "id_token"} {
		if v, ok2 := tokens[k]; ok2 {
			if s, ok3 := v.(string); ok3 && strings.TrimSpace(s) != "" {
				ok = true
				break
			}
		}
	}
	if !ok {
		return "", "", "", "", "", nil, &ValidationError{Message: "auth json tokens missing refresh_token/access_token/id_token"}
	}

	accountID = getStringAny(tokens["account_id"])
	if accountID == "" {
		accountID = getStringAny(payload["account_id"])
	}
	refreshToken := getStringAny(tokens["refresh_token"])
	if refreshToken == "" {
		refreshToken = getStringAny(tokens["refreshToken"])
	}
	if refreshToken == "" {
		refreshToken = getStringAny(payload["refresh_token"])
	}
	if refreshToken == "" {
		refreshToken = getStringAny(payload["refreshToken"])
	}
	accessToken := getStringAny(tokens["access_token"])
	if accessToken == "" {
		accessToken = getStringAny(tokens["accessToken"])
	}
	if accessToken == "" {
		accessToken = getStringAny(payload["access_token"])
	}
	if accessToken == "" {
		accessToken = getStringAny(payload["accessToken"])
	}
	idToken := getStringAny(tokens["id_token"])
	if idToken == "" {
		idToken = getStringAny(tokens["idToken"])
	}
	if idToken == "" {
		idToken = getStringAny(payload["id_token"])
	}
	if idToken == "" {
		idToken = getStringAny(payload["idToken"])
	}
	accountType = normalizeAccountType(getStringAny(tokens["account_type"]))
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(tokens["accountType"]))
	}
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(payload["account_type"]))
	}
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(payload["accountType"]))
	}
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(tokens["plan_type"]))
	}
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(tokens["planType"]))
	}
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(payload["plan_type"]))
	}
	if accountType == "" {
		accountType = normalizeAccountType(getStringAny(payload["planType"]))
	}
	if accountType == "" && idToken != "" {
		accountType = normalizeAccountType(codexauth.ExtractPlanType(idToken))
	}
	if ts := getStringAny(payload["last_refresh"]); ts != "" {
		lastRefresh = ts
	} else if ts := getStringAny(tokens["last_refresh"]); ts != "" {
		lastRefresh = ts
	}

	if accountID == "" {
		return "", "", "", "", "", nil, &ValidationError{Message: "auth json missing account_id"}
	}
	if refreshToken != "" {
		dupKey = "acct:" + accountID + "|rt:" + tokenFingerprint(refreshToken)
	} else if idToken != "" {
		dupKey = "acct:" + accountID + "|id:" + tokenFingerprint(idToken)
	} else if accessToken != "" {
		dupKey = "acct:" + accountID + "|at:" + tokenFingerprint(accessToken)
	}

	bearerToken := accessToken
	if bearerToken == "" {
		bearerToken = idToken
	}
	if exp := jwtExpiryUnix(bearerToken); exp > 0 {
		accessTokenExpiresAt = &exp
	}

	buf, e := json.MarshalIndent(payload, "", "  ")
	if e != nil {
		return "", "", "", "", "", nil, e
	}
	normalized = strings.TrimSpace(string(buf)) + "\n"
	return normalized, dupKey, accountID, accountType, lastRefresh, accessTokenExpiresAt, nil
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

func normalizeAccountType(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer("-", "_", " ", "_", "/", "_")
	raw = replacer.Replace(raw)
	for strings.Contains(raw, "__") {
		raw = strings.ReplaceAll(raw, "__", "_")
	}
	raw = strings.Trim(raw, "_")
	for _, candidate := range []string{
		"enterprise",
		"business",
		"team",
		"plus",
		"pro",
		"go",
		"personal",
		"free",
		"api_key",
	} {
		if raw == candidate || strings.HasPrefix(raw, candidate+"_") || strings.HasSuffix(raw, "_"+candidate) {
			return candidate
		}
	}
	return raw
}
func (s *Service) CurrentRevision() int64 {
	return s.rev.Load()
}

func (s *Service) TouchRevision() int64 {
	return s.bumpRevision()
}

func (s *Service) WithRevisionBatch(fn func() error) error {
	if s == nil {
		if fn == nil {
			return nil
		}
		return fn()
	}

	s.batchMu.Lock()
	s.batchDepth++
	s.batchMu.Unlock()

	defer func() {
		s.batchMu.Lock()
		s.batchDepth--
		shouldFlush := s.batchDepth == 0 && s.batchDirty
		if shouldFlush {
			s.batchDirty = false
		}
		s.batchMu.Unlock()
		if shouldFlush {
			s.bumpRevisionNow()
		}
	}()

	if fn == nil {
		return nil
	}
	return fn()
}

func (s *Service) WaitForRevision(ctx context.Context, since int64, timeout time.Duration) (int64, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		cur := s.rev.Load()
		if cur > since {
			return cur, true
		}
		ch := s.revisionChannel()
		cur = s.rev.Load()
		if cur > since {
			return cur, true
		}
		select {
		case <-ch:
		case <-timer.C:
			return s.rev.Load(), false
		case <-ctx.Done():
			return s.rev.Load(), false
		}
	}
}

func (s *Service) bumpRevision() int64 {
	if s == nil {
		return 0
	}

	s.batchMu.Lock()
	if s.batchDepth > 0 {
		s.batchDirty = true
		rev := s.rev.Load()
		s.batchMu.Unlock()
		return rev
	}
	s.batchMu.Unlock()

	return s.bumpRevisionNow()
}

func (s *Service) bumpRevisionNow() int64 {
	rev := time.Now().UnixNano()
	s.rev.Store(rev)
	s.revMu.Lock()
	close(s.revCh)
	s.revCh = make(chan struct{})
	s.revMu.Unlock()
	return rev
}

func (s *Service) revisionChannel() <-chan struct{} {
	s.revMu.Lock()
	ch := s.revCh
	s.revMu.Unlock()
	return ch
}

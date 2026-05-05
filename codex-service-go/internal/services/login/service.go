package login

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"codex-service-go/internal/config"
	instsvc "codex-service-go/internal/services/instances"
)

const defaultUserAgent = "Mozilla/5.0 (compatible; CodexLogin/1.0)"

type Service struct {
	cfg      *config.Config
	instances *instsvc.Service
	client   *http.Client
	mu       sync.Mutex
	sessions map[int64]*session
	history  map[int64]*LoginResult
}

type session struct {
	instance    instsvc.InstanceWithPaths
	authURL     string
	redirectURL string
	port        int
	verifier    string
	state       string
	server      *http.Server
	startedAt   time.Time
	resultMu    sync.Mutex
	result      *LoginResult
	done        chan LoginResult
	cancelOnce  sync.Once
}

type LoginResult struct {
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	AuthFile  string    `json:"auth_file,omitempty"`
	Completed time.Time `json:"completed_at,omitempty"`
}

type SessionInfo struct {
	InstanceID  int64        `json:"instance_id"`
	AuthURL     string       `json:"auth_url"`
	RedirectURL string       `json:"redirect_url"`
	Port        int          `json:"port"`
	StartedAt   time.Time    `json:"started_at"`
	Result      *LoginResult `json:"result,omitempty"`
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:      cfg,
		instances: nil,
		client:   &http.Client{Timeout: 20 * time.Second},
		sessions: make(map[int64]*session),
		history:  make(map[int64]*LoginResult),
	}
}

func NewServiceWithInstances(cfg *config.Config, instances *instsvc.Service) *Service {
	s := NewService(cfg)
	s.instances = instances
	return s
}

func (s *Service) Start(ctx context.Context, inst instsvc.InstanceWithPaths) (*SessionInfo, error) {
	s.mu.Lock()
	if existing, ok := s.sessions[inst.ID]; ok {
		info := s.sessionInfo(existing)
		s.mu.Unlock()
		return info, nil
	}

	listenHost := strings.TrimSpace(s.cfg.LoginCallbackHost)
	if listenHost == "" {
		listenHost = "127.0.0.1"
	}
	redirectHost := strings.TrimSpace(s.cfg.LoginRedirectHost)
	if redirectHost == "" {
		redirectHost = "localhost"
	}

	listener, port, err := s.listen(listenHost)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	delete(s.history, inst.ID)

	verifier, challenge, err := generatePKCE()
	if err != nil {
		listener.Close()
		s.mu.Unlock()
		return nil, err
	}
	state, err := generateState()
	if err != nil {
		listener.Close()
		s.mu.Unlock()
		return nil, err
	}

	redirectURL := fmt.Sprintf("http://%s:%d/auth/callback", redirectHost, port)
	authURL := buildAuthorizeURL(s.cfg.LoginIssuer, s.cfg.LoginClientID, redirectURL, challenge, state)

	sess := &session{
		instance:    inst,
		authURL:     authURL,
		redirectURL: redirectURL,
		port:        port,
		verifier:    verifier,
		state:       state,
		startedAt:   time.Now(),
		done:        make(chan LoginResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, authURL, http.StatusFound)
	})
	mux.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Login cancelled")
		go sess.finish(LoginResult{Success: false, Message: "cancelled"})
	})
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			go sess.finish(LoginResult{Success: false, Message: "state mismatch"})
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			go sess.finish(LoginResult{Success: false, Message: "missing authorization code"})
			return
		}
		tokens, err := s.exchangeCode(code, verifier, redirectURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("token exchange failed: %v", err), http.StatusInternalServerError)
			go sess.finish(LoginResult{Success: false, Message: err.Error()})
			return
		}
		apiKey, _ := s.obtainAPIKey(tokens.IDToken)
		if s.instances == nil {
			http.Error(w, "instances service not configured", http.StatusInternalServerError)
			go sess.finish(LoginResult{Success: false, Message: "instances service not configured"})
			return
		}
		if err := persistAuthToDB(r.Context(), s.instances, inst.ID, tokens, apiKey); err != nil {
			http.Error(w, fmt.Sprintf("persist auth failed: %v", err), http.StatusInternalServerError)
			go sess.finish(LoginResult{Success: false, Message: err.Error()})
			return
		}
		successHTML := `<!doctype html><meta charset="utf-8" /><title>Login success</title>
<style>body{font-family:ui-sans-serif,system-ui,Segoe UI,Roboto,Helvetica,Arial;padding:24px;max-width:720px;margin:auto}</style>
<h2>Login successful</h2>
<p>Credentials saved to database.</p>
<p>You can close this window.</p>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, successHTML)
		go sess.finish(LoginResult{Success: true, Message: "login completed", AuthFile: "database", Completed: time.Now()})
	})

	srv := &http.Server{Handler: mux}
	sess.server = srv

	s.sessions[inst.ID] = sess
	info := s.sessionInfo(sess)
	s.mu.Unlock()

	go func() {
		err := srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			sess.finish(LoginResult{Success: false, Message: err.Error()})
		}
	}()

	go func() {
		res := <-sess.done
		s.mu.Lock()
		delete(s.sessions, inst.ID)
		copy := res
		s.history[inst.ID] = &copy
		s.mu.Unlock()
	}()

	go func() {
		<-ctx.Done()
		sess.finish(LoginResult{Success: false, Message: "context cancelled"})
	}()

	return info, nil
}

func (s *Service) Cancel(instanceID int64) {
	s.mu.Lock()
	sess, ok := s.sessions[instanceID]
	s.mu.Unlock()
	if ok {
		sess.finish(LoginResult{Success: false, Message: "cancelled"})
	}
}

func (s *Service) SessionInfo(instanceID int64) *SessionInfo {
	s.mu.Lock()
	sess, ok := s.sessions[instanceID]
	result := s.history[instanceID]
	s.mu.Unlock()
	if ok {
		return s.sessionInfo(sess)
	}
	if result != nil {
		copy := *result
		return &SessionInfo{InstanceID: instanceID, Result: &copy}
	}
	return nil
}

func (s *Service) sessionInfo(sess *session) *SessionInfo {
	sess.resultMu.Lock()
	defer sess.resultMu.Unlock()
	info := &SessionInfo{
		InstanceID:  sess.instance.ID,
		AuthURL:     sess.authURL,
		RedirectURL: sess.redirectURL,
		Port:        sess.port,
		StartedAt:   sess.startedAt,
	}
	if sess.result != nil {
		resultCopy := *sess.result
		info.Result = &resultCopy
	}
	return info
}

func (s *Service) listen(host string) (net.Listener, int, error) {
	port := s.cfg.LoginCallbackPort
	for i := 0; i < 10; i++ {
		addr := fmt.Sprintf("%s:%d", host, port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, port, nil
		}
		port++
	}
	return nil, 0, errors.New("failed to bind login callback port")
}

type tokenSet struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
}

func (s *Service) exchangeCode(code, verifier, redirectURL string) (tokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)
	form.Set("client_id", s.cfg.LoginClientID)
	form.Set("code_verifier", verifier)

	endpoint := strings.TrimSuffix(s.cfg.LoginIssuer, "/") + "/oauth/token"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenSet{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Originator", "codex_proxy_node")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return tokenSet{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return tokenSet{}, fmt.Errorf("token endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return tokenSet{}, err
	}
	if payload.IDToken == "" || payload.AccessToken == "" {
		return tokenSet{}, errors.New("token endpoint returned empty tokens")
	}
	return tokenSet{
		IDToken:      payload.IDToken,
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
	}, nil
}

func (s *Service) obtainAPIKey(idToken string) (string, error) {
	if idToken == "" {
		return "", nil
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	form.Set("client_id", s.cfg.LoginClientID)
	form.Set("requested_token", "openai-api-key")
	form.Set("subject_token", idToken)
	form.Set("subject_token_type", "urn:ietf:params:oauth:token-type:id_token")

	endpoint := strings.TrimSuffix(s.cfg.LoginIssuer, "/") + "/oauth/token"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Originator", "codex_proxy_node")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", nil
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.AccessToken, nil
}

func (sess *session) finish(res LoginResult) {
	sess.cancelOnce.Do(func() {
		sess.resultMu.Lock()
		sess.result = &res
		sess.resultMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if sess.server != nil {
			_ = sess.server.Shutdown(ctx)
		}
		select {
		case sess.done <- res:
		default:
		}
	})
}

func persistAuthToDB(ctx context.Context, instances *instsvc.Service, instanceID int64, tokens tokenSet, apiKey string) error {
	if instances == nil {
		return errors.New("instances service not configured")
	}
	var current struct {
		OpenAIAPIKey string `json:"OPENAI_API_KEY,omitempty"`
		Tokens       struct {
			IDToken      string  `json:"id_token"`
			AccessToken  string  `json:"access_token"`
			RefreshToken string  `json:"refresh_token"`
			AccountID    *string `json:"account_id,omitempty"`
		} `json:"tokens"`
		LastRefresh string `json:"last_refresh"`
	}
	if rec, err := instances.GetAuth(ctx, instanceID); err == nil && rec != nil && strings.TrimSpace(rec.AuthJSON) != "" {
		_ = json.Unmarshal([]byte(rec.AuthJSON), &current)
	}

	current.Tokens.IDToken = tokens.IDToken
	current.Tokens.AccessToken = tokens.AccessToken
	current.Tokens.RefreshToken = tokens.RefreshToken
	if current.Tokens.AccountID == nil {
		if acct := parseAccountID(tokens.IDToken); acct != "" {
			current.Tokens.AccountID = &acct
		}
	}
	if apiKey != "" {
		current.OpenAIAPIKey = apiKey
	}
	current.LastRefresh = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return instances.SaveAuth(ctx, instanceID, string(data))
}

func parseAccountID(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var body struct {
		APIAuth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
		Auth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	if acct := body.APIAuth.ChatGPTAccountID; acct != "" {
		return acct
	}
	if acct := body.Auth.ChatGPTAccountID; acct != "" {
		return acct
	}
	return ""
}

func generatePKCE() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge, nil
}

func generateState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func buildAuthorizeURL(issuer, clientID, redirectURL, challenge, state string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURL)
	params.Set("scope", "openid profile email offline_access")
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	params.Set("state", state)
	params.Set("originator", "codex_proxy_node")
	return strings.TrimSuffix(issuer, "/") + "/oauth/authorize?" + params.Encode()
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")
	return replacer.Replace(s)
}

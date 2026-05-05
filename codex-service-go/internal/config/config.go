package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const defaultBaseInstructions = "You are a precise, safe, and helpful coding assistant. Follow user intent, provide accurate answers, avoid unsafe actions, and keep responses concise."

var loadedEnvFile string

func LoadedEnvFile() string {
	return loadedEnvFile
}

type Config struct {
	ServerHost string
	ServerPort int
	AccessLog  bool
	ProxyDebug bool

	DatabasePath         string
	AuthDir              string
	LogDir               string
	CodexHome            string
	AdminStoreDir        string
	AdminCredentialsFile string
	// WebBasePath is used when the HTTP router is mounted under a prefix (e.g. "/cx").
	// It should be empty or start with "/" and have no trailing "/".
	WebBasePath string

	ServiceToken              string
	DefaultUpstreamBaseURL    string
	DefaultUpstreamAPIKey     string
	DefaultProxyAuthMode      string
	ProxyConfigFile           string
	ProxyAuthFile             string
	ProxyRuntimeFile          string
	ProxyExpireAt             string
	ProxyEnableCompatOpenAI   bool
	ProxyEnableStreamCompat   bool
	ProxyEnableAggregation    bool
	ProxyForceStreamCompat    bool
	ProxyReasoningCompat      string
	ProxyReasoningEffort      string
	ProxyDisableInstructions  bool
	ProxyBaseInstructions     string
	ProxyBaseInstructionsFile string
	ProxyOriginator           string
	ProxyChatGPTAccountID     string

	LoginClientID     string
	LoginIssuer       string
	LoginCallbackHost string
	LoginRedirectHost string
	LoginCallbackPort int

	AdminUser string
	AdminPass string

	// Refresh (scheduled)
	RefreshEnabled bool
	RefreshMinDays int
	RefreshMaxDays int

	// Responses API base URL (fixed to OpenAI by default)
	DefaultResponsesBaseURL string
}

func Load() (*Config, error) {
	return load(true)
}

// LoadWithoutDotenv reads configuration from process environment only, without
// attempting to load/override from any .env file. This is useful when the host
// application (e.g. Transfer API) is already managing dotenv loading.
func LoadWithoutDotenv() (*Config, error) {
	return load(false)
}

func load(withDotenv bool) (*Config, error) {
	if withDotenv {
		loadEnvFile()
	} else {
		loadedEnvFile = ""
	}

	cfg := &Config{
		ServerHost: getEnvFirst([]string{"SERVER_HOST", "ADMIN_HOST"}, "0.0.0.0"),
		ServerPort: getEnvAsInt([]string{"SERVER_PORT", "ADMIN_PORT"}, 8099),
		AccessLog:  getEnvAsBool([]string{"PROXY_ACCESS_LOG"}, true),
		ProxyDebug: getEnvAsBool([]string{"PROXY_DEBUG"}, false),

		DatabasePath:         getEnv("DATABASE_PATH", filepath.Join("db", "codex.db")),
		AuthDir:              getEnv("AUTH_DIR", "auth"),
		LogDir:               getEnv("LOG_DIR", "log"),
		CodexHome:            strings.TrimSpace(getEnv("CODEX_HOME", "")),
		AdminStoreDir:        strings.TrimSpace(getEnv("ADMIN_STORE_DIR", filepath.Join("run", "webadmin"))),
		AdminCredentialsFile: strings.TrimSpace(getEnv("ADMIN_CREDENTIALS_FILE", "")),

		ServiceToken: strings.TrimSpace(getEnvFirst([]string{"PROXY_INTERNAL_TOKEN", "TOKEN"}, "")),
		// 延后设置默认上游基址，根据鉴权模式决定（chatgpt -> chatgpt.com/codex，api_key -> api.openai.com/v1）
		DefaultUpstreamBaseURL:    strings.TrimSpace(getEnvFirst([]string{"DEFAULT_UPSTREAM_BASE_URL", "UPSTREAM_BASE_URL"}, "")),
		DefaultUpstreamAPIKey:     strings.TrimSpace(getEnv("UPSTREAM_OPENAI_API_KEY", "")),
		ProxyConfigFile:           strings.TrimSpace(getEnv("PROXY_CONFIG_FILE", "")),
		ProxyAuthFile:             strings.TrimSpace(getEnvFirst([]string{"PROXY_AUTH_FILE", "CODEX_AUTH_FILE"}, "")),
		ProxyRuntimeFile:          strings.TrimSpace(getEnvFirst([]string{"PROXY_RUNTIME_FILE"}, filepath.Join("run", "runtime.json"))),
		ProxyExpireAt:             strings.TrimSpace(getEnv("PROXY_EXPIRE_AT", "")),
		ProxyEnableCompatOpenAI:   getEnvAsBool([]string{"PROXY_ENABLE_COMPAT_OPENAI"}, true),
		ProxyEnableStreamCompat:   getEnvAsBool([]string{"PROXY_ENABLE_STREAM_COMPAT"}, true),
		ProxyEnableAggregation:    getEnvAsBool([]string{"PROXY_ENABLE_RESPONSE_AGGREGATION"}, true),
		ProxyForceStreamCompat:    getEnvAsBool([]string{"PROXY_FORCE_STREAM_COMPAT"}, false),
		ProxyReasoningCompat:      strings.ToLower(strings.TrimSpace(getEnv("PROXY_REASONING_COMPAT", "think-tags"))),
		ProxyReasoningEffort:      strings.ToLower(strings.TrimSpace(getEnv("PROXY_REASONING_EFFORT", ""))),
		ProxyDisableInstructions:  getEnvAsBool([]string{"PROXY_DISABLE_BASE_INSTRUCTIONS"}, false),
		ProxyBaseInstructions:     strings.TrimSpace(getEnv("PROXY_BASE_INSTRUCTIONS", "")),
		ProxyBaseInstructionsFile: strings.TrimSpace(getEnv("PROXY_BASE_INSTRUCTIONS_FILE", "")),
		ProxyOriginator:           strings.TrimSpace(getEnv("PROXY_ORIGINATOR", "")),
		ProxyChatGPTAccountID:     strings.TrimSpace(getEnvFirst([]string{"PROXY_CHATGPT_ACCOUNT_ID", "CHATGPT_ACCOUNT_ID"}, "")),

		LoginClientID:     strings.TrimSpace(getEnvFirst([]string{"CHATGPT_CLIENT_ID", "LOGIN_CLIENT_ID"}, "app_EMoamEEZ73f0CkXaXp7hrann")),
		LoginIssuer:       strings.TrimSpace(getEnvFirst([]string{"CHATGPT_ISSUER", "LOGIN_ISSUER"}, "https://auth.openai.com")),
		LoginCallbackHost: strings.TrimSpace(getEnvFirst([]string{"LOGIN_CALLBACK_HOST"}, "127.0.0.1")),
		LoginRedirectHost: strings.TrimSpace(getEnvFirst([]string{"LOGIN_REDIRECT_HOST"}, "localhost")),
		LoginCallbackPort: getEnvAsInt([]string{"LOGIN_CALLBACK_PORT", "CHATGPT_LOGIN_PORT"}, 1455),

		AdminUser: strings.TrimSpace(getEnv("WEB_ADMIN_USER", "")),
		AdminPass: strings.TrimSpace(getEnv("WEB_ADMIN_PASS", "")),

		RefreshEnabled: getEnvAsBool([]string{"REFRESH_ENABLED"}, false),
		RefreshMinDays: getEnvAsInt([]string{"REFRESH_MIN_DAYS"}, 8),
		RefreshMaxDays: getEnvAsInt([]string{"REFRESH_MAX_DAYS"}, 8),

		DefaultResponsesBaseURL: strings.TrimSpace(getEnv("RESPONSES_BASE_URL", "https://api.openai.com/v1")),
	}

	upstreamAuthMode := strings.ToLower(strings.TrimSpace(getEnv("PROXY_AUTH_MODE", "")))
	if upstreamAuthMode == "" {
		if cfg.DefaultUpstreamAPIKey != "" {
			upstreamAuthMode = "api_key"
		} else {
			upstreamAuthMode = "chatgpt"
		}
	}
	cfg.DefaultProxyAuthMode = upstreamAuthMode

	if err := cfg.applyProxyConfigFile(); err != nil {
		return nil, err
	}

	cfg.DefaultProxyAuthMode = normalizeAuthMode(cfg.DefaultProxyAuthMode, cfg.DefaultUpstreamAPIKey)

	// 若未显式配置上游基址，则按模式设置默认值，保持与 Node 行为一致
	if strings.TrimSpace(cfg.DefaultUpstreamBaseURL) == "" {
		if cfg.DefaultProxyAuthMode == "chatgpt" {
			cfg.DefaultUpstreamBaseURL = "https://chatgpt.com/backend-api/codex"
		} else {
			cfg.DefaultUpstreamBaseURL = "https://api.openai.com/v1"
		}
	}

	if cfg.ProxyOriginator == "" {
		cfg.ProxyOriginator = "codex_cli_rs"
	}

	cfg.applyCodexHomeDefaults()
	cfg.normalizePaths()

	if strings.TrimSpace(cfg.AdminStoreDir) == "" {
		cfg.AdminStoreDir = filepath.Join("run", "webadmin")
	}
	if strings.TrimSpace(cfg.AdminCredentialsFile) == "" {
		cfg.AdminCredentialsFile = filepath.Join(cfg.AdminStoreDir, "credentials.json")
	}

	if err := cfg.ensureDirectories(); err != nil {
		return nil, err
	}

	cfg.ProxyBaseInstructions = resolveBaseInstructions(cfg.ProxyBaseInstructions, cfg.ProxyBaseInstructionsFile)

	return cfg, nil
}

type proxyFileConfig struct {
	ProxyInternalToken      *string `json:"proxy_internal_token" yaml:"proxy_internal_token"`
	AuthMode                *string `json:"auth_mode" yaml:"auth_mode"`
	ProxyAuthFile           *string `json:"proxy_auth_file" yaml:"proxy_auth_file"`
	UpstreamBaseURL         *string `json:"upstream_base_url" yaml:"upstream_base_url"`
	UpstreamOpenAIAPIKey    *string `json:"upstream_openai_api_key" yaml:"upstream_openai_api_key"`
	Debug                   *bool   `json:"debug" yaml:"debug"`
	ProxyRuntimeFile        *string `json:"proxy_runtime_file" yaml:"proxy_runtime_file"`
	ProxyExpireAt           *string `json:"proxy_expire_at" yaml:"proxy_expire_at"`
	EnableCompatOpenAI      *bool   `json:"enable_compat_openai" yaml:"enable_compat_openai"`
	EnableStreamCompat      *bool   `json:"enable_stream_compat" yaml:"enable_stream_compat"`
	EnableAggregation       *bool   `json:"enable_response_aggregation" yaml:"enable_response_aggregation"`
	ForceStreamCompat       *bool   `json:"force_stream_compat" yaml:"force_stream_compat"`
	ReasoningCompat         *string `json:"reasoning_compat" yaml:"reasoning_compat"`
	ReasoningEffort         *string `json:"reasoning_effort" yaml:"reasoning_effort"`
	DisableBaseInstructions *bool   `json:"disable_base_instructions" yaml:"disable_base_instructions"`
	BaseInstructions        *string `json:"proxy_base_instructions" yaml:"proxy_base_instructions"`
	BaseInstructionsFile    *string `json:"proxy_base_instructions_file" yaml:"proxy_base_instructions_file"`
	Originator              *string `json:"proxy_originator" yaml:"proxy_originator"`
	ChatGPTAccountID        *string `json:"proxy_chatgpt_account_id" yaml:"proxy_chatgpt_account_id"`
	CodexHome               *string `json:"codex_home" yaml:"codex_home"`
	AdminStoreDir           *string `json:"admin_store_dir" yaml:"admin_store_dir"`
	AdminCredentialsFile    *string `json:"admin_credentials_file" yaml:"admin_credentials_file"`
	LoginClientID           *string `json:"login_client_id" yaml:"login_client_id"`
	LoginIssuer             *string `json:"login_issuer" yaml:"login_issuer"`
	LoginCallbackHost       *string `json:"login_callback_host" yaml:"login_callback_host"`
	LoginRedirectHost       *string `json:"login_redirect_host" yaml:"login_redirect_host"`
	LoginCallbackPort       *int    `json:"login_callback_port" yaml:"login_callback_port"`
}

func (c *Config) applyProxyConfigFile() error {
	path := strings.TrimSpace(c.ProxyConfigFile)
	if path == "" {
		return nil
	}
	resolved := expandPath(path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("load proxy config file: %w", err)
	}
	var parsed proxyFileConfig
	switch strings.ToLower(filepath.Ext(resolved)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("parse proxy config file: %w", err)
		}
	default:
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("parse proxy config file: %w", err)
		}
	}
	parsed.apply(c)
	return nil
}

func (p *proxyFileConfig) apply(c *Config) {
	if p == nil {
		return
	}
	if p.ProxyInternalToken != nil {
		c.ServiceToken = strings.TrimSpace(*p.ProxyInternalToken)
	}
	if p.AuthMode != nil {
		c.DefaultProxyAuthMode = strings.TrimSpace(*p.AuthMode)
	}
	if p.ProxyAuthFile != nil {
		c.ProxyAuthFile = strings.TrimSpace(*p.ProxyAuthFile)
	}
	if p.UpstreamBaseURL != nil {
		c.DefaultUpstreamBaseURL = strings.TrimSpace(*p.UpstreamBaseURL)
	}
	if p.UpstreamOpenAIAPIKey != nil {
		c.DefaultUpstreamAPIKey = strings.TrimSpace(*p.UpstreamOpenAIAPIKey)
	}
	if p.Debug != nil {
		c.ProxyDebug = *p.Debug
	}
	if p.ProxyRuntimeFile != nil {
		if trimmed := strings.TrimSpace(*p.ProxyRuntimeFile); trimmed != "" {
			c.ProxyRuntimeFile = trimmed
		}
	}
	if p.ProxyExpireAt != nil {
		c.ProxyExpireAt = strings.TrimSpace(*p.ProxyExpireAt)
	}
	if p.EnableCompatOpenAI != nil {
		c.ProxyEnableCompatOpenAI = *p.EnableCompatOpenAI
	}
	if p.EnableStreamCompat != nil {
		c.ProxyEnableStreamCompat = *p.EnableStreamCompat
	}
	if p.EnableAggregation != nil {
		c.ProxyEnableAggregation = *p.EnableAggregation
	}
	if p.ForceStreamCompat != nil {
		c.ProxyForceStreamCompat = *p.ForceStreamCompat
	}
	if p.ReasoningCompat != nil {
		c.ProxyReasoningCompat = strings.ToLower(strings.TrimSpace(*p.ReasoningCompat))
	}
	if p.ReasoningEffort != nil {
		c.ProxyReasoningEffort = strings.ToLower(strings.TrimSpace(*p.ReasoningEffort))
	}
	if p.DisableBaseInstructions != nil {
		c.ProxyDisableInstructions = *p.DisableBaseInstructions
	}
	if p.BaseInstructions != nil {
		c.ProxyBaseInstructions = strings.TrimSpace(*p.BaseInstructions)
	}
	if p.BaseInstructionsFile != nil {
		c.ProxyBaseInstructionsFile = strings.TrimSpace(*p.BaseInstructionsFile)
	}
	if p.Originator != nil {
		c.ProxyOriginator = strings.TrimSpace(*p.Originator)
	}
	if p.ChatGPTAccountID != nil {
		c.ProxyChatGPTAccountID = strings.TrimSpace(*p.ChatGPTAccountID)
	}
	if p.CodexHome != nil {
		c.CodexHome = strings.TrimSpace(*p.CodexHome)
	}
	if p.AdminStoreDir != nil {
		c.AdminStoreDir = strings.TrimSpace(*p.AdminStoreDir)
	}
	if p.AdminCredentialsFile != nil {
		c.AdminCredentialsFile = strings.TrimSpace(*p.AdminCredentialsFile)
	}
	if p.LoginClientID != nil {
		c.LoginClientID = strings.TrimSpace(*p.LoginClientID)
	}
	if p.LoginIssuer != nil {
		c.LoginIssuer = strings.TrimSpace(*p.LoginIssuer)
	}
	if p.LoginCallbackHost != nil {
		c.LoginCallbackHost = strings.TrimSpace(*p.LoginCallbackHost)
	}
	if p.LoginRedirectHost != nil {
		c.LoginRedirectHost = strings.TrimSpace(*p.LoginRedirectHost)
	}
	if p.LoginCallbackPort != nil {
		c.LoginCallbackPort = *p.LoginCallbackPort
	}
}

func (c *Config) applyCodexHomeDefaults() {
	home := strings.TrimSpace(c.CodexHome)
	if home == "" {
		return
	}
	home = expandPath(home)
	c.CodexHome = home
	if isDefaultPath(c.AuthDir, "auth") {
		c.AuthDir = filepath.Join(home, "auth")
	}
	if isDefaultPath(c.LogDir, "log") {
		c.LogDir = filepath.Join(home, "log")
	}
	if strings.TrimSpace(c.ProxyAuthFile) == "" {
		c.ProxyAuthFile = filepath.Join(home, "auth.json")
	}
	if isDefaultPath(c.ProxyRuntimeFile, filepath.Join("run", "runtime.json")) {
		c.ProxyRuntimeFile = filepath.Join(home, "run", "runtime.json")
	}
	if isDefaultPath(c.AdminStoreDir, filepath.Join("run", "webadmin")) {
		c.AdminStoreDir = filepath.Join(home, "run", "webadmin")
	}
}

func (c *Config) normalizePaths() {
	c.DatabasePath = expandPath(c.DatabasePath)
	c.AuthDir = expandPath(c.AuthDir)
	c.LogDir = expandPath(c.LogDir)
	c.ProxyAuthFile = expandPath(c.ProxyAuthFile)
	c.ProxyRuntimeFile = expandPath(c.ProxyRuntimeFile)
	c.ProxyBaseInstructionsFile = expandPath(c.ProxyBaseInstructionsFile)
	c.AdminStoreDir = expandPath(c.AdminStoreDir)
	c.AdminCredentialsFile = expandPath(c.AdminCredentialsFile)
	c.ProxyConfigFile = expandPath(c.ProxyConfigFile)
}

func normalizeAuthMode(mode, apiKey string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m != "chatgpt" && m != "api_key" {
		if strings.TrimSpace(apiKey) != "" {
			return "api_key"
		}
		return "chatgpt"
	}
	return m
}

func expandPath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if len(trimmed) == 1 {
				trimmed = home
			} else if trimmed[1] == '/' || trimmed[1] == '\\' {
				trimmed = filepath.Join(home, trimmed[2:])
			}
		}
	}
	return filepath.Clean(os.ExpandEnv(trimmed))
}

func isDefaultPath(value, def string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	return filepath.Clean(trimmed) == filepath.Clean(def)
}

func (c *Config) ensureDirectories() error {
	if err := ensureDir(c.AuthDir); err != nil {
		return fmt.Errorf("ensure auth dir: %w", err)
	}
	if err := ensureDir(c.LogDir); err != nil {
		return fmt.Errorf("ensure log dir: %w", err)
	}
	if err := ensureDir(c.AdminStoreDir); err != nil {
		return fmt.Errorf("ensure admin store dir: %w", err)
	}
	if dir := filepath.Dir(c.DatabasePath); dir != "" {
		if err := ensureDir(dir); err != nil {
			return fmt.Errorf("ensure db dir: %w", err)
		}
	}
	return nil
}

func ensureDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("empty directory path")
	}
	return os.MkdirAll(dir, 0o755)
}

func resolveBaseInstructions(initial, explicitFile string) string {
	if txt := strings.TrimSpace(initial); txt != "" {
		return txt
	}
	candidates := []string{}
	if file := strings.TrimSpace(explicitFile); file != "" {
		candidates = append(candidates, file)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "codex-service", "proxy", "prompt.md"),
			filepath.Join(wd, "Codex2API", "codex2api", "prompt.md"),
		)
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if text := strings.TrimSpace(string(data)); text != "" {
			return text
		}
	}
	return defaultBaseInstructions
}

func getEnv(key, def string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return def
}

func getEnvFirst(keys []string, def string) string {
	for _, key := range keys {
		if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
			return val
		}
	}
	return def
}

func getEnvAsInt(keys []string, def int) int {
	for _, key := range keys {
		if valStr, ok := os.LookupEnv(key); ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				return val
			}
		}
	}
	return def
}

func getEnvAsBool(keys []string, def bool) bool {
	for _, key := range keys {
		if val, ok := os.LookupEnv(key); ok {
			trimmed := strings.TrimSpace(strings.ToLower(val))
			switch trimmed {
			case "1", "true", "yes", "on":
				return true
			case "0", "false", "no", "off":
				return false
			}
		}
	}
	return def
}

func loadEnvFile() {
	candidates := []string{".env"}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, walkUpEnvPaths(wd)...)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
		candidates = append(candidates, walkUpEnvPaths(root)...)
	}
	seen := make(map[string]struct{})
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Overload(path); err != nil {
				continue
			}
			loadedEnvFile = path
			return
		}
	}
}

func walkUpEnvPaths(start string) []string {
	var paths []string
	d := start
	for {
		paths = append(paths, filepath.Join(d, ".env"))
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return paths
}

func UpdateEnvFile(path string, updates map[string]string) error {
	return UpdateEnvFileWithMarker(path, updates, "# Refresh settings (managed via web admin)")
}

func UpdateEnvFileWithMarker(path string, updates map[string]string, marker string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("env file path is empty")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" {
		marker = "# Managed via web admin"
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			keys := make([]string, 0, len(updates))
			for k := range updates {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			lines := make([]string, 0, len(keys)+2)
			lines = append(lines, "# Managed via web admin")
			for _, k := range keys {
				lines = append(lines, k+"="+updates[k])
			}
			content := strings.Join(lines, "\n") + "\n"
			return os.WriteFile(path, []byte(content), 0o644)
		}
		return err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	lines := strings.Split(text, "\n")

	updated := make(map[string]bool, len(updates))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		body := strings.TrimLeft(line, " \t")
		prefix := ""
		if strings.HasPrefix(body, "export ") {
			prefix = "export "
			body = strings.TrimSpace(body[len("export "):])
		}
		eq := strings.IndexRune(body, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(body[:eq])
		if val, ok := updates[key]; ok {
			lines[i] = leading + prefix + key + "=" + val
			updated[key] = true
		}
	}

	missing := make([]string, 0, len(updates))
	for k := range updates {
		if !updated[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		hasMarker := false
		for _, line := range lines {
			if strings.TrimSpace(line) == marker {
				hasMarker = true
				break
			}
		}
		if len(lines) == 0 || strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		if !hasMarker {
			lines = append(lines, marker)
		}
		for _, k := range missing {
			lines = append(lines, k+"="+updates[k])
		}
	}

	out := strings.Join(lines, eol)
	if !strings.HasSuffix(out, eol) {
		out += eol
	}
	return os.WriteFile(path, []byte(out), info.Mode())
}

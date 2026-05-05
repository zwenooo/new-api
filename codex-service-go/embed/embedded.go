package embed

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"

	"codex-service-go/internal/config"
	"codex-service-go/internal/db"
	"codex-service-go/internal/server"
	instsvc "codex-service-go/internal/services/instances"
	loginsvc "codex-service-go/internal/services/login"
	proxysvc "codex-service-go/internal/services/proxy"
	refreshsvc "codex-service-go/internal/services/refresh"
	systemsvc "codex-service-go/internal/services/system"
)

type Embedded struct {
	App       *server.App
	Config    *config.Config
	Instances *instsvc.Service
	Proxy     *proxysvc.Service
	Login     *loginsvc.Service
	System    *systemsvc.Service
	Refresher *refreshsvc.Service
	Cancel    context.CancelFunc
	DB        *sql.DB
}

type RequestStage = proxysvc.RequestStage

type RequestStageRecorder = proxysvc.RequestStageRecorder

type StartWithDBOptions struct {
	DB      *sql.DB
	Dialect string
	// TablePrefix avoids colliding with other apps using the same DB.
	TablePrefix string
	// LogDir stores per-instance log files (optional but recommended for admin UI).
	LogDir string
	// OnRequestStage receives fine-grained proxy stages in embedded mode.
	OnRequestStage RequestStageRecorder
}

func Start(parent context.Context, basePath string) (*Embedded, error) {
	cfg, err := config.LoadWithoutDotenv()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ServiceToken) == "" {
		return nil, fmt.Errorf("PROXY_INTERNAL_TOKEN/TOKEN is required")
	}

	// Embedded mode uses Transfer API's admin session gate; avoid double-auth via WEB_ADMIN_USER/PASS.
	cfg.AdminUser = ""
	cfg.AdminPass = ""

	sqliteDB, err := db.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	if err := db.Migrate(sqliteDB); err != nil {
		_ = sqliteDB.Close()
		return nil, err
	}

	repo := instsvc.NewRepository(sqliteDB)
	instanceService := instsvc.NewService(repo, instsvc.Options{
		AuthDir:         cfg.AuthDir,
		LogDir:          cfg.LogDir,
		DefaultUpstream: cfg.DefaultUpstreamBaseURL,
		DefaultAPIKey:   cfg.DefaultUpstreamAPIKey,
		DefaultAuthMode: cfg.DefaultProxyAuthMode,
		SharedToken:     cfg.ServiceToken,
	})

	if err := repo.SetAllInternalTokens(context.Background(), cfg.ServiceToken); err != nil {
		_ = sqliteDB.Close()
		return nil, err
	}

	if _, err := instanceService.MigrateLegacyAuthFiles(context.Background()); err != nil {
		_ = sqliteDB.Close()
		return nil, err
	}

	proxyService := proxysvc.NewService(proxysvc.Options{
		Debug:                  cfg.ProxyDebug,
		AccessLog:              cfg.AccessLog,
		Originator:             cfg.ProxyOriginator,
		RuntimeFile:            cfg.ProxyRuntimeFile,
		RuntimeExpireAt:        cfg.ProxyExpireAt,
		ChatGPTClientID:        cfg.LoginClientID,
		ChatGPTAccountID:       cfg.ProxyChatGPTAccountID,
		DefaultAuthFile:        cfg.ProxyAuthFile,
		DefaultAuthMode:        cfg.DefaultProxyAuthMode,
		DefaultUpstreamBaseURL: cfg.DefaultUpstreamBaseURL,
		DefaultUpstreamAPIKey:  cfg.DefaultUpstreamAPIKey,
		ResponsesBaseURL:       cfg.DefaultResponsesBaseURL,
		AuthStore:              instanceService,
		OnRequestStage:         nil,
		OnRuntimeChange: func(_ int64) {
			instanceService.TouchRevision()
		},
	})

	loginService := loginsvc.NewServiceWithInstances(cfg, instanceService)
	systemService := systemsvc.NewService()

	refresher := refreshsvc.NewService(instanceService, proxyService)
	refresher.SetEnabled(cfg.RefreshEnabled)
	refresher.SetRangeDays(cfg.RefreshMinDays, cfg.RefreshMaxDays)
	proxysvc.SetTokenRefreshEnabled(cfg.RefreshEnabled)

	ctx, cancel := context.WithCancel(parent)
	cancelOnce := sync.Once{}
	cancelWithClose := func() {
		cancelOnce.Do(func() {
			cancel()
			_ = sqliteDB.Close()
		})
	}

	app, err := server.NewAppWithBasePath(cfg, server.Services{
		Instances: instanceService,
		Proxy:     proxyService,
		Login:     loginService,
		System:    systemService,
		Refresher: refresher,
		Restart:   cancelWithClose,
	}, basePath)
	if err != nil {
		cancelWithClose()
		return nil, err
	}

	go refresher.Start(ctx.Done())

	return &Embedded{
		App:       app,
		Config:    cfg,
		Instances: instanceService,
		Proxy:     proxyService,
		Login:     loginService,
		System:    systemService,
		Refresher: refresher,
		Cancel:    cancelWithClose,
		DB:        sqliteDB,
	}, nil
}

// StartWithDB runs codex-service-go embedded but stores instances/auth in the
// provided SQL database (e.g. Transfer API's MySQL). It intentionally does NOT rely
// on DATABASE_PATH/SQLite.
func StartWithDB(parent context.Context, basePath string, opts StartWithDBOptions) (*Embedded, error) {
	if opts.DB == nil {
		return nil, fmt.Errorf("db is required")
	}

	cfg, err := config.LoadWithoutDotenv()
	if err != nil {
		return nil, err
	}

	// Embedded mode uses Transfer API's admin session gate; avoid double-auth via WEB_ADMIN_USER/PASS.
	cfg.AdminUser = ""
	cfg.AdminPass = ""
	// Embedded mode should not expose /internal/* nor require a shared token.
	cfg.ServiceToken = ""

	// Make sure the log dir exists (admin UI writes per-instance logs).
	if strings.TrimSpace(opts.LogDir) != "" {
		if err := os.MkdirAll(opts.LogDir, 0o755); err != nil {
			return nil, err
		}
		cfg.LogDir = strings.TrimSpace(opts.LogDir)
	}

	repo := instsvc.NewRepositoryWithOptions(opts.DB, instsvc.RepositoryOptions{
		TablePrefix: strings.TrimSpace(opts.TablePrefix),
		Dialect:     strings.TrimSpace(opts.Dialect),
	})
	if err := repo.RepairNullTimestamps(context.Background()); err != nil {
		return nil, err
	}
	instanceService := instsvc.NewService(repo, instsvc.Options{
		AuthDir:         "",
		LogDir:          cfg.LogDir,
		DefaultUpstream: cfg.DefaultUpstreamBaseURL,
		DefaultAPIKey:   cfg.DefaultUpstreamAPIKey,
		DefaultAuthMode: cfg.DefaultProxyAuthMode,
		SharedToken:     "",
	})

	proxyService := proxysvc.NewService(proxysvc.Options{
		Debug:                  cfg.ProxyDebug,
		AccessLog:              cfg.AccessLog,
		Originator:             cfg.ProxyOriginator,
		RuntimeFile:            cfg.ProxyRuntimeFile,
		RuntimeExpireAt:        cfg.ProxyExpireAt,
		ChatGPTClientID:        cfg.LoginClientID,
		ChatGPTAccountID:       cfg.ProxyChatGPTAccountID,
		DefaultAuthFile:        cfg.ProxyAuthFile,
		DefaultAuthMode:        cfg.DefaultProxyAuthMode,
		DefaultUpstreamBaseURL: cfg.DefaultUpstreamBaseURL,
		DefaultUpstreamAPIKey:  cfg.DefaultUpstreamAPIKey,
		ResponsesBaseURL:       cfg.DefaultResponsesBaseURL,
		AuthStore:              instanceService,
		OnRequestStage:         opts.OnRequestStage,
		OnRuntimeChange: func(_ int64) {
			instanceService.TouchRevision()
		},
	})

	loginService := loginsvc.NewServiceWithInstances(cfg, instanceService)
	systemService := systemsvc.NewService()

	refresher := refreshsvc.NewService(instanceService, proxyService)
	refresher.SetEnabled(cfg.RefreshEnabled)
	refresher.SetRangeDays(cfg.RefreshMinDays, cfg.RefreshMaxDays)
	proxysvc.SetTokenRefreshEnabled(cfg.RefreshEnabled)

	ctx, cancel := context.WithCancel(parent)
	cancelOnce := sync.Once{}
	cancelOnly := func() {
		cancelOnce.Do(func() {
			cancel()
		})
	}

	app, err := server.NewAppWithBasePath(cfg, server.Services{
		Instances: instanceService,
		Proxy:     proxyService,
		Login:     loginService,
		System:    systemService,
		Refresher: refresher,
		Restart:   cancelOnly,
	}, basePath)
	if err != nil {
		cancelOnly()
		return nil, err
	}

	go refresher.Start(ctx.Done())

	return &Embedded{
		App:       app,
		Config:    cfg,
		Instances: instanceService,
		Proxy:     proxyService,
		Login:     loginService,
		System:    systemService,
		Refresher: refresher,
		Cancel:    cancelOnly,
		DB:        opts.DB,
	}, nil
}

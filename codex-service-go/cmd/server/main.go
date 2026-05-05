package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"codex-service-go/internal/config"
	"codex-service-go/internal/db"
	"codex-service-go/internal/server"
	instsvc "codex-service-go/internal/services/instances"
	loginsvc "codex-service-go/internal/services/login"
	proxysvc "codex-service-go/internal/services/proxy"
	refreshsvc "codex-service-go/internal/services/refresh"
	systemsvc "codex-service-go/internal/services/system"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	sqliteDB, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer sqliteDB.Close()

	if err := db.Migrate(sqliteDB); err != nil {
		log.Fatalf("migrate database: %v", err)
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
	if strings.TrimSpace(cfg.ServiceToken) != "" {
		if err := repo.SetAllInternalTokens(context.Background(), cfg.ServiceToken); err != nil {
			log.Fatalf("sync internal tokens: %v", err)
		}
	}
	if n, err := instanceService.MigrateLegacyAuthFiles(context.Background()); err != nil {
		log.Fatalf("migrate legacy auth files: %v", err)
	} else if n > 0 {
		log.Printf("[migrate] imported %d legacy auth file(s) into db", n)
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
		OnRuntimeChange: func(_ int64) {
			instanceService.TouchRevision()
		},
	})
	loginService := loginsvc.NewServiceWithInstances(cfg, instanceService)
	systemService := systemsvc.NewService()

	// 启动凭据刷新器（定期根据 last_refresh 刷新 auth.json）
	refresher := refreshsvc.NewService(instanceService, proxyService)
	refresher.SetEnabled(cfg.RefreshEnabled)
	refresher.SetRangeDays(cfg.RefreshMinDays, cfg.RefreshMaxDays)
	proxysvc.SetTokenRefreshEnabled(cfg.RefreshEnabled)

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	app, err := server.NewApp(cfg, server.Services{
		Instances: instanceService,
		Proxy:     proxyService,
		Login:     loginService,
		System:    systemService,
		Refresher: refresher,
		Restart:   cancel,
	})
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	log.Printf("codex-service-go listening on %s", addr)
	log.Printf("[compat] 模式: %s  上游: %s", cfg.DefaultProxyAuthMode, cfg.DefaultUpstreamBaseURL)
	log.Printf("[compat] 调试: %v  Chat兼容: %v  流转换: %v  非流聚合: %v  强制流转换: %v  思考策略: %s",
		cfg.ProxyDebug,
		cfg.ProxyEnableCompatOpenAI,
		cfg.ProxyEnableStreamCompat,
		cfg.ProxyEnableAggregation,
		cfg.ProxyForceStreamCompat,
		cfg.ProxyReasoningCompat,
	)
	if strings.EqualFold(cfg.DefaultProxyAuthMode, "chatgpt") {
		log.Println("[compat] ChatGPT OAuth：按实例凭据存储在 SQLite（通过管理界面维护）")
	}

	// 启动刷新器
	go refresher.Start(ctx.Done())

	if err := app.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) && err != context.Canceled {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}

package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"codex-service-go/internal/config"
	"codex-service-go/internal/handlers"
	instsvc "codex-service-go/internal/services/instances"
	loginsvc "codex-service-go/internal/services/login"
	proxysvc "codex-service-go/internal/services/proxy"
	systemsvc "codex-service-go/internal/services/system"
)

//go:embed templates/*.html
var templateFS embed.FS

// 提供最小占位静态资源，抑制旧版页面对 /css/modules/* 的请求产生的 404 噪音。
//
//go:embed static/**
var staticFS embed.FS

type App struct {
	Engine   *gin.Engine
	cfg      *config.Config
	services Services
}

type Services struct {
	Instances *instsvc.Service
	Proxy     *proxysvc.Service
	Login     *loginsvc.Service
	System    *systemsvc.Service
	Refresher interface {
		IsEnabled() bool
		SetEnabled(bool)
	}
	Restart func()
}

func NewApp(cfg *config.Config, services Services) (*App, error) {
	return NewAppWithBasePath(cfg, services, "")
}

func NewAppWithBasePath(cfg *config.Config, services Services, basePath string) (*App, error) {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	normalizedBasePath := normalizeBasePath(basePath)
	if cfg != nil {
		cfg.WebBasePath = normalizedBasePath
	}

	funcs := template.FuncMap{
		"mount": func(p string) string {
			p = strings.TrimSpace(p)
			if p == "" {
				return normalizedBasePath
			}
			if normalizedBasePath == "" {
				return p
			}
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			if strings.HasPrefix(p, normalizedBasePath+"/") || p == normalizedBasePath {
				return p
			}
			return normalizedBasePath + p
		},
	}

	t, err := template.New("").Funcs(funcs).ParseFS(templateFS,
		"templates/layout.html",
		"templates/admin_list.html",
		"templates/instances_table.html",
		"templates/instances_table_with_modal.html",
		"templates/instance_form.html",
		"templates/debug_form.html",
		"templates/debug_bulk_form.html",
		"templates/auth_form.html",
		"templates/auth_form_with_table.html",
		"templates/auth_bulk_form.html",
		"templates/auth_view.html",
		"templates/log_view.html",
		"templates/log_full.html",
		"templates/login_modal.html",
		"templates/login_modal_with_table.html",
		"templates/login.html",
		"templates/env_form.html",
		"templates/refresh_status.html",
		"templates/ssh_status.html",
	)
	if err != nil {
		return nil, err
	}
	r.SetHTMLTemplate(t)

	if services.Instances == nil {
		return nil, fmt.Errorf("instances service is required")
	}
	if services.Proxy == nil {
		return nil, fmt.Errorf("proxy service is required")
	}
	if services.Login == nil {
		services.Login = loginsvc.NewServiceWithInstances(cfg, services.Instances)
	}
	if services.System == nil {
		services.System = systemsvc.NewService()
	}

	authManager, err := handlers.NewAuthManager(cfg)
	if err != nil {
		return nil, err
	}
	admin := handlers.NewAdminHandler(cfg, services.Instances, services.Proxy, services.Login, services.System, authManager)
	if services.Refresher != nil {
		admin.AttachRefresher(services.Refresher)
	}
	if services.Restart != nil {
		admin.AttachRestart(services.Restart)
	}
	api := handlers.NewAPIHandler(cfg, services.Instances, services.Proxy)

	r.GET("/healthz", api.HandleHealth)

	// 静态资源与兼容兜底：避免被 "/:instanceID/*path" 误匹配导致 400
	// - favicon：返回 204，兼容带尾斜杠的路径
	r.GET("/favicon.ico", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/favicon.ico/*any", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	// - 旧版页面可能请求的 /css/* 资源：提供内置占位样式，避免 404 干扰日志
	if cssSub, err := fs.Sub(staticFS, "static/css"); err == nil {
		r.StaticFS("/css", http.FS(cssSub))
	}

	r.GET("/admin/login", admin.LoginPage)
	r.POST("/admin/login", admin.LoginSubmit)

	adminGroup := r.Group("/admin")
	if authManager != nil {
		adminGroup.Use(authManager.Middleware())
	}
	if authManager != nil && authManager.IsEnabled() {
		log.Printf("[admin] 登录已启用，账号: %s，凭据文件: %s", cfg.AdminUser, cfg.AdminCredentialsFile)
	} else {
		log.Printf("[admin] 登录未启用。请设置 WEB_ADMIN_USER/WEB_ADMIN_PASS 或编辑 %s", cfg.AdminCredentialsFile)
	}
	adminGroup.GET("", admin.Index)
	adminGroup.GET("/settings", admin.Index)
	adminGroup.POST("/logout", admin.Logout)
	adminGroup.GET("/instances/table", admin.InstancesTable)
	adminGroup.GET("/instances/new", admin.NewInstanceForm)
	adminGroup.POST("/instances", admin.CreateInstance)
	adminGroup.GET("/instances/:id/edit", admin.EditInstanceForm)
	adminGroup.POST("/instances/:id", admin.UpdateInstance)
	adminGroup.GET("/instances/:id/auth", admin.AuthForm)
	adminGroup.GET("/instances/:id/auth/view", admin.AuthView)
	adminGroup.POST("/instances/:id/auth", admin.SaveAuth)
	adminGroup.DELETE("/instances/:id/auth", admin.DeleteAuth)
	adminGroup.GET("/auth/bulk", admin.BulkAuthForm)
	adminGroup.GET("/auth/export", admin.ExportAuthBulk)
	adminGroup.POST("/auth/bulk", admin.BulkAuthSubmit)
	adminGroup.GET("/env", admin.EnvForm)
	adminGroup.POST("/env", admin.EnvSubmit)
	adminGroup.POST("/restart", admin.Restart)
	adminGroup.GET("/instances/:id/log", admin.LogView)
	adminGroup.GET("/instances/:id/log/full", admin.LogFullPage)
	adminGroup.GET("/instances/:id/log/raw", admin.LogRaw)
	adminGroup.DELETE("/instances/:id/log", admin.DeleteLog)
	adminGroup.GET("/instances/:id/login", admin.LoginModal)
	adminGroup.POST("/instances/:id/login", admin.StartLogin)
	adminGroup.POST("/instances/:id/login/cancel", admin.CancelLogin)
	adminGroup.POST("/instances/:id/login/force", admin.LoginForceCancel)
	adminGroup.GET("/instances/:id/login/status", admin.LoginStatus)
	adminGroup.POST("/instances/:id/login/close", admin.LoginClose)
	adminGroup.POST("/instances/bulk/enable", admin.BulkSetInstanceEnabled)
	adminGroup.GET("/instances/bulk/debug", admin.BulkDebugForm)
	adminGroup.POST("/instances/bulk/debug", admin.BulkSetInstanceDebug)
	adminGroup.POST("/instances/bulk/delete", admin.BulkDeleteInstances)
	adminGroup.POST("/instances/:id/refresh", admin.RefreshInstance)
	adminGroup.POST("/instances/:id/enable", admin.SetInstanceEnabled)
	adminGroup.GET("/instances/:id/debug", admin.DebugForm)
	adminGroup.POST("/instances/:id/debug", admin.SetInstanceDebug)
	adminGroup.DELETE("/instances/:id", admin.DeleteInstance)
	adminGroup.GET("/close", admin.CloseModal)

	// 旧版日志页内的相对 CSS 请求，提供与根级一致的占位样式，进一步消除 404
	if cssSub, err := fs.Sub(staticFS, "static/css"); err == nil {
		cssFS := http.FS(cssSub)
		adminGroup.GET("/instances/:id/log/css/*filepath", func(c *gin.Context) {
			fp := c.Param("filepath")
			if len(fp) > 0 && fp[0] == '/' {
				fp = fp[1:]
			}
			if fp == "" {
				c.Status(http.StatusNotFound)
				return
			}
			c.FileFromFS(fp, cssFS)
		})
	}

	// 刷新计划开关
	adminGroup.GET("/refresh/status", admin.RefreshStatus)
	adminGroup.POST("/refresh/toggle", admin.RefreshToggle)
	adminGroup.POST("/refresh/config", admin.RefreshConfig)

	// 兼容：与 Node 版对齐的管理查询 API
	adminGroup.GET("/api/compat", admin.CompatStatus)

		// 主从同步：供 Transfer API 拉取实例列表并自动生成渠道
		// - embedded 模式下 Transfer API 会直接调用 Instances/Proxy 服务对象，不暴露 internal/http 接口
		if cfg != nil && strings.TrimSpace(cfg.ServiceToken) != "" {
			r.GET("/internal/instances", api.HandleInternalInstances)
			r.GET("/internal/instances/watch", api.HandleInternalInstancesWatch)
		}

	apiGroup := r.Group("/:instanceID")
	{
		apiGroup.Any("/*path", api.HandleAny)
	}

	// Safety net: some stale clients might still poll "/raw/" due to an old template.
	// Return 204 to avoid noisy 400s in logs. This does not affect the correct
	// endpoint "/admin/instances/:id/log/raw".
	r.GET("/raw/*any", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	return &App{Engine: r, cfg: cfg, services: services}, nil
}

func (a *App) Run(ctx context.Context) error {
	addr := a.cfg.ServerHost + ":" + strconv.Itoa(a.cfg.ServerPort)
	srv := &http.Server{Addr: addr, Handler: a.Engine}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	return srv.ListenAndServe()
}

func Asset(pathParts ...string) string {
	return path.Join(pathParts...)
}

func normalizeBasePath(basePath string) string {
	trimmed := strings.TrimSpace(basePath)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "/" {
		return ""
	}
	return trimmed
}

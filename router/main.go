package router

import (
	"embed"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/controller"
	"one-api/middleware"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	router.GET("/desktop-latest.json", middleware.DisableCache(), controller.GetClawBoxInstalledUpdate)
	router.GET("/desktop/releases/:version/download", controller.DownloadClawBoxInstalledRelease)
	router.GET("/portable-status.json", middleware.DisableCache(), controller.GetClawBoxPortableUpdateStatus)
	router.GET("/portable-latest.json", middleware.DisableCache(), controller.GetClawBoxPortableUpdate)
	router.GET("/portable-releases.json", middleware.DisableCache(), controller.GetClawBoxPortableReleaseCatalog)
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetVideoRouter(router)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, buildFS, indexPage)
	} else {
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")
		router.NoRoute(func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s%s", frontendBaseUrl, c.Request.RequestURI))
		})
	}
}

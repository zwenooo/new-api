package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/logger"
	"one-api/types"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	lastCPUOverloadLogUnix    atomic.Int64
	lastMemoryOverloadLogUnix atomic.Int64
	lastDiskOverloadLogUnix   atomic.Int64
)

func SystemPerformanceCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		err, status, config := checkSystemPerformance()
		if err != nil {
			logSystemPerformanceOverloadAsync(c, err, status, config)
			if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
				c.JSON(err.StatusCode, gin.H{
					"type":  "error",
					"error": err.ToClaudeError(),
				})
			} else {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToOpenAIError(),
				})
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

func checkSystemPerformance() (*types.NewAPIError, common.SystemStatus, common.PerformanceMonitorConfig) {
	config := common.GetPerformanceMonitorConfig()
	if !config.Enabled {
		return nil, common.SystemStatus{}, config
	}

	status := common.GetSystemStatus()
	if config.CPUThreshold > 0 && int(status.CPUUsage) > config.CPUThreshold {
		return types.NewErrorWithStatusCode(errors.New("system cpu overloaded"), types.ErrorCode("system_cpu_overloaded"), http.StatusServiceUnavailable), status, config
	}
	if config.MemoryThreshold > 0 && int(status.MemoryUsage) > config.MemoryThreshold {
		return types.NewErrorWithStatusCode(errors.New("system memory overloaded"), types.ErrorCode("system_memory_overloaded"), http.StatusServiceUnavailable), status, config
	}
	if config.DiskThreshold > 0 && int(status.DiskUsage) > config.DiskThreshold {
		return types.NewErrorWithStatusCode(errors.New("system disk overloaded"), types.ErrorCode("system_disk_overloaded"), http.StatusServiceUnavailable), status, config
	}
	return nil, status, config
}

func performanceMonitorTargetPath() string {
	cachePath := strings.TrimSpace(common.GetDiskCachePath())
	if cachePath != "" {
		return cachePath
	}
	return os.TempDir()
}

func shouldLogPerformanceOverload(errCode string) bool {
	now := time.Now().Unix()
	var slot *atomic.Int64
	switch errCode {
	case "system_cpu_overloaded":
		slot = &lastCPUOverloadLogUnix
	case "system_memory_overloaded":
		slot = &lastMemoryOverloadLogUnix
	case "system_disk_overloaded":
		slot = &lastDiskOverloadLogUnix
	default:
		return true
	}
	last := slot.Load()
	if now-last < 10 {
		return false
	}
	slot.Store(now)
	return true
}

func logSystemPerformanceOverloadAsync(c *gin.Context, err *types.NewAPIError, status common.SystemStatus, config common.PerformanceMonitorConfig) {
	if c == nil || err == nil {
		return
	}
	errCode := string(err.GetErrorCode())
	if !shouldLogPerformanceOverload(errCode) {
		return
	}
	method := c.Request.Method
	path := c.Request.URL.Path
	targetPath := performanceMonitorTargetPath()
	ctx := c.Request.Context()
	common.RelayCtxGo(ctx, func() {
		logger.LogWarn(
			ctx,
			fmt.Sprintf(
				"[system_overload] code=%s method=%s path=%s cpu=%.2f/%d memory=%.2f/%d disk=%.2f/%d monitor_target=%s",
				errCode,
				method,
				path,
				status.CPUUsage,
				config.CPUThreshold,
				status.MemoryUsage,
				config.MemoryThreshold,
				status.DiskUsage,
				config.DiskThreshold,
				targetPath,
			),
		)
	})
}

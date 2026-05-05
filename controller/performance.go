package controller

import (
	"net/http"
	"one-api/common"
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

type PerformanceStats struct {
	CacheStats    common.DiskCacheStats `json:"cache_stats"`
	MemoryStats   MemoryStats           `json:"memory_stats"`
	DiskCacheInfo DiskCacheInfo         `json:"disk_cache_info"`
	DiskSpaceInfo common.DiskSpaceInfo  `json:"disk_space_info"`
	Config        PerformanceConfig     `json:"config"`
}

type MemoryStats struct {
	Alloc        uint64 `json:"alloc"`
	TotalAlloc   uint64 `json:"total_alloc"`
	Sys          uint64 `json:"sys"`
	NumGC        uint32 `json:"num_gc"`
	NumGoroutine int    `json:"num_goroutine"`
}

type DiskCacheInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

type PerformanceConfig struct {
	DiskCacheEnabled       bool   `json:"disk_cache_enabled"`
	DiskCacheThresholdMB   int    `json:"disk_cache_threshold_mb"`
	DiskCacheMaxSizeMB     int    `json:"disk_cache_max_size_mb"`
	DiskCachePath          string `json:"disk_cache_path"`
	IsRunningInContainer   bool   `json:"is_running_in_container"`
	MonitorEnabled         bool   `json:"monitor_enabled"`
	MonitorCPUThreshold    int    `json:"monitor_cpu_threshold"`
	MonitorMemoryThreshold int    `json:"monitor_memory_threshold"`
	MonitorDiskThreshold   int    `json:"monitor_disk_threshold"`
}

func GetPerformanceStats(c *gin.Context) {
	cacheStats := common.GetDiskCacheStats()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	diskConfig := common.GetDiskCacheConfig()
	monitorConfig := common.GetPerformanceMonitorConfig()
	stats := PerformanceStats{
		CacheStats: cacheStats,
		MemoryStats: MemoryStats{
			Alloc:        memStats.Alloc,
			TotalAlloc:   memStats.TotalAlloc,
			Sys:          memStats.Sys,
			NumGC:        memStats.NumGC,
			NumGoroutine: runtime.NumGoroutine(),
		},
		DiskCacheInfo: getDiskCacheInfo(),
		DiskSpaceInfo: common.GetDiskSpaceInfo(),
		Config: PerformanceConfig{
			DiskCacheEnabled:       diskConfig.Enabled,
			DiskCacheThresholdMB:   diskConfig.ThresholdMB,
			DiskCacheMaxSizeMB:     diskConfig.MaxSizeMB,
			DiskCachePath:          diskConfig.Path,
			IsRunningInContainer:   common.IsRunningInContainer(),
			MonitorEnabled:         monitorConfig.Enabled,
			MonitorCPUThreshold:    monitorConfig.CPUThreshold,
			MonitorMemoryThreshold: monitorConfig.MemoryThreshold,
			MonitorDiskThreshold:   monitorConfig.DiskThreshold,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

func ClearDiskCache(c *gin.Context) {
	if err := common.CleanupOldDiskCacheFiles(10 * time.Minute); err != nil {
		common.ApiError(c, err)
		return
	}
	common.SyncDiskCacheStats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "不活跃的磁盘缓存已清理",
	})
}

func ResetPerformanceStats(c *gin.Context) {
	common.ResetDiskCacheStats()
	common.SyncDiskCacheStats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "统计信息已重置",
	})
}

func ForceGC(c *gin.Context) {
	runtime.GC()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "GC 已执行",
	})
}

func getDiskCacheInfo() DiskCacheInfo {
	dir := common.GetDiskCacheDir()
	info := DiskCacheInfo{
		Path:   dir,
		Exists: false,
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return info
	}

	info.Exists = true
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info.FileCount++
		if fileInfo, err := entry.Info(); err == nil {
			info.TotalSize += fileInfo.Size()
		}
	}
	return info
}

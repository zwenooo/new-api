package common

import (
	"sync"
	"sync/atomic"
)

type DiskCacheConfig struct {
	Enabled     bool
	ThresholdMB int
	MaxSizeMB   int
	Path        string
}

var diskCacheConfig = DiskCacheConfig{
	Enabled:     false,
	ThresholdMB: 10,
	MaxSizeMB:   1024,
	Path:        "",
}
var diskCacheConfigMu sync.RWMutex
var diskCacheStatsMu sync.Mutex

func GetDiskCacheConfig() DiskCacheConfig {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return diskCacheConfig
}

func SetDiskCacheConfig(config DiskCacheConfig) {
	diskCacheConfigMu.Lock()
	defer diskCacheConfigMu.Unlock()
	diskCacheConfig = config
}

func IsDiskCacheEnabled() bool {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return diskCacheConfig.Enabled
}

func GetDiskCacheThresholdBytes() int64 {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return int64(diskCacheConfig.ThresholdMB) << 20
}

func GetDiskCacheMaxSizeBytes() int64 {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return int64(diskCacheConfig.MaxSizeMB) << 20
}

func GetDiskCachePath() string {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return diskCacheConfig.Path
}

type DiskCacheStats struct {
	ActiveDiskFiles         int64 `json:"active_disk_files"`
	CurrentDiskUsageBytes   int64 `json:"current_disk_usage_bytes"`
	ReservedDiskUsageBytes  int64 `json:"reserved_disk_usage_bytes"`
	ActiveMemoryBuffers     int64 `json:"active_memory_buffers"`
	CurrentMemoryUsageBytes int64 `json:"current_memory_usage_bytes"`
	DiskCacheHits           int64 `json:"disk_cache_hits"`
	MemoryCacheHits         int64 `json:"memory_cache_hits"`
	DiskCacheMaxBytes       int64 `json:"disk_cache_max_bytes"`
	DiskCacheThresholdBytes int64 `json:"disk_cache_threshold_bytes"`
}

var diskCacheStats DiskCacheStats

func GetDiskCacheStats() DiskCacheStats {
	return DiskCacheStats{
		ActiveDiskFiles:         atomic.LoadInt64(&diskCacheStats.ActiveDiskFiles),
		CurrentDiskUsageBytes:   atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes),
		ReservedDiskUsageBytes:  atomic.LoadInt64(&diskCacheStats.ReservedDiskUsageBytes),
		ActiveMemoryBuffers:     atomic.LoadInt64(&diskCacheStats.ActiveMemoryBuffers),
		CurrentMemoryUsageBytes: atomic.LoadInt64(&diskCacheStats.CurrentMemoryUsageBytes),
		DiskCacheHits:           atomic.LoadInt64(&diskCacheStats.DiskCacheHits),
		MemoryCacheHits:         atomic.LoadInt64(&diskCacheStats.MemoryCacheHits),
		DiskCacheMaxBytes:       GetDiskCacheMaxSizeBytes(),
		DiskCacheThresholdBytes: GetDiskCacheThresholdBytes(),
	}
}

func IncrementDiskFiles(size int64) {
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()
	atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, 1)
	atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, size)
}

func DecrementDiskFiles(size int64) {
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()
	if atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, -1) < 0 {
		atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, 0)
	}
	if atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, -size) < 0 {
		atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, 0)
	}
}

func IncrementMemoryBuffers(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveMemoryBuffers, 1)
	atomic.AddInt64(&diskCacheStats.CurrentMemoryUsageBytes, size)
}

func DecrementMemoryBuffers(size int64) {
	if atomic.AddInt64(&diskCacheStats.ActiveMemoryBuffers, -1) < 0 {
		atomic.StoreInt64(&diskCacheStats.ActiveMemoryBuffers, 0)
	}
	if atomic.AddInt64(&diskCacheStats.CurrentMemoryUsageBytes, -size) < 0 {
		atomic.StoreInt64(&diskCacheStats.CurrentMemoryUsageBytes, 0)
	}
}

func IncrementDiskCacheHits() {
	atomic.AddInt64(&diskCacheStats.DiskCacheHits, 1)
}

func IncrementMemoryCacheHits() {
	atomic.AddInt64(&diskCacheStats.MemoryCacheHits, 1)
}

func ResetDiskCacheStats() {
	atomic.StoreInt64(&diskCacheStats.DiskCacheHits, 0)
	atomic.StoreInt64(&diskCacheStats.MemoryCacheHits, 0)
}

func ResetDiskCacheUsage() {
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()
	atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, 0)
	atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, 0)
	atomic.StoreInt64(&diskCacheStats.ReservedDiskUsageBytes, 0)
}

func SyncDiskCacheStats() {
	fileCount, totalSize, err := GetDiskCacheInfo()
	if err != nil {
		return
	}
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()
	atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, int64(fileCount))
	atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, totalSize)
}

func IsDiskCacheAvailable(requestSize int64) bool {
	if !IsDiskCacheEnabled() {
		return false
	}
	currentUsage := atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes)
	reservedUsage := atomic.LoadInt64(&diskCacheStats.ReservedDiskUsageBytes)
	return currentUsage+reservedUsage+requestSize <= GetDiskCacheMaxSizeBytes()
}

func TryReserveDiskCache(requestSize int64) bool {
	if !IsDiskCacheEnabled() {
		return false
	}
	if requestSize < 0 {
		requestSize = 0
	}
	maxBytes := GetDiskCacheMaxSizeBytes()
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()
	currentUsage := atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes)
	reservedUsage := atomic.LoadInt64(&diskCacheStats.ReservedDiskUsageBytes)
	if currentUsage+reservedUsage+requestSize > maxBytes {
		return false
	}
	atomic.StoreInt64(&diskCacheStats.ReservedDiskUsageBytes, reservedUsage+requestSize)
	return true
}

func ReleaseDiskCacheReservation(requestSize int64) {
	if requestSize < 0 {
		requestSize = 0
	}
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()
	if atomic.AddInt64(&diskCacheStats.ReservedDiskUsageBytes, -requestSize) < 0 {
		atomic.StoreInt64(&diskCacheStats.ReservedDiskUsageBytes, 0)
	}
}

func CommitDiskCacheReservation(reservedSize int64, actualSize int64) {
	if reservedSize < 0 {
		reservedSize = 0
	}
	if actualSize < 0 {
		actualSize = 0
	}
	diskCacheStatsMu.Lock()
	defer diskCacheStatsMu.Unlock()

	reservedUsage := atomic.LoadInt64(&diskCacheStats.ReservedDiskUsageBytes) - reservedSize
	if reservedUsage < 0 {
		reservedUsage = 0
	}
	atomic.StoreInt64(&diskCacheStats.ReservedDiskUsageBytes, reservedUsage)
	atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, 1)
	atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, actualSize)
}

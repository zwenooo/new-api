package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type DiskCacheType string

const (
	DiskCacheTypeBody DiskCacheType = "body"
	DiskCacheTypeFile DiskCacheType = "file"
)

const diskCacheDir = "new-api-body-cache"

var activeDiskCacheFiles sync.Map

func GetDiskCacheDir() string {
	cachePath := GetDiskCachePath()
	if cachePath == "" {
		cachePath = os.TempDir()
	}
	return filepath.Join(cachePath, diskCacheDir)
}

func EnsureDiskCacheDir() error {
	return os.MkdirAll(GetDiskCacheDir(), 0o755)
}

func CreateDiskCacheFile(cacheType DiskCacheType) (string, *os.File, error) {
	if err := EnsureDiskCacheDir(); err != nil {
		return "", nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%s-%d.tmp", cacheType, uuid.New().String()[:8], time.Now().UnixNano())
	filePath := filepath.Join(GetDiskCacheDir(), filename)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create cache file: %w", err)
	}
	return filePath, file, nil
}

func WriteDiskCacheFile(cacheType DiskCacheType, data []byte) (string, error) {
	filePath, file, err := CreateDiskCacheFile(cacheType)
	if err != nil {
		return "", err
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(filePath)
		return "", fmt.Errorf("failed to write cache file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(filePath)
		return "", fmt.Errorf("failed to close cache file: %w", err)
	}
	return filePath, nil
}

func WriteDiskCacheFileString(cacheType DiskCacheType, data string) (string, error) {
	return WriteDiskCacheFile(cacheType, []byte(data))
}

func ReadDiskCacheFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

func ReadDiskCacheFileString(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func RemoveDiskCacheFile(filePath string) error {
	return os.Remove(filePath)
}

func MarkDiskCacheFileActive(filePath string) {
	if filePath == "" {
		return
	}
	activeDiskCacheFiles.Store(filePath, struct{}{})
}

func UnmarkDiskCacheFileActive(filePath string) {
	if filePath == "" {
		return
	}
	activeDiskCacheFiles.Delete(filePath)
}

func IsDiskCacheFileActive(filePath string) bool {
	if filePath == "" {
		return false
	}
	_, ok := activeDiskCacheFiles.Load(filePath)
	return ok
}

func CleanupOldDiskCacheFiles(maxAge time.Duration) error {
	dir := GetDiskCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		if IsDiskCacheFileActive(filePath) {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			if err := os.Remove(filePath); err == nil {
				DecrementDiskFiles(info.Size())
			}
		}
	}
	return nil
}

func GetDiskCacheInfo() (fileCount int, totalSize int64, err error) {
	dir := GetDiskCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fileCount++
		totalSize += info.Size()
	}
	return fileCount, totalSize, nil
}

func ShouldUseDiskCache(dataSize int64) bool {
	if !IsDiskCacheEnabled() {
		return false
	}
	if dataSize < GetDiskCacheThresholdBytes() {
		return false
	}
	return IsDiskCacheAvailable(dataSize)
}

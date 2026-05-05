//go:build !windows

package common

import (
	"os"

	"golang.org/x/sys/unix"
)

func GetDiskSpaceInfo() DiskSpaceInfo {
	cachePath := GetDiskCachePath()
	if cachePath == "" {
		if !IsDiskCacheEnabled() {
			return DiskSpaceInfo{}
		}
		cachePath = os.TempDir()
	}

	info := DiskSpaceInfo{}
	var stat unix.Statfs_t
	if err := unix.Statfs(cachePath, &stat); err != nil {
		return info
	}

	bsize := uint64(stat.Bsize)
	info.Total = uint64(stat.Blocks) * bsize
	info.Free = uint64(stat.Bavail) * bsize
	info.Used = info.Total - uint64(stat.Bfree)*bsize
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}
	return info
}

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"one-api/common"
	"one-api/logger"
	"one-api/model"
)

var requestTraceCleanupOnce sync.Once

// When the request trace spool directory's filesystem is running low on space,
// proactively delete the oldest persisted traces to keep the service healthy.
//
// This is a safety guard against ENOSPC causing log/trace writes to fail.
// Note: 10 GiB here uses a binary GB (1024^3) consistent with common.Bytes2Size.
const requestTraceMinFreeBytes = uint64(10 * 1024 * 1024 * 1024)

func StartRequestTraceCleanup() {
	if !common.IsMasterNode {
		return
	}
	requestTraceCleanupOnce.Do(func() {
		go func() {
			ctx := context.Background()
			const diskCheckInterval = time.Minute

			var lastRetentionRun time.Time

			var lastRetentionErrText string
			var lastRetentionErrLoggedAt time.Time

			var lastDiskErrText string
			var lastDiskErrLoggedAt time.Time

			for {
				// 1) Time-based retention cleanup (option-controlled)
				retentionMinutes, err := requestTraceDesiredRetentionMinutes()
				if err != nil {
					msg := err.Error()
					if msg != lastRetentionErrText || time.Since(lastRetentionErrLoggedAt) >= 30*time.Minute {
						lastRetentionErrText = msg
						lastRetentionErrLoggedAt = time.Now()
						logger.LogError(ctx, fmt.Sprintf("[request_trace] cleanup: invalid retention option: %v", err))
					}
					retentionMinutes = 0
				}

				retentionInterval := time.Duration(0)
				if retentionMinutes > 0 {
					retentionInterval = 30 * time.Minute
					half := time.Duration(retentionMinutes) * time.Minute / 2
					if half < time.Minute {
						half = time.Minute
					}
					if half < retentionInterval {
						retentionInterval = half
					}
				}

				if retentionInterval > 0 && (lastRetentionRun.IsZero() || time.Since(lastRetentionRun) >= retentionInterval) {
					cutoff := common.GetTimestamp() - int64(retentionMinutes)*60
					if cutoff > 0 {
						for i := 0; i < 50; i++ {
							deleted, err := cleanupRequestTraceBatch(ctx, cutoff, 200)
							if err != nil {
								logger.LogError(ctx, fmt.Sprintf("[request_trace] cleanup batch failed: %v", err))
								break
							}
							if deleted == 0 {
								break
							}
						}
					}
					lastRetentionRun = time.Now()
				}

				// 2) Disk-space safety guard (always-on)
				root := strings.TrimSpace(globalRequestTraceManager.spoolDir)
				if root == "" {
					root = strings.TrimSpace(common.RequestTraceSpoolDir)
				}
				if root != "" {
					avail, derr := diskAvailBytes(root)
					if derr != nil {
						msg := derr.Error()
						if msg != lastDiskErrText || time.Since(lastDiskErrLoggedAt) >= 30*time.Minute {
							lastDiskErrText = msg
							lastDiskErrLoggedAt = time.Now()
							logger.LogError(ctx, fmt.Sprintf("[request_trace] disk guard: statfs failed: dir=%s err=%v", root, derr))
						}
					} else if avail < requestTraceMinFreeBytes {
						logger.LogWarn(ctx, fmt.Sprintf(
							"[request_trace] low disk: dir=%s avail=%s min=%s; deleting oldest traces",
							root,
							common.Bytes2Size(int64(avail)),
							common.Bytes2Size(int64(requestTraceMinFreeBytes)),
						))

						deletedTotal := 0
						for i := 0; i < 500; i++ {
							availNow, err := diskAvailBytes(root)
							if err != nil {
								logger.LogError(ctx, fmt.Sprintf("[request_trace] disk guard: statfs failed during cleanup: dir=%s err=%v", root, err))
								break
							}
							if availNow >= requestTraceMinFreeBytes {
								break
							}

							// Use a cutoff in the future so the query selects the oldest sessions first.
							cutoffAll := common.GetTimestamp() + 1
							deleted, err := cleanupRequestTraceBatch(ctx, cutoffAll, 200)
							if err != nil {
								logger.LogError(ctx, fmt.Sprintf("[request_trace] disk guard cleanup batch failed: %v", err))
								break
							}
							if deleted == 0 {
								break
							}
							deletedTotal += deleted
						}

						availAfter, err := diskAvailBytes(root)
						if err == nil {
							if availAfter < requestTraceMinFreeBytes {
								logger.LogWarn(ctx, fmt.Sprintf(
									"[request_trace] low disk cleanup incomplete: dir=%s avail=%s min=%s deleted=%d",
									root,
									common.Bytes2Size(int64(availAfter)),
									common.Bytes2Size(int64(requestTraceMinFreeBytes)),
									deletedTotal,
								))
							} else if deletedTotal > 0 {
								logger.LogWarn(ctx, fmt.Sprintf(
									"[request_trace] low disk cleanup done: dir=%s avail=%s deleted=%d",
									root,
									common.Bytes2Size(int64(availAfter)),
									deletedTotal,
								))
							}
						}
					}
				}

				time.Sleep(diskCheckInterval)
			}
		}()
	})
}

func cleanupRequestTraceBatch(ctx context.Context, cutoffSec int64, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}

	var sessions []model.RequestTraceSession
	if err := model.DB.
		Select("request_id,created_at").
		Where("created_at < ?", cutoffSec).
		Order("created_at asc").
		Limit(limit).
		Find(&sessions).Error; err != nil {
		return 0, err
	}
	if len(sessions) == 0 {
		return 0, nil
	}

	root := strings.TrimSpace(globalRequestTraceManager.spoolDir)
	if root == "" {
		root = strings.TrimSpace(common.RequestTraceSpoolDir)
	}
	if root == "" {
		return 0, fmt.Errorf("REQUEST_TRACE_SPOOL_DIR is empty")
	}

	deletedIDs := make([]string, 0, len(sessions))
	for _, s := range sessions {
		requestID := strings.TrimSpace(s.RequestId)
		if requestID == "" || s.CreatedAt <= 0 {
			continue
		}

		t := time.Unix(s.CreatedAt, 0).In(time.Local)
		dirKey := fmt.Sprintf("request_traces/%04d/%02d/%02d/%s", t.Year(), int(t.Month()), t.Day(), requestID)
		dirPath := filepath.Join(root, filepath.FromSlash(dirKey))
		if err := os.RemoveAll(dirPath); err != nil {
			logger.LogError(ctx, fmt.Sprintf("[request_trace] cleanup remove dir failed: request_id=%s dir=%s err=%v", requestID, dirPath, err))
			continue
		}
		deletedIDs = append(deletedIDs, requestID)
	}

	if len(deletedIDs) == 0 {
		return 0, nil
	}

	if err := model.DB.Where("request_id IN ?", deletedIDs).Delete(&model.RequestTraceNode{}).Error; err != nil {
		return 0, err
	}
	if err := model.DB.Where("request_id IN ?", deletedIDs).Delete(&model.RequestTraceSession{}).Error; err != nil {
		return 0, err
	}
	return len(deletedIDs), nil
}

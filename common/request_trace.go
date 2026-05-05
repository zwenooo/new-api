package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Request trace (full request/response capture) settings.
//
// This feature is intentionally opt-in because it can persist sensitive headers
// (e.g. Authorization) and large bodies.
var (
	RequestTraceEnabled bool

	// RequestTraceSpoolDir is where trace objects are persisted on disk.
	// It must be a persistent volume in production.
	RequestTraceSpoolDir string

	// RequestTraceRetentionMinutes controls automatic cleanup of request trace data.
	// 0 means keep forever.
	RequestTraceRetentionMinutes int

	// RequestTraceRetentionDays is a legacy retention knob kept for backward compatibility.
	// New code should use RequestTraceRetentionMinutes.
	// 0 means keep forever.
	RequestTraceRetentionDays int
)

func initRequestTraceEnv() {
	RequestTraceEnabled = GetEnvOrDefaultBool("REQUEST_TRACE_ENABLED", false)

	// Spool directory defaults under LOG_DIR.
	spool := strings.TrimSpace(os.Getenv("REQUEST_TRACE_SPOOL_DIR"))
	if spool == "" {
		spool = filepath.Join(*LogDir, "request_traces")
	}
	if abs, err := filepath.Abs(spool); err == nil {
		spool = abs
	}
	RequestTraceSpoolDir = spool

	RequestTraceRetentionDays = GetEnvOrDefault("REQUEST_TRACE_RETENTION_DAYS", 0)

	// Prefer minutes-based retention; fallback to legacy days env.
	retentionMinutesRaw := strings.TrimSpace(os.Getenv("REQUEST_TRACE_RETENTION_MINUTES"))
	if retentionMinutesRaw == "" {
		RequestTraceRetentionMinutes = RequestTraceRetentionDays * 24 * 60
		return
	}
	minutes, err := strconv.Atoi(retentionMinutesRaw)
	if err != nil || minutes < 0 {
		SysError(fmt.Sprintf(
			"failed to parse REQUEST_TRACE_RETENTION_MINUTES: %v, using REQUEST_TRACE_RETENTION_DAYS=%d",
			err,
			RequestTraceRetentionDays,
		))
		RequestTraceRetentionMinutes = RequestTraceRetentionDays * 24 * 60
		return
	}
	RequestTraceRetentionMinutes = minutes
}

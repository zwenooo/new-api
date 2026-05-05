package proxy

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type codexUsageHeaderSnapshot struct {
	PrimaryUsedPercent         *float64
	PrimaryResetAfterSeconds   *int
	PrimaryWindowMinutes       *int
	SecondaryUsedPercent       *float64
	SecondaryResetAfterSeconds *int
	SecondaryWindowMinutes     *int
	UpdatedAt                  time.Time
}

type normalizedCodexUsageSnapshot struct {
	Used5hPercent *float64
	Reset5hAt     *int64
	Used7dPercent *float64
	Reset7dAt     *int64
}

type codexUsageSnapshotStore interface {
	SaveCodexUsageSnapshot(ctx context.Context, id int64, updatedAt int64, used5hPercent *float64, reset5hAt *int64, used7dPercent *float64, reset7dAt *int64) error
}

func parseCodexUsageHeaderSnapshot(headers http.Header, now time.Time) *codexUsageHeaderSnapshot {
	if headers == nil {
		return nil
	}

	parseFloat := func(key string) *float64 {
		raw := strings.TrimSpace(headers.Get(key))
		if raw == "" {
			return nil
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil
		}
		return &value
	}

	parseInt := func(key string) *int {
		raw := strings.TrimSpace(headers.Get(key))
		if raw == "" {
			return nil
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return nil
		}
		return &value
	}

	snapshot := &codexUsageHeaderSnapshot{UpdatedAt: now}
	hasData := false

	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
	}
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
	}

	if !hasData {
		return nil
	}
	return snapshot
}

func (s *codexUsageHeaderSnapshot) normalize() *normalizedCodexUsageSnapshot {
	if s == nil {
		return nil
	}

	result := &normalizedCodexUsageSnapshot{}
	primaryWindow, hasPrimaryWindow := derefInt(s.PrimaryWindowMinutes)
	secondaryWindow, hasSecondaryWindow := derefInt(s.SecondaryWindowMinutes)

	use5hFromPrimary := false
	use7dFromPrimary := false

	switch {
	case hasPrimaryWindow && hasSecondaryWindow:
		if primaryWindow < secondaryWindow {
			use5hFromPrimary = true
		} else {
			use7dFromPrimary = true
		}
	case hasPrimaryWindow:
		if primaryWindow <= 360 {
			use5hFromPrimary = true
		} else {
			use7dFromPrimary = true
		}
	case hasSecondaryWindow:
		if secondaryWindow <= 360 {
			use7dFromPrimary = true
		} else {
			use5hFromPrimary = true
		}
	default:
		use7dFromPrimary = true
	}

	baseUnix := s.UpdatedAt.Unix()
	toResetAt := func(resetAfterSeconds *int) *int64 {
		if resetAfterSeconds == nil {
			return nil
		}
		seconds := *resetAfterSeconds
		if seconds < 0 {
			seconds = 0
		}
		resetAt := baseUnix + int64(seconds)
		return &resetAt
	}

	if use5hFromPrimary {
		result.Used5hPercent = s.PrimaryUsedPercent
		result.Reset5hAt = toResetAt(s.PrimaryResetAfterSeconds)
		result.Used7dPercent = s.SecondaryUsedPercent
		result.Reset7dAt = toResetAt(s.SecondaryResetAfterSeconds)
	} else if use7dFromPrimary {
		result.Used5hPercent = s.SecondaryUsedPercent
		result.Reset5hAt = toResetAt(s.SecondaryResetAfterSeconds)
		result.Used7dPercent = s.PrimaryUsedPercent
		result.Reset7dAt = toResetAt(s.PrimaryResetAfterSeconds)
	}

	if result.Used5hPercent == nil && result.Reset5hAt == nil && result.Used7dPercent == nil && result.Reset7dAt == nil {
		return nil
	}
	return result
}

func derefInt(value *int) (int, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func (s *Service) persistCodexUsageSnapshot(ctx context.Context, instanceID int64, headers http.Header) {
	if s == nil || instanceID <= 0 || headers == nil {
		return
	}
	store, ok := s.opts.AuthStore.(codexUsageSnapshotStore)
	if !ok || store == nil {
		return
	}
	snapshot := parseCodexUsageHeaderSnapshot(headers, time.Now())
	if snapshot == nil {
		return
	}
	normalized := snapshot.normalize()
	if normalized == nil {
		return
	}
	if err := store.SaveCodexUsageSnapshot(
		ctx,
		instanceID,
		snapshot.UpdatedAt.Unix(),
		normalized.Used5hPercent,
		normalized.Reset5hAt,
		normalized.Used7dPercent,
		normalized.Reset7dAt,
	); err != nil {
		s.logDebug("persist codex usage snapshot failed: instance=%d err=%v", instanceID, err)
	}
}

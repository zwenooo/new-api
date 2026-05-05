package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type testCodexUsageSnapshotStore struct {
	called        bool
	instanceID    int64
	updatedAt     int64
	used5hPercent *float64
	reset5hAt     *int64
	used7dPercent *float64
	reset7dAt     *int64
}

func (s *testCodexUsageSnapshotStore) SaveCodexUsageSnapshot(ctx context.Context, id int64, updatedAt int64, used5hPercent *float64, reset5hAt *int64, used7dPercent *float64, reset7dAt *int64) error {
	s.called = true
	s.instanceID = id
	s.updatedAt = updatedAt
	s.used5hPercent = used5hPercent
	s.reset5hAt = reset5hAt
	s.used7dPercent = used7dPercent
	s.reset7dAt = reset7dAt
	return nil
}

func TestParseCodexUsageHeaderSnapshot_NormalizesPrimaryAndSecondaryWindows(t *testing.T) {
	now := time.Unix(1_710_000_000, 0)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "83.5")
	headers.Set("x-codex-primary-reset-after-seconds", "7200")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "14")
	headers.Set("x-codex-secondary-reset-after-seconds", "900")
	headers.Set("x-codex-secondary-window-minutes", "300")

	snapshot := parseCodexUsageHeaderSnapshot(headers, now)
	if snapshot == nil {
		t.Fatalf("expected snapshot to be parsed")
	}

	normalized := snapshot.normalize()
	if normalized == nil {
		t.Fatalf("expected snapshot to normalize")
	}
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 14 {
		t.Fatalf("expected 5h window to come from secondary headers, got %#v", normalized.Used5hPercent)
	}
	if normalized.Reset5hAt == nil || *normalized.Reset5hAt != now.Unix()+900 {
		t.Fatalf("expected 5h reset to be based on secondary headers, got %#v", normalized.Reset5hAt)
	}
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 83.5 {
		t.Fatalf("expected 7d window to come from primary headers, got %#v", normalized.Used7dPercent)
	}
	if normalized.Reset7dAt == nil || *normalized.Reset7dAt != now.Unix()+7200 {
		t.Fatalf("expected 7d reset to be based on primary headers, got %#v", normalized.Reset7dAt)
	}
}

func TestPersistCodexUsageSnapshot_SavesCanonical5hAnd7dFields(t *testing.T) {
	store := &testCodexUsageSnapshotStore{}
	svc := NewService(Options{AuthStore: store})
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "92")
	headers.Set("x-codex-primary-reset-after-seconds", "3600")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "31")
	headers.Set("x-codex-secondary-reset-after-seconds", "600")
	headers.Set("x-codex-secondary-window-minutes", "300")

	before := time.Now().Unix()
	svc.persistCodexUsageSnapshot(context.Background(), 42, headers)
	after := time.Now().Unix()

	if !store.called {
		t.Fatalf("expected usage snapshot to be persisted")
	}
	if store.instanceID != 42 {
		t.Fatalf("expected snapshot to be saved for instance 42, got %d", store.instanceID)
	}
	if store.updatedAt < before || store.updatedAt > after {
		t.Fatalf("expected updatedAt to be captured at persist time, got %d outside [%d, %d]", store.updatedAt, before, after)
	}
	if store.used5hPercent == nil || *store.used5hPercent != 31 {
		t.Fatalf("expected 5h usage percent to come from secondary headers, got %#v", store.used5hPercent)
	}
	if store.used7dPercent == nil || *store.used7dPercent != 92 {
		t.Fatalf("expected 7d usage percent to come from primary headers, got %#v", store.used7dPercent)
	}
	if store.reset5hAt == nil || *store.reset5hAt != store.updatedAt+600 {
		t.Fatalf("expected 5h reset to be updatedAt+600, got %#v", store.reset5hAt)
	}
	if store.reset7dAt == nil || *store.reset7dAt != store.updatedAt+3600 {
		t.Fatalf("expected 7d reset to be updatedAt+3600, got %#v", store.reset7dAt)
	}
}

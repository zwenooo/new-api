package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncFromPayload_UsageLimitCreatesSleep(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "runtime.json")
	m := New(file, "", nil)

	payload := map[string]any{
		"error": map[string]any{
			"type":            "usage_limit_reached",
			"resetsInSeconds": 5,
			"message":         "Usage limit reached",
			"plan_type":       "free",
		},
	}
	data, _ := json.Marshal(payload)
	if err := m.SyncFromPayload(429, "application/json", data); err != nil {
		t.Fatalf("SyncFromPayload error: %v", err)
	}
	// runtime file should exist
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("runtime file not written: %v", err)
	}
	// ShouldBlock during window
	res, err := m.ShouldBlock(time.Now())
	if err != nil {
		t.Fatalf("ShouldBlock error: %v", err)
	}
	if !res.Blocked || res.RetryAfter <= 0 {
		t.Fatalf("expected blocked with retry after, got: %+v", res)
	}
}

func TestSyncFromPayload_PaymentRequiredCreatesBlock(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "runtime.json")
	m := New(file, "", nil)

	payload := map[string]any{
		"detail": map[string]any{
			"code": "deactivated_workspace",
		},
	}
	data, _ := json.Marshal(payload)
	if err := m.SyncFromPayload(402, "application/json", data); err != nil {
		t.Fatalf("SyncFromPayload error: %v", err)
	}
	res, err := m.ShouldBlock(time.Now())
	if err != nil {
		t.Fatalf("ShouldBlock error: %v", err)
	}
	if !res.Blocked || res.Reason != "deactivated_workspace" {
		t.Fatalf("expected blocked deactivated_workspace, got: %+v", res)
	}
}

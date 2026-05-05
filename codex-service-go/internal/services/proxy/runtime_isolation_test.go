package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pruntime "codex-service-go/internal/proxy/runtime"
)

func TestRuntimeIsolation_PerInstanceCooldown(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "runtime.json")
	svc := NewService(Options{RuntimeFile: base})

	instA := int64(1)
	instB := int64(2)

	pathA := runtimeFileForInstance(base, instA)
	pathB := runtimeFileForInstance(base, instB)
	if pathA == pathB {
		t.Fatalf("expected different runtime files, got %s", pathA)
	}

	if err := os.WriteFile(pathB, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write runtime file B: %v", err)
	}

	runtimeA := svc.runtimeForInstance(instA)
	if runtimeA == nil {
		t.Fatalf("expected runtime manager for instance A")
	}
	if err := runtimeA.RecordUsageLimit(pruntime.UsageLimitInfo{ResetsInSeconds: 60}); err != nil {
		t.Fatalf("record usage limit: %v", err)
	}

	resA, err := svc.ShouldBlock(context.Background(), instA)
	if err != nil {
		t.Fatalf("ShouldBlock A error: %v", err)
	}
	if !resA.Blocked {
		t.Fatalf("expected A blocked, got %+v", resA)
	}

	resB, err := svc.ShouldBlock(context.Background(), instB)
	if err != nil {
		t.Fatalf("ShouldBlock B error: %v", err)
	}
	if resB.Blocked {
		t.Fatalf("expected B not blocked, got %+v", resB)
	}
}

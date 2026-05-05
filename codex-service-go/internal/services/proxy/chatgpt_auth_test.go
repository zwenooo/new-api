package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestChatGPTAuth_GetBearer_AccessTokenWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{
  "OPENAI_API_KEY": null,
  "tokens": {
    "access_token": "test-access-token",
    "refresh_token": null,
    "id_token": "test-id-token"
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	auth := NewChatGPTAuth(authPath, "app_EMoamEEZ73f0CkXaXp7hrann")
	got, err := auth.getBearer(context.Background())
	if err != nil {
		t.Fatalf("getBearer returned error: %v", err)
	}
	if got != "test-access-token" {
		t.Fatalf("expected access token, got %q", got)
	}
}

func TestChatGPTAuth_GetBearer_IdTokenOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{
  "OPENAI_API_KEY": null,
  "tokens": {
    "access_token": null,
    "refresh_token": null,
    "id_token": "test-id-token"
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	auth := NewChatGPTAuth(authPath, "app_EMoamEEZ73f0CkXaXp7hrann")
	got, err := auth.getBearer(context.Background())
	if err != nil {
		t.Fatalf("getBearer returned error: %v", err)
	}
	if got != "test-id-token" {
		t.Fatalf("expected id token fallback, got %q", got)
	}
}

func TestChatGPTAuth_RefreshNow_WithoutRefreshToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{
  "OPENAI_API_KEY": null,
  "tokens": {
    "access_token": "test-access-token",
    "refresh_token": null,
    "id_token": "test-id-token"
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	auth := NewChatGPTAuth(authPath, "app_EMoamEEZ73f0CkXaXp7hrann")
	if err := auth.RefreshNow(); err == nil {
		t.Fatalf("expected RefreshNow to fail without refresh_token")
	}
}

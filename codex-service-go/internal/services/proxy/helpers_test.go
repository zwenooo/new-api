package proxy

import (
	"net/http"
	"testing"
)

func TestSanitizeHeaders_RemovesSensitiveAndBrowserNoise(t *testing.T) {
	in := http.Header{}
	in.Set("Cookie", "a=b")
	in.Set("Referer", "http://example")
	in.Set("Sec-CH-UA", "chromium")
	in.Set("X-API-Key", "secret")
	in.Set("Authorization", "Bearer x")
	in.Set("Content-Type", "application/json")
	out := sanitizeHeaders(in)
	if out.Get("Cookie") != "" || out.Get("Referer") != "" || out.Get("X-API-Key") != "" || out.Get("Authorization") != "" {
		t.Fatalf("sensitive headers should be stripped: %+v", out)
	}
	if out.Get("Content-Type") != "application/json" {
		t.Fatalf("content-type should be preserved")
	}
}

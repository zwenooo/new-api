package service

import "testing"

func TestNormalizeClawBoxInstalledSemver(t *testing.T) {
	cases := map[string]string{
		"v2026.04.06.01": "2026.406.1",
		"2026.04.06.02":  "2026.406.2",
		"2026.406.2":     "2026.406.2",
		"v0.1.0":         "0.1.0",
		"v2026.13.06.01": "",
	}

	for input, want := range cases {
		if got := NormalizeClawBoxInstalledSemver(input); got != want {
			t.Fatalf("NormalizeClawBoxInstalledSemver(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeClawBoxInstalledTagVersion(t *testing.T) {
	cases := map[string]string{
		"2026.406.1":     "2026.04.06.01",
		"v2026.04.06.02": "2026.04.06.02",
		"0.1.0":          "0.1.0",
		"2026.1306.1":    "2026.1306.1",
	}

	for input, want := range cases {
		if got := NormalizeClawBoxInstalledTagVersion(input); got != want {
			t.Fatalf("NormalizeClawBoxInstalledTagVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildClawBoxInstalledProxyURLUsesSemverPath(t *testing.T) {
	got := buildClawBoxInstalledProxyURL("https://clawbox.example.com", "2026.04.06.02", "ClawBox_2026.406.2_x64-setup.exe")
	want := "https://clawbox.example.com/api/clawbox/update/desktop/releases/2026.406.2/download?asset=ClawBox_2026.406.2_x64-setup.exe"
	if got != want {
		t.Fatalf("buildClawBoxInstalledProxyURL() = %q, want %q", got, want)
	}
}

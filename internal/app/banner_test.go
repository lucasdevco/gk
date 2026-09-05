package app

import (
	"strings"
	"testing"

	"gk/internal/platform/config"
	"gk/internal/platform/version"
)

func TestStartupBanner(t *testing.T) {
	cfg := config.Config{Banner: true, LogFormat: "text", Environment: "test", Addr: ":9090"}
	plain := startupBanner(cfg)
	for _, want := range []string{"GK", version.Version, "Starting", "test", "Listen: http://localhost:9090", "Docs:   http://localhost:9090/api/docs", "Spec:   http://localhost:9090/api/openapi.yaml"} {
		if !strings.Contains(plain, want) {
			t.Errorf("banner missing %q", want)
		}
	}
	if strings.Contains(plain, "\x1b") {
		t.Fatal("plain banner contains ANSI colors")
	}
	cfg.LogColor = true
	colored := startupBanner(cfg)
	if !strings.Contains(colored, "\x1b[36m") || !strings.Contains(colored, "\x1b[0m") {
		t.Fatal("colored banner missing color or reset")
	}
	cfg.LogFormat = "json"
	if startupBanner(cfg) != "" {
		t.Fatal("JSON logging must suppress banner")
	}
	cfg.LogFormat = "text"
	cfg.Banner = false
	if startupBanner(cfg) != "" {
		t.Fatal("disabled banner must be empty")
	}
}

func TestBannerURL(t *testing.T) {
	for _, tc := range []struct{ addr, want string }{
		{":8080", "http://localhost:8080"},
		{"0.0.0.0:8080", "http://localhost:8080"},
		{"[::]:8080", "http://localhost:8080"},
		{"127.0.0.1:9090", "http://127.0.0.1:9090"},
		{"localhost:8080", "http://localhost:8080"},
		{"192.168.1.10:8080", "http://192.168.1.10:8080"},
		{"[::1]:8080", "http://[::1]:8080"},
		{"", "http://localhost:80"},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			if got := bannerURL(tc.addr); got != tc.want {
				t.Fatalf("bannerURL(%q) = %q, want %q", tc.addr, got, tc.want)
			}
		})
	}
}

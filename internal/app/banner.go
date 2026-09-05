package app

import (
	"fmt"
	"net"
	"net/url"

	"gk/internal/platform/config"
	"gk/internal/platform/version"
)

// startupBanner announces startup, not readiness. JSON output stays log-only.
func startupBanner(cfg config.Config) string {
	if !cfg.Banner || cfg.LogFormat == "json" {
		return ""
	}
	logo := "  ██████╗ ██╗  ██╗\n  ██╔════╝ ██║ ██╔╝\n  ██║  ███╗█████╔╝ \n  ██║   ██║██╔═██╗ \n  ╚██████╔╝██║  ██╗\n   ╚═════╝ ╚═╝  ╚═╝"
	if cfg.LogColor {
		logo = "\x1b[36m" + logo + "\x1b[0m"
	}
	baseURL := bannerURL(cfg.Addr)
	return fmt.Sprintf("\n%s\n\n  GK · Go + React starter\n  Starting %s (%s)\n  Listen: %s\n  Docs:   %s/api/docs\n  Spec:   %s/api/openapi.yaml\n\n", logo, version.Version, cfg.Environment, baseURL, baseURL, baseURL)
}

func bannerURL(addr string) string {
	if addr == "" {
		addr = ":80"
	}
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		if host == "" || net.ParseIP(host).IsUnspecified() {
			host = "localhost"
		}
		addr = net.JoinHostPort(host, port)
	}
	return (&url.URL{Scheme: "http", Host: addr}).String()
}

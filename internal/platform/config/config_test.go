package config

import "testing"

func TestLogColor(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
	for _, value := range []string{"", "false", "true", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("LOG_COLOR", value)
			cfg, err := Load()
			if value == "invalid" {
				if err == nil {
					t.Fatal("expected invalid boolean error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LogColor != (value != "false") {
				t.Fatalf("LogColor = %v for %q", cfg.LogColor, value)
			}
		})
	}
}

// Package config owns process configuration and its validation.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Environment     string        `env:"APP_ENV" envDefault:"development"`
	Banner          bool          `env:"APP_BANNER" envDefault:"true"`
	Addr            string        `env:"APP_ADDR" envDefault:":8080"`
	PublicURL       string        `env:"APP_PUBLIC_URL" envDefault:"http://localhost:8080"`
	DatabaseURL     string        `env:"DATABASE_URL" envDefault:"postgres://gk:gk@localhost:5432/gk?sslmode=disable"`
	LogLevel        string        `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat       string        `env:"LOG_FORMAT" envDefault:"text"`
	LogColor        bool          `env:"LOG_COLOR" envDefault:"true"`
	ServiceName     string        `env:"OTEL_SERVICE_NAME" envDefault:"gk"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		return Config{}, fmt.Errorf("LOG_FORMAT must be text or json")
	}
	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive")
	}
	return cfg, nil
}

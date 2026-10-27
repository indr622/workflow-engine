// Package config memuat konfigurasi engine dari environment variable.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config adalah seluruh konfigurasi runtime engine.
type Config struct {
	HTTPAddr        string        `env:"ENGINE_HTTP_ADDR" envDefault:":8080"`
	DatabaseURL     string        `env:"DATABASE_URL" envDefault:"postgres://workflow:workflow@localhost:5433/workflow?sslmode=disable"`
	Migrate         bool          `env:"ENGINE_MIGRATE" envDefault:"true"`
	LogLevel        string        `env:"ENGINE_LOG_LEVEL" envDefault:"info"`
	ShutdownTimeout time.Duration `env:"ENGINE_SHUTDOWN_TIMEOUT" envDefault:"15s"`
	CORSOrigins     []string      `env:"ENGINE_CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:4200"`
}

// Load membaca konfigurasi dari environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

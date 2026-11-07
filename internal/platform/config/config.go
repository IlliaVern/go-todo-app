// Package config loads application settings from environment variables.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
)

// Config is read from the environment; defaults suit `make run` against the
// Compose database on localhost:5436.
type Config struct {
	Server
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info"`
	SigningKey string `env:"AUTH_SIGNING_KEY,notEmpty"`
	// AuthRequestsPerMinute limits sign-up and sign-in requests per client IP.
	AuthRequestsPerMinute int `env:"AUTH_RATE_LIMIT_PER_MINUTE" envDefault:"20"`
	DB                    DB  `envPrefix:"DB_"`
}

// Server holds the settings shared with the healthcheck subcommand.
type Server struct {
	Port string `env:"PORT" envDefault:"8000"`
}

// LoadServer parses only the Server settings, which need no secrets.
func LoadServer() (Server, error) {
	s, err := env.ParseAs[Server]()
	if err != nil {
		return Server{}, fmt.Errorf("load config: %w", err)
	}
	return s, nil
}

type DB struct {
	Host     string `env:"HOST" envDefault:"localhost"`
	Port     string `env:"PORT" envDefault:"5436"`
	Username string `env:"USERNAME" envDefault:"postgres"`
	Password string `env:"PASSWORD,notEmpty"`
	Name     string `env:"DBNAME" envDefault:"postgres"`
	SSLMode  string `env:"SSLMODE" envDefault:"disable"`
}

func (d DB) Postgres() postgres.Config {
	return postgres.Config{
		Host:     d.Host,
		Port:     d.Port,
		Username: d.Username,
		Password: d.Password,
		DBName:   d.Name,
		SSLMode:  d.SSLMode,
	}
}

// Load reports all missing or invalid variables at once.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	if cfg.AuthRequestsPerMinute < 1 {
		return Config{}, fmt.Errorf("load config: AUTH_RATE_LIMIT_PER_MINUTE must be at least 1, got %d", cfg.AuthRequestsPerMinute)
	}
	return cfg, nil
}

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
	Port       string `env:"PORT" envDefault:"8000"`
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info"`
	SigningKey string `env:"AUTH_SIGNING_KEY,notEmpty"`
	DB         DB     `envPrefix:"DB_"`
}

// DB holds the DB_* connection settings.
type DB struct {
	Host     string `env:"HOST" envDefault:"localhost"`
	Port     string `env:"PORT" envDefault:"5436"`
	Username string `env:"USERNAME" envDefault:"postgres"`
	Password string `env:"PASSWORD,notEmpty"`
	Name     string `env:"DBNAME" envDefault:"postgres"`
	SSLMode  string `env:"SSLMODE" envDefault:"disable"`
}

// Postgres converts the settings to the connection config.
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

// Load parses the environment. Missing required variables are reported together.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

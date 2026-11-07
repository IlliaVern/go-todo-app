// Package config loads application settings from configs/config.yml, an
// optional .env file and the environment.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
)

type Config struct {
	Port       string
	DB         postgres.Config
	SigningKey string
	LogLevel   string
}

// Load reads configs/config.yml relative to the working directory. Every key
// can be overridden by an environment variable named after it in upper case
// with dots replaced by underscores (db.host -> DB_HOST). A .env file in the
// working directory is loaded first when present; in containers configuration
// comes from the environment alone.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	v := viper.New()
	v.AddConfigPath("configs")
	v.SetConfigName("config")
	v.SetDefault("log.level", "info")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	cfg := Config{
		Port: v.GetString("port"),
		DB: postgres.Config{
			Host:     v.GetString("db.host"),
			Port:     v.GetString("db.port"),
			Username: v.GetString("db.username"),
			Password: v.GetString("db.password"),
			DBName:   v.GetString("db.dbname"),
			SSLMode:  v.GetString("db.sslmode"),
		},
		SigningKey: v.GetString("auth.signing_key"),
		LogLevel:   v.GetString("log.level"),
	}

	if cfg.DB.Password == "" {
		return Config{}, errors.New("DB_PASSWORD must be set")
	}
	if cfg.SigningKey == "" {
		return Config{}, errors.New("AUTH_SIGNING_KEY must be set")
	}

	return cfg, nil
}

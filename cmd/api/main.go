package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/IlliaVern/go-todo-app/internal/api"
	"github.com/IlliaVern/go-todo-app/internal/platform/config"
	"github.com/IlliaVern/go-todo-app/internal/platform/httpserver"
	"github.com/IlliaVern/go-todo-app/internal/platform/logging"
	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
)

// shutdownTimeout must stay below stop_grace_period in docker/compose.yaml.
const shutdownTimeout = 10 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("todo app stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	// gin's debug mode prints plain-text lines to stdout; keep stdout JSON
	// unless debug mode is requested explicitly.
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.DB.Postgres())
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("closing database connection failed", "error", err)
		}
	}()

	handler, err := api.NewHandler(db, cfg.SigningKey, logger)
	if err != nil {
		return fmt.Errorf("build api: %w", err)
	}

	srv := httpserver.New(cfg.Port, handler)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Run()
	}()

	slog.Info("todo app started", "port", cfg.Port)

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	// Restore default signal handling so a second Ctrl+C exits immediately.
	stop()
	slog.Info("todo app shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}

	return nil
}

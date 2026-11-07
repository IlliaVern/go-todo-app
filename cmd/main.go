package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/IlliaVern/go-todo-app"
	"github.com/IlliaVern/go-todo-app/pkg/handler"
	"github.com/IlliaVern/go-todo-app/pkg/repository"
	"github.com/IlliaVern/go-todo-app/pkg/service"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

const shutdownTimeout = 10 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("todo app stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// .env is optional: in containers configuration comes from the environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	if err := initConfig(); err != nil {
		return fmt.Errorf("init config: %w", err)
	}

	signingKey := viper.GetString("auth.signing_key")
	if len(signingKey) < service.MinSigningKeyLength {
		return fmt.Errorf("AUTH_SIGNING_KEY must be set and at least %d bytes long", service.MinSigningKeyLength)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := repository.NewPostgresDB(ctx, repository.Config{
		Host:     viper.GetString("db.host"),
		Port:     viper.GetString("db.port"),
		Username: viper.GetString("db.username"),
		DBName:   viper.GetString("db.dbname"),
		SSLMode:  viper.GetString("db.sslmode"),
		Password: viper.GetString("db.password"),
	})
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("closing database connection failed", "error", err)
		}
	}()

	repos := repository.NewRepository(db)
	services := service.NewService(repos, signingKey)
	handlers := handler.NewHandler(services)

	srv := todo.NewServer(viper.GetString("port"), handlers.InitRoutes())

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Run()
	}()

	slog.Info("todo app started", "port", viper.GetString("port"))

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

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

// initConfig reads configs/config.yml. Every key can be overridden by an
// environment variable named after it with dots replaced by underscores,
// e.g. db.host -> DB_HOST.
func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	return viper.ReadInConfig()
}

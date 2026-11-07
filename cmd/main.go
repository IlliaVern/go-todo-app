package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/IlliaVern/go-todo-app"
	"github.com/IlliaVern/go-todo-app/pkg/handler"
	"github.com/IlliaVern/go-todo-app/pkg/repository"
	"github.com/IlliaVern/go-todo-app/pkg/service"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := initConfig(); err != nil {
		fatal("failed to initialize config", err)
	}

	if err := godotenv.Load(); err != nil {
		fatal("failed to load env variables", err)
	}

	db, err := repository.NewPostgresDB(repository.Config{
		Host:     viper.GetString("db.host"),
		Port:     viper.GetString("db.port"),
		Username: viper.GetString("db.username"),
		DBName:   viper.GetString("db.dbname"),
		SSLMode:  viper.GetString("db.sslmode"),
		Password: os.Getenv("DB_PASSWORD"),
	})
	if err != nil {
		fatal("failed to connect to database", err)
	}

	repos := repository.NewRepository(db)
	services := service.NewService(repos)
	handlers := handler.NewHandler(services)

	srv := new(todo.Server)
	go func() {
		if err := srv.Run(viper.GetString("port"), handlers.InitRoutes()); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("http server failed", err)
		}
	}()

	slog.Info("todo app started", "port", viper.GetString("port"))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	slog.Info("todo app shutting down")

	if err := srv.Shutdown(context.Background()); err != nil {
		slog.Error("http server shutdown failed", "error", err)
	}
	if err := db.Close(); err != nil {
		slog.Error("closing database connection failed", "error", err)
	}
}

func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	return viper.ReadInConfig()
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

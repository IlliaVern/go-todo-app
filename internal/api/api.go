// Package api is the composition root: it wires the bounded contexts'
// repositories, services and HTTP adapters into one handler.
package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	identityapp "github.com/IlliaVern/go-todo-app/internal/identity/app"
	identityhttp "github.com/IlliaVern/go-todo-app/internal/identity/httpapi"
	"github.com/IlliaVern/go-todo-app/internal/identity/password"
	identitypg "github.com/IlliaVern/go-todo-app/internal/identity/postgres"
	"github.com/IlliaVern/go-todo-app/internal/identity/token"
	"github.com/IlliaVern/go-todo-app/internal/platform/health"
	"github.com/IlliaVern/go-todo-app/internal/platform/httpx"
	todoapp "github.com/IlliaVern/go-todo-app/internal/todo/app"
	todohttp "github.com/IlliaVern/go-todo-app/internal/todo/httpapi"
	todopg "github.com/IlliaVern/go-todo-app/internal/todo/postgres"
)

const tokenTTL = 12 * time.Hour

type Options struct {
	SigningKey string
	// AuthRequestsPerMinute limits credential and username-changing requests per client.
	AuthRequestsPerMinute int
	Logger                *slog.Logger
}

func NewHandler(db *pgxpool.Pool, opts Options) (http.Handler, error) {
	tokens, err := token.NewJWT(opts.SigningKey, tokenTTL)
	if err != nil {
		return nil, err
	}

	identitySvc, err := identityapp.NewService(identitypg.NewUserRepository(db), password.NewArgon2id(), tokens)
	if err != nil {
		return nil, fmt.Errorf("identity service: %w", err)
	}
	todoSvc := todoapp.NewService(todopg.NewListRepository(db), todopg.NewItemRepository(db))

	mux := http.NewServeMux()
	health.Register(mux, map[string]health.Pinger{"database": db})

	identity := identityhttp.NewHandler(identitySvc)
	identity.Register(mux, httpx.NewRateLimiter(opts.AuthRequestsPerMinute).Middleware)
	todohttp.NewHandler(todoSvc).Register(mux, identity.Authenticate)

	handler := httpx.Recovery(httpx.JSONErrors(mux))
	return httpx.RequestLogger(opts.Logger, handler, health.LivenessPath, health.ReadinessPath), nil
}

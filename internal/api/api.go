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
	"github.com/IlliaVern/go-todo-app/internal/platform/httpx"
	todoapp "github.com/IlliaVern/go-todo-app/internal/todo/app"
	todohttp "github.com/IlliaVern/go-todo-app/internal/todo/httpapi"
	todopg "github.com/IlliaVern/go-todo-app/internal/todo/postgres"
)

const tokenTTL = 12 * time.Hour

// NewHandler returns the HTTP handler serving the whole API.
func NewHandler(db *pgxpool.Pool, signingKey string, logger *slog.Logger) (http.Handler, error) {
	tokens, err := token.NewJWT(signingKey, tokenTTL)
	if err != nil {
		return nil, err
	}

	identitySvc, err := identityapp.NewService(identitypg.NewUserRepository(db), password.NewBcrypt(), tokens)
	if err != nil {
		return nil, fmt.Errorf("identity service: %w", err)
	}
	todoSvc := todoapp.NewService(todopg.NewListRepository(db), todopg.NewItemRepository(db))

	mux := http.NewServeMux()
	mux.HandleFunc("/", httpx.NotFound)

	identity := identityhttp.NewHandler(identitySvc)
	identity.Register(mux)
	todohttp.NewHandler(todoSvc).Register(mux, identity.Authenticate)

	return httpx.RequestLogger(logger, httpx.Recovery(mux)), nil
}

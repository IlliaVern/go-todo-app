// Package api is the composition root: it wires the bounded contexts'
// repositories, services and HTTP adapters into one router.
package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

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
func NewHandler(db *sqlx.DB, signingKey string, logger *slog.Logger) (http.Handler, error) {
	tokens, err := token.NewJWT(signingKey, tokenTTL)
	if err != nil {
		return nil, err
	}

	identitySvc, err := identityapp.NewService(identitypg.NewUserRepository(db), password.NewBcrypt(), tokens)
	if err != nil {
		return nil, fmt.Errorf("identity service: %w", err)
	}
	todoSvc := todoapp.NewService(todopg.NewListRepository(db), todopg.NewItemRepository(db))

	router := gin.New()
	// client_ip is the direct peer; no proxy headers are trusted.
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	router.Use(httpx.RequestLogger(logger), httpx.Recovery())
	router.NoRoute(func(c *gin.Context) {
		httpx.Error(c, http.StatusNotFound, "route not found")
	})

	identity := identityhttp.NewHandler(identitySvc)
	identity.Register(router)

	protected := router.Group("/api", identity.Authenticate)
	todohttp.NewHandler(todoSvc).Register(protected)

	return router, nil
}

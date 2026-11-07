// Package httpapi exposes the identity use cases over HTTP with gin and
// provides the authentication middleware other contexts mount behind.
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/IlliaVern/go-todo-app/internal/identity/app"
	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
	"github.com/IlliaVern/go-todo-app/internal/platform/httpx"
)

type Handler struct {
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(r gin.IRouter) {
	auth := r.Group("/auth")
	{
		auth.POST("/sign-up", h.signUp)
		auth.POST("/sign-in", h.signIn)
	}
}

type signUpRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) signUp(c *gin.Context) {
	var req signUpRequest
	if !httpx.BindJSON(c, &req) {
		return
	}

	id, err := h.svc.SignUp(c.Request.Context(), req.Name, req.Username, req.Password)
	var invalid *domain.ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUsernameTaken):
		httpx.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		httpx.InternalError(c, err)
	default:
		c.JSON(http.StatusOK, httpx.IDResponse{ID: id})
	}
}

type signInRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type signInResponse struct {
	Token string `json:"token"`
}

func (h *Handler) signIn(c *gin.Context) {
	var req signInRequest
	if !httpx.BindJSON(c, &req) {
		return
	}

	token, err := h.svc.SignIn(c.Request.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, app.ErrInvalidCredentials):
		httpx.Error(c, http.StatusUnauthorized, err.Error())
	case err != nil:
		httpx.InternalError(c, err)
	default:
		c.JSON(http.StatusOK, signInResponse{Token: token})
	}
}

// Authenticate is middleware requiring "Authorization: Bearer <token>". It
// stores the caller's user id for httpx.UserID.
func (h *Handler) Authenticate(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if header == "" {
		httpx.Error(c, http.StatusUnauthorized, "empty authorization header")
		return
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		httpx.Error(c, http.StatusUnauthorized, "invalid authorization header")
		return
	}

	userID, err := h.svc.Authenticate(token)
	if err != nil {
		httpx.ErrorWithReason(c, http.StatusUnauthorized, app.ErrInvalidToken.Error(), err.Error())
		return
	}

	httpx.SetUserID(c, userID)
	c.Next()
}

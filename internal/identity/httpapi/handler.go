// Package httpapi exposes the identity use cases over net/http and provides
// the authentication middleware other contexts mount behind.
package httpapi

import (
	"errors"
	"net/http"
	"strings"

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

// Register mounts the auth routes on mux behind limit, which throttles
// credential guessing and sign-up floods.
func (h *Handler) Register(mux *http.ServeMux, limit func(http.Handler) http.Handler) {
	mux.Handle("POST /auth/sign-up", limit(http.HandlerFunc(h.signUp)))
	mux.Handle("POST /auth/sign-in", limit(http.HandlerFunc(h.signIn)))
}

type signUpRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) signUp(w http.ResponseWriter, r *http.Request) {
	var req signUpRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	id, err := h.svc.SignUp(r.Context(), req.Name, req.Username, req.Password)
	var invalid *domain.ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.Error(w, r, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUsernameTaken):
		httpx.Error(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, app.ErrHasherBusy):
		busy(w, r)
	case err != nil:
		httpx.InternalError(w, r, err)
	default:
		httpx.JSON(w, r, http.StatusOK, httpx.IDResponse{ID: id})
	}
}

type signInRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type signInResponse struct {
	Token string `json:"token"`
}

func (h *Handler) signIn(w http.ResponseWriter, r *http.Request) {
	var req signInRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	token, err := h.svc.SignIn(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, app.ErrInvalidCredentials):
		httpx.Error(w, r, http.StatusUnauthorized, err.Error())
	case errors.Is(err, app.ErrHasherBusy):
		busy(w, r)
	case err != nil:
		httpx.InternalError(w, r, err)
	default:
		httpx.JSON(w, r, http.StatusOK, signInResponse{Token: token})
	}
}

// busy sheds load while password hashing is saturated.
func busy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "2")
	httpx.Error(w, r, http.StatusServiceUnavailable, app.ErrHasherBusy.Error())
}

// Authenticate is middleware requiring "Authorization: Bearer <token>". It
// stores the caller's user id for httpx.UserID.
func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			httpx.Error(w, r, http.StatusUnauthorized, "empty authorization header")
			return
		}

		scheme, token, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
			httpx.Error(w, r, http.StatusUnauthorized, "invalid authorization header")
			return
		}

		userID, err := h.svc.Authenticate(token)
		if err != nil {
			httpx.ErrorWithReason(w, r, http.StatusUnauthorized, app.ErrInvalidToken.Error(), err.Error())
			return
		}

		next.ServeHTTP(w, r.WithContext(httpx.WithUserID(r.Context(), userID)))
	})
}

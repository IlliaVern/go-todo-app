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

const accountPath = "/api/users/me"

type Handler struct {
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the auth and account routes on mux. limit guards every route
// that verifies a password or reveals whether a username is taken, against
// credential guessing, username enumeration and hashing floods.
func (h *Handler) Register(mux *http.ServeMux, limit func(http.Handler) http.Handler) {
	httpx.Handle(mux, http.MethodPost, "/auth/sign-up", limit(http.HandlerFunc(h.signUp)))
	httpx.Handle(mux, http.MethodPost, "/auth/sign-in", limit(http.HandlerFunc(h.signIn)))

	httpx.Handle(mux, http.MethodGet, accountPath, h.Authenticate(http.HandlerFunc(h.getProfile)))
	httpx.Handle(mux, http.MethodPatch, accountPath, limit(h.Authenticate(http.HandlerFunc(h.updateProfile))))
	httpx.Handle(mux, http.MethodDelete, accountPath, h.Authenticate(http.HandlerFunc(h.deleteAccount)))
	httpx.Handle(mux, http.MethodPost, accountPath+"/password", limit(h.Authenticate(http.HandlerFunc(h.changePassword))))
}

type signUpRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) signUp(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.DecodeJSON[signUpRequest](w, r)
	if !ok {
		return
	}

	id, err := h.svc.SignUp(r.Context(), app.SignUpInput{Name: req.Name, Username: req.Username, Password: req.Password})
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, httpx.IDResponse{ID: int64(id)})
}

type signInRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type signInResponse struct {
	Token string `json:"token"`
}

func (h *Handler) signIn(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.DecodeJSON[signInRequest](w, r)
	if !ok {
		return
	}

	token, err := h.svc.SignIn(r.Context(), app.SignInInput{Username: req.Username, Password: req.Password})
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, signInResponse{Token: string(token)})
}

// caller returns the authenticated user.
func caller(w http.ResponseWriter, r *http.Request) (domain.UserID, bool) {
	id, ok := httpx.CallerID(w, r)
	return domain.UserID(id), ok
}

type profileResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}

	user, err := h.svc.Profile(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, profileResponse{
		ID:       int64(user.ID()),
		Name:     user.Name().String(),
		Username: user.Username().String(),
	})
}

type updateProfileRequest struct {
	Name     *string `json:"name"`
	Username *string `json:"username"`
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}
	req, ok := httpx.DecodeJSON[updateProfileRequest](w, r)
	if !ok {
		return
	}

	patch := domain.ProfilePatch{Name: req.Name, Username: req.Username}
	if err := h.svc.UpdateProfile(r.Context(), id, patch); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}
	req, ok := httpx.DecodeJSON[changePasswordRequest](w, r)
	if !ok {
		return
	}

	in := app.ChangePasswordInput{Current: req.CurrentPassword, New: req.NewPassword}
	if err := h.svc.ChangePassword(r.Context(), id, in); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := caller(w, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteAccount(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

// writeError maps identity errors to HTTP statuses and client-facing messages.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *domain.ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.Error(w, r, http.StatusBadRequest, err.Error())
	case errors.Is(err, app.ErrInvalidCredentials):
		httpx.Error(w, r, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		httpx.ErrorWithReason(w, r, http.StatusUnauthorized, "authenticated user no longer exists", err.Error())
	case errors.Is(err, app.ErrWrongPassword):
		httpx.Error(w, r, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrUsernameTaken):
		httpx.Error(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrConcurrentUpdate):
		httpx.ErrorWithReason(w, r, http.StatusConflict, "the resource was modified concurrently, retry the request", err.Error())
	case errors.Is(err, app.ErrHasherBusy):
		w.Header().Set("Retry-After", "2")
		httpx.Error(w, r, http.StatusServiceUnavailable, app.ErrHasherBusy.Error())
	default:
		httpx.InternalError(w, r, err)
	}
}

// Authenticate is middleware requiring "Authorization: Bearer <token>". It
// stores the caller's user id for httpx.CallerID.
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

		userID, err := h.svc.Authenticate(r.Context(), domain.AccessToken(token))
		if errors.Is(err, app.ErrInvalidToken) {
			httpx.ErrorWithReason(w, r, http.StatusUnauthorized, app.ErrInvalidToken.Error(), err.Error())
			return
		}
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}

		next.ServeHTTP(w, r.WithContext(httpx.WithCallerID(r.Context(), int64(userID))))
	})
}

// Package httpx holds net/http helpers shared by the bounded contexts' HTTP
// adapters: JSON envelopes, request decoding, path ids, the authenticated
// caller's user id, and request logging and recovery middleware.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
)

// MaxBodyBytes bounds request bodies.
const MaxBodyBytes = 1 << 20 // 1 MiB

type errorResponse struct {
	Message string `json:"message"`
}

type statusResponse struct {
	Status string `json:"status"`
}

// IDResponse is returned by endpoints that create a resource.
type IDResponse struct {
	ID int64 `json:"id"`
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.DebugContext(r.Context(), "writing response failed", "error", err)
	}
}

// OK writes the {"status": "ok"} envelope.
func OK(w http.ResponseWriter, r *http.Request) {
	JSON(w, r, http.StatusOK, statusResponse{Status: "ok"})
}

// Error answers with a client-visible message, which RequestLogger records as
// the rejection reason.
func Error(w http.ResponseWriter, r *http.Request, status int, message string) {
	ErrorWithReason(w, r, status, message, message)
}

// ErrorWithReason answers with a client-visible message while logging a more
// detailed reason, e.g. why a token was rejected.
func ErrorWithReason(w http.ResponseWriter, r *http.Request, status int, message, reason string) {
	if o := outcomeFrom(r.Context()); o != nil {
		o.reason = reason
	}
	JSON(w, r, status, errorResponse{Message: message})
}

// InternalError answers with a generic 500, keeping internals out of the
// response; RequestLogger records err.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	if o := outcomeFrom(r.Context()); o != nil {
		o.err = err.Error()
	}
	JSON(w, r, http.StatusInternalServerError, errorResponse{Message: "internal server error"})
}

// NotFound answers unmatched routes.
func NotFound(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusNotFound, "route not found")
}

// DecodeJSON decodes the body into dst, answering 400 on malformed JSON and
// 413 on bodies over MaxBodyBytes.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			Error(w, r, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		Error(w, r, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// PathID parses the positive integer path wildcard {id}, answering 400 otherwise.
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, r, http.StatusBadRequest, "invalid id param")
		return 0, false
	}
	return id, true
}

type userIDKey struct{}

// WithUserID returns a context carrying the authenticated caller's id.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserID returns the authenticated caller's id, answering 500 when the
// authentication middleware did not run.
func UserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if id, ok := r.Context().Value(userIDKey{}).(int64); ok {
		return id, true
	}
	InternalError(w, r, errors.New("user id missing from request context: authentication middleware not mounted"))
	return 0, false
}

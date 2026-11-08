// Package httpx holds net/http helpers shared by the bounded contexts' HTTP
// adapters: JSON envelopes, request decoding, path ids, the authenticated
// caller's id, and request logging and recovery middleware.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

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

// Handle registers h for method and path, with and without a trailing slash.
func Handle(mux *http.ServeMux, method, path string, h http.Handler) {
	mux.Handle(method+" "+path, h)
	mux.Handle(method+" "+path+"/{$}", h)
}

func JSON[T any](w http.ResponseWriter, r *http.Request, status int, v T) {
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

// StatusClientClosedRequest is the de facto status (from nginx) for requests
// the client abandoned; it only appears in logs.
const StatusClientClosedRequest = 499

// InternalError answers with a generic 500, keeping internals out of the
// response; RequestLogger records err. A request the client cancelled is
// logged as 499 instead, since nobody receives the answer.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		ErrorWithReason(w, r, StatusClientClosedRequest, "client closed request", err.Error())
		return
	}
	if o := outcomeFrom(r.Context()); o != nil {
		o.err = err.Error()
	}
	JSON(w, r, http.StatusInternalServerError, errorResponse{Message: "internal server error"})
}

// DecodeJSON decodes the body, which must be a single JSON value with only
// fields T declares. It answers 400 otherwise and 413 for bodies over
// MaxBodyBytes, returning false in both cases.
func DecodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	// The innermost writer lets net/http close the connection after an
	// oversized body.
	r.Body = http.MaxBytesReader(innermost(w), r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var dst T
	err := dec.Decode(&dst)
	if err == nil && !errors.Is(dec.Decode(&struct{}{}), io.EOF) {
		err = errors.New("unexpected data after the JSON value")
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		Error(w, r, http.StatusRequestEntityTooLarge, "request body too large")
	case err != nil:
		Error(w, r, http.StatusBadRequest, "invalid request body: "+err.Error())
	default:
		return dst, true
	}
	return dst, false
}

func innermost(w http.ResponseWriter) http.ResponseWriter {
	for {
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return w
		}
		w = u.Unwrap()
	}
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

type callerIDKey struct{}

func WithCallerID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, callerIDKey{}, id)
}

// CallerID returns the authenticated caller's id, answering 500 when the
// authentication middleware did not run.
func CallerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if id, ok := r.Context().Value(callerIDKey{}).(int64); ok {
		return id, true
	}
	InternalError(w, r, errors.New("caller id missing from request context: authentication middleware not mounted"))
	return 0, false
}

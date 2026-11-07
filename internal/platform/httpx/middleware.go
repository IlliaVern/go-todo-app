package httpx

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/IlliaVern/go-todo-app/internal/platform/logging"
)

// RequestIDHeader is read from requests and always set on responses.
const RequestIDHeader = "X-Request-ID"

const maxRequestIDLength = 128

// outcome collects what handlers report about a request for its log line.
type outcome struct {
	reason string
	err    string
}

type outcomeKey struct{}

func outcomeFrom(ctx context.Context) *outcome {
	o, _ := ctx.Value(outcomeKey{}).(*outcome)
	return o
}

// RequestLogger assigns each request an id (reusing a sane incoming
// X-Request-ID), stores it in the request context for logging, and writes
// exactly one access log line per request once it completes: ERROR for 5xx
// (with the error), WARN for 4xx (with the rejection reason), INFO otherwise.
// Successful requests to quietPaths, such as health probes, log at DEBUG.
// Handlers between it and the ServeMux must pass the request on unchanged so
// the matched pattern is visible.
func RequestLogger(logger *slog.Logger, next http.Handler, quietPaths ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		id := r.Header.Get(RequestIDHeader)
		if !validRequestID(id) {
			id = rand.Text()
		}
		w.Header().Set(RequestIDHeader, id)

		o := &outcome{}
		ctx := context.WithValue(logging.WithRequestID(r.Context(), id), outcomeKey{}, o)
		r = r.WithContext(ctx)
		rec := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		// ServeMux records the matched pattern ("GET /api/lists/{id}") on r;
		// trailing-slash variants ("/api/lists/{$}") log as their base route.
		_, route, _ := strings.Cut(r.Pattern, " ")
		if route == "" {
			route = r.Pattern
		}
		route = strings.TrimSuffix(route, "/{$}")
		status := rec.statusCode()
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Int("bytes", rec.bytes),
			slog.String("client_ip", clientIP(r)),
		}
		if route == "" { // no route matched: record what was asked for
			attrs = append(attrs, slog.String("path", r.URL.Path))
		}
		if o.reason != "" {
			attrs = append(attrs, slog.String("reason", o.reason))
		}
		if o.err != "" {
			attrs = append(attrs, slog.String("error", o.err))
		}

		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		case slices.Contains(quietPaths, route):
			level = slog.LevelDebug
		}
		logger.LogAttrs(ctx, level, "request", attrs...)
	})
}

// Recovery turns a handler panic into a 500 whose access log line carries the
// panic value and stack trace.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(recovered) // let net/http abort the connection as intended
			}
			InternalError(w, r, fmt.Errorf("panic: %v\n%s", recovered, debug.Stack()))
		}()
		next.ServeHTTP(w, r)
	})
}

// JSONErrors serves mux, answering requests that match no route with a JSON
// 404, or with a JSON 405 and an Allow header when only the method is wrong.
func JSONErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// ServeMux's own answer tells 404 from 405 and lists allowed methods.
		probe := &headerRecorder{header: http.Header{}}
		mux.ServeHTTP(probe, r)
		if probe.status == http.StatusMethodNotAllowed {
			w.Header().Set("Allow", probe.header.Get("Allow"))
			Error(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		Error(w, r, http.StatusNotFound, "route not found")
	})
}

// headerRecorder captures status and headers and discards the body.
type headerRecorder struct {
	header http.Header
	status int
}

func (h *headerRecorder) Header() http.Header         { return h.header }
func (h *headerRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (h *headerRecorder) WriteHeader(code int)        { h.status = code }

// clientIP is the direct peer; no proxy headers are trusted.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// validRequestID accepts short, printable ASCII ids so clients cannot inject
// arbitrary content into logs.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := range len(id) {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

// statusRecorder captures the status code and body size written by handlers.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *statusRecorder) statusCode() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}

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
// It must wrap the ServeMux directly so the matched pattern is visible.
func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
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

		// ServeMux records the matched pattern ("GET /api/lists/{id}") on r.
		_, route, _ := strings.Cut(r.Pattern, " ")
		if route == "" {
			route = r.Pattern
		}
		status := rec.statusCode()
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Int("bytes", rec.bytes),
			slog.String("client_ip", clientIP(r)),
		}
		if route == "" || route == "/" { // no API route matched: record what was asked for
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

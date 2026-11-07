package httpx

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/IlliaVern/go-todo-app/internal/platform/logging"
)

// RequestIDHeader is read from requests and always set on responses.
const RequestIDHeader = "X-Request-ID"

const maxRequestIDLength = 128

const (
	reasonKey = "httpx.reason"
	errorKey  = "httpx.error"
)

// RequestLogger assigns each request an id (reusing a sane incoming
// X-Request-ID), stores it in the request context for logging, and writes
// exactly one access log line per request once it completes: ERROR for 5xx
// (with the error), WARN for 4xx (with the rejection reason), INFO otherwise.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		id := c.GetHeader(RequestIDHeader)
		if !validRequestID(id) {
			id = rand.Text()
		}
		c.Header(RequestIDHeader, id)
		ctx := logging.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		status := c.Writer.Status()
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("route", c.FullPath()),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Int("bytes", max(c.Writer.Size(), 0)),
			slog.String("client_ip", c.ClientIP()),
		}
		if c.FullPath() == "" { // no route matched: record what was asked for
			attrs = append(attrs, slog.String("path", c.Request.URL.Path))
		}
		if reason := c.GetString(reasonKey); reason != "" {
			attrs = append(attrs, slog.String("reason", reason))
		}
		if err, ok := c.Get(errorKey); ok {
			attrs = append(attrs, slog.Any("error", err))
		}

		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}
		logger.LogAttrs(ctx, level, "request", attrs...)
	}
}

// Recovery turns a handler panic into a 500 whose access log line carries the
// panic value and stack trace.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		InternalError(c, fmt.Errorf("panic: %v\n%s", recovered, debug.Stack()))
	})
}

// validRequestID accepts short, printable ASCII ids so clients cannot inject
// arbitrary content into logs.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

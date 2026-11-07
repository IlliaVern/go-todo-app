package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/IlliaVern/go-todo-app/internal/platform/httpx"
	"github.com/IlliaVern/go-todo-app/internal/platform/logging"
)

func newRouter(t *testing.T) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger, err := logging.New(&buf, "debug")
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.Use(httpx.RequestLogger(logger), httpx.Recovery())
	r.GET("/ok", func(c *gin.Context) {
		slog.New(logger.Handler()).InfoContext(c.Request.Context(), "inside handler")
		httpx.OK(c)
	})
	r.GET("/rejected", func(c *gin.Context) { httpx.Error(c, http.StatusNotFound, "thing not found") })
	r.GET("/token", func(c *gin.Context) {
		httpx.ErrorWithReason(c, http.StatusUnauthorized, "invalid token", "token is expired")
	})
	r.GET("/failed", func(c *gin.Context) { httpx.InternalError(c, errors.New("db is down")) })
	r.GET("/panic", func(*gin.Context) { panic("boom") })
	return r, &buf
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", raw)
		}
		lines = append(lines, m)
	}
	return lines
}

func serve(r http.Handler, path, requestID string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	if requestID != "" {
		req.Header.Set(httpx.RequestIDHeader, requestID)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRequestLoggerWritesOneLinePerRequest(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus int
		wantLevel  string
		wantField  string
		wantValue  string
	}{
		{path: "/rejected", wantStatus: 404, wantLevel: "WARN", wantField: "reason", wantValue: "thing not found"},
		{path: "/token", wantStatus: 401, wantLevel: "WARN", wantField: "reason", wantValue: "token is expired"},
		{path: "/failed", wantStatus: 500, wantLevel: "ERROR", wantField: "error", wantValue: "db is down"},
		{path: "/missing", wantStatus: 404, wantLevel: "WARN", wantField: "path", wantValue: "/missing"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			r, buf := newRouter(t)
			rec := serve(r, tt.path, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			lines := logLines(t, buf)
			if len(lines) != 1 {
				t.Fatalf("got %d log lines, want 1: %v", len(lines), lines)
			}
			line := lines[0]
			if line["msg"] != "request" || line["level"] != tt.wantLevel || line["status"] != float64(tt.wantStatus) {
				t.Errorf("line = %v", line)
			}
			if line[tt.wantField] != tt.wantValue {
				t.Errorf("%s = %v, want %q", tt.wantField, line[tt.wantField], tt.wantValue)
			}
			if id := rec.Header().Get(httpx.RequestIDHeader); id == "" || line["request_id"] != id {
				t.Errorf("request_id = %v, response header = %q", line["request_id"], id)
			}
		})
	}
}

func TestRequestLoggerPropagatesRequestID(t *testing.T) {
	r, buf := newRouter(t)
	rec := serve(r, "/ok", "client-abc-123")
	if got := rec.Header().Get(httpx.RequestIDHeader); got != "client-abc-123" {
		t.Errorf("response request id = %q", got)
	}

	lines := logLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2", len(lines))
	}
	for _, line := range lines {
		if line["request_id"] != "client-abc-123" {
			t.Errorf("line %v lacks the request id", line)
		}
	}
	access := lines[1]
	if access["level"] != "INFO" || access["route"] != "/ok" || access["method"] != "GET" {
		t.Errorf("access line = %v", access)
	}
	for _, field := range []string{"duration_ms", "bytes", "client_ip"} {
		if _, ok := access[field]; !ok {
			t.Errorf("access line lacks %s", field)
		}
	}
}

func TestRequestLoggerReplacesUnsafeRequestID(t *testing.T) {
	for _, id := range []string{"has space", "new\nline", strings.Repeat("a", 129)} {
		r, _ := newRouter(t)
		if got := serve(r, "/ok", id).Header().Get(httpx.RequestIDHeader); got == id || got == "" {
			t.Errorf("incoming id %q: response id = %q, want a generated one", id, got)
		}
	}
}

func TestRecoveryLogsPanicAs500(t *testing.T) {
	r, buf := newRouter(t)
	rec := serve(r, "/panic", "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	lines := logLines(t, buf)
	if len(lines) != 1 || lines[0]["level"] != "ERROR" || !strings.HasPrefix(lines[0]["error"].(string), "panic: boom") {
		t.Errorf("lines = %v", lines)
	}
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	if _, err := logging.New(&bytes.Buffer{}, "loud"); err == nil {
		t.Error("expected error for unknown level")
	}
}

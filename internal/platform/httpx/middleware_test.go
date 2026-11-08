package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/platform/httpx"
	"github.com/IlliaVern/go-todo-app/internal/platform/logging"
)

func newHandler(t *testing.T) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger, err := logging.New(&buf, "debug")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { httpx.OK(w, r) })
	httpx.Handle(mux, http.MethodGet, "/things", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { httpx.OK(w, r) }))
	mux.HandleFunc("GET /cancelled", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		cancel()
		httpx.InternalError(w, r.WithContext(ctx), fmt.Errorf("hash password: %w", ctx.Err()))
	})
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		slog.New(logger.Handler()).InfoContext(r.Context(), "inside handler")
		httpx.OK(w, r)
	})
	mux.HandleFunc("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, http.StatusNotFound, "thing not found")
	})
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		httpx.ErrorWithReason(w, r, http.StatusUnauthorized, "invalid token", "token is expired")
	})
	mux.HandleFunc("GET /failed", func(w http.ResponseWriter, r *http.Request) {
		httpx.InternalError(w, r, errors.New("db is down"))
	})
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := httpx.DecodeJSON[echoBody](w, r); ok {
			httpx.OK(w, r)
		}
	})
	return httpx.RequestLogger(logger, httpx.Recovery(httpx.JSONErrors(mux)), "/healthz"), &buf
}

type echoBody struct {
	A string `json:"a"`
}

type logLine struct {
	Msg        string   `json:"msg"`
	Level      string   `json:"level"`
	Method     string   `json:"method"`
	Route      string   `json:"route"`
	Path       string   `json:"path"`
	Status     int      `json:"status"`
	Reason     string   `json:"reason"`
	Error      string   `json:"error"`
	RequestID  string   `json:"request_id"`
	Bytes      int      `json:"bytes"`
	ClientIP   string   `json:"client_ip"`
	DurationMS *float64 `json:"duration_ms"`
}

// outcome keeps the fields a table test pins down, dropping per-request noise.
func (l logLine) outcome() logLine {
	l.RequestID, l.Bytes, l.ClientIP, l.DurationMS = "", 0, "", nil
	return l
}

func logLines(t *testing.T, buf *bytes.Buffer) []logLine {
	t.Helper()
	var lines []logLine
	for raw := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var l logLine
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("log line is not JSON: %q", raw)
		}
		lines = append(lines, l)
	}
	return lines
}

func serve(h http.Handler, method, path, requestID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if requestID != "" {
		req.Header.Set(httpx.RequestIDHeader, requestID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequestLoggerWritesOneLinePerRequest(t *testing.T) {
	tests := []struct {
		path string
		want logLine
	}{
		{path: "/things/7", want: logLine{Status: 404, Level: "WARN", Route: "/things/{id}", Reason: "thing not found"}},
		{path: "/token", want: logLine{Status: 401, Level: "WARN", Route: "/token", Reason: "token is expired"}},
		{path: "/failed", want: logLine{Status: 500, Level: "ERROR", Route: "/failed", Error: "db is down"}},
		{path: "/missing", want: logLine{Status: 404, Level: "WARN", Path: "/missing", Reason: "route not found"}},
		{path: "/echo", want: logLine{Status: 405, Level: "WARN", Path: "/echo", Reason: "method not allowed"}},
		{path: "/cancelled", want: logLine{Status: 499, Level: "WARN", Route: "/cancelled", Reason: "hash password: context canceled"}},
		{path: "/things/", want: logLine{Status: 200, Level: "INFO", Route: "/things"}},
		{path: "/healthz", want: logLine{Status: 200, Level: "DEBUG", Route: "/healthz"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			h, buf := newHandler(t)
			rec := serve(h, http.MethodGet, tt.path, "", "")
			if rec.Code != tt.want.Status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want.Status)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q", ct)
			}

			lines := logLines(t, buf)
			if len(lines) != 1 {
				t.Fatalf("got %d log lines, want 1: %+v", len(lines), lines)
			}
			line := lines[0]
			want := tt.want
			want.Msg, want.Method = "request", http.MethodGet
			if line.outcome() != want {
				t.Errorf("line = %+v, want %+v", line.outcome(), want)
			}
			if id := rec.Header().Get(httpx.RequestIDHeader); id == "" || line.RequestID != id {
				t.Errorf("request_id = %q, response header = %q", line.RequestID, id)
			}
		})
	}
}

func TestRequestLoggerPropagatesRequestID(t *testing.T) {
	h, buf := newHandler(t)
	rec := serve(h, http.MethodGet, "/ok", "client-abc-123", "")
	if got := rec.Header().Get(httpx.RequestIDHeader); got != "client-abc-123" {
		t.Errorf("response request id = %q", got)
	}

	lines := logLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2", len(lines))
	}
	for _, line := range lines {
		if line.RequestID != "client-abc-123" {
			t.Errorf("line %+v lacks the request id", line)
		}
	}
	access := lines[1]
	if access.Level != "INFO" || access.Route != "/ok" || access.Method != http.MethodGet || access.Bytes != rec.Body.Len() {
		t.Errorf("access line = %+v", access)
	}
	if access.DurationMS == nil || access.ClientIP == "" {
		t.Errorf("access line lacks duration_ms or client_ip: %+v", access)
	}
}

func TestRequestLoggerReplacesUnsafeRequestID(t *testing.T) {
	for _, id := range []string{"has space", "new\nline", strings.Repeat("a", 129)} {
		h, _ := newHandler(t)
		if got := serve(h, http.MethodGet, "/ok", id, "").Header().Get(httpx.RequestIDHeader); got == id || got == "" {
			t.Errorf("incoming id %q: response id = %q, want a generated one", id, got)
		}
	}
}

func TestRecoveryLogsPanicAs500(t *testing.T) {
	h, buf := newHandler(t)
	rec := serve(h, http.MethodGet, "/panic", "", "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	lines := logLines(t, buf)
	if len(lines) != 1 || lines[0].Level != "ERROR" || !strings.HasPrefix(lines[0].Error, "panic: boom") {
		t.Errorf("lines = %+v", lines)
	}
}

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "valid", body: `{"a":"b"}`, wantStatus: 200},
		{name: "malformed", body: `{"a":`, wantStatus: 400},
		{name: "empty", body: ``, wantStatus: 400},
		{name: "too large", body: `{"a":"` + strings.Repeat("x", httpx.MaxBodyBytes) + `"}`, wantStatus: 413},
		{name: "trailing data", body: `{"a":"b"} {"c":"d"}`, wantStatus: 400},
		{name: "unknown field", body: `{"a":"b","x":1}`, wantStatus: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newHandler(t)
			if rec := serve(h, http.MethodPost, "/echo", "", tt.body); rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
		})
	}
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	if _, err := logging.New(&bytes.Buffer{}, "loud"); err == nil {
		t.Error("expected error for unknown level")
	}
}

func TestMethodNotAllowedListsAllowedMethods(t *testing.T) {
	h, _ := newHandler(t)
	rec := serve(h, http.MethodDelete, "/echo", "", "")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
		t.Errorf("got %d Allow=%q, want 405 Allow=POST", rec.Code, rec.Header().Get("Allow"))
	}
}

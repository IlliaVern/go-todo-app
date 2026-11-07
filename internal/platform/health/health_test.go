package health_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/platform/health"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func serve(t *testing.T, deps map[string]health.Pinger, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	health.Register(mux, deps)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestProbes(t *testing.T) {
	up := map[string]health.Pinger{"database": pinger{}}
	down := map[string]health.Pinger{"database": pinger{errors.New("connection refused")}}

	tests := []struct {
		name       string
		deps       map[string]health.Pinger
		path       string
		wantStatus int
		wantBody   string
	}{
		{"liveness ignores dependencies", down, health.LivenessPath, http.StatusOK, `{"status":"ok"}`},
		{"ready", up, health.ReadinessPath, http.StatusOK, `{"status":"ready"}`},
		{"not ready", down, health.ReadinessPath, http.StatusServiceUnavailable, `{"status":"database unavailable"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tt.deps, tt.path)
			if rec.Code != tt.wantStatus || strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Errorf("got %d %s, want %d %s", rec.Code, rec.Body, tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	for _, tt := range []struct {
		name    string
		deps    map[string]health.Pinger
		wantErr bool
	}{
		{"ready", map[string]health.Pinger{"database": pinger{}}, false},
		{"not ready", map[string]health.Pinger{"database": pinger{errors.New("down")}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			health.Register(mux, tt.deps)
			srv := httptest.NewServer(mux)
			defer srv.Close()
			_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

			if err := health.Check(t.Context(), port); (err != nil) != tt.wantErr {
				t.Errorf("Check err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	if err := health.Check(t.Context(), "1"); err == nil {
		t.Error("Check against a closed port succeeded")
	}
}

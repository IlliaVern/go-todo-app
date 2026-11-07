// Package health serves liveness and readiness probes and checks them from
// the command line for container health checks.
package health

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	LivenessPath  = "/healthz"
	ReadinessPath = "/readyz"
	checkTimeout  = 2 * time.Second
)

// Pinger reports whether a dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Register mounts GET /healthz, answering 200 while the process serves HTTP,
// and GET /readyz, answering 200 only when every dependency responds.
func Register(mux *http.ServeMux, deps map[string]Pinger) {
	mux.HandleFunc("GET "+LivenessPath, func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET "+ReadinessPath, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()
		for name, dep := range deps {
			if err := dep.Ping(ctx); err != nil {
				slog.WarnContext(ctx, "readiness check failed", "dependency", name, "error", err)
				writeStatus(w, http.StatusServiceUnavailable, name+" unavailable")
				return
			}
		}
		writeStatus(w, http.StatusOK, "ready")
	})
}

func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, "{\"status\":%q}\n", status)
}

// Check requests the readiness endpoint on the local port and returns an error
// unless it answers 200. The container HEALTHCHECK runs it via the
// `healthcheck` subcommand, since the image has no shell or curl.
func Check(ctx context.Context, port string) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout+time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+ReadinessPath, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness probe answered %s", resp.Status)
	}
	return nil
}

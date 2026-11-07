package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/api"
	"github.com/IlliaVern/go-todo-app/internal/platform/logging"
	"github.com/IlliaVern/go-todo-app/internal/platform/postgres/pgtest"
)

// newAPI wires the real composition root against a test database and returns
// the handler and its log output.
func newAPI(t *testing.T, authPerMinute int) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger, err := logging.New(&logs, "debug")
	if err != nil {
		t.Fatal(err)
	}
	h, err := api.NewHandler(pgtest.Start(t), api.Options{
		SigningKey:            "api-test-signing-key-0123456789abcdef",
		AuthRequestsPerMinute: authPerMinute,
		Logger:                logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h, &logs
}

func call(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func lastLogLine(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	var line map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &line); err != nil {
		t.Fatal(err)
	}
	return line
}

func TestAuthenticatedRequestsLogTheirRoute(t *testing.T) {
	h, logs := newAPI(t, 100)
	creds := `{"name":"Ada","username":"ada","password":"s3cret"}`
	if rec := call(h, http.MethodPost, "/auth/sign-up", "", creds); rec.Code != http.StatusCreated {
		t.Fatalf("sign-up: %d %s", rec.Code, rec.Body)
	}
	rec := call(h, http.MethodPost, "/auth/sign-in", "", `{"username":"ada","password":"s3cret"}`)
	var signIn struct{ Token string }
	if err := json.NewDecoder(rec.Body).Decode(&signIn); err != nil || signIn.Token == "" {
		t.Fatalf("sign-in: %d %v", rec.Code, err)
	}

	for _, path := range []string{"/api/items/abc", "/api/items/abc/"} {
		if rec := call(h, http.MethodGet, path, signIn.Token, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
		}
		line := lastLogLine(t, logs)
		if line["route"] != "/api/items/{id}" || line["request_id"] == "" {
			t.Errorf("GET %s logged %v, want route /api/items/{id} with a request id", path, line)
		}
	}
}

func TestRateLimitCoversAuthRoutesOnly(t *testing.T) {
	h, _ := newAPI(t, 1)

	body := `{"username":"nobody","password":"x"}`
	if rec := call(h, http.MethodPost, "/auth/sign-in", "", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("first sign-in: %d, want 401", rec.Code)
	}
	if rec := call(h, http.MethodPost, "/auth/sign-in", "", body); rec.Code != http.StatusTooManyRequests {
		t.Errorf("second sign-in: %d, want 429", rec.Code)
	}

	for range 3 {
		if rec := call(h, http.MethodGet, "/api/lists", "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET /api/lists without token: %d, want 401 (not rate-limited)", rec.Code)
		}
	}
}

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/identity/app"
	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
	"github.com/IlliaVern/go-todo-app/internal/identity/httpapi"
)

type users struct{}

func (users) Add(context.Context, domain.User) (int64, error) { return 1, nil }
func (users) ByUsername(context.Context, domain.Username) (domain.User, error) {
	return domain.RestoreUser(1, "Ada", "ada", "stored-hash"), nil
}

// saturatedHasher builds its startup dummy hash, then reports busy like a
// hasher whose slots are all taken.
type saturatedHasher struct{ calls int }

func (h *saturatedHasher) Hash(context.Context, string) (string, error) {
	h.calls++
	if h.calls == 1 {
		return "dummy", nil
	}
	return "", app.ErrHasherBusy
}

func (h *saturatedHasher) Matches(context.Context, string, string) (bool, error) {
	return false, app.ErrHasherBusy
}

type tokens struct{}

func (tokens) Issue(int64) (string, error)  { return "t", nil }
func (tokens) Verify(string) (int64, error) { return 1, nil }

func TestBusyHasherAnswers503(t *testing.T) {
	svc, err := app.NewService(users{}, &saturatedHasher{}, tokens{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	httpapi.NewHandler(svc).Register(mux, func(h http.Handler) http.Handler { return h })

	for path, body := range map[string]string{
		"/auth/sign-in": `{"username":"ada","password":"pw"}`,
		"/auth/sign-up": `{"name":"Ada","username":"ada2","password":"pw"}`,
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d Retry-After=%q, want 503 with Retry-After", path, rec.Code, rec.Header().Get("Retry-After"))
		}
	}
}

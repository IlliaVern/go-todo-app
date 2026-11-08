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

var storedUser = domain.RestoreUser(1, "Ada", "ada", "stored-hash", 1)

func (users) Add(context.Context, domain.User) (domain.UserID, error) { return 1, nil }
func (users) Exists(context.Context, domain.UserID) (bool, error)     { return true, nil }
func (users) Get(context.Context, domain.UserID) (domain.User, error) { return storedUser, nil }
func (users) Update(context.Context, domain.User) error               { return nil }
func (users) Delete(context.Context, domain.UserID) error             { return nil }
func (users) ByUsername(context.Context, domain.Username) (domain.User, error) {
	return storedUser, nil
}

// saturatedHasher builds its startup dummy hash, then reports busy like a
// hasher whose slots are all taken.
type saturatedHasher struct{ calls int }

func (h *saturatedHasher) Hash(context.Context, domain.Password) (domain.PasswordHash, error) {
	h.calls++
	if h.calls == 1 {
		return "dummy", nil
	}
	return "", app.ErrHasherBusy
}

func (h *saturatedHasher) Matches(context.Context, domain.PasswordHash, domain.Password) (bool, error) {
	return false, app.ErrHasherBusy
}

type tokens struct{}

func (tokens) Issue(domain.UserID) (domain.AccessToken, error)  { return "t", nil }
func (tokens) Verify(domain.AccessToken) (domain.UserID, error) { return 1, nil }

func TestBusyHasherAnswers503(t *testing.T) {
	svc, err := app.NewService(users{}, &saturatedHasher{}, tokens{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	httpapi.NewHandler(svc).Register(mux, func(h http.Handler) http.Handler { return h })

	for path, body := range map[string]string{
		"/auth/sign-in":          `{"username":"ada","password":"pw"}`,
		"/auth/sign-up":          `{"name":"Ada","username":"ada2","password":"pw"}`,
		"/api/users/me/password": `{"current_password":"pw","new_password":"pw2"}`,
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer t")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d Retry-After=%q, want 503 with Retry-After", path, rec.Code, rec.Header().Get("Retry-After"))
		}
	}
}

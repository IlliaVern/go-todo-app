package app_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/identity/app"
	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

type fakeUsers struct {
	byUsername map[string]domain.User
	nextID     int64
}

func (f *fakeUsers) Add(_ context.Context, u domain.User) (int64, error) {
	if _, taken := f.byUsername[u.Username()]; taken {
		return 0, domain.ErrUsernameTaken
	}
	f.nextID++
	f.byUsername[u.Username()] = domain.RestoreUser(f.nextID, u.Name(), u.Username(), u.PasswordHash())
	return f.nextID, nil
}

func (f *fakeUsers) ByUsername(_ context.Context, username string) (domain.User, error) {
	u, ok := f.byUsername[username]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

// fakeHasher "hashes" by prefixing and records every call.
type fakeHasher struct {
	hashed   int
	compared []string
}

func (h *fakeHasher) Hash(_ context.Context, p string) (string, error) {
	h.hashed++
	return "hashed:" + p, nil
}
func (h *fakeHasher) Matches(_ context.Context, hash, p string) (bool, error) {
	h.compared = append(h.compared, hash)
	return hash == "hashed:"+p, nil
}

type fakeTokens struct{}

func (fakeTokens) Issue(id int64) (string, error) { return fmt.Sprintf("token-for-%d", id), nil }
func (fakeTokens) Verify(tok string) (int64, error) {
	idText, ok := strings.CutPrefix(tok, "token-for-")
	if !ok {
		return 0, errors.New("bad token")
	}
	return strconv.ParseInt(idText, 10, 64)
}

func newService(t *testing.T) (*app.Service, *fakeHasher) {
	t.Helper()
	hasher := &fakeHasher{}
	svc, err := app.NewService(&fakeUsers{byUsername: map[string]domain.User{}}, hasher, fakeTokens{})
	if err != nil {
		t.Fatal(err)
	}
	return svc, hasher
}

func TestSignUpAndSignIn(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	id, err := svc.SignUp(ctx, "Ada", "ada", "s3cret")
	if err != nil || id != 1 {
		t.Fatalf("SignUp = %d, %v; want 1, nil", id, err)
	}

	tok, err := svc.SignIn(ctx, "ada", "s3cret")
	if err != nil || tok != "token-for-1" {
		t.Fatalf("SignIn = %q, %v; want token-for-1, nil", tok, err)
	}

	userID, err := svc.Authenticate(tok)
	if err != nil || userID != 1 {
		t.Fatalf("Authenticate = %d, %v; want 1, nil", userID, err)
	}
}

func TestSignUpErrors(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	if _, err := svc.SignUp(ctx, "Ada", "ada", "s3cret"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, fullName, username, password string
		wantErr                            error
	}{
		{name: "duplicate username", fullName: "Ada", username: "ada", password: "x", wantErr: domain.ErrUsernameTaken},
		{name: "empty password", fullName: "Bob", username: "bob", password: "", wantErr: domain.ErrPasswordRequired},
		{name: "password too long", fullName: "Bob", username: "bob", password: strings.Repeat("p", 1025), wantErr: domain.ErrPasswordTooLong},
		{name: "missing name", fullName: "", username: "bob", password: "x", wantErr: domain.ErrNameRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.SignUp(ctx, tt.fullName, tt.username, tt.password); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignInRejectsBadCredentials(t *testing.T) {
	ctx := context.Background()
	svc, hasher := newService(t)
	if _, err := svc.SignUp(ctx, "Ada", "ada", "s3cret"); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SignIn(ctx, "ada", "wrong"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("wrong password: err = %v, want %v", err, app.ErrInvalidCredentials)
	}

	hasher.compared = nil
	if _, err := svc.SignIn(ctx, "nobody", "wrong"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("unknown user: err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if len(hasher.compared) != 1 {
		t.Errorf("unknown user: %d hash comparisons, want 1 to keep timing uniform", len(hasher.compared))
	}
}

func TestSignUpValidatesProfileBeforeHashing(t *testing.T) {
	svc, hasher := newService(t)
	hasher.hashed = 0 // ignore the dummy hash computed by NewService

	if _, err := svc.SignUp(context.Background(), " ", "ada", "s3cret"); !errors.Is(err, domain.ErrNameRequired) {
		t.Fatalf("err = %v, want %v", err, domain.ErrNameRequired)
	}
	if hasher.hashed != 0 {
		t.Errorf("password hashed %d times for an invalid profile, want 0", hasher.hashed)
	}
}

func TestSignInRejectsNULUsername(t *testing.T) {
	svc, hasher := newService(t)
	if _, err := svc.SignIn(context.Background(), "a\x00da", "pw"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if len(hasher.compared) != 1 {
		t.Errorf("%d hash comparisons, want 1 to keep timing uniform", len(hasher.compared))
	}
}

func TestAuthenticateRejectsInvalidToken(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.Authenticate("forged"); !errors.Is(err, app.ErrInvalidToken) {
		t.Errorf("err = %v, want %v", err, app.ErrInvalidToken)
	}
}

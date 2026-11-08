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
	"github.com/IlliaVern/go-todo-app/internal/platform/retry"
)

// fakeUsers has the same uniqueness and version checks as the PostgreSQL adapter.
type fakeUsers struct {
	byID   map[domain.UserID]domain.User
	nextID domain.UserID
	// conflicts makes the next N updates fail as if another writer won.
	conflicts int
}

func (f *fakeUsers) taken(username domain.Username, except domain.UserID) bool {
	for id, u := range f.byID {
		if id != except && u.Username() == username {
			return true
		}
	}
	return false
}

func (f *fakeUsers) Add(_ context.Context, u domain.User) (domain.UserID, error) {
	if f.taken(u.Username(), 0) {
		return 0, domain.ErrUsernameTaken
	}
	f.nextID++
	f.byID[f.nextID] = domain.RestoreUser(f.nextID, u.Name().String(), u.Username().String(), u.PasswordHash(), 1)
	return f.nextID, nil
}

func (f *fakeUsers) Get(_ context.Context, id domain.UserID) (domain.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeUsers) Exists(_ context.Context, id domain.UserID) (bool, error) {
	_, ok := f.byID[id]
	return ok, nil
}

func (f *fakeUsers) ByUsername(_ context.Context, username domain.Username) (domain.User, error) {
	for _, u := range f.byID {
		if u.Username() == username {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeUsers) Update(ctx context.Context, u domain.User) error {
	stored, err := f.Get(ctx, u.ID())
	if err != nil {
		return err
	}
	if f.conflicts > 0 {
		f.conflicts--
		return domain.ErrConcurrentUpdate
	}
	if stored.Version() != u.Version() {
		return domain.ErrConcurrentUpdate
	}
	if f.taken(u.Username(), u.ID()) {
		return domain.ErrUsernameTaken
	}
	f.byID[u.ID()] = domain.RestoreUser(u.ID(), u.Name().String(), u.Username().String(), u.PasswordHash(), u.Version()+1)
	return nil
}

func (f *fakeUsers) Delete(_ context.Context, id domain.UserID) error {
	if _, ok := f.byID[id]; !ok {
		return domain.ErrUserNotFound
	}
	delete(f.byID, id)
	return nil
}

// fakeHasher "hashes" by prefixing and records every call.
type fakeHasher struct {
	hashed   int
	compared []domain.PasswordHash
}

func (h *fakeHasher) Hash(_ context.Context, p domain.Password) (domain.PasswordHash, error) {
	h.hashed++
	return domain.PasswordHash("hashed:" + p.Reveal()), nil
}

func (h *fakeHasher) Matches(_ context.Context, hash domain.PasswordHash, p domain.Password) (bool, error) {
	h.compared = append(h.compared, hash)
	return string(hash) == "hashed:"+p.Reveal(), nil
}

type fakeTokens struct{}

func (fakeTokens) Issue(id domain.UserID) (domain.AccessToken, error) {
	return domain.AccessToken(fmt.Sprintf("token-for-%d", id)), nil
}

func (fakeTokens) Verify(tok domain.AccessToken) (domain.UserID, error) {
	idText, ok := strings.CutPrefix(string(tok), "token-for-")
	if !ok {
		return 0, errors.New("bad token")
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	return domain.UserID(id), err
}

func newService(t *testing.T) (*app.Service, *fakeHasher) {
	t.Helper()
	svc, hasher, _ := newServiceWithUsers(t)
	return svc, hasher
}

func newServiceWithUsers(t *testing.T) (*app.Service, *fakeHasher, *fakeUsers) {
	t.Helper()
	hasher := &fakeHasher{}
	users := &fakeUsers{byID: map[domain.UserID]domain.User{}}
	svc, err := app.NewService(users, hasher, fakeTokens{})
	if err != nil {
		t.Fatal(err)
	}
	return svc, hasher, users
}

func ada(t *testing.T, svc *app.Service) domain.UserID {
	t.Helper()
	id, err := svc.SignUp(context.Background(), app.SignUpInput{Name: "Ada", Username: "ada", Password: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ptr[T any](v T) *T { return &v }

func TestSignUpAndSignIn(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	id, err := svc.SignUp(ctx, app.SignUpInput{Name: "Ada", Username: "ada", Password: "s3cret"})
	if err != nil || id != 1 {
		t.Fatalf("SignUp = %d, %v; want 1, nil", id, err)
	}

	tok, err := svc.SignIn(ctx, app.SignInInput{Username: "ada", Password: "s3cret"})
	if err != nil || tok != "token-for-1" {
		t.Fatalf("SignIn = %q, %v; want token-for-1, nil", tok, err)
	}

	userID, err := svc.Authenticate(ctx, tok)
	if err != nil || userID != 1 {
		t.Fatalf("Authenticate = %d, %v; want 1, nil", userID, err)
	}
}

func TestSignUpErrors(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	ada(t, svc)

	tests := []struct {
		name    string
		input   app.SignUpInput
		wantErr error
	}{
		{name: "duplicate username", input: app.SignUpInput{Name: "Ada", Username: "ada", Password: "x"}, wantErr: domain.ErrUsernameTaken},
		{name: "empty password", input: app.SignUpInput{Name: "Bob", Username: "bob"}, wantErr: domain.ErrPasswordRequired},
		{name: "password too long", input: app.SignUpInput{Name: "Bob", Username: "bob", Password: strings.Repeat("p", 1025)}, wantErr: domain.ErrPasswordTooLong},
		{name: "missing name", input: app.SignUpInput{Username: "bob", Password: "x"}, wantErr: domain.ErrNameRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.SignUp(ctx, tt.input); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignInRejectsBadCredentials(t *testing.T) {
	ctx := context.Background()
	svc, hasher := newService(t)
	ada(t, svc)

	if _, err := svc.SignIn(ctx, app.SignInInput{Username: "ada", Password: "wrong"}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("wrong password: err = %v, want %v", err, app.ErrInvalidCredentials)
	}

	hasher.compared = nil
	if _, err := svc.SignIn(ctx, app.SignInInput{Username: "nobody", Password: "wrong"}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("unknown user: err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if len(hasher.compared) != 1 {
		t.Errorf("unknown user: %d hash comparisons, want 1 to keep timing uniform", len(hasher.compared))
	}
}

func TestSignUpValidatesProfileBeforeHashing(t *testing.T) {
	svc, hasher := newService(t)
	hasher.hashed = 0 // ignore the dummy hash computed by NewService

	_, err := svc.SignUp(context.Background(), app.SignUpInput{Name: " ", Username: "ada", Password: "s3cret"})
	if !errors.Is(err, domain.ErrNameRequired) {
		t.Fatalf("err = %v, want %v", err, domain.ErrNameRequired)
	}
	if hasher.hashed != 0 {
		t.Errorf("password hashed %d times for an invalid profile, want 0", hasher.hashed)
	}
}

func TestSignInRejectsNULUsername(t *testing.T) {
	svc, hasher := newService(t)
	_, err := svc.SignIn(context.Background(), app.SignInInput{Username: "a\x00da", Password: "pw"})
	if !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if len(hasher.compared) != 1 {
		t.Errorf("%d hash comparisons, want 1 to keep timing uniform", len(hasher.compared))
	}
}

func TestSignInRejectsEmptyPasswordWithoutHashing(t *testing.T) {
	svc, hasher := newService(t)
	ada(t, svc)
	hasher.compared = nil

	_, err := svc.SignIn(context.Background(), app.SignInInput{Username: "ada"})
	if !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if len(hasher.compared) != 0 {
		t.Errorf("%d hash comparisons, want 0", len(hasher.compared))
	}
}

func TestAuthenticateRejectsInvalidToken(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.Authenticate(context.Background(), "forged"); !errors.Is(err, app.ErrInvalidToken) {
		t.Errorf("err = %v, want %v", err, app.ErrInvalidToken)
	}
}

func TestAuthenticateRejectsTokensOfDeletedUsers(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.Authenticate(context.Background(), "token-for-42"); !errors.Is(err, app.ErrInvalidToken) {
		t.Errorf("token for unknown user 42: err = %v, want %v", err, app.ErrInvalidToken)
	}
}

func TestSignInIsCaseInsensitiveAndBoundsPasswords(t *testing.T) {
	ctx := context.Background()
	svc, hasher := newService(t)
	if _, err := svc.SignUp(ctx, app.SignUpInput{Name: "Ada", Username: "Ada", Password: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignUp(ctx, app.SignUpInput{Name: "Ada", Username: "ADA", Password: "other"}); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Errorf("case variant sign-up: err = %v, want %v", err, domain.ErrUsernameTaken)
	}
	if _, err := svc.SignIn(ctx, app.SignInInput{Username: "aDa", Password: "s3cret"}); err != nil {
		t.Errorf("case-insensitive sign-in: %v", err)
	}

	hasher.compared = nil
	oversized := strings.Repeat("p", domain.MaxPasswordBytes+1)
	if _, err := svc.SignIn(ctx, app.SignInInput{Username: "ada", Password: oversized}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("oversized password: err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if len(hasher.compared) != 0 {
		t.Errorf("oversized password was hashed %d times, want 0", len(hasher.compared))
	}
}

func TestUpdateProfile(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	id := ada(t, svc)
	if _, err := svc.SignUp(ctx, app.SignUpInput{Name: "Bob", Username: "bob", Password: "x"}); err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateProfile(ctx, id, domain.ProfilePatch{Name: ptr("Ada L."), Username: ptr("LOVELACE")}); err != nil {
		t.Fatal(err)
	}
	user, err := svc.Profile(ctx, id)
	if err != nil || user.Name().String() != "Ada L." || user.Username().String() != "lovelace" {
		t.Fatalf("Profile = %+v, %v", user, err)
	}

	for _, tt := range []struct {
		name    string
		patch   domain.ProfilePatch
		wantErr error
	}{
		{"empty", domain.ProfilePatch{}, domain.ErrEmptyUpdate},
		{"blank name", domain.ProfilePatch{Name: ptr(" ")}, domain.ErrNameRequired},
		{"taken username", domain.ProfilePatch{Username: ptr("Bob")}, domain.ErrUsernameTaken},
	} {
		if err := svc.UpdateProfile(ctx, id, tt.patch); !errors.Is(err, tt.wantErr) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
		}
	}
	if err := svc.UpdateProfile(ctx, 999, domain.ProfilePatch{Name: ptr("x")}); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("unknown user: err = %v, want %v", err, domain.ErrUserNotFound)
	}
}

func TestUpdateProfileRetriesAfterConcurrentModification(t *testing.T) {
	ctx := context.Background()
	svc, _, users := newServiceWithUsers(t)
	id := ada(t, svc)

	users.conflicts = retry.MaxAttempts - 1
	if err := svc.UpdateProfile(ctx, id, domain.ProfilePatch{Name: ptr("Ada L.")}); err != nil {
		t.Fatalf("after %d conflicts: %v", retry.MaxAttempts-1, err)
	}

	users.conflicts = retry.MaxAttempts
	if err := svc.UpdateProfile(ctx, id, domain.ProfilePatch{Name: ptr("Ada X.")}); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Errorf("persistent conflict: err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	id := ada(t, svc)

	if err := svc.ChangePassword(ctx, id, app.ChangePasswordInput{Current: "s3cret", New: "n3w"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignIn(ctx, app.SignInInput{Username: "ada", Password: "s3cret"}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("old password: err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if _, err := svc.SignIn(ctx, app.SignInInput{Username: "ada", Password: "n3w"}); err != nil {
		t.Errorf("new password: %v", err)
	}
}

func TestChangePasswordRejections(t *testing.T) {
	ctx := context.Background()
	svc, hasher := newService(t)
	id := ada(t, svc)
	hasher.hashed = 0

	for _, tt := range []struct {
		name    string
		input   app.ChangePasswordInput
		wantErr error
	}{
		{"wrong current", app.ChangePasswordInput{Current: "wrong", New: "n3w"}, app.ErrWrongPassword},
		{"empty current", app.ChangePasswordInput{New: "n3w"}, app.ErrWrongPassword},
		{"empty new", app.ChangePasswordInput{Current: "s3cret"}, domain.ErrPasswordRequired},
		{"oversized new", app.ChangePasswordInput{Current: "s3cret", New: strings.Repeat("p", 1025)}, domain.ErrPasswordTooLong},
	} {
		if err := svc.ChangePassword(ctx, id, tt.input); !errors.Is(err, tt.wantErr) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
		}
	}
	if hasher.hashed != 0 {
		t.Errorf("hashed a new password %d times despite rejections", hasher.hashed)
	}
}

func TestChangePasswordIsNotRetriedOnConflict(t *testing.T) {
	ctx := context.Background()
	svc, hasher, users := newServiceWithUsers(t)
	id := ada(t, svc)
	hasher.compared = nil

	users.conflicts = 1
	err := svc.ChangePassword(ctx, id, app.ChangePasswordInput{Current: "s3cret", New: "n3w"})
	if !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Fatalf("err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}
	if len(hasher.compared) != 1 {
		t.Errorf("%d password checks, want 1", len(hasher.compared))
	}
}

func TestDeleteAccountInvalidatesTokens(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	id := ada(t, svc)
	tok, err := svc.SignIn(ctx, app.SignInInput{Username: "ada", Password: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAccount(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, tok); !errors.Is(err, app.ErrInvalidToken) {
		t.Errorf("token after deletion: err = %v, want %v", err, app.ErrInvalidToken)
	}
	if err := svc.DeleteAccount(ctx, id); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("second delete: err = %v, want %v", err, domain.ErrUserNotFound)
	}
}

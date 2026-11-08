package password_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
	"github.com/IlliaVern/go-todo-app/internal/identity/password"
)

// cheap keeps tests fast; the format and logic are the same as DefaultParams.
var cheap = password.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func pw(t *testing.T, s string) domain.Password {
	t.Helper()
	p, err := domain.NewPassword(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHashAndMatch(t *testing.T) {
	ctx := context.Background()
	h := password.NewArgon2idWithParams(cheap, 2, time.Second)

	hash, err := h.Hash(ctx, pw(t, "correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\$argon2id\$v=19\$m=64,t=1,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`).MatchString(string(hash)) {
		t.Errorf("hash %q is not a PHC argon2id string", hash)
	}

	for _, tt := range []struct {
		password string
		want     bool
	}{
		{"correct horse", true},
		{"correct horsE", false},
	} {
		got, err := h.Matches(ctx, hash, pw(t, tt.password))
		if err != nil || got != tt.want {
			t.Errorf("Matches(%q) = %v, %v; want %v", tt.password, got, err, tt.want)
		}
	}

	again, _ := h.Hash(ctx, pw(t, "correct horse"))
	if again == hash {
		t.Error("two hashes of the same password are identical: salt is not random")
	}
}

func TestMatchesUsesParamsStoredInHash(t *testing.T) {
	ctx := context.Background()
	old := password.NewArgon2idWithParams(cheap, 1, time.Second)
	hash, _ := old.Hash(ctx, pw(t, "pw"))

	stronger := password.NewArgon2idWithParams(password.Params{Memory: 128, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32}, 1, time.Second)
	if ok, err := stronger.Matches(ctx, hash, pw(t, "pw")); err != nil || !ok {
		t.Errorf("hash made with other params did not verify: %v, %v", ok, err)
	}
}

func TestDefaultParamsMeetOWASPMinimum(t *testing.T) {
	p := password.DefaultParams
	if p.Memory < 19*1024 || p.Time < 2 || p.Threads < 1 || p.SaltLen < 16 || p.KeyLen < 32 {
		t.Errorf("DefaultParams %+v are below the OWASP argon2id minimum", p)
	}
}

func TestMatchesRejectsMalformedHashes(t *testing.T) {
	h := password.NewArgon2idWithParams(cheap, 1, time.Second)
	for _, bad := range []domain.PasswordHash{
		"",
		"$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234", // bcrypt
		"$argon2i$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$a2V5",
		"$argon2id$v=16$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$a2V5",
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$a2V5",
		"$argon2id$v=19$m=64,t=1,p=1$!!!$a2V5",
		"$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$",
	} {
		if _, err := h.Matches(context.Background(), bad, pw(t, "pw")); !errors.Is(err, password.ErrMalformedHash) {
			t.Errorf("Matches(%q) err = %v, want ErrMalformedHash", bad, err)
		}
	}
}

func TestHashRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := password.NewArgon2idWithParams(cheap, 1, time.Second).Hash(ctx, pw(t, "pw")); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestLongPasswordsAreNotTruncated(t *testing.T) {
	ctx := context.Background()
	h := password.NewArgon2idWithParams(cheap, 1, time.Second)
	long := strings.Repeat("a", 100)
	hash, _ := h.Hash(ctx, pw(t, long))
	if ok, _ := h.Matches(ctx, hash, pw(t, long[:72])); ok {
		t.Error("a 72-byte prefix matched a 100-byte password")
	}
}

package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

const testKey = "0123456789abcdef0123456789abcdef"

func TestJWTRoundTrip(t *testing.T) {
	m, err := NewJWT(testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Errorf("user id = %d, want 42", got)
	}
}

func TestJWTRejects(t *testing.T) {
	issuedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m, _ := NewJWT(testKey, time.Hour)
	m.now = func() time.Time { return issuedAt }
	valid, _ := m.Issue(42)

	other, _ := NewJWT("ffffffffffffffffffffffffffffffff", time.Hour)
	foreign, _ := other.Issue(42)

	noExpiry, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{UserID: 42}).SignedString([]byte(testKey))
	noUser, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(issuedAt.Add(time.Hour))},
	}).SignedString([]byte(testKey))
	unsigned, _ := jwt.NewWithClaims(jwt.SigningMethodNone, &claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(issuedAt.Add(time.Hour))},
		UserID:           42,
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	tests := []struct {
		name  string
		token domain.AccessToken
		at    time.Time
	}{
		{name: "expired", token: valid, at: issuedAt.Add(2 * time.Hour)},
		{name: "signed with another key", token: foreign, at: issuedAt},
		{name: "no expiry", token: domain.AccessToken(noExpiry), at: issuedAt},
		{name: "no user id", token: domain.AccessToken(noUser), at: issuedAt},
		{name: "alg none", token: domain.AccessToken(unsigned), at: issuedAt},
		{name: "garbage", token: "a.b.c", at: issuedAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.now = func() time.Time { return tt.at }
			if id, err := m.Verify(tt.token); err == nil {
				t.Errorf("Verify accepted token, user id %d", id)
			}
		})
	}
}

func TestNewJWTRejectsShortKey(t *testing.T) {
	if _, err := NewJWT("too-short", time.Hour); err == nil {
		t.Error("expected error for a key shorter than 32 bytes")
	}
}

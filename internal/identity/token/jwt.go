// Package token implements app.TokenManager with HS256-signed JWTs.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

// MinKeyLength is the minimum HS256 signing key size in bytes.
const MinKeyLength = 32

type claims struct {
	jwt.RegisteredClaims
	UserID domain.UserID `json:"user_id"`
}

type JWT struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewJWT fails for keys shorter than MinKeyLength.
func NewJWT(signingKey string, ttl time.Duration) (*JWT, error) {
	if len(signingKey) < MinKeyLength {
		return nil, fmt.Errorf("token signing key must be at least %d bytes", MinKeyLength)
	}
	return &JWT{key: []byte(signingKey), ttl: ttl, now: time.Now}, nil
}

func (m *JWT) Issue(userID domain.UserID) (domain.AccessToken, error) {
	now := m.now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID: userID,
	})
	signed, err := token.SignedString(m.key)
	return domain.AccessToken(signed), err
}

func (m *JWT) Verify(token domain.AccessToken) (domain.UserID, error) {
	var c claims
	_, err := jwt.ParseWithClaims(string(token), &c,
		func(*jwt.Token) (any, error) { return m.key, nil }, //nolint:forbidigo // jwt.Keyfunc's required signature
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return 0, err
	}
	if c.UserID <= 0 {
		return 0, errors.New("token has no user id")
	}
	return c.UserID, nil
}

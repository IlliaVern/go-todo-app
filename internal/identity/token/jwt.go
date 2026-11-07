// Package token implements app.TokenManager with HS256-signed JWTs.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MinKeyLength is the minimum HS256 signing key size in bytes.
const MinKeyLength = 32

type claims struct {
	jwt.RegisteredClaims
	UserID int64 `json:"user_id"`
}

type JWT struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewJWT returns a manager issuing tokens valid for ttl.
func NewJWT(signingKey string, ttl time.Duration) (*JWT, error) {
	if len(signingKey) < MinKeyLength {
		return nil, fmt.Errorf("token signing key must be at least %d bytes", MinKeyLength)
	}
	return &JWT{key: []byte(signingKey), ttl: ttl, now: time.Now}, nil
}

func (m *JWT) Issue(userID int64) (string, error) {
	now := m.now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID: userID,
	})
	return token.SignedString(m.key)
}

func (m *JWT) Verify(tokenString string) (int64, error) {
	var c claims
	_, err := jwt.ParseWithClaims(tokenString, &c,
		func(*jwt.Token) (any, error) { return m.key, nil },
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

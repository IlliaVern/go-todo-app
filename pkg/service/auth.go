package service

import (
	"context"
	"errors"
	"time"

	"github.com/IlliaVern/go-todo-app"
	"github.com/IlliaVern/go-todo-app/pkg/repository"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const tokenTTL = 12 * time.Hour

// MinSigningKeyLength is the minimum HS256 signing key size in bytes.
const MinSigningKeyLength = 32

// ErrInvalidCredentials is returned when the username is unknown or the password does not match.
var ErrInvalidCredentials = errors.New("invalid username or password")

// dummyHash is compared against when the user does not exist, so sign-in takes
// roughly the same time for unknown usernames and wrong passwords.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

type tokenClaims struct {
	jwt.RegisteredClaims
	UserId int `json:"user_id"`
}

type AuthService struct {
	repo       repository.Authorization
	signingKey []byte
}

func NewAuthService(repo repository.Authorization, signingKey string) *AuthService {
	return &AuthService{repo: repo, signingKey: []byte(signingKey)}
}

func (s *AuthService) CreateUser(ctx context.Context, user todo.User) (int, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	user.Password = string(hash)

	return s.repo.CreateUser(ctx, user)
}

func (s *AuthService) GenerateToken(ctx context.Context, username, password string) (string, error) {
	user, err := s.repo.GetUser(ctx, username)
	if errors.Is(err, todo.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserId: user.Id,
	})

	return token.SignedString(s.signingKey)
}

func (s *AuthService) ParseToken(accessToken string) (int, error) {
	token, err := jwt.ParseWithClaims(accessToken, &tokenClaims{}, func(token *jwt.Token) (any, error) {
		return s.signingKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(*tokenClaims)
	if !ok {
		return 0, errors.New("token claims are not of type *tokenClaims")
	}

	return claims.UserId, nil
}

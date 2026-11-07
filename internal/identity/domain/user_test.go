package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name, fullName, username, hash string
		wantErr                        error
	}{
		{name: "valid", fullName: "Ada", username: "ada", hash: "h"},
		{name: "blank name", fullName: " ", username: "ada", hash: "h", wantErr: domain.ErrNameRequired},
		{name: "long name", fullName: strings.Repeat("n", 256), username: "ada", hash: "h", wantErr: domain.ErrNameTooLong},
		{name: "empty username", fullName: "Ada", username: "", hash: "h", wantErr: domain.ErrUsernameRequired},
		{name: "long username", fullName: "Ada", username: strings.Repeat("u", 256), hash: "h", wantErr: domain.ErrUsernameTooLong},
		{name: "missing hash", fullName: "Ada", username: "ada", hash: "", wantErr: domain.ErrPasswordHashEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := domain.NewUser(tt.fullName, tt.username, tt.hash)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (user.Name() != tt.fullName || user.Username() != tt.username || user.PasswordHash() != tt.hash) {
				t.Errorf("got %+v", user)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "valid", password: "s3cret"},
		{name: "empty", password: "", wantErr: domain.ErrPasswordRequired},
		{name: "72 bytes", password: strings.Repeat("p", 72)},
		{name: "73 bytes", password: strings.Repeat("p", 73), wantErr: domain.ErrPasswordTooLong},
		{name: "multibyte over limit", password: strings.Repeat("я", 37), wantErr: domain.ErrPasswordTooLong}, // 74 bytes
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := domain.ValidatePassword(tt.password); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

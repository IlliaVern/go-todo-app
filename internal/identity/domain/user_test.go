package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

func TestNewName(t *testing.T) {
	tests := []struct {
		name, input string
		wantErr     error
	}{
		{name: "valid", input: "Ada Lovelace"},
		{name: "at limit", input: strings.Repeat("я", 255)},
		{name: "blank", input: " ", wantErr: domain.ErrNameRequired},
		{name: "too long", input: strings.Repeat("n", 256), wantErr: domain.ErrNameTooLong},
		{name: "NUL", input: "A\x00da", wantErr: domain.ErrNULCharacter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewName(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.String() != tt.input {
				t.Errorf("String() = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestNewUsername(t *testing.T) {
	tests := []struct {
		name, input string
		wantErr     error
	}{
		{name: "valid", input: "ada"},
		{name: "empty", input: "", wantErr: domain.ErrUsernameRequired},
		{name: "leading space", input: " ada", wantErr: domain.ErrUsernameWhitespace},
		{name: "trailing tab", input: "ada\t", wantErr: domain.ErrUsernameWhitespace},
		{name: "too long", input: strings.Repeat("u", 256), wantErr: domain.ErrUsernameTooLong},
		{name: "NUL", input: "a\x00da", wantErr: domain.ErrNULCharacter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewUsername(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.String() != tt.input {
				t.Errorf("String() = %q, want %q", got, tt.input)
			}
		})
	}
}

func TestNewUser(t *testing.T) {
	name, _ := domain.NewName("Ada")
	username, _ := domain.NewUsername("ada")

	user, err := domain.NewUser(name, username, "hash")
	if err != nil {
		t.Fatal(err)
	}
	if user.Name() != name || user.Username() != username || user.PasswordHash() != "hash" || user.ID() != 0 {
		t.Errorf("got %+v", user)
	}

	if _, err := domain.NewUser(name, username, ""); !errors.Is(err, domain.ErrPasswordHashEmpty) {
		t.Errorf("missing hash: err = %v, want %v", err, domain.ErrPasswordHashEmpty)
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
		{name: "1024 bytes", password: strings.Repeat("p", 1024)},
		{name: "1025 bytes", password: strings.Repeat("p", 1025), wantErr: domain.ErrPasswordTooLong},
		{name: "multibyte over limit", password: strings.Repeat("я", 513), wantErr: domain.ErrPasswordTooLong}, // 1026 bytes
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := domain.ValidatePassword(tt.password); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewUsernameLowercases(t *testing.T) {
	for _, in := range []string{"Ada", "ADA", "aDa"} {
		if got, err := domain.NewUsername(in); err != nil || got.String() != "ada" {
			t.Errorf("NewUsername(%q) = %q, %v; want ada", in, got, err)
		}
	}
}

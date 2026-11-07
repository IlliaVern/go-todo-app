// Package password implements app.PasswordHasher with bcrypt.
package password

import "golang.org/x/crypto/bcrypt"

type Bcrypt struct {
	cost int
}

// NewBcrypt returns a hasher using bcrypt.DefaultCost.
func NewBcrypt() Bcrypt {
	return Bcrypt{cost: bcrypt.DefaultCost}
}

// NewBcryptWithCost returns a hasher with a custom cost; tests use bcrypt.MinCost.
func NewBcryptWithCost(cost int) Bcrypt {
	return Bcrypt{cost: cost}
}

func (b Bcrypt) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), b.cost)
	return string(hash), err
}

func (b Bcrypt) Matches(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

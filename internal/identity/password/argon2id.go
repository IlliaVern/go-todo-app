// Package password implements app.PasswordHasher with argon2id, encoding
// hashes in the PHC string format:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<base64 salt>$<base64 key>
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/IlliaVern/go-todo-app/internal/identity/app"
)

// Params are argon2id cost parameters. Memory is in KiB.
type Params struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	SaltLen uint32
	KeyLen  uint32
}

// DefaultParams are OWASP's minimum recommendation for argon2id
// (19 MiB, 2 iterations, 1 degree of parallelism).
var DefaultParams = Params{Memory: 19 * 1024, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32}

var ErrMalformedHash = errors.New("malformed argon2id hash")

var b64 = base64.RawStdEncoding

// DefaultMaxWait is how long a call waits for a free hashing slot before
// failing with app.ErrHasherBusy.
const DefaultMaxWait = 2 * time.Second

// Argon2id hashes passwords. Each hash allocates Params.Memory KiB, so at most
// a fixed number run at once. Further calls wait up to maxWait for a slot and
// then fail fast with app.ErrHasherBusy (load shedding), or give up earlier
// when their context ends.
type Argon2id struct {
	params  Params
	slots   chan struct{}
	maxWait time.Duration
}

// NewArgon2id returns a hasher using DefaultParams that runs at most
// GOMAXPROCS hashes concurrently and waits at most DefaultMaxWait for a slot.
func NewArgon2id() *Argon2id {
	return NewArgon2idWithParams(DefaultParams, runtime.GOMAXPROCS(0), DefaultMaxWait)
}

func NewArgon2idWithParams(p Params, maxConcurrent int, maxWait time.Duration) *Argon2id {
	return &Argon2id{params: p, slots: make(chan struct{}, max(maxConcurrent, 1)), maxWait: maxWait}
}

func (a *Argon2id) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, a.params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key, err := a.derive(ctx, password, salt, a.params)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, a.params.Memory, a.params.Time, a.params.Threads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Matches verifies password with the parameters stored in encoded.
func (a *Argon2id) Matches(ctx context.Context, encoded, password string) (bool, error) {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return false, err
	}
	got, err := a.derive(ctx, password, salt, p)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func (a *Argon2id) derive(ctx context.Context, password string, salt []byte, p Params) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wait := time.NewTimer(a.maxWait)
	defer wait.Stop()
	select {
	case a.slots <- struct{}{}:
	case <-wait.C:
		return nil, app.ErrHasherBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-a.slots }()
	return argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen), nil
}

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrMalformedHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Params{}, nil, nil, ErrMalformedHash
	}

	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil ||
		p.Memory == 0 || p.Time == 0 || p.Threads == 0 {
		return Params{}, nil, nil, ErrMalformedHash
	}

	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, ErrMalformedHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Params{}, nil, nil, ErrMalformedHash
	}
	p.SaltLen = uint32(len(salt)) //nolint:gosec // decoded from a short PHC field
	p.KeyLen = uint32(len(key))   //nolint:gosec // decoded from a short PHC field
	return p, salt, key, nil
}

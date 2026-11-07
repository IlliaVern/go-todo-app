package password

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IlliaVern/go-todo-app/internal/identity/app"
)

func TestHashShedsLoadWhenSaturated(t *testing.T) {
	h := NewArgon2idWithParams(Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}, 1, 50*time.Millisecond)
	h.slots <- struct{}{} // occupy the only hashing slot

	start := time.Now()
	_, err := h.Hash(context.Background(), "pw")
	if !errors.Is(err, app.ErrHasherBusy) {
		t.Fatalf("err = %v, want app.ErrHasherBusy", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("waited %v, want about the 50ms limit", waited)
	}

	<-h.slots // free the slot: hashing works again
	if _, err := h.Hash(context.Background(), "pw"); err != nil {
		t.Errorf("after release: %v", err)
	}
}

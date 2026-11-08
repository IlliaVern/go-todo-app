// Package retry re-runs optimistic concurrency cycles that lost a race.
package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// MaxAttempts bounds how often an update is re-applied to a freshly loaded
// aggregate. Contention beyond that surfaces as the conflict error.
const MaxAttempts = 5

// baseDelay is the first backoff step; each retry doubles it, with jitter.
const baseDelay = 2 * time.Millisecond

// OnConflict runs a load-apply-save cycle again when it fails with conflict,
// so concurrent patches to different fields all land. Randomised exponential
// backoff spreads competing writers apart.
func OnConflict(ctx context.Context, conflict error, update func() error) error {
	var err error
	for attempt := range MaxAttempts {
		if attempt > 0 {
			step := baseDelay << (attempt - 1)
			wait := step + rand.N(step) //nolint:gosec // jitter, not a secret
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = update(); !errors.Is(err, conflict) {
			return err
		}
	}
	return err
}

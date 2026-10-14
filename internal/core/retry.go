package core

import (
	"context"
	"fmt"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

const (
	maxRetry429Attempts = 4
	// A wait longer than this is the server saying "come back later", not "slow down":
	// give up and let ExplainError report it rather than hang a window.
	maxRetryWait      = 30 * time.Second
	maxRetryTotalWait = 90 * time.Second
)

// Vars so tests can run without real sleeps.
var (
	retryBackoffBase = 2 * time.Second
	sleepCtx         = func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
)

// ServerBusyMessage is the status shown while a rate-limited call waits.
func ServerBusyMessage(wait time.Duration) string {
	return fmt.Sprintf("server busy, retrying in %s...", wait.Round(time.Second))
}

// WithRetry429 reruns fn after a 429, honoring Retry-After and otherwise
// doubling from a short base. Other errors return at once; onWait (may be
// nil) is told each wait before it starts.
func WithRetry429(ctx context.Context, onWait func(time.Duration), fn func() error) error {
	delay := retryBackoffBase
	var total time.Duration
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil || !isRateLimited(err) || attempt >= maxRetry429Attempts {
			return err
		}
		wait := delay
		if ra, ok := client.RetryAfter(err); ok {
			wait = ra
		}
		if wait > maxRetryWait || total+wait > maxRetryTotalWait {
			return err
		}
		if onWait != nil {
			onWait(wait)
		}
		if serr := sleepCtx(ctx, wait); serr != nil {
			return serr
		}
		total += wait
		delay *= 2
	}
}

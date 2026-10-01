package core

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

// ExplainError rewords the server errors a user can act on: the three
// supersede refusals, and the one fixed by waiting —
// a 429 becomes "rate limited, try again in Ns", using the server's own
// Retry-After (the exact wait until the budget has a slot; see API.md)
// when it sent one. Every other error passes through untouched, so both
// surfaces can route all their errors here without changing any wording
// they already rely on.
func ExplainError(err error) error {
	switch {
	case errors.Is(err, client.ErrSupersedeNotFound):
		return errors.New("that track no longer exists on the node, so there is nothing to revise")
	case errors.Is(err, client.ErrSupersedeConflict):
		return errors.New("that track is no longer the latest version of its subtitle, or you are not allowed to replace it (an AI-generated file cannot replace a human-made one)")
	case errors.Is(err, client.ErrSupersedeLocked):
		return errors.New("that subtitle is locked against revisions")
	}
	if status, ok := client.StatusCode(err); !ok || status != http.StatusTooManyRequests {
		return err
	}
	if wait, ok := client.RetryAfter(err); ok {
		return fmt.Errorf("the server is rate-limiting requests from here — try again in %s", wait.Round(time.Second))
	}
	return errors.New("the server is rate-limiting requests from here — try again in a minute")
}

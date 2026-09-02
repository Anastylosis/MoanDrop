package core

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

// ExplainError rewords the one server error a user can act on by waiting:
// a 429 becomes "rate limited, try again in Ns", using the server's own
// Retry-After (the exact wait until the budget has a slot; see API.md)
// when it sent one. Every other error passes through untouched, so both
// surfaces can route all their errors here without changing any wording
// they already rely on.
func ExplainError(err error) error {
	if status, ok := client.StatusCode(err); !ok || status != http.StatusTooManyRequests {
		return err
	}
	if wait, ok := client.RetryAfter(err); ok {
		return fmt.Errorf("the server is rate-limiting requests from here — try again in %s", wait.Round(time.Second))
	}
	return errors.New("the server is rate-limiting requests from here — try again in a minute")
}

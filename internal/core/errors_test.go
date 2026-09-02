package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

// versionErr fetches GET /api/v1/version from a server answering status
// with the given headers, returning the client's error for it.
func versionErr(t *testing.T, status int, headers map[string]string) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	_, err := client.New(srv.URL, "").Version(context.Background())
	if err == nil {
		t.Fatalf("HTTP %d produced no error", status)
	}
	return err
}

func TestExplainError_RateLimitUsesRetryAfter(t *testing.T) {
	err := ExplainError(versionErr(t, http.StatusTooManyRequests, map[string]string{"Retry-After": "12"}))
	if got := err.Error(); !strings.Contains(got, "rate-limiting") || !strings.Contains(got, "12s") {
		t.Errorf("err = %q, want the rate-limit wording with the server's 12s wait", got)
	}
}

func TestExplainError_RateLimitWithoutHeaderStillReadsAsWait(t *testing.T) {
	err := ExplainError(versionErr(t, http.StatusTooManyRequests, nil))
	if got := err.Error(); !strings.Contains(got, "rate-limiting") || !strings.Contains(got, "in a minute") {
		t.Errorf("err = %q, want the rate-limit wording with a generic wait", got)
	}
}

func TestExplainError_PassesOtherErrorsThrough(t *testing.T) {
	orig := versionErr(t, http.StatusInternalServerError, map[string]string{"Retry-After": "12"})
	if got := ExplainError(orig); !errors.Is(got, orig) {
		t.Errorf("a non-429 error was reworded: %v", got)
	}
	plain := errors.New("disk full")
	if got := ExplainError(plain); !errors.Is(got, plain) {
		t.Errorf("a non-HTTP error was reworded: %v", got)
	}
	if got := ExplainError(nil); got != nil {
		t.Errorf("ExplainError(nil) = %v", got)
	}
}

package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

// instantSleep records waits instead of sleeping.
func instantSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	orig := sleepCtx
	sleepCtx = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	t.Cleanup(func() { sleepCtx = orig })
	return &waits
}

// limitedServer answers 429 (with retryAfter, if set) n times, then serves.
func limitedServer(t *testing.T, n int32, retryAfter string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	inner := batchServer(t, []client.Release{relFor(1, 'a', "en")}, new(atomic.Int32))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= n {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		http.Redirect(w, r, inner.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWithRetry429_HonorsRetryAfterThenSucceeds(t *testing.T) {
	waits := instantSleep(t)
	var calls atomic.Int32
	srv := limitedServer(t, 2, "3", &calls)
	var told []time.Duration
	var track *client.Track
	err := WithRetry429(context.Background(), func(d time.Duration) { told = append(told, d) }, func() (e error) {
		track, e = client.New(srv.URL, "").GetTrack(context.Background(), 1)
		return e
	})
	if err != nil || track == nil {
		t.Fatalf("err = %v, track = %v", err, track)
	}
	if len(*waits) != 2 || (*waits)[0] != 3*time.Second || len(told) != 2 {
		t.Errorf("waits = %v told = %v", *waits, told)
	}
}

func TestWithRetry429_BacksOffWithoutHeader(t *testing.T) {
	waits := instantSleep(t)
	var calls atomic.Int32
	srv := limitedServer(t, 3, "", &calls)
	err := WithRetry429(context.Background(), nil, func() error {
		_, e := client.New(srv.URL, "").GetTrack(context.Background(), 1)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i, w := range want {
		if (*waits)[i] != w {
			t.Fatalf("waits = %v, want %v", *waits, want)
		}
	}
}

func TestWithRetry429_GivesUpAfterMaxAttempts(t *testing.T) {
	waits := instantSleep(t)
	var calls atomic.Int32
	srv := limitedServer(t, 100, "1", &calls)
	err := WithRetry429(context.Background(), nil, func() error {
		_, e := client.New(srv.URL, "").GetTrack(context.Background(), 1)
		return e
	})
	if !isRateLimited(err) {
		t.Fatalf("err = %v, want the 429", err)
	}
	if got := calls.Load(); got != maxRetry429Attempts+1 || len(*waits) != maxRetry429Attempts {
		t.Errorf("calls = %d waits = %v", got, *waits)
	}
}

func TestWithRetry429_GivesUpOnLongRetryAfter(t *testing.T) {
	waits := instantSleep(t)
	var calls atomic.Int32
	srv := limitedServer(t, 100, "600", &calls)
	err := WithRetry429(context.Background(), nil, func() error {
		_, e := client.New(srv.URL, "").GetTrack(context.Background(), 1)
		return e
	})
	if !isRateLimited(err) || calls.Load() != 1 || len(*waits) != 0 {
		t.Errorf("err = %v calls = %d waits = %v", err, calls.Load(), *waits)
	}
}

func TestWithRetry429_StopsWhenTotalWaitExceeded(t *testing.T) {
	waits := instantSleep(t)
	var calls atomic.Int32
	srv := limitedServer(t, 100, "30", &calls)
	_ = WithRetry429(context.Background(), nil, func() error {
		_, e := client.New(srv.URL, "").GetTrack(context.Background(), 1)
		return e
	})
	if len(*waits) != 3 {
		t.Errorf("waits = %v, want 3 x 30s", *waits)
	}
}

func TestWithRetry429_CancelDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	orig := sleepCtx
	sleepCtx = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	t.Cleanup(func() { sleepCtx = orig })
	var calls atomic.Int32
	srv := limitedServer(t, 100, "1", &calls)
	err := WithRetry429(ctx, nil, func() error {
		_, e := client.New(srv.URL, "").GetTrack(context.Background(), 1)
		return e
	})
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Errorf("err = %v calls = %d", err, calls.Load())
	}
}

func TestWithRetry429_OtherErrorsReturnAtOnce(t *testing.T) {
	waits := instantSleep(t)
	n := 0
	err := WithRetry429(context.Background(), nil, func() error { n++; return context.DeadlineExceeded })
	if err == nil || n != 1 || len(*waits) != 0 {
		t.Errorf("err = %v n = %d", err, n)
	}
}

func TestRunBatch_RetriesRateLimitedLookup(t *testing.T) {
	waits := instantSleep(t)
	var calls atomic.Int32
	srv := limitedServer(t, 1, "2", &calls)
	dir := t.TempDir()
	videos := []string{mkVideo(t, dir, "a.mp4", 'a')}
	var busy []time.Duration
	sum := RunBatch(context.Background(), client.New(srv.URL, ""), videos, BatchOptions{
		Fingerprint: fakeFP, Langs: []string{"en"}, Write: true,
		OnBusy: func(d time.Duration) { busy = append(busy, d) },
	})
	if sum.Counts[BatchWritten] != 1 {
		t.Fatalf("counts = %v, results = %v", sum.Counts, sum.Results)
	}
	if len(*waits) != 1 || len(busy) != 1 || busy[0] != 2*time.Second {
		t.Errorf("waits = %v busy = %v", *waits, busy)
	}
}

func TestRunBatch_RetriesRateLimitedDownload(t *testing.T) {
	waits := instantSleep(t)
	inner := batchServer(t, []client.Release{relFor(1, 'a', "en")}, new(atomic.Int32))
	var dl atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lookup/batch" && dl.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		http.Redirect(w, r, inner.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	videos := []string{mkVideo(t, dir, "a.mp4", 'a')}
	sum := RunBatch(context.Background(), client.New(srv.URL, ""), videos, BatchOptions{
		Fingerprint: fakeFP, Langs: []string{"en"}, Write: true,
	})
	if sum.Counts[BatchWritten] != 1 || len(*waits) != 1 {
		t.Fatalf("counts = %v waits = %v", sum.Counts, *waits)
	}
}

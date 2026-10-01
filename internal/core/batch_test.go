package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
	"github.com/Anastylosis/MoanSubs/hash"
)

// batchServer serves the batch lookup from releases (by oshash prefix) and
// subtitle downloads, counting lookup requests.
func batchServer(t *testing.T, releases []client.Release, lookups *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/lookup/batch" {
			lookups.Add(1)
			var req struct {
				OshashPrefixes []string `json:"oshash_prefixes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			res := map[string][]client.Release{}
			for _, p := range req.OshashPrefixes {
				for _, rel := range releases {
					if strings.HasPrefix(rel.OSHash, p) {
						res["oshash:"+p] = append(res["oshash:"+p], rel)
					}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": res})
			return
		}
		_ = json.NewEncoder(w).Encode(client.Track{ID: 1, Lang: "en", Body: "1\n00:00:01,000 --> 00:00:02,000\nhi\n"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeFP derives an oshash from the file's first byte so tests pick which
// release a video "is" without ffmpeg.
func fakeFP(_ context.Context, path string) (Fingerprint, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Fingerprint{}, err
	}
	if string(b) == "boom" {
		return Fingerprint{}, errors.New("fingerprint failed")
	}
	oh, err := hash.ParseOSHash(fmt.Sprintf("%016x", int(b[0])*0x1111111111))
	if err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{OSHash: oh}, nil
}

func relFor(id int64, first byte, langs ...string) client.Release {
	r := client.Release{ID: id, OSHash: fmt.Sprintf("%016x", int(first)*0x1111111111)}
	for i, l := range langs {
		r.Tracks = append(r.Tracks, client.TrackSummary{ID: id*10 + int64(i), Lang: l})
	}
	return r
}

func mkVideo(t *testing.T, dir, name string, first byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte{first}, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunBatch_WritesSkipsAndClassifies(t *testing.T) {
	var lookups atomic.Int32
	srv := batchServer(t, []client.Release{
		relFor(1, 'a', "en"),
		relFor(2, 'b', "de"),
		relFor(3, 'c', "en"), relFor(4, 'c', "en"),
	}, &lookups)
	dir := t.TempDir()
	matched := mkVideo(t, dir, "matched.mp4", 'a')
	nolang := mkVideo(t, dir, "nolang.mp4", 'b')
	ambiguous := mkVideo(t, dir, "ambiguous.mp4", 'c')
	nomatch := mkVideo(t, dir, "nomatch.mp4", 'z')
	had := mkVideo(t, dir, "had.mp4", 'a')
	if err := os.WriteFile(filepath.Join(dir, "had.en.srt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken.mp4")
	if err := os.WriteFile(broken, []byte("boom"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var seen []string
	sum := RunBatch(context.Background(), client.New(srv.URL, ""),
		[]string{matched, nolang, ambiguous, nomatch, had, broken},
		BatchOptions{Langs: []string{"en"}, Write: true, Jobs: 3, Fingerprint: fakeFP,
			OnResult: func(r BatchResult) { mu.Lock(); seen = append(seen, r.Path); mu.Unlock() }})

	want := map[string]BatchStatus{
		matched: BatchWritten, nolang: BatchNoMatch, ambiguous: BatchAmbiguous,
		nomatch: BatchNoMatch, had: BatchHad, broken: BatchError,
	}
	for _, r := range sum.Results {
		if r.Status != want[r.Path] {
			t.Errorf("%s: status %q, want %q (%s)", filepath.Base(r.Path), r.Status, want[r.Path], r.Detail)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("OnResult called %d times, want %d", len(seen), len(want))
	}
	if b, err := os.ReadFile(filepath.Join(dir, "matched.en.srt")); err != nil || !strings.Contains(string(b), "hi") {
		t.Errorf("matched sidecar = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ambiguous.en.srt")); err == nil {
		t.Error("an ambiguous match must never be written")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "had.en.srt")); string(b) != "mine" {
		t.Errorf("existing sidecar was touched: %q", b)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("lookup requests = %d, want 1 for 5 videos", got)
	}
	if sum.ExitCode() != 1 {
		t.Errorf("ExitCode = %d, want 1 (one error)", sum.ExitCode())
	}
	line := sum.Line(true)
	if !strings.Contains(line, "1 written") || !strings.Contains(line, "1 already had") || !strings.Contains(line, "1 ambiguous") || !strings.Contains(line, "2 no match") || !strings.Contains(line, "1 error") {
		t.Errorf("summary = %q", line)
	}
}

func TestRunBatch_ChunksLookups(t *testing.T) {
	var lookups atomic.Int32
	srv := batchServer(t, nil, &lookups)
	dir := t.TempDir()
	var videos []string
	for i := 0; i < BatchLookupVideos*2+1; i++ {
		videos = append(videos, mkVideo(t, dir, fmt.Sprintf("v%02d.mp4", i), byte('a'+i%20)))
	}
	sum := RunBatch(context.Background(), client.New(srv.URL, ""), videos, BatchOptions{Fingerprint: fakeFP})
	if got := lookups.Load(); got != 3 {
		t.Errorf("lookup requests = %d, want 3", got)
	}
	if sum.Counts[BatchNoMatch] != len(videos) || sum.ExitCode() != 2 {
		t.Errorf("counts = %v exit %d, want all no-match, exit 2", sum.Counts, sum.ExitCode())
	}
}

func TestRunBatch_NoWriteReportsMatch(t *testing.T) {
	var lookups atomic.Int32
	srv := batchServer(t, []client.Release{relFor(1, 'a', "en")}, &lookups)
	dir := t.TempDir()
	v := mkVideo(t, dir, "m.mp4", 'a')
	sum := RunBatch(context.Background(), client.New(srv.URL, ""), []string{v}, BatchOptions{Fingerprint: fakeFP})
	if sum.Results[0].Status != BatchMatched || sum.ExitCode() != 0 {
		t.Fatalf("got %+v exit %d", sum.Results[0], sum.ExitCode())
	}
	if _, err := os.Stat(filepath.Join(dir, "m.en.srt")); err == nil {
		t.Error("nothing may be written without Write")
	}
}

func TestRunBatch_RateLimitStopsAndReports(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	videos := []string{mkVideo(t, dir, "a.mp4", 'a'), mkVideo(t, dir, "b.mp4", 'b')}
	sum := RunBatch(context.Background(), client.New(srv.URL, ""), videos, BatchOptions{Fingerprint: fakeFP})
	if sum.Counts[BatchError] != 2 || sum.ExitCode() != 1 {
		t.Fatalf("counts = %v exit %d", sum.Counts, sum.ExitCode())
	}
	if d := sum.Results[0].Detail; !strings.Contains(d, "try again in 30s") {
		t.Errorf("detail = %q, want the Retry-After wording", d)
	}
}

func TestRunBatch_CancelStopsAndBoundsConcurrency(t *testing.T) {
	var lookups atomic.Int32
	srv := batchServer(t, nil, &lookups)
	dir := t.TempDir()
	var videos []string
	for i := 0; i < 8; i++ {
		videos = append(videos, mkVideo(t, dir, fmt.Sprintf("v%d.mp4", i), 'a'))
	}
	ctx, cancel := context.WithCancel(context.Background())
	var cur, peak atomic.Int32
	fp := func(ctx context.Context, p string) (Fingerprint, error) {
		n := cur.Add(1)
		defer cur.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return Fingerprint{}, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		cancel()
		return fakeFP(ctx, p)
	}
	sum := RunBatch(ctx, client.New(srv.URL, ""), videos, BatchOptions{Jobs: 2, Fingerprint: fp})
	if peak.Load() > 2 {
		t.Errorf("peak concurrency %d, want <= 2", peak.Load())
	}
	if sum.Counts[BatchError] == 0 || sum.ExitCode() != 1 {
		t.Errorf("a canceled run must report errors: counts %v", sum.Counts)
	}
	if len(sum.Results) != len(videos) {
		t.Errorf("results = %d, want one per video (%d)", len(sum.Results), len(videos))
	}
}

func TestConfidentMatch(t *testing.T) {
	ex := Candidate{Confidence: ConfidenceExact}
	hi := Candidate{Confidence: ConfidenceHigh}
	of := Candidate{Confidence: ConfidenceOffer}
	cases := []struct {
		name string
		in   []Candidate
		ok   bool
	}{
		{"none", nil, false},
		{"single exact", []Candidate{ex}, true},
		{"exact then offer", []Candidate{ex, of}, true},
		{"exact then high", []Candidate{ex, hi}, true},
		{"two high", []Candidate{hi, hi}, false},
		{"only offer", []Candidate{of}, false},
	}
	for _, c := range cases {
		if _, ok := ConfidentMatch(c.in); ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
		}
	}
}

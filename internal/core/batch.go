package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Anastylosis/MoanSubs/client"
)

// BatchLookupVideos is the videos per lookup: 6 bucket entries each, server takes 100 per request.
const BatchLookupVideos = 16

// BatchStatus is a video's outcome.
type BatchStatus string

// Batch outcomes.
const (
	BatchWritten   BatchStatus = "written"
	BatchHad       BatchStatus = "already had"
	BatchMatched   BatchStatus = "matched"
	BatchAmbiguous BatchStatus = "ambiguous"
	BatchNoMatch   BatchStatus = "no match"
	BatchError     BatchStatus = "error"
)

// BatchResult is one video's outcome.
type BatchResult struct {
	Path       string      `json:"path"`
	Status     BatchStatus `json:"status"`
	Detail     string      `json:"detail,omitempty"`
	ReleaseID  int64       `json:"release_id,omitempty"`
	Confidence string      `json:"confidence,omitempty"`
	Written    []string    `json:"written,omitempty"`
	Err        error       `json:"-"`
}

// Line renders the result.
func (r BatchResult) Line() string {
	s := fmt.Sprintf("%-11s %s", r.Status, r.Path)
	if r.Detail != "" {
		s += "  (" + r.Detail + ")"
	}
	return s
}

// BatchSummary tallies a batch.
type BatchSummary struct {
	Results []BatchResult
	Counts  map[BatchStatus]int
}

// Line renders the tally.
func (s BatchSummary) Line(write bool) string {
	order := []BatchStatus{BatchWritten, BatchHad, BatchAmbiguous, BatchNoMatch, BatchError}
	if !write {
		order = []BatchStatus{BatchMatched, BatchHad, BatchAmbiguous, BatchNoMatch, BatchError}
	}
	parts := make([]string, 0, len(order))
	for _, st := range order {
		parts = append(parts, fmt.Sprintf("%d %s", s.Counts[st], st))
	}
	return fmt.Sprintf("%d videos: %s", len(s.Results), strings.Join(parts, ", "))
}

// ExitCode is the process exit code.
func (s BatchSummary) ExitCode() int {
	switch {
	case s.Counts[BatchError] > 0:
		return 1
	case s.Counts[BatchWritten]+s.Counts[BatchMatched]+s.Counts[BatchHad] == 0:
		return 2
	}
	return 0
}

// BatchOptions configures RunBatch.
type BatchOptions struct {
	Langs           []string
	AllLangs        bool
	Write           bool
	Overwrite       bool
	Jobs            int
	FFmpeg, FFprobe string
	Fingerprint     func(ctx context.Context, path string) (Fingerprint, error)
	// Called once per video, never concurrently.
	OnResult func(BatchResult)
}

// ConfidentMatch never auto-writes offers or ties at the top tier.
func ConfidentMatch(candidates []Candidate) (Candidate, bool) {
	if len(candidates) == 0 || candidates[0].Confidence == ConfidenceOffer {
		return Candidate{}, false
	}
	if len(candidates) > 1 && candidates[1].Confidence == candidates[0].Confidence {
		return Candidate{}, false
	}
	return candidates[0], true
}

type fpResult struct {
	idx int
	fp  Fingerprint
	err error
}

// RunBatch runs a folder batch.
func RunBatch(ctx context.Context, c *client.Client, videos []string, opts BatchOptions) BatchSummary {
	if opts.Jobs < 1 {
		opts.Jobs = 2
	}
	fingerprint := opts.Fingerprint
	if fingerprint == nil {
		fingerprint = func(ctx context.Context, p string) (Fingerprint, error) {
			return FingerprintFile(ctx, opts.FFmpeg, opts.FFprobe, p)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]*BatchResult, len(videos))
	var abort error
	finish := func(i int, r BatchResult) {
		r.Path = videos[i]
		results[i] = &r
		if opts.OnResult != nil {
			opts.OnResult(r)
		}
	}
	fail := func(i int, err error) {
		finish(i, BatchResult{Status: BatchError, Err: ExplainError(err), Detail: ExplainError(err).Error()})
	}

	var todo []int
	for i, v := range videos {
		if !opts.Overwrite && HasSidecarFor(v, opts.Langs) {
			finish(i, BatchResult{Status: BatchHad, Detail: "sidecar already exists"})
			continue
		}
		todo = append(todo, i)
	}

	jobs := make(chan int)
	out := make(chan fpResult, opts.Jobs)
	var wg sync.WaitGroup
	for w := 0; w < opts.Jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				fp, err := fingerprint(ctx, videos[i])
				out <- fpResult{idx: i, fp: fp, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, i := range todo {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()

	var pending []fpResult
	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = nil
		if abort != nil {
			return
		}
		keys := make([]client.SceneKeys, len(batch))
		for k, p := range batch {
			keys[k] = client.SceneKeys{OSHash: p.fp.OSHash, PHash: p.fp.PHash}
		}
		perScene, err := c.LookupBucketsBatch(ctx, keys)
		if err != nil {
			if isRateLimited(err) || ctx.Err() != nil {
				abort = err
				cancel()
				return
			}
			for _, p := range batch {
				fail(p.idx, err)
			}
			return
		}
		for k, p := range batch {
			if abort != nil {
				return
			}
			finish(p.idx, resolveOne(ctx, c, videos[p.idx], p.fp, perScene[k], opts, &abort, cancel))
		}
	}

	for r := range out {
		if r.err != nil {
			fail(r.idx, r.err)
			continue
		}
		pending = append(pending, r)
		if len(pending) >= BatchLookupVideos {
			flush()
		}
	}
	flush()

	if abort == nil && ctx.Err() != nil {
		abort = ctx.Err()
	}
	if abort == nil {
		abort = errors.New("not processed")
	}
	sum := BatchSummary{Counts: map[BatchStatus]int{}}
	for i, r := range results {
		if r == nil {
			fail(i, abort)
			r = results[i]
		}
		sum.Results = append(sum.Results, *r)
		sum.Counts[r.Status]++
	}
	return sum
}

func isRateLimited(err error) bool {
	status, ok := client.StatusCode(err)
	return ok && status == http.StatusTooManyRequests
}

func resolveOne(ctx context.Context, c *client.Client, video string, fp Fingerprint, releases []client.Release, opts BatchOptions, abort *error, cancel context.CancelFunc) BatchResult {
	candidates := RankCandidates(releases, fp, false)
	if len(candidates) == 0 {
		return BatchResult{Status: BatchNoMatch}
	}
	top, ok := ConfidentMatch(candidates)
	if !ok {
		return BatchResult{Status: BatchAmbiguous, Detail: fmt.Sprintf("%d candidates; run match on this file to choose", len(candidates))}
	}
	res := BatchResult{ReleaseID: top.Release.ID, Confidence: top.Confidence}
	if !opts.Write {
		res.Status = BatchMatched
		res.Detail = fmt.Sprintf("release %d, %s", top.Release.ID, top.Confidence)
		return res
	}

	tracks := append([]client.TrackSummary(nil), top.Release.Tracks...)
	SortTracksByPreference(tracks, opts.Langs)
	selected := SelectTracks(tracks, opts.Langs, opts.AllLangs)
	if len(selected) == 0 {
		res.Status = BatchNoMatch
		res.Detail = fmt.Sprintf("release %d has no track in %s", top.Release.ID, strings.Join(opts.Langs, ", "))
		return res
	}
	forRelease := ForRelease(top)
	had := 0
	for _, t := range selected {
		d, err := DownloadTrack(ctx, c, video, t.ID, forRelease, t.Lang, opts.Overwrite)
		switch {
		case errors.Is(err, ErrSidecarExists):
			had++
		case err != nil:
			if isRateLimited(err) {
				*abort = err
				cancel()
			}
			res.Status = BatchError
			res.Err = ExplainError(err)
			res.Detail = res.Err.Error()
			return res
		default:
			res.Written = append(res.Written, d.Path)
		}
	}
	switch {
	case len(res.Written) > 0:
		res.Status = BatchWritten
		res.Detail = strings.Join(res.Written, ", ")
	case had > 0:
		res.Status = BatchHad
		res.Detail = "sidecar already exists"
	}
	return res
}

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anastylosis/MoanSubs/client"
)

// PushResult mirrors the server's upload outcome, worded once here so the
// CLI and GUI can never phrase the same event differently.
type PushResult struct {
	TrackID   int64
	ReleaseID int64
	Generated bool
	// Duplicate means a byte-identical track already existed server-side —
	// not an error, and not new sharing (the server never duplicates
	// identical bytes).
	Duplicate bool
}

// Message is the one-line outcome both surfaces print/show verbatim.
func (r PushResult) Message() string {
	switch {
	case r.Duplicate:
		return fmt.Sprintf("already on the node: track %d (release %d) — nothing new to share", r.TrackID, r.ReleaseID)
	case r.Generated:
		return fmt.Sprintf("uploaded as track %d (release %d), detected as AI-generated", r.TrackID, r.ReleaseID)
	default:
		return fmt.Sprintf("uploaded as track %d (release %d)", r.TrackID, r.ReleaseID)
	}
}

// ReadSubtitle reads a subtitle file and enforces the server's size cap.
// Callers run it BEFORE resolving ffmpeg so an oversized file fails with
// its own error instead of hiding behind an unrelated fingerprinting one.
func ReadSubtitle(subPath string) ([]byte, error) {
	body, err := os.ReadFile(subPath)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxTrackBytes {
		return nil, fmt.Errorf("%s is %d bytes, over the server's %d byte cap for a subtitle", subPath, len(body), MaxTrackBytes)
	}
	return body, nil
}

// PushSidecar fingerprints videoPath and uploads body (a subtitle read via
// ReadSubtitle) as a track for it. lang must already be resolved (the
// CLI's InferSidecarLang-or-flag choice, the GUI's inferred-or-asked
// choice) — callers own that decision because only they know how to ask
// the user for one; PushSidecar just refuses to guess with an empty tag.
// ffmpegPath/ffprobePath empty (mirroring FingerprintFile) uploads with
// the exact file hash only.
func PushSidecar(ctx context.Context, c *client.Client, videoPath, lang string, body []byte, ffmpegPath, ffprobePath string) (PushResult, error) {
	if lang == "" {
		return PushResult{}, fmt.Errorf("no language given for the subtitle")
	}

	fp, err := FingerprintFile(ctx, ffmpegPath, ffprobePath, videoPath)
	if err != nil {
		return PushResult{}, err
	}

	req := client.UploadRequest{
		OSHash:     fp.OSHash.String(),
		DurationMs: fp.DurationMs,
		Lang:       lang,
		Body:       string(body),
		// The filename stem feeds the server's name-based fallback matching;
		// it is the one piece of metadata a non-Stash user reliably has.
		Stem: strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)),
	}
	if fp.PHash != nil {
		req.PHash = fp.PHash.String()
	}

	res, err := c.Upload(ctx, req)
	if err != nil {
		return PushResult{}, err
	}
	return PushResult{TrackID: res.TrackID, ReleaseID: res.ReleaseID, Generated: res.Generated, Duplicate: res.Duplicate}, nil
}

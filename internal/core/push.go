package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anastylosis/MoanSubs/client"
)

// Authorship values are the server's closed upload-time vocabulary (feature
// "authorship"): declared, never enforced.
const (
	AuthorshipShared     = "shared"
	AuthorshipCredited   = "credited"
	AuthorshipUncredited = "uncredited"
)

// AuthorshipOrder is the order both surfaces present the choices in — the
// server's default first.
var AuthorshipOrder = []string{AuthorshipShared, AuthorshipCredited, AuthorshipUncredited}

// AuthorshipDescriptions is each choice's one-line wording, shared so the
// CLI's flag help and the GUI's radio labels read the same.
var AuthorshipDescriptions = map[string]string{
	AuthorshipShared:     "shared: passing along a file you found (no claim)",
	AuthorshipCredited:   "credited: you made it, and your account name shows beside it",
	AuthorshipUncredited: "uncredited: you made it, but want no public credit",
}

// ValidateAuthorship accepts the empty string (say nothing; the server keeps
// its default or, on a re-upload, the stored value) or one of the closed
// vocabulary, so a typo fails before any fingerprinting or network.
func ValidateAuthorship(v string) error {
	if v == "" || AuthorshipDescriptions[v] != "" {
		return nil
	}
	return fmt.Errorf("authorship %q: want one of %s", v, strings.Join(AuthorshipOrder, ", "))
}

// GeneratedDeclarationLabel is the voluntary AI-generated declaration's
// wording, shown beside the GUI's checkbox and in the CLI's flag help. It
// only ever adds the label: the server ORs it with its own detection and no
// later upload can clear it, so the wording says so up front.
const GeneratedDeclarationLabel = "this subtitle is AI-generated (adds the AI label; a declaration can't be withdrawn later)"

// PushOptions is what an uploader says about a subtitle beyond its language:
// both fields are omitted from the request when zero, so a server predating
// the authorship feature sees exactly the request it always did.
type PushOptions struct {
	// Authorship is "" (say nothing) or one of AuthorshipOrder.
	Authorship string
	// Generated is the voluntary AI-generated declaration.
	Generated bool
}

// PushResult mirrors the server's upload outcome, worded once here so the
// CLI and GUI can never phrase the same event differently.
type PushResult struct {
	TrackID   int64
	ReleaseID int64
	Generated bool
	// GeneratedSource is GeneratedSourceProvenance or GeneratedSourceDeclared
	// when Generated is set; empty on a server predating the distinction.
	GeneratedSource string
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
	case r.Generated && r.GeneratedSource == GeneratedSourceDeclared:
		return fmt.Sprintf("uploaded as track %d (release %d), labelled AI-generated as you declared", r.TrackID, r.ReleaseID)
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
// the exact file hash only. opts is the uploader's authorship/declaration,
// validated here so no surface can send a value the server would refuse.
func PushSidecar(ctx context.Context, c *client.Client, videoPath, lang string, body []byte, ffmpegPath, ffprobePath string, opts PushOptions) (PushResult, error) {
	if lang == "" {
		return PushResult{}, fmt.Errorf("no language given for the subtitle")
	}
	if err := ValidateAuthorship(opts.Authorship); err != nil {
		return PushResult{}, err
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
		Stem:       strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)),
		Authorship: opts.Authorship,
		Generated:  opts.Generated,
	}
	if fp.PHash != nil {
		req.PHash = fp.PHash.String()
	}

	res, err := c.Upload(ctx, req)
	if err != nil {
		return PushResult{}, err
	}
	return PushResult{
		TrackID: res.TrackID, ReleaseID: res.ReleaseID,
		Generated: res.Generated, GeneratedSource: res.GeneratedSource,
		Duplicate: res.Duplicate,
	}, nil
}

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anastylosis/MoanSubs/client"
)

// ErrSidecarExists identifies WriteSidecar's refusal to clobber an existing
// caption, so a caller — the GUI, specifically — can catch just this case
// (errors.Is) and ask "replace it?" instead of surfacing a raw error.
var ErrSidecarExists = errors.New("sidecar already exists")

// sidecarExistsError carries the path so its Error() text is byte-identical
// to what WriteSidecar has always returned (the CLI prints it verbatim),
// while still unwrapping to ErrSidecarExists for callers that need to test
// for this one case rather than parse the message.
type sidecarExistsError struct {
	path string
}

func (e *sidecarExistsError) Error() string {
	return fmt.Sprintf("caption %s already exists; pass --overwrite to replace it", e.path)
}

func (e *sidecarExistsError) Unwrap() error { return ErrSidecarExists }

// MaxTrackBytes mirrors the moansubs server's own upload cap
// (internal/subtitle.MaxBytes, 2 MiB). A track body over it here is lying
// about its size, not a legitimate subtitle to write next to a video.
const MaxTrackBytes = 2 * 1024 * 1024

// SidecarPath computes the caption path for a video file: same directory,
// same stem, `.<base>.srt` suffix — the convention Plex, Jellyfin, Kodi and
// VLC all pick up without a scan step.
func SidecarPath(videoPath string, lang CaptionLang) string {
	ext := filepath.Ext(videoPath)
	stem := strings.TrimSuffix(videoPath, ext)
	return fmt.Sprintf("%s.%s.srt", stem, lang.Base)
}

// WriteSidecar writes body next to the video file. It refuses to overwrite
// an existing caption unless overwrite is set — the existing file may be a
// hand-made subtitle, which must never be destroyed by a tool.
//
// Returns the written path and whether the file is genuinely new (false
// when an existing caption was overwritten in place).
func WriteSidecar(videoPath string, lang CaptionLang, body string, overwrite bool) (path string, created bool, err error) {
	path = SidecarPath(videoPath, lang)
	_, statErr := os.Stat(path)
	exists := statErr == nil

	if exists && !overwrite {
		return "", false, &sidecarExistsError{path: path}
	}

	if len(body) > MaxTrackBytes {
		return "", false, fmt.Errorf("track body is %d bytes, over the %d byte cap; refusing to write it", len(body), MaxTrackBytes)
	}

	if err := writeFileAtomic(path, []byte(body), 0o644); err != nil {
		return "", false, fmt.Errorf("writing sidecar: %w", err)
	}
	return path, !exists, nil
}

// DownloadResult is the outcome of downloading and writing one track,
// shared by the CLI's --write loop and the GUI's one-click download so the
// two can never format a success (or an AI/normalized-language footnote)
// differently.
type DownloadResult struct {
	Path      string
	Created   bool // false means an existing sidecar was replaced
	Generated bool
	Lang      CaptionLang
}

// DownloadTrack fetches trackID — retimed for forRelease when that is
// nonzero, the sibling-download path (see RankCandidates' SiblingOf) — and
// writes it beside videoPath. It refuses to overwrite an existing sidecar
// unless overwrite is set, returning an error wrapping ErrSidecarExists.
func DownloadTrack(ctx context.Context, c *client.Client, videoPath string, trackID, forRelease int64, langTag string, overwrite bool) (DownloadResult, error) {
	lang, err := ResolveCaptionLang(langTag)
	if err != nil {
		return DownloadResult{}, err
	}
	track, err := c.GetTrackFor(ctx, trackID, forRelease)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("downloading track %d: %w", trackID, err)
	}
	path, created, err := WriteSidecar(videoPath, lang, track.Body, overwrite)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Path: path, Created: created, Generated: track.Generated, Lang: lang}, nil
}

// Verb is the CLI's "wrote"/"replaced" choice, exposed so the GUI's success
// state uses the same word for the same outcome.
func (r DownloadResult) Verb() string {
	if r.Created {
		return "wrote"
	}
	return "replaced"
}

// Note is the footnote the CLI appends after a successful download — the
// AI-generated caveat, the normalized-language caveat, or both — so the
// GUI's success state cannot phrase either differently.
func (r DownloadResult) Note() string {
	note := ""
	if r.Generated {
		note = "  (AI-generated subtitle)"
	}
	if r.Lang.Normalized {
		note += fmt.Sprintf("  (%s stored as %s — sidecar names take bare language codes)", r.Lang.Original, r.Lang.Base)
	}
	return note
}

// writeFileAtomic writes data to a randomly-named temp file in target's
// directory, fsyncs and closes it, then renames it into place. A write that
// fails partway — disk full, permission denied — must never leave a
// truncated file at target: WriteSidecar's never-overwrite guard only ever
// checks the final name, so a half-written file there would be "protected"
// as if it were a real caption forever after. Any failure removes the temp
// file rather than leaving it behind.
func writeFileAtomic(target string, data []byte, perm os.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close()
		err = werr
		return err
	}
	if serr := tmp.Sync(); serr != nil {
		_ = tmp.Close()
		err = serr
		return err
	}
	if cerr := tmp.Close(); cerr != nil {
		err = cerr
		return err
	}
	if cherr := os.Chmod(tmpPath, perm); cherr != nil {
		err = cherr
		return err
	}
	if rerr := os.Rename(tmpPath, target); rerr != nil {
		err = rerr
		return err
	}
	return nil
}

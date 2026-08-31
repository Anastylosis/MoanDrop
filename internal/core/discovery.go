package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SidecarCandidate is one subtitle file FindSidecars found sitting beside a
// video, with whatever language its name reveals.
type SidecarCandidate struct {
	Path string
	// Lang is "" for a bare <stem>.srt — the writer never produces this
	// shape itself (WriteSidecar always names a language), but a hand-made
	// subtitle commonly does, so the caller must ask rather than assume one.
	Lang string
}

// sidecarExts are the extensions InferSidecarLang and this package's own
// writer both recognize as a subtitle sidecar.
var sidecarExts = []string{".srt", ".vtt"}

// FindSidecars lists the subtitle files already sitting next to videoPath —
// the naming rules SidecarPath writes and InferSidecarLang reads, in
// reverse: `<stem>.<lang>.srt` and bare `<stem>.srt`. The video itself and
// anything with an unrelated stem or an extra name segment that isn't a
// parseable language tag (a year, a resolution) are not sidecars and are
// left out rather than guessed at.
func FindSidecars(videoPath string) ([]SidecarCandidate, error) {
	dir := filepath.Dir(videoPath)
	videoBase := filepath.Base(videoPath)
	stem := strings.TrimSuffix(videoBase, filepath.Ext(videoBase))
	prefix := stem + "."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var out []SidecarCandidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == videoBase {
			continue
		}
		ext := filepath.Ext(name)
		if !hasSidecarExt(ext) {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		path := filepath.Join(dir, name)

		if base == stem {
			out = append(out, SidecarCandidate{Path: path})
			continue
		}
		if !strings.HasPrefix(base, prefix) {
			continue
		}
		lang := InferSidecarLang(name)
		if lang == "" {
			// The extra segment doesn't parse as a language (e.g. a year or
			// a resolution tag) — not the sidecar shape, so it's left alone
			// rather than offered with a guessed-empty language.
			continue
		}
		out = append(out, SidecarCandidate{Path: path, Lang: lang})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func hasSidecarExt(ext string) bool {
	for _, e := range sidecarExts {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

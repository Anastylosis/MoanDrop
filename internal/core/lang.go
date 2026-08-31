package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/text/language"
)

// CaptionLang resolves a stored language tag (full BCP-47, possibly
// regional like pt-BR) to the bare ISO 639 subtag a sidecar filename needs.
// Media players — and Stash, should the file ever meet one — parse the
// suffix as a bare base subtag; anything else silently never attaches, so
// this is validated before any file is written, with a loud error instead.
//
// Regional tags lose their region: pt-BR becomes pt, and the caller is told
// (Normalized) so the UI can say so.
type CaptionLang struct {
	// Base is the bare subtag written into the filename, e.g. "pt".
	Base string
	// Normalized is true when region/script information was dropped.
	Normalized bool
	// Original is the tag as stored on the track, e.g. "pt-BR".
	Original string
}

// ResolveCaptionLang validates and normalizes tag. It fails rather than
// guessing: writing a sidecar with an unparseable suffix produces a file,
// an empty player, and nothing in any log.
func ResolveCaptionLang(tag string) (CaptionLang, error) {
	if tag == "" {
		return CaptionLang{}, fmt.Errorf("track has no language tag; refusing to write a sidecar that would never attach")
	}
	b, err := baseSubtag(tag)
	if err != nil {
		return CaptionLang{}, fmt.Errorf("language tag %q has no usable base subtag (%w); refusing to write a sidecar that would never attach", tag, err)
	}
	return CaptionLang{
		Base:       b,
		Normalized: !strings.EqualFold(b, tag),
		Original:   tag,
	}, nil
}

// baseSubtag reduces a BCP-47 tag to its base language subtag — the same
// reduction MoanSubs' server applies to download filenames, so the two can
// never disagree on what a caption file is called. x/text can widen
// 2-letter codes to 3-letter bases for exotic tags; both widths attach.
func baseSubtag(tag string) (string, error) {
	t, err := language.Parse(tag)
	if err != nil {
		return "", err
	}
	b, conf := t.Base()
	if conf == language.No {
		return "", fmt.Errorf("no identifiable base language in %q", tag)
	}
	return b.String(), nil
}

// InferSidecarLang reads the language out of a subtitle filename shaped
// like `<stem>.<lang>.srt` (or .vtt), the sidecar convention this tool
// itself writes. Returns "" when the name carries no parseable tag — the
// caller must then ask for an explicit language rather than guess.
func InferSidecarLang(subtitlePath string) string {
	name := filepath.Base(subtitlePath)
	name = strings.TrimSuffix(name, filepath.Ext(name)) // drop .srt/.vtt
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	tag := name[i+1:]
	// A stem like "Scene.Title.2019" ends in something tag-shaped often
	// enough that only short, cleanly-parsing candidates are trusted.
	if len(tag) < 2 || len(tag) > 8 {
		return ""
	}
	if _, err := baseSubtag(tag); err != nil {
		return ""
	}
	return tag
}

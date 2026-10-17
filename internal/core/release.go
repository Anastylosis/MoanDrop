package core

import (
	"fmt"
	"strings"

	"github.com/Anastylosis/MoanSubs/client"
)

// ReleaseLabel is the card title: the server's display title (curated by
// a human, else derived from a cleaned upload filename) when the lookup
// carries one, else a descriptor built from what it always carries:
// resolution, runtime, codec.
func ReleaseLabel(r client.Release) string {
	if r.Title != "" {
		return r.Title
	}
	var parts []string
	if r.Height != nil && *r.Height > 0 {
		parts = append(parts, fmt.Sprintf("%dp", *r.Height))
	}
	if secs := r.DurationMs / 1000; secs > 0 {
		if secs >= 3600 {
			parts = append(parts, fmt.Sprintf("%d:%02d:%02d", secs/3600, secs%3600/60, secs%60))
		} else {
			parts = append(parts, fmt.Sprintf("%d:%02d", secs/60, secs%60))
		}
	}
	if r.VideoCodec != nil && *r.VideoCodec != "" {
		parts = append(parts, *r.VideoCodec)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("release %d", r.ID)
	}
	return strings.Join(parts, " · ")
}

// ReleaseByline is the card's credits line: studio and performers as the
// server knows them, with the catalogue's own collapse — a studio that is
// just the sole performer's name reads once, not twice. Empty when the
// server sent neither (an older node simply has no byline).
func ReleaseByline(r client.Release) string {
	perfs := strings.Join(r.Performers, ", ")
	switch {
	case r.Studio == "":
		return perfs
	case len(r.Performers) == 1 && strings.EqualFold(r.Studio, r.Performers[0]):
		return r.Studio
	case perfs == "":
		return r.Studio
	default:
		return r.Studio + " — " + perfs
	}
}

package ui

import (
	"fmt"
	"strings"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// TrackRow is one clickable track in the candidate list: everything a click
// handler needs to call core.DownloadTrack, plus the AI badge text if any.
type TrackRow struct {
	Track      client.TrackSummary
	ForRelease int64
	Badge      string // core.LabelGenerated, or "" for a human-made track
	Tooltip    string // core.GeneratedExplainer when Badge is set
}

// CandidateRow is one release offered to the user; its evidence wording is
// pre-built from core.EvidenceParts so the view layer never invents its own phrasing.
type CandidateRow struct {
	Release    client.Release
	Confidence string
	Evidence   string // joined EvidenceParts, "" when there is nothing to add
	Tracks     []TrackRow
}

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

// BuildCandidateRows turns ranked candidates into what the window renders,
// without re-sorting: tracks keep RankCandidates' order (human-made before generated).
func BuildCandidateRows(candidates []core.Candidate) []CandidateRow {
	rows := make([]CandidateRow, 0, len(candidates))
	for _, c := range candidates {
		row := CandidateRow{
			Release:    c.Release,
			Confidence: c.Confidence,
			Evidence:   strings.Join(core.EvidenceParts(c), "; "),
		}
		forRelease := core.ForRelease(c)
		for _, t := range c.Release.Tracks {
			tr := TrackRow{Track: t, ForRelease: forRelease}
			if t.Generated {
				tr.Badge = core.LabelGenerated
				tr.Tooltip = core.GeneratedExplainer
			}
			row.Tracks = append(row.Tracks, tr)
		}
		rows = append(rows, row)
	}
	return rows
}

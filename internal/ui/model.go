package ui

import (
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

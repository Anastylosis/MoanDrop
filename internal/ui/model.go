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
	Badge      string // core.LabelGenerated/LabelDeclaredGenerated, or "" for a human-made track
	Tooltip    string // core.GeneratedExplainer when Badge is set
	Credit     string // core.CreditLine of the track's public credit, "" when none
	FromSearch bool   // found by title, so its sync is unknown
}

// CandidateRow is one release offered to the user; its evidence wording is
// pre-built from core.EvidenceParts so the view layer never invents its own phrasing.
type CandidateRow struct {
	Release    client.Release
	Confidence string
	Evidence   string // joined EvidenceParts, "" when there is nothing to add
	Tracks     []TrackRow
}

// ReleaseLabel delegates to core so the CLI words releases the same way.
func ReleaseLabel(r client.Release) string { return core.ReleaseLabel(r) }

// ReleaseByline delegates to core.
func ReleaseByline(r client.Release) string { return core.ReleaseByline(r) }

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
		row.Tracks = buildTrackRows(c.Release.Tracks, core.ForRelease(c), false)
		rows = append(rows, row)
	}
	return rows
}

func buildTrackRows(tracks []client.TrackSummary, forRelease int64, fromSearch bool) []TrackRow {
	out := make([]TrackRow, 0, len(tracks))
	for _, t := range tracks {
		tr := TrackRow{Track: t, ForRelease: forRelease, Credit: core.CreditLine(t.CreditedTo), FromSearch: fromSearch}
		if t.Generated {
			tr.Badge = core.GeneratedLabel(t.Generated, t.GeneratedSource)
			tr.Tooltip = core.GeneratedExplainer
		}
		out = append(out, tr)
	}
	return out
}

// SearchRow is one searched release; its tracks download with no retiming.
type SearchRow struct {
	Release client.Release
	Tracks  []TrackRow
}

// BuildSearchRows turns search hits into what the window renders.
func BuildSearchRows(hits []core.SearchHit) []SearchRow {
	rows := make([]SearchRow, 0, len(hits))
	for _, h := range hits {
		rows = append(rows, SearchRow{Release: h.Release, Tracks: buildTrackRows(h.Tracks, 0, true)})
	}
	return rows
}

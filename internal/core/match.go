package core

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
	"github.com/Anastylosis/MoanSubs/hash"
)

// Confidence levels for a candidate release, levels 1-4 (level 0, stash-box
// identity, is absent — a MoanDrop user has no stash-box ids, so matching for them is hash + duration only).
const (
	// ConfidenceExact: oshash matches — byte-identical file.
	ConfidenceExact = "exact"
	// ConfidenceHigh: phash within Hamming 4 AND duration within 1s — same
	// content, different encode.
	ConfidenceHigh = "high"
	// ConfidenceOffer: phash Hamming 5-8 with close duration, or a sibling
	// track authored against another cut — offered, never auto-applied.
	ConfidenceOffer = "offer"
)

// durationGate is the |Δduration| bound that upgrades a near phash to a
// trustworthy match.
const durationGate = time.Second

// Candidate is one release the local file might match, with the evidence.
type Candidate struct {
	Release    client.Release `json:"release"`
	Confidence string         `json:"confidence"`
	// HammingDistance is -1 for oshash-exact matches (not applicable).
	HammingDistance int `json:"hamming_distance"`
	// DurationDeltaMs is local duration minus release duration.
	DurationDeltaMs int64 `json:"duration_delta_ms"`
	// CrossRelease is true when the subtitle was timed against a different
	// encode than the local file — sync may be off.
	CrossRelease bool `json:"cross_release"`
	// SiblingOf is the release this candidate's track was authored against,
	// when that differs from the release the local file matched (zero
	// otherwise). Stronger than CrossRelease: a sibling is an asserted
	// grouping that can carry a measured correction, not just a near-phash guess.
	SiblingOf int64 `json:"sibling_of,omitempty"`
	// SiblingOffsetMs is the shift the server applies on download;
	// SiblingSyncKnown says whether one is recorded — unknown sync is offered but never presented as a fit.
	SiblingOffsetMs  int64 `json:"sibling_offset_ms,omitempty"`
	SiblingSyncKnown bool  `json:"sibling_sync_known,omitempty"`
	// SiblingOffsetSource is how the number came about: "measured"/"manual"
	// means somebody checked both files; "duration-delta" means it was
	// inferred from the runtime difference. The UI must not present the
	// latter as the former — a guess that looks like a measurement is how sync silently drifts.
	SiblingOffsetSource string `json:"sibling_offset_source,omitempty"`
	// SiblingSyncVerified is the server's fit-report verdict: enough
	// independent accounts confirmed the pairing plays in sync as served.
	SiblingSyncVerified bool `json:"sibling_sync_verified,omitempty"`
}

// RankCandidates filters lookup results down to real matches, client-side
// — true oshash equality and Hamming distances are computed only here, so
// the server never learns which candidate matched (the bucketed lookup's
// privacy guarantee).
//
// fromExactMode widens the fuzzy radius to Hamming 5-8 (offer-only), valid
// only when releases came from LookupExact's wider search.
func RankCandidates(releases []client.Release, fp Fingerprint, fromExactMode bool) []Candidate {
	var out []Candidate
	for _, r := range releases {
		deltaMs := fp.DurationMs - r.DurationMs

		if r.OSHash == fp.OSHash.String() {
			out = append(out, Candidate{
				Release: r, Confidence: ConfidenceExact,
				HammingDistance: -1, DurationDeltaMs: deltaMs,
			})
			out = append(out, siblingCandidates(r, deltaMs)...)
			continue
		}

		if fp.PHash == nil || r.PHash == nil {
			continue
		}
		rp, err := hash.ParsePHash(*r.PHash)
		if err != nil {
			continue
		}
		d := hash.Hamming(*fp.PHash, rp)
		absDelta := deltaMs
		if absDelta < 0 {
			absDelta = -absDelta
		}

		switch {
		case d <= 4 && absDelta <= durationGate.Milliseconds():
			out = append(out, Candidate{
				Release: r, Confidence: ConfidenceHigh, CrossRelease: true,
				HammingDistance: d, DurationDeltaMs: deltaMs,
			})
		case fromExactMode && d >= 5 && d <= 8 && absDelta <= 5*durationGate.Milliseconds():
			out = append(out, Candidate{
				Release: r, Confidence: ConfidenceOffer, CrossRelease: true,
				HammingDistance: d, DurationDeltaMs: deltaMs,
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return confidenceRank(out[i].Confidence) < confidenceRank(out[j].Confidence)
	})
	return out
}

func confidenceRank(c string) int {
	switch c {
	case ConfidenceExact:
		return 0
	case ConfidenceHigh:
		return 1
	default: // ConfidenceOffer
		return 2
	}
}

// siblingCandidates turns a matched release's sibling tracks into offers:
// they ride on the already-matched release rather than matching on their
// own, which is how this reaches a re-cut that phash cannot — a trimmed
// intro shifts every sampled frame, so two copies of one film can sit 14
// bits apart with no shared MIH block. Confidence stays ConfidenceOffer
// even with known sync: the grouping is advisory, never the same claim as a subtitle authored for this exact file.
func siblingCandidates(r client.Release, deltaMs int64) []Candidate {
	if len(r.Siblings) == 0 {
		return nil
	}
	out := make([]Candidate, 0, len(r.Siblings))
	for _, sb := range r.Siblings {
		// A sibling is presented as its own one-track release so the UI can
		// render it with the machinery it already has.
		rel := r
		rel.Tracks = []client.TrackSummary{{
			ID: sb.ID, Lang: sb.Lang, Generated: sb.Generated, Downloads: sb.Downloads,
		}}
		rel.Siblings = nil
		c := Candidate{
			Release: rel, Confidence: ConfidenceOffer,
			HammingDistance: -1, DurationDeltaMs: deltaMs,
			CrossRelease: true,
			SiblingOf:    sb.ReleaseID,
		}
		c.SiblingSyncVerified = sb.SyncVerified
		if sb.OffsetMs != nil {
			c.SiblingSyncKnown = true
			c.SiblingOffsetMs = *sb.OffsetMs
			if sb.OffsetFrom != nil {
				c.SiblingOffsetSource = *sb.OffsetFrom
			}
		}
		out = append(out, c)
	}
	return out
}

// ForRelease returns the release id GetTrackFor should retime a candidate's
// track against: the matched release's id for a sibling offer (so the
// server applies its recorded shift), or 0 for every other candidate.
func ForRelease(c Candidate) int64 {
	if c.SiblingOf != 0 {
		return c.Release.ID
	}
	return 0
}

// EvidenceParts explains why a candidate matched, in the wording both the
// CLI and GUI render verbatim (joined with "; " for the CLI's one-line
// form) — so verified, estimated, and unknown sync claims never blur between surfaces.
func EvidenceParts(c Candidate) []string {
	var parts []string
	switch {
	case c.SiblingOf != 0 && c.SiblingSyncKnown && c.SiblingOffsetSource != "duration-delta":
		parts = append(parts, fmt.Sprintf("another cut of this video, verified shift %+dms", c.SiblingOffsetMs))
	case c.SiblingOf != 0 && c.SiblingSyncKnown:
		parts = append(parts, fmt.Sprintf("another cut of this video, estimated shift %+dms — sync unverified", c.SiblingOffsetMs))
	case c.SiblingOf != 0:
		parts = append(parts, "another cut of this video — sync unknown")
	case c.Confidence == ConfidenceExact:
		parts = append(parts, "byte-identical file")
	case c.CrossRelease:
		parts = append(parts, fmt.Sprintf("same video, different encode (distance %d, Δ%+dms) — sync usually fine", c.HammingDistance, c.DurationDeltaMs))
	}
	// A separate signal from a measured shift — users saying it lined up as
	// served — appended so it never replaces the offset provenance above.
	if c.SiblingOf != 0 && c.SiblingSyncVerified {
		parts = append(parts, "sync confirmed by users")
	}
	return parts
}

// SortTracksByPreference stably orders tracks language-first, so the
// server's human-made-before-generated order survives ties. Never drops a track.
func SortTracksByPreference(tracks []client.TrackSummary, languages []string) {
	langRank := func(t client.TrackSummary) int {
		base, err := baseSubtag(t.Lang)
		if err != nil {
			return len(languages)
		}
		for i, l := range languages {
			if l == base {
				return i
			}
		}
		return len(languages)
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		return langRank(tracks[i]) < langRank(tracks[j])
	})
}

// SelectTracks picks at most one track per base language from an already
// preference-sorted list (languages is the preference order, allLanguages
// widens it to every language the release has). Grouping by base — not the
// raw tag — keeps variants like pt-BR/pt-PT, or default/sdh, from producing two sidecars that collide on disk.
func SelectTracks(tracks []client.TrackSummary, languages []string, allLanguages bool) []client.TrackSummary {
	seen := make(map[string]bool, len(tracks))
	var out []client.TrackSummary
	for _, t := range tracks {
		base, err := baseSubtag(t.Lang)
		if err != nil || seen[base] {
			continue
		}
		if !allLanguages && !slices.Contains(languages, base) {
			continue
		}
		seen[base] = true
		out = append(out, t)
	}
	return out
}

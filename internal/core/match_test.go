package core

import (
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
	"github.com/Anastylosis/MoanSubs/hash"
)

func mustPHash(t *testing.T, s string) *hash.PHash {
	t.Helper()
	p, err := hash.ParsePHash(s)
	if err != nil {
		t.Fatalf("ParsePHash(%q): %v", s, err)
	}
	return &p
}

func strptr(s string) *string { return &s }

func fpFor(t *testing.T, oshash, phash string, durationMs int64) Fingerprint {
	t.Helper()
	oh, err := hash.ParseOSHash(oshash)
	if err != nil {
		t.Fatalf("ParseOSHash: %v", err)
	}
	fp := Fingerprint{OSHash: oh, DurationMs: durationMs}
	if phash != "" {
		fp.PHash = mustPHash(t, phash)
	}
	return fp
}

func TestRankCandidates_Levels(t *testing.T) {
	const localOSHash = "00000000000000aa"
	local := fpFor(t, localOSHash, "00000000000000f0", 600_000)

	releases := []client.Release{
		// Exact oshash match — wins regardless of phash/duration.
		{ID: 1, OSHash: localOSHash, DurationMs: 700_000},
		// Hamming 1, Δ500ms — high confidence.
		{ID: 2, OSHash: "00000000000000bb", PHash: strptr("00000000000000f1"), DurationMs: 600_500},
		// Hamming 1 but Δ5s — fails the duration gate, dropped.
		{ID: 3, OSHash: "00000000000000cc", PHash: strptr("00000000000000f1"), DurationMs: 605_000},
		// Hamming 6, Δ500ms — offer, but only from exact mode.
		{ID: 4, OSHash: "00000000000000dd", PHash: strptr("00000000000000cf"), DurationMs: 600_500},
		// No phash on the release — cannot be fuzzy-matched.
		{ID: 5, OSHash: "00000000000000ee", DurationMs: 600_000},
	}

	bucketed := RankCandidates(releases, local, false)
	if len(bucketed) != 2 {
		t.Fatalf("bucketed: got %d candidates, want 2: %+v", len(bucketed), bucketed)
	}
	if bucketed[0].Release.ID != 1 || bucketed[0].Confidence != ConfidenceExact {
		t.Errorf("bucketed[0] = release %d %s, want release 1 exact", bucketed[0].Release.ID, bucketed[0].Confidence)
	}
	if bucketed[0].HammingDistance != -1 {
		t.Errorf("exact match HammingDistance = %d, want -1", bucketed[0].HammingDistance)
	}
	if bucketed[1].Release.ID != 2 || bucketed[1].Confidence != ConfidenceHigh || !bucketed[1].CrossRelease {
		t.Errorf("bucketed[1] = %+v, want release 2 high cross-release", bucketed[1])
	}

	exact := RankCandidates(releases, local, true)
	if len(exact) != 3 {
		t.Fatalf("exact mode: got %d candidates, want 3", len(exact))
	}
	if exact[2].Release.ID != 4 || exact[2].Confidence != ConfidenceOffer {
		t.Errorf("exact[2] = release %d %s, want release 4 offer", exact[2].Release.ID, exact[2].Confidence)
	}
}

func TestRankCandidates_NoPhashLocal(t *testing.T) {
	local := fpFor(t, "00000000000000aa", "", 0)
	releases := []client.Release{
		{ID: 1, OSHash: "00000000000000aa", DurationMs: 600_000},
		{ID: 2, OSHash: "00000000000000bb", PHash: strptr("00000000000000f1"), DurationMs: 600_000},
	}
	got := RankCandidates(releases, local, false)
	if len(got) != 1 || got[0].Confidence != ConfidenceExact {
		t.Fatalf("oshash-only: got %+v, want exactly the exact match", got)
	}
}

func TestRankCandidates_Siblings(t *testing.T) {
	offset := int64(1500)
	source := "measured"
	local := fpFor(t, "00000000000000aa", "", 600_000)
	releases := []client.Release{{
		ID: 1, OSHash: "00000000000000aa", DurationMs: 600_000,
		Tracks: []client.TrackSummary{{ID: 10, Lang: "en"}},
		Siblings: []client.Sibling{
			{ID: 20, Lang: "de", ReleaseID: 7, OffsetMs: &offset, OffsetFrom: &source},
			{ID: 21, Lang: "en", ReleaseID: 8},
		},
	}}

	got := RankCandidates(releases, local, false)
	if len(got) != 3 {
		t.Fatalf("got %d candidates, want exact + 2 sibling offers", len(got))
	}
	known := got[1]
	if known.Confidence != ConfidenceOffer || known.SiblingOf != 7 || !known.SiblingSyncKnown ||
		known.SiblingOffsetMs != 1500 || known.SiblingOffsetSource != "measured" {
		t.Errorf("known-sync sibling = %+v", known)
	}
	if len(known.Release.Tracks) != 1 || known.Release.Tracks[0].ID != 20 {
		t.Errorf("sibling should be presented as its own one-track release: %+v", known.Release.Tracks)
	}
	unknown := got[2]
	if unknown.SiblingSyncKnown || unknown.SiblingOf != 8 {
		t.Errorf("unknown-sync sibling = %+v", unknown)
	}
}

func TestEvidenceParts(t *testing.T) {
	verifiedOffset := int64(500)
	estOffset := int64(-250)
	measured, durationDelta := "measured", "duration-delta"

	cases := []struct {
		name string
		c    Candidate
		want []string
	}{
		{"exact", Candidate{Confidence: ConfidenceExact}, []string{"byte-identical file"}},
		{"cross-release", Candidate{Confidence: ConfidenceHigh, CrossRelease: true, HammingDistance: 3, DurationDeltaMs: 120},
			[]string{"same video, different encode (distance 3, Δ+120ms) — sync usually fine"}},
		{"sibling verified", Candidate{SiblingOf: 7, SiblingSyncKnown: true, SiblingOffsetMs: verifiedOffset, SiblingOffsetSource: measured},
			[]string{"another cut of this video, verified shift +500ms"}},
		{"sibling estimated (duration-delta source)", Candidate{SiblingOf: 7, SiblingSyncKnown: true, SiblingOffsetMs: estOffset, SiblingOffsetSource: durationDelta},
			[]string{"another cut of this video, estimated shift -250ms — sync unverified"}},
		{"sibling unknown sync", Candidate{SiblingOf: 7}, []string{"another cut of this video — sync unknown"}},
		{"plain high confidence, no evidence to add", Candidate{Confidence: ConfidenceHigh}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvidenceParts(tc.c)
			if len(got) != len(tc.want) {
				t.Fatalf("EvidenceParts(%+v) = %v, want %v", tc.c, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("part %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestForRelease(t *testing.T) {
	sibling := Candidate{SiblingOf: 7, Release: client.Release{ID: 42}}
	if got := ForRelease(sibling); got != 42 {
		t.Errorf("sibling: ForRelease = %d, want the matched release's id (42)", got)
	}
	plain := Candidate{Confidence: ConfidenceExact, Release: client.Release{ID: 1}}
	if got := ForRelease(plain); got != 0 {
		t.Errorf("non-sibling: ForRelease = %d, want 0 (author's own timing)", got)
	}
}

func TestSortAndSelectTracks(t *testing.T) {
	tracks := []client.TrackSummary{
		{ID: 1, Lang: "de"},
		{ID: 2, Lang: "en", Generated: true},
		{ID: 3, Lang: "en"},
		{ID: 4, Lang: "pt-BR"},
		{ID: 5, Lang: "pt-PT"},
	}
	SortTracksByPreference(tracks, []string{"en", "pt"})
	if tracks[0].Lang != "en" || tracks[0].ID != 2 {
		t.Errorf("sort must be stable within a language (server puts its preferred first): got %+v", tracks[0])
	}

	sel := SelectTracks(tracks, []string{"en", "pt"}, false)
	if len(sel) != 2 || sel[0].ID != 2 || sel[1].ID != 4 {
		t.Fatalf("SelectTracks = %+v, want one en and one pt (base-language dedup)", sel)
	}

	all := SelectTracks(tracks, nil, true)
	if len(all) != 3 {
		t.Errorf("all-languages: got %d tracks, want 3 (en, pt, de)", len(all))
	}
}

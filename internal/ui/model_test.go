package ui

import (
	"strings"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
	"github.com/Anastylosis/MoanSubs/hash"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

func mustPHash(t *testing.T, s string) *hash.PHash {
	t.Helper()
	p, err := hash.ParsePHash(s)
	if err != nil {
		t.Fatalf("ParsePHash(%q): %v", s, err)
	}
	return &p
}

func TestBuildCandidateRows_ExactAndGenerated(t *testing.T) {
	local := core.Fingerprint{DurationMs: 600_000}
	oh, err := hash.ParseOSHash("00000000000000aa")
	if err != nil {
		t.Fatal(err)
	}
	local.OSHash = oh

	releases := []client.Release{{
		ID: 1, OSHash: "00000000000000aa", DurationMs: 600_000,
		Tracks: []client.TrackSummary{
			{ID: 10, Lang: "en", Generated: false},
			{ID: 11, Lang: "de", Generated: true},
		},
	}}
	candidates := core.RankCandidates(releases, local, false)
	rows := BuildCandidateRows(candidates)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.Confidence != core.ConfidenceExact {
		t.Errorf("Confidence = %q, want exact", row.Confidence)
	}
	if !strings.Contains(row.Evidence, "byte-identical") {
		t.Errorf("Evidence = %q, want the CLI's byte-identical wording", row.Evidence)
	}
	if len(row.Tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(row.Tracks))
	}
	if row.Tracks[0].Badge != "" {
		t.Errorf("human-made track got badge %q", row.Tracks[0].Badge)
	}
	if row.Tracks[1].Badge != core.LabelGenerated {
		t.Errorf("generated track Badge = %q, want %q", row.Tracks[1].Badge, core.LabelGenerated)
	}
	if row.Tracks[1].Tooltip != core.GeneratedExplainer {
		t.Errorf("generated track Tooltip does not match core.GeneratedExplainer")
	}
	if row.Tracks[0].ForRelease != 0 || row.Tracks[1].ForRelease != 0 {
		t.Errorf("an ordinary exact match must not ask for a retime: %+v", row.Tracks)
	}
}

func TestBuildCandidateRows_SiblingUsesMatchedReleaseForRetime(t *testing.T) {
	offset := int64(500)
	source := "measured"
	oh, err := hash.ParseOSHash("00000000000000aa")
	if err != nil {
		t.Fatal(err)
	}
	local := core.Fingerprint{OSHash: oh, DurationMs: 600_000}
	releases := []client.Release{{
		ID: 42, OSHash: "00000000000000aa", DurationMs: 600_000,
		Tracks: []client.TrackSummary{{ID: 1, Lang: "en"}},
		Siblings: []client.Sibling{
			{ID: 99, Lang: "fr", ReleaseID: 7, OffsetMs: &offset, OffsetFrom: &source},
		},
	}}
	candidates := core.RankCandidates(releases, local, false)
	rows := BuildCandidateRows(candidates)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want exact + 1 sibling offer", len(rows))
	}
	sib := rows[1]
	if !strings.Contains(sib.Evidence, "verified shift") {
		t.Errorf("sibling Evidence = %q, want verified-shift wording", sib.Evidence)
	}
	if len(sib.Tracks) != 1 || sib.Tracks[0].ForRelease != 42 {
		t.Fatalf("sibling track must retime against the matched release (42): %+v", sib.Tracks)
	}
}

func TestBuildCandidateRows_NoEvidenceForPlainHighConfidence(t *testing.T) {
	oh, err := hash.ParseOSHash("00000000000000aa")
	if err != nil {
		t.Fatal(err)
	}
	local := core.Fingerprint{OSHash: oh, PHash: mustPHash(t, "00000000000000f0"), DurationMs: 600_000}
	releases := []client.Release{
		{ID: 2, OSHash: "00000000000000bb", PHash: strPtr("00000000000000f1"), DurationMs: 600_500},
	}
	rows := BuildCandidateRows(core.RankCandidates(releases, local, false))
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Evidence == "" {
		t.Errorf("cross-release high confidence should still explain itself")
	}
}

func strPtr(s string) *string { return &s }

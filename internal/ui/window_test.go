package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanSubs/client"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// walkObjects visits every text-bearing widget reachable from o, so a test
// can assert on wording without hard-coding the window's layout.
func walkObjects(o fyne.CanvasObject, visit func(text string)) {
	switch v := o.(type) {
	case nil:
		return
	case *widget.Label:
		visit(v.Text)
	case *widget.Button:
		visit(v.Text)
	case *widget.Card:
		visit(v.Title)
		visit(v.Subtitle)
		walkObjects(v.Content, visit)
	case *container.Scroll:
		walkObjects(v.Content, visit)
	case *fyne.Container:
		for _, c := range v.Objects {
			walkObjects(c, visit)
		}
	}
}

func TestBuild_ShowsPrivacyLineVerbatim(t *testing.T) {
	u := newTestApp(test.NewApp())

	found := false
	walkObjects(u.win.Content(), func(s string) {
		if strings.Contains(s, core.PrivacyLine) {
			found = true
		}
	})
	if !found {
		t.Errorf("window content never renders the exact privacy phrase %q", core.PrivacyLine)
	}
}

func TestRenderCandidates_ShowsExplainerOnlyWhenGenerated(t *testing.T) {
	u := newTestApp(test.NewApp())

	rows := []CandidateRow{{
		Release:    client.Release{ID: 1},
		Confidence: core.ConfidenceExact,
		Tracks: []TrackRow{
			{Track: client.TrackSummary{ID: 1, Lang: "en"}},
		},
	}}
	u.renderCandidates(rows)
	if len(u.list.Objects) != 1 {
		t.Fatalf("no generated tracks: got %d list children, want 1 (just the card)", len(u.list.Objects))
	}

	rows[0].Tracks = append(rows[0].Tracks, TrackRow{
		Track: client.TrackSummary{ID: 2, Lang: "de", Generated: true},
		Badge: core.LabelGenerated, Tooltip: core.GeneratedExplainer,
	})
	u.renderCandidates(rows)
	if len(u.list.Objects) != 2 {
		t.Fatalf("with a generated track: got %d list children, want 2 (card + explainer)", len(u.list.Objects))
	}
}

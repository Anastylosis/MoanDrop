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

// findRadioGroup finds the first *widget.RadioGroup reachable from o.
func findRadioGroup(o fyne.CanvasObject) *widget.RadioGroup {
	var found *widget.RadioGroup
	walkCanvas(o, func(c fyne.CanvasObject) {
		if found != nil {
			return
		}
		if rg, ok := c.(*widget.RadioGroup); ok {
			found = rg
		}
	})
	return found
}

func TestPromptSettings_PersistsServerTokenAndCloseBehavior(t *testing.T) {
	u := newTestApp(test.NewApp())
	p := u.app.Preferences()

	u.promptSettings()

	entries := []*widget.Entry{}
	walkCanvas(topOverlay(u.win), func(c fyne.CanvasObject) {
		if e, ok := c.(*widget.Entry); ok {
			entries = append(entries, e)
		}
	})
	if len(entries) != 2 {
		t.Fatalf("settings dialog has %d entries, want 2 (server, token)", len(entries))
	}
	// SetText, not test.Type: the server entry pre-fills with the current
	// value (core.DefaultServerURL here), and Type appends at the cursor
	// rather than replacing it.
	entries[0].SetText("https://example.invalid")
	entries[1].SetText("s3cr3t")

	rg := findRadioGroup(topOverlay(u.win))
	if rg == nil {
		t.Fatal("settings dialog has no close-behavior radio group")
	}
	rg.SetSelected(closeBehaviorQuitLabel)

	tapButtonOn(t, u.win, "Save")

	if got := serverURL(p); got != "https://example.invalid" {
		t.Errorf("serverURL() = %q, want the saved entry", got)
	}
	if got := token(p); got != "s3cr3t" {
		t.Errorf("token() = %q, want the saved entry", got)
	}
	if got := closeBehavior(p); got != closeBehaviorQuit {
		t.Errorf("closeBehavior() = %q, want %q", got, closeBehaviorQuit)
	}
}

func TestPromptSettings_DefaultsCloseBehaviorRadioToHide(t *testing.T) {
	u := newTestApp(test.NewApp())
	u.promptSettings()

	rg := findRadioGroup(topOverlay(u.win))
	if rg == nil {
		t.Fatal("settings dialog has no close-behavior radio group")
	}
	if rg.Selected != closeBehaviorHideLabel {
		t.Errorf("default radio selection = %q, want %q", rg.Selected, closeBehaviorHideLabel)
	}
}

func TestPromptSettings_CancelDiscardsChanges(t *testing.T) {
	u := newTestApp(test.NewApp())
	p := u.app.Preferences()
	setServerURL(p, "https://original.invalid")

	u.promptSettings()
	entries := []*widget.Entry{}
	walkCanvas(topOverlay(u.win), func(c fyne.CanvasObject) {
		if e, ok := c.(*widget.Entry); ok {
			entries = append(entries, e)
		}
	})
	entries[0].SetText("https://changed.invalid")

	tapButtonOn(t, u.win, "Cancel")

	if got := serverURL(p); got != "https://original.invalid" {
		t.Errorf("serverURL() after Cancel = %q, want the pre-dialog value untouched", got)
	}
}

// TestPromptSettings_SaveAppliesCloseBehaviorImmediately covers the spec
// requirement that the close-behavior choice takes effect on save, not
// just on the next launch: Save must call applyCloseBehavior on the live
// window, not merely persist the preference for next time.
// TestApplyCloseBehavior_* already covers what each pref value wires the
// intercept to; this test covers that promptSettings' Save actually
// triggers that wiring.
func TestPromptSettings_SaveAppliesCloseBehaviorImmediately(t *testing.T) {
	a := test.NewApp()
	cw := &captureWindow{Window: a.NewWindow("MoanDrop")}
	u := &appUI{app: a, win: cw}
	u.build()

	u.promptSettings()
	rg := findRadioGroup(topOverlay(u.win))
	if rg == nil {
		t.Fatal("settings dialog has no close-behavior radio group")
	}
	rg.SetSelected(closeBehaviorQuitLabel)
	tapButtonOn(t, u.win, "Save")

	if cw.intercept == nil {
		t.Fatal("Save must apply the close-behavior pref immediately (applyCloseBehavior sets the intercept unconditionally for the quit choice)")
	}
}

func TestPromptSettings_ReopenReflectsSavedQuit(t *testing.T) {
	u := newTestApp(test.NewApp())
	setCloseBehavior(u.app.Preferences(), closeBehaviorQuit)
	u.promptSettings()

	rg := findRadioGroup(topOverlay(u.win))
	if rg == nil {
		t.Fatal("settings dialog has no close-behavior radio group")
	}
	if rg.Selected != closeBehaviorQuitLabel {
		t.Errorf("radio selection = %q, want %q after saving quit", rg.Selected, closeBehaviorQuitLabel)
	}
}

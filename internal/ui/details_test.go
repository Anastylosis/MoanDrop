package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func findEntryByPlaceholder(o fyne.CanvasObject, ph string) *widget.Entry {
	var found *widget.Entry
	walkCanvas(o, func(c fyne.CanvasObject) {
		if e, ok := c.(*widget.Entry); ok && e.PlaceHolder == ph {
			found = e
		}
	})
	return found
}

func metadataServer(t *testing.T, features []string) (*httptest.Server, *[]client.MetadataEntry) {
	t.Helper()
	var got []client.MetadataEntry
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "test", "features": features})
	})
	mux.HandleFunc("POST /api/v1/metadata", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Entries []client.MetadataEntry `json:"entries"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = body.Entries
		_, _ = w.Write([]byte(`{"results":[{"release_id":1,"known":true,"recorded":true}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestAskShareOptions_SceneFieldsOnlyWhenOffered(t *testing.T) {
	win := dialogWindow()
	askShareOptions(win, "/v/scene.en.srt", "en", core.AuthorshipShared, func(core.PushOptions) {})
	if findEntryByPlaceholder(topOverlay(win), core.DateHintText) != nil {
		t.Error("scene fields shown on a node that was not asked for them")
	}
	tapButtonOn(t, win, "Cancel")

	var got *core.PushOptions
	askShareOptionsWithDetails(win, "/v/scene.en.srt", "en", core.AuthorshipShared, true, func(o core.PushOptions) { got = &o })
	ov := topOverlay(win)
	findEntryByPlaceholder(ov, "title").SetText(" A Title ")
	findEntryByPlaceholder(ov, core.DateHintText).SetText("2024-03-01")
	findEntryByPlaceholder(ov, core.PerformersPlaceholder).SetText("Alice, Bob")
	tapButtonOn(t, win, shareOptionsConfirm)
	if got == nil || got.Title != "A Title" || got.Date != "2024-03-01" || got.Studio != "" || len(got.Performers) != 2 {
		t.Errorf("opts = %+v", got)
	}
}

func TestAskShareOptions_BadDateBlocksShare(t *testing.T) {
	win := dialogWindow()
	called := false
	askShareOptionsWithDetails(win, "/v/scene.en.srt", "en", core.AuthorshipShared, true, func(core.PushOptions) { called = true })
	findEntryByPlaceholder(topOverlay(win), core.DateHintText).SetText("1/3/24")
	tapButtonOn(t, win, shareOptionsConfirm)
	if called {
		t.Error("a malformed date must not be shared")
	}
	if !strings.Contains(strings.Join(collectTexts(topOverlay(win)), "\n"), "YYYY-MM-DD") {
		t.Error("the date error must be shown")
	}
}

func untitledExact() []CandidateRow {
	return BuildCandidateRows([]core.Candidate{{
		Confidence: core.ConfidenceExact,
		Release:    client.Release{ID: 1, OSHash: "0123456789abcdef", Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}}},
	}})
}

func TestOfferDetails_SendsDetailsForTheExactFile(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, got := metadataServer(t, []string{"metadata"})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.renderCandidates(untitledExact())
	waitDo(t, doneCh) // feature probe
	tapButtonOn(t, u.win, core.AddSceneDetailsLabel)
	findEntryByPlaceholder(topOverlay(u.win), "title").SetText("Found Title")
	tapButtonOn(t, u.win, "Send")
	waitDo(t, doneCh)

	if len(*got) != 1 || (*got)[0].OSHash != "0123456789abcdef" || (*got)[0].Title != "Found Title" {
		t.Fatalf("server saw %+v", *got)
	}
	if !strings.Contains(u.status.Text, "recorded scene details for release 1") {
		t.Errorf("status = %q", u.status.Text)
	}
}

func TestOfferDetails_NoTokenAsksFirst(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, _ := metadataServer(t, []string{"metadata"})
	setServerURL(u.app.Preferences(), srv.URL)
	t.Setenv("MOANDROP_TOKEN", "")

	u.renderCandidates(untitledExact())
	waitDo(t, doneCh)
	tapButtonOn(t, u.win, core.AddSceneDetailsLabel)
	if findButton(topOverlay(u.win), "Save") == nil {
		t.Error("no token must open the token dialog before anything is sent")
	}
}

func TestOfferDetails_HiddenWhenNotApplicable(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, _ := metadataServer(t, []string{"lookup"})
	setServerURL(u.app.Preferences(), srv.URL)

	u.renderCandidates(untitledExact())
	waitDo(t, doneCh)
	if findButton(u.win.Content(), core.AddSceneDetailsLabel) != nil {
		t.Error("a node without the metadata feature must not be offered details")
	}

	srv2, _ := metadataServer(t, []string{"metadata"})
	u2 := newTestApp(test.NewApp())
	setServerURL(u2.app.Preferences(), srv2.URL)
	rows := untitledExact()
	rows[0].Release.Title = "Has Title"
	u2.renderCandidates(rows)
	rows = untitledExact()
	rows[0].Confidence = core.ConfidenceHigh
	u2.renderCandidates(rows)
	if findButton(u2.win.Content(), core.AddSceneDetailsLabel) != nil {
		t.Error("titled or non-exact releases must not be offered details")
	}
}

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func fitServer(t *testing.T, features []string) (*httptest.Server, *struct {
	puts    int
	fits    bool
	deletes int
	delete  string
}) {
	t.Helper()
	got := &struct {
		puts    int
		fits    bool
		deletes int
		delete  string
	}{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "test", "features": features})
	})
	mux.HandleFunc("PUT /api/v1/subtitles/{id}/fit", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ReleaseID int64 `json:"release_id"`
			Fits      bool  `json:"fits"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got.puts++
		got.fits = body.Fits
		_ = json.NewEncoder(w).Encode(map[string]int{"fits": 1, "misfits": 0})
	})
	mux.HandleFunc("DELETE /api/v1/subtitles/{id}/fit", func(w http.ResponseWriter, r *http.Request) {
		got.deletes++
		got.delete = r.PathValue("id") + "/" + r.URL.Query().Get("release_id")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, got
}

func TestOfferFitPrompt_SiblingShowsAfterFeatureProbe(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, _ := fitServer(t, []string{"lookup", "fit"})
	setServerURL(u.app.Preferences(), srv.URL)

	u.offerFitPrompt(TrackRow{Track: client.TrackSummary{ID: 9}, ForRelease: 4})
	waitDo(t, doneCh) // the probe's fyne.Do re-enters offerFitPrompt

	if findButton(u.win.Content(), "fits") == nil || findButton(u.win.Content(), "doesn't fit") == nil {
		t.Fatal("sibling download must offer the fit prompt once the feature probe passes")
	}
}

func TestOfferFitPrompt_NoSiblingOrNoFeatureStaysHidden(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, _ := fitServer(t, []string{"lookup"}) // no "fit"
	setServerURL(u.app.Preferences(), srv.URL)

	u.offerFitPrompt(TrackRow{Track: client.TrackSummary{ID: 9}}) // ForRelease == 0
	if findButton(u.win.Content(), "fits") != nil {
		t.Fatal("an own-release download must not prompt")
	}

	u.offerFitPrompt(TrackRow{Track: client.TrackSummary{ID: 9}, ForRelease: 4})
	waitDo(t, doneCh)
	if findButton(u.win.Content(), "fits") != nil {
		t.Fatal("a node without the fit feature must never be asked")
	}
}

func TestOnFitReport_SendsVerdictAndHidesPrompt(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, got := fitServer(t, []string{"fit"})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.offerFitPrompt(TrackRow{Track: client.TrackSummary{ID: 9}, ForRelease: 4})
	waitDo(t, doneCh)
	tapButtonOn(t, u.win, "fits")
	waitDo(t, doneCh)

	if got.puts != 1 || !got.fits {
		t.Fatalf("server saw puts=%d fits=%v, want one true verdict", got.puts, got.fits)
	}
	if findButton(u.win.Content(), "fits") != nil {
		t.Error("prompt must hide after a recorded verdict")
	}
	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), "recorded") {
		t.Error("status must acknowledge the recorded verdict")
	}
}

func TestOnFitReport_ShowsAnswerWithUndo(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, _ := fitServer(t, []string{"fit"})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.offerFitPrompt(TrackRow{Track: client.TrackSummary{ID: 9}, ForRelease: 4})
	waitDo(t, doneCh)
	tapButtonOn(t, u.win, "doesn't fit")
	waitDo(t, doneCh)

	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), core.FitAnswerLabel(false)) {
		t.Error("the reported verdict must stay visible")
	}
	if findButton(u.win.Content(), "undo") == nil {
		t.Error("a reported verdict must offer undo")
	}
}

func TestOnFitRetract_WithdrawsAndRestoresPrompt(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv, got := fitServer(t, []string{"fit"})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.offerFitPrompt(TrackRow{Track: client.TrackSummary{ID: 9}, ForRelease: 4})
	waitDo(t, doneCh)
	tapButtonOn(t, u.win, "fits")
	waitDo(t, doneCh)
	tapButtonOn(t, u.win, "undo")
	waitDo(t, doneCh)

	if got.deletes != 1 || got.delete != "9/4" {
		t.Fatalf("server saw deletes=%d target=%q, want one retract of 9/4", got.deletes, got.delete)
	}
	if findButton(u.win.Content(), "fits") == nil || findButton(u.win.Content(), "doesn't fit") == nil {
		t.Error("the prompt must return to its unanswered state")
	}
	if findButton(u.win.Content(), "undo") != nil {
		t.Error("undo must go away once the report is withdrawn")
	}
	if u.status.Text != core.FitRetractedMessage {
		t.Errorf("status = %q", u.status.Text)
	}
}

func TestOnFitRetract_NoTokenAsksForOne(t *testing.T) {
	u, _ := newFlowApp(t)
	t.Setenv("MOANDROP_TOKEN", "")
	srv, got := fitServer(t, []string{"fit"})
	setServerURL(u.app.Preferences(), srv.URL)

	u.onFitRetract(TrackRow{Track: client.TrackSummary{ID: 9}, ForRelease: 4})
	if findButton(topOverlay(u.win), "Save") == nil {
		t.Error("no token must open the token dialog")
	}
	if got.deletes != 0 {
		t.Error("nothing may be sent without a token")
	}
}

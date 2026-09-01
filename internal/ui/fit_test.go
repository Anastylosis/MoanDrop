package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

func fitServer(t *testing.T, features []string) (*httptest.Server, *struct {
	puts int
	fits bool
}) {
	t.Helper()
	got := &struct {
		puts int
		fits bool
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

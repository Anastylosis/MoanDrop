package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

const searchBody = "1\n00:00:01,000 --> 00:00:02,000\nhi\n"

// searchHandler serves no lookup match, the version probe, search and a track.
func searchHandler(features []string, searches *atomic.Int32, searchStatus int) http.HandlerFunc {
	nomatch := lookupBatchHandler(nil)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"features": features})
		case "/api/v1/search":
			searches.Add(1)
			if searchStatus != 0 {
				w.Header().Set("Retry-After", "600")
				http.Error(w, "slow", searchStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(client.SearchResult{Truncated: true, Releases: []client.Release{{
				ID: 9, Title: "Some Scene", Studio: "Studio X",
				Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}},
			}}})
		case "/api/v1/lookup/batch":
			nomatch(w, r)
		default:
			_ = json.NewEncoder(w).Encode(client.Track{ID: 5, Lang: "en", Body: searchBody})
		}
	}
}

func startNoMatch(t *testing.T, features []string, searchStatus int) (*appUI, chan struct{}, string, *atomic.Int32) {
	t.Helper()
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)
	var searches atomic.Int32
	srv := httptest.NewServer(searchHandler(features, &searches, searchStatus))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)
	video := tempVideo(t, "Some.Scene.1080p.mp4", 0xAB)
	u.startVideo(video)
	waitDo(t, doneCh) // ffmpeg-missing dialog
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // looking up
	waitDo(t, doneCh) // no match
	return u, doneCh, video, &searches
}

func TestNoMatch_OffersPrefilledSearchAndDownloads(t *testing.T) {
	u, doneCh, video, _ := startNoMatch(t, []string{core.FeatureSearch}, 0)
	waitDo(t, doneCh) // feature probe -> panel

	entry := findEntry(u.list)
	if entry == nil || entry.Text != "Some Scene" {
		t.Fatalf("search entry = %+v, want prefilled with the cleaned stem", entry)
	}
	tapButtonOn(t, u.win, "Search")
	waitDo(t, doneCh)

	all := strings.Join(collectTexts(u.list), "\n")
	for _, want := range []string{"Some Scene", "Studio X", core.SearchSyncUnknown, core.SearchTruncatedNote} {
		if !strings.Contains(all, want) {
			t.Errorf("results lack %q:\n%s", want, all)
		}
	}

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh)
	got, err := os.ReadFile(core.SidecarPath(video, core.CaptionLang{Base: "en"}))
	if err != nil || string(got) != searchBody {
		t.Fatalf("sidecar = %q, %v", got, err)
	}
	if !strings.Contains(u.status.Text, core.SearchSyncUnknown) {
		t.Errorf("status = %q, want the sync caveat", u.status.Text)
	}
}

func TestNoMatch_NoSearchWithoutFeature(t *testing.T) {
	u, doneCh, _, _ := startNoMatch(t, nil, 0)
	waitDo(t, doneCh) // feature probe
	if findEntry(u.list) != nil || findButton(u.list, "Search") != nil {
		t.Error("search UI shown on a node without the feature")
	}
}

func TestSearch_RateLimitShowsServerMessage(t *testing.T) {
	u, doneCh, _, _ := startNoMatch(t, []string{core.FeatureSearch}, http.StatusTooManyRequests)
	waitDo(t, doneCh)
	tapButtonOn(t, u.win, "Search")
	waitDo(t, doneCh)
	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), "try again in 10m0s") {
		t.Error("rate-limit dialog lacks the server's wait")
	}
	if u.searchBusy {
		t.Error("searchBusy stuck after an error")
	}
}

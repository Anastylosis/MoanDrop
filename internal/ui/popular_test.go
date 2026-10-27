package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func popularServer(t *testing.T, features []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "test", "features": features})
	})
	mux.HandleFunc("GET /api/v1/trending", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"releases": []client.TrendingRelease{{
			Release: client.Release{ID: 12, Title: "Late Checkout", Studio: "Real Studio",
				Tracks: []client.TrackSummary{{ID: 1, Lang: "en"}, {ID: 2, Lang: "de"}, {ID: 3, Lang: "en"}}},
			WindowDownloads: 87,
		}}})
	})
	mux.HandleFunc("GET /api/v1/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Stats{Tracks: 120, Releases: 80, Languages: map[string]int64{"en": 100, "de": 20}, DownloadsTotal: 999})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestShowPopular_ListsReleasesWithPageLinks(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv := popularServer(t, []string{"trending"})
	setServerURL(u.app.Preferences(), srv.URL)

	u.showPopular()
	waitDo(t, doneCh) // feature probe opens the dialog
	waitDo(t, doneCh) // list arrives

	ov := topOverlay(u.win)
	all := strings.Join(collectTexts(ov), "\n")
	for _, want := range []string{"Late Checkout", "Real Studio", "de, en", "87 downloads"} {
		if !strings.Contains(all, want) {
			t.Errorf("popular dialog missing %q; got:\n%s", want, all)
		}
	}
	var link *widget.Hyperlink
	walkCanvas(ov, func(c fyne.CanvasObject) {
		if h, ok := c.(*widget.Hyperlink); ok {
			link = h
		}
	})
	if link == nil || link.URL.String() != srv.URL+"/release/12" {
		t.Errorf("release link = %v, want %s/release/12", link, srv.URL)
	}
	if findButton(ov, "Download") != nil {
		t.Error("popular releases have no local file to attach a subtitle to")
	}
}

func TestShowPopular_OldNodeSaysSo(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv := popularServer(t, []string{"lookup"})
	setServerURL(u.app.Preferences(), srv.URL)

	u.showPopular()
	waitDo(t, doneCh)
	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), core.TrendingUnsupportedMessage) {
		t.Error("a node without trending must be told so, not queried")
	}
}

func TestShowNodeTotals_ShowsSummary(t *testing.T) {
	u, doneCh := newFlowApp(t)
	srv := popularServer(t, []string{"stats"})
	setServerURL(u.app.Preferences(), srv.URL)

	u.showNodeTotals()
	waitDo(t, doneCh)
	waitDo(t, doneCh)
	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), "120 subtitles for 80 videos in 2 languages") {
		t.Errorf("totals missing; got %v", collectTexts(topOverlay(u.win)))
	}
}

func TestHelpMenu_HasPopularAndTotalsAndNoSecondQuit(t *testing.T) {
	u, _ := newFlowApp(t)
	var labels []string
	for _, m := range u.win.MainMenu().Items {
		for _, it := range m.Items {
			labels = append(labels, m.Label+"/"+it.Label)
		}
	}
	joined := strings.Join(labels, "\n")
	for _, want := range []string{"Help/What's popular...", "Help/About this node..."} {
		if !strings.Contains(joined, want) {
			t.Errorf("menu missing %q", want)
		}
	}
	if strings.Contains(joined, "Quit") {
		t.Error("fyne adds Quit itself; the app must not")
	}
}

package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func TestRunFolder_ListsEachVideoAndSummarizes(t *testing.T) {
	u, doneCh := newFlowApp(t)
	hit := tempVideo(t, "hit.mp4", 0xAA)
	miss := tempVideo(t, "miss.mp4", 0xBB)

	rel := client.Release{ID: 7, OSHash: fileOSHash(t, hit), Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}}}
	lookup := lookupBatchHandler(&rel)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/lookup/batch") {
			lookup(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.Track{ID: 5, Lang: "en", Body: "1\n00:00:01,000 --> 00:00:02,000\nhi\n"})
	}))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	u.matchGen++
	u.runFolder(context.Background(), u.matchGen, []string{hit, miss}, core.BatchOptions{Langs: []string{"en"}, Write: true})
	waitDo(t, doneCh)
	waitDo(t, doneCh)
	waitDo(t, doneCh) // summary

	texts := strings.Join(collectTexts(u.list), "\n")
	if !strings.Contains(texts, "written") || !strings.Contains(texts, "no match") {
		t.Errorf("list = %q, want a written and a no-match line", texts)
	}
	if want := "2 videos: 1 written, 0 already had, 0 ambiguous, 1 no match, 0 error"; u.status.Text != want {
		t.Errorf("status = %q, want %q", u.status.Text, want)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(hit), "hit.en.srt")); err != nil {
		t.Errorf("sidecar not written: %v", err)
	}
}

func TestDroppedFolder(t *testing.T) {
	dir := t.TempDir()
	file := tempVideo(t, "a.mp4", 1)
	if !droppedFolder([]string{file, dir}) || droppedFolder([]string{file}) {
		t.Error("droppedFolder misclassified")
	}
}

func TestParseLangs(t *testing.T) {
	got, err := parseLangs("en, pt-BR de")
	if err != nil || strings.Join(got, ",") != "en,pt,de" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseLangs("  "); err == nil {
		t.Error("empty input must error")
	}
}

func TestStartFolder_DialogAsksLanguage(t *testing.T) {
	u := newTestApp(test.NewApp())
	u.startFolder([]string{t.TempDir()})
	if findButton(topOverlay(u.win), "Start") == nil {
		t.Fatal("folder dialog has no Start button")
	}
}

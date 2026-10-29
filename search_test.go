package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func searchTestServer(t *testing.T, features []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"features": features})
		case "/api/v1/search":
			_ = json.NewEncoder(w).Encode(client.SearchResult{Truncated: true, Releases: []client.Release{{
				ID: 7, Title: "Some Scene", Studio: "Studio X",
				Tracks: []client.TrackSummary{{ID: 70, Lang: "en"}, {ID: 71, Lang: "de", Generated: true}},
			}}})
		default:
			_ = json.NewEncoder(w).Encode(client.Track{ID: 70, Lang: "en", Body: "1\n00:00:01,000 --> 00:00:02,000\nhi\n"})
		}
	}))
	t.Cleanup(srv.Close)
	flagServer = srv.URL
	t.Cleanup(func() { flagServer = "" })
	return srv
}

func TestRunSearch_ListsReleases(t *testing.T) {
	searchTestServer(t, []string{core.FeatureSearch})
	out := captureStdout(t, func() {
		if err := runSearch(context.Background(), "some scene", "", "", 0, false); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"release 7  Some Scene", "Studio X", "track 70", "AI", core.SearchTruncatedNote, core.SearchSyncUnknown} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRunSearch_WritesChosenTrack(t *testing.T) {
	searchTestServer(t, []string{core.FeatureSearch})
	video := filepath.Join(t.TempDir(), "scene.mp4")
	out := captureStdout(t, func() {
		if err := runSearch(context.Background(), "some scene", "en", video, 0, false); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(filepath.Dir(video), "scene.en.srt")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, core.SearchSyncUnknown) {
		t.Errorf("no sync caveat in %q", out)
	}
}

func TestRunSearch_RefusesWithoutFeature(t *testing.T) {
	searchTestServer(t, nil)
	err := runSearch(context.Background(), "x", "", "", 0, false)
	if err == nil || err.Error() != core.SearchUnsupportedNotice {
		t.Errorf("err = %v", err)
	}
}

func TestRunSearch_TrackNeedsWrite(t *testing.T) {
	if err := runSearch(context.Background(), "x", "", "", 5, false); err == nil {
		t.Error("want an error")
	}
}

func TestPrintSearchHints_OnlyWithFeatureAndNeverSearches(t *testing.T) {
	var searches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"features": []string{core.FeatureSearch}})
		case "/api/v1/search":
			searches++
		}
	}))
	t.Cleanup(srv.Close)
	sum := core.BatchSummary{Results: []core.BatchResult{
		{Path: "/v/Some.Scene.mp4", Status: core.BatchNoMatch},
		{Path: "/v/ok.mp4", Status: core.BatchWritten},
	}}
	out := captureStdout(t, func() {
		printSearchHints(context.Background(), client.New(srv.URL, ""), sum, []string{"en", "de"})
	})
	if want := core.SearchHint("/v/Some.Scene.mp4", "en") + "\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if searches != 0 {
		t.Error("hints must not search")
	}

	bare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"features": []string{}})
	}))
	t.Cleanup(bare.Close)
	out = captureStdout(t, func() {
		printSearchHints(context.Background(), client.New(bare.URL, ""), sum, []string{"en"})
	})
	if out != "" {
		t.Errorf("node without search printed %q", out)
	}
}

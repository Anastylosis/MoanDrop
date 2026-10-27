package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

func popularNode(t *testing.T, features string) (*httptest.Server, *string) {
	t.Helper()
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"t","features":[` + features + `]}`))
		case "/api/v1/trending":
			query = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"releases":[{"release":{"id":12,"title":"Late Checkout","studio":"Real Studio","tracks":[{"id":1,"lang":"en"}]},"window_downloads":87}]}`))
		case "/api/v1/stats":
			_, _ = w.Write([]byte(`{"tracks":120,"releases":80,"languages":{"en":1,"de":1},"generated_share":0.5,"downloads_total":999}`))
		}
	}))
	t.Cleanup(srv.Close)
	flagServer = srv.URL
	t.Cleanup(func() { flagServer = "" })
	return srv, &query
}

func TestRunTrending_ListsRowsAndPassesWindow(t *testing.T) {
	srv, query := popularNode(t, `"trending"`)
	out := captureStdout(t, func() {
		if err := runTrending(context.Background(), 3, 5); err != nil {
			t.Fatal(err)
		}
	})
	if *query != "days=3&limit=5" {
		t.Errorf("query = %q", *query)
	}
	for _, want := range []string{"last 3 days", "Late Checkout", "Real Studio", "en", "87 downloads", srv.URL + "/release/12"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunStats_PrintsTotals(t *testing.T) {
	popularNode(t, `"stats"`)
	out := captureStdout(t, func() {
		if err := runStats(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "120 subtitles for 80 videos in 2 languages") || !strings.Contains(out, "999 downloads") {
		t.Errorf("output:\n%s", out)
	}
}

func TestTrendingAndStats_OldNodeIsToldNotAsked(t *testing.T) {
	popularNode(t, `"lookup"`)
	if err := runTrending(context.Background(), 0, 0); err == nil || err.Error() != core.TrendingUnsupportedMessage {
		t.Errorf("trending err = %v", err)
	}
	if err := runStats(context.Background()); err == nil || err.Error() != core.StatsUnsupportedMessage {
		t.Errorf("stats err = %v", err)
	}
}

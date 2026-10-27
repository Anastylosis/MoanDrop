package core

import (
	"reflect"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

func TestTrendingRows_LabelBylineLanguagesAndOrder(t *testing.T) {
	rows := TrendingRows([]client.TrendingRelease{
		{Release: client.Release{ID: 2, Title: "Two", Studio: "S", Performers: []string{"A"}, Tracks: []client.TrackSummary{{Lang: "en"}, {Lang: "de"}, {Lang: "en"}}}, WindowDownloads: 9},
		{Release: client.Release{ID: 1}, WindowDownloads: 1},
	})
	if len(rows) != 2 || rows[0].ReleaseID != 2 {
		t.Fatalf("rows = %+v, want the server's order kept", rows)
	}
	if rows[0].Label != "Two" || rows[0].Byline != "S — A" || !reflect.DeepEqual(rows[0].Languages, []string{"de", "en"}) {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if want := "S — A  ·  de, en  ·  9 downloads"; rows[0].Detail() != want {
		t.Errorf("Detail = %q, want %q", rows[0].Detail(), want)
	}
	if rows[1].Label != "release 1" || rows[1].DownloadsText() != "1 download" {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestTrendingHeadingAndURLs(t *testing.T) {
	if TrendingHeading(7) != "Most downloaded in the last 7 days" || TrendingHeading(1) != "Most downloaded in the last day" || TrendingHeading(0) != "Most downloaded recently" {
		t.Error("heading wording")
	}
	if got := ReleaseURL("https://x.example/", 12); got != "https://x.example/release/12" {
		t.Errorf("ReleaseURL = %q", got)
	}
}

func TestStatsLines(t *testing.T) {
	s := &client.Stats{Tracks: 120, Releases: 80, Languages: map[string]int64{"en": 1, "de": 1}, GeneratedShare: 0.31, DownloadsTotal: 999}
	got := StatsLines(s)
	want := []string{"120 subtitles for 80 videos in 2 languages", "999 downloads in total", "31% of the subtitles are AI-generated"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("StatsLines = %q, want %q", got, want)
	}
}

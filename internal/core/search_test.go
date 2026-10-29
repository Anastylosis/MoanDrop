package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

func searchServer(t *testing.T, res client.SearchResult, queries *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*queries = append(*queries, r.URL.RawQuery)
		_ = json.NewEncoder(w).Encode(res)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCleanStem(t *testing.T) {
	for in, want := range map[string]string{
		"/v/Some.Scene.Title.1080p.WEB-DL.x264.mp4": "Some Scene Title",
		"/v/[Studio] Some_Scene (2019) 4K.mkv":      "Studio Some Scene 2019",
		"plain.mp4":                                 "plain",
	} {
		if got := CleanStem(in); got != want {
			t.Errorf("CleanStem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearch_RanksTracksAndDropsEmptyReleases(t *testing.T) {
	var qs []string
	srv := searchServer(t, client.SearchResult{Truncated: true, Releases: []client.Release{
		{ID: 1, Tracks: []client.TrackSummary{{ID: 10, Lang: "en", Generated: true}, {ID: 11, Lang: "en"}}},
		{ID: 2},
	}}, &qs)
	out, err := Search(context.Background(), client.New(srv.URL, ""), "  some scene ", "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 1 || !out.Truncated || out.Hits[0].Tracks[0].ID != 11 {
		t.Fatalf("out = %+v", out)
	}
	if len(qs) != 1 || !strings.Contains(qs[0], "q=some+scene") || !strings.Contains(qs[0], "lang=en") {
		t.Errorf("query = %v", qs)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	if _, err := Search(context.Background(), client.New("http://unused", ""), "  ", "", nil); err == nil {
		t.Fatal("want an error")
	}
}

func TestSearch_Retries429(t *testing.T) {
	waits := instantSleep(t)
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "slow", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"releases":[],"truncated":false}`))
	}))
	t.Cleanup(srv.Close)
	var told []time.Duration
	if _, err := Search(context.Background(), client.New(srv.URL, ""), "x", "", func(d time.Duration) { told = append(told, d) }); err != nil {
		t.Fatal(err)
	}
	if len(*waits) != 1 || len(told) != 1 || told[0] != 5*time.Second {
		t.Errorf("waits = %v told = %v", *waits, told)
	}
}

func TestChooseSearchTrack(t *testing.T) {
	o := SearchOutcome{Hits: []SearchHit{
		{Tracks: []client.TrackSummary{{ID: 1, Lang: "en"}, {ID: 2, Lang: "de"}}},
		{Tracks: []client.TrackSummary{{ID: 3, Lang: "en-US"}}},
	}}
	cases := []struct {
		id      int64
		lang    string
		want    int64
		wantErr string
	}{
		{id: 3, want: 3},
		{lang: "de", want: 2},
		{lang: "en", wantErr: "2 tracks fit en"},
		{lang: "fr", wantErr: "no track in fr"},
		{wantErr: "needs --track"},
		{id: 99, wantErr: "not among"},
	}
	for _, c := range cases {
		got, err := ChooseSearchTrack(o, c.id, c.lang)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("id=%d lang=%q err = %v, want %q", c.id, c.lang, err, c.wantErr)
			}
			continue
		}
		if err != nil || got.ID != c.want {
			t.Errorf("id=%d lang=%q got %v err %v", c.id, c.lang, got.ID, err)
		}
	}
}

func TestSearchLangChoices(t *testing.T) {
	got := SearchLangChoices("sk")
	if got[0] != SearchAnyLanguage || got[1] != "sk" {
		t.Errorf("choices = %v, want any then the extra preferred", got)
	}
	if again := SearchLangChoices("en"); slices.Contains(again[1:2], "sk") || len(again) != len(SearchLanguages)+1 {
		t.Errorf("a listed preferred must not be duplicated: %v", again)
	}
	if SearchLangValue(SearchAnyLanguage) != "" || SearchLangValue("de") != "de" {
		t.Error("SearchLangValue mapping wrong")
	}
}

func TestSearchHint(t *testing.T) {
	got := SearchHint("/v/Some.Scene.1080p.mp4", "en")
	want := `no match: moandrop search "Some Scene" --write "/v/Some.Scene.1080p.mp4" --lang en`
	if got != want {
		t.Errorf("hint = %q, want %q", got, want)
	}
	if strings.Contains(SearchHint("x.mp4", ""), "--lang") {
		t.Error("no language, no --lang")
	}
}

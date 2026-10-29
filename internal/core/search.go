package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

// FeatureSearch is the version feature that advertises GET /api/v1/search.
const FeatureSearch = "search"

// Search wording, shared by the CLI and the window.
const (
	// A searched track was timed against some other file, never this one.
	SearchSyncUnknown       = "found by title, not by fingerprint — sync unknown, check it plays in time"
	SearchTruncatedNote     = "more releases matched than are shown — narrow the search"
	NoSearchResultsMessage  = "nothing found for that search"
	SearchUnsupportedNotice = "this node does not support searching yet"
	SearchPrompt            = "Search the catalogue by title, studio or performer:"
	SearchAnyLanguage       = "Any language"
)

// SearchLanguages are the window's language choices; a preferred language outside them is added.
var SearchLanguages = []string{"en", "es", "fr", "de", "it", "pt", "nl", "pl", "ru", "sv", "ja", "ko", "zh"}

// SearchLangChoices is the "any" choice followed by SearchLanguages, with preferred inserted when missing.
func SearchLangChoices(preferred string) []string {
	out := []string{SearchAnyLanguage}
	if preferred != "" && !slices.Contains(SearchLanguages, preferred) {
		out = append(out, preferred)
	}
	return append(out, SearchLanguages...)
}

// SearchLangValue maps a choice to the lang the server filters on.
func SearchLangValue(choice string) string {
	if choice == SearchAnyLanguage {
		return ""
	}
	return choice
}

// SearchHint is the command that searches for an unmatched video and writes the track beside it.
func SearchHint(videoPath, lang string) string {
	cmd := fmt.Sprintf("moandrop search %s --write %s", strconv.Quote(CleanStem(videoPath)), strconv.Quote(videoPath))
	if lang != "" {
		cmd += " --lang " + lang
	}
	return "no match: " + cmd
}

// SearchHit is a release with the tracks worth offering from it.
type SearchHit struct {
	Release client.Release        `json:"release"`
	Tracks  []client.TrackSummary `json:"tracks"`
}

// SearchOutcome is a ranked search result.
type SearchOutcome struct {
	Hits      []SearchHit `json:"hits"`
	Truncated bool        `json:"truncated"`
}

// Search runs the catalogue search, retrying a 429. The server's release
// order is kept; within a release human-made tracks come before generated.
func Search(ctx context.Context, c *client.Client, q, lang string, onWait func(time.Duration)) (SearchOutcome, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return SearchOutcome{}, errors.New("enter something to search for")
	}
	var res *client.SearchResult
	err := WithRetry429(ctx, onWait, func() (serr error) {
		res, serr = c.Search(ctx, q, lang)
		return serr
	})
	if err != nil {
		return SearchOutcome{}, err
	}
	out := SearchOutcome{Truncated: res.Truncated}
	for _, r := range res.Releases {
		if len(r.Tracks) == 0 {
			continue
		}
		tracks := append([]client.TrackSummary(nil), r.Tracks...)
		sort.SliceStable(tracks, func(i, j int) bool { return !tracks[i].Generated && tracks[j].Generated })
		out.Hits = append(out.Hits, SearchHit{Release: r, Tracks: tracks})
	}
	return out, nil
}

// ChooseSearchTrack picks the track to write: the one named by trackID, or
// else the only one matching lang. It never guesses between several.
func ChooseSearchTrack(o SearchOutcome, trackID int64, lang string) (client.TrackSummary, error) {
	var fits []client.TrackSummary
	for _, h := range o.Hits {
		for _, t := range h.Tracks {
			if trackID != 0 && t.ID == trackID {
				return t, nil
			}
			if trackID == 0 && lang != "" && sameBaseLang(t.Lang, lang) {
				fits = append(fits, t)
			}
		}
	}
	switch {
	case trackID != 0:
		return client.TrackSummary{}, fmt.Errorf("track %d is not among the search results", trackID)
	case lang == "":
		return client.TrackSummary{}, errors.New("--write needs --track <id> or --lang to choose a track")
	case len(fits) == 0:
		return client.TrackSummary{}, fmt.Errorf("no track in %s among the search results", lang)
	case len(fits) > 1:
		return client.TrackSummary{}, fmt.Errorf("%d tracks fit %s — pick one with --track <id>", len(fits), lang)
	}
	return fits[0], nil
}

func sameBaseLang(a, b string) bool {
	x, err1 := baseSubtag(a)
	y, err2 := baseSubtag(b)
	return err1 == nil && err2 == nil && x == y
}

var noiseToken = regexp.MustCompile(`(?i)^(\d{3,4}[pi]|[248]k|x26[45]|h26[45]|hevc|avc|web-?dl|web-?rip|blu-?ray|bd-?rip|hd-?rip|dvd-?rip|aac\d*|ac3|mp4|mkv|xxx)$`)

// CleanStem turns a video filename into a search query: extension, bracket
// characters, dot/underscore separators and release-tag noise are dropped.
func CleanStem(videoPath string) string {
	base := filepath.Base(videoPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.NewReplacer(".", " ", "_", " ", "[", " ", "]", " ", "(", " ", ")", " ", "{", " ", "}", " ").Replace(stem)
	var keep []string
	for _, f := range strings.Fields(stem) {
		if !noiseToken.MatchString(f) {
			keep = append(keep, f)
		}
	}
	return strings.Join(keep, " ")
}

package core

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/Anastylosis/MoanSubs/client"
)

// Server features behind the popularity views.
const (
	FeatureTrending = "trending"
	FeatureStats    = "stats"
)

// Wording for the popularity views, shared by the CLI and the window.
const (
	TrendingUnsupportedMessage = "this node does not publish what's popular yet"
	StatsUnsupportedMessage    = "this node does not publish its totals yet"
	TrendingEmptyMessage       = "nothing has been downloaded enough to list yet"
	TrendingNoFileNote         = "These are releases on the node, not files on this machine — drop your own video to find its subtitles."
)

// TrendingHeading names the window the list was ranked over; days <= 0 is
// the server's own default, which this client does not assume.
func TrendingHeading(days int) string {
	switch {
	case days == 1:
		return "Most downloaded in the last day"
	case days > 1:
		return fmt.Sprintf("Most downloaded in the last %d days", days)
	default:
		return "Most downloaded recently"
	}
}

// ReleaseURL is a release's page on the node.
func ReleaseURL(server string, id int64) string {
	return fmt.Sprintf("%s/release/%d", strings.TrimRight(server, "/"), id)
}

// ReleaseLink is ReleaseURL parsed, for opening in a browser.
func ReleaseLink(server string, id int64) (*url.URL, error) {
	return url.Parse(ReleaseURL(server, id))
}

// TrendingRow is one trending release, worded for display.
type TrendingRow struct {
	ReleaseID int64
	Label     string
	Byline    string
	Languages []string
	Downloads int64
}

// TrendingRows words the server's ranking, keeping its order.
func TrendingRows(items []client.TrendingRelease) []TrendingRow {
	rows := make([]TrendingRow, 0, len(items))
	for _, it := range items {
		var langs []string
		for _, t := range it.Release.Tracks {
			if t.Lang != "" && !slices.Contains(langs, t.Lang) {
				langs = append(langs, t.Lang)
			}
		}
		slices.Sort(langs)
		rows = append(rows, TrendingRow{
			ReleaseID: it.Release.ID,
			Label:     ReleaseLabel(it.Release),
			Byline:    ReleaseByline(it.Release),
			Languages: langs,
			Downloads: it.WindowDownloads,
		})
	}
	return rows
}

// DownloadsText is "N downloads", singular-aware.
func (r TrendingRow) DownloadsText() string {
	if r.Downloads == 1 {
		return "1 download"
	}
	return fmt.Sprintf("%d downloads", r.Downloads)
}

// Detail is the line under the title: byline, languages, downloads.
func (r TrendingRow) Detail() string {
	var parts []string
	if r.Byline != "" {
		parts = append(parts, r.Byline)
	}
	if len(r.Languages) > 0 {
		parts = append(parts, strings.Join(r.Languages, ", "))
	}
	parts = append(parts, r.DownloadsText())
	return strings.Join(parts, "  ·  ")
}

// StatsSummary is the node's size in one line.
func StatsSummary(s *client.Stats) string {
	return fmt.Sprintf("%d subtitles for %d videos in %d languages", s.Tracks, s.Releases, len(s.Languages))
}

// StatsLines is the full CLI printout: the summary plus what else the node
// reports about itself.
func StatsLines(s *client.Stats) []string {
	lines := []string{
		StatsSummary(s),
		fmt.Sprintf("%d downloads in total", s.DownloadsTotal),
		fmt.Sprintf("%.0f%% of the subtitles are AI-generated", s.GeneratedShare*100),
	}
	return lines
}

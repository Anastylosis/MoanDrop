package ui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// offerSearch adds the search box after a no-match, only on a node that has the feature.
func (u *appUI) offerSearch(videoPath string) {
	u.withFeature(core.FeatureSearch, func(supported bool) {
		if !supported {
			return
		}
		u.showSearchPanel(videoPath)
	})
}

func (u *appUI) showSearchPanel(videoPath string) {
	results := container.NewVBox()
	entry := widget.NewEntry()
	entry.SetText(core.CleanStem(videoPath))

	run := func() { u.runSearch(entry.Text, results) }
	entry.OnSubmitted = func(string) { run() }
	button := widget.NewButtonWithIcon("Search", theme.SearchIcon(), run)

	prompt := widget.NewLabel(core.SearchPrompt)
	prompt.Wrapping = fyne.TextWrapWord
	u.list.Add(container.NewVBox(prompt, container.NewBorder(nil, nil, nil, button, entry), results))
	u.list.Refresh()
}

func (u *appUI) runSearch(query string, results *fyne.Container) {
	if u.searchBusy || strings.TrimSpace(query) == "" {
		return
	}
	u.searchBusy = true
	gen := u.matchGen
	u.setBusy(true)
	u.setStatus("searching...")
	c := client.New(serverURL(u.app.Preferences()), "")
	go func() {
		out, err := core.Search(context.Background(), c, query, "", u.busyNotice(gen))
		fyne.Do(func() {
			u.searchBusy = false
			if gen != u.matchGen {
				return
			}
			u.setBusy(false)
			u.setStatus("")
			if err != nil {
				showError(u.win, err)
				return
			}
			u.renderSearchResults(results, out)
		})
	}()
}

func (u *appUI) renderSearchResults(into *fyne.Container, out core.SearchOutcome) {
	into.RemoveAll()
	if len(out.Hits) == 0 {
		into.Add(widget.NewLabel(core.NoSearchResultsMessage))
		into.Refresh()
		return
	}
	sawGenerated := false
	for _, row := range BuildSearchRows(out.Hits) {
		trackRows := container.NewVBox()
		for _, tr := range row.Tracks {
			trackRows.Add(u.searchTrackRowWidget(tr))
			if tr.Badge != "" {
				sawGenerated = true
			}
		}
		title := widget.NewLabelWithStyle(ReleaseLabel(row.Release), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		title.Wrapping = fyne.TextWrapWord
		entry := container.NewVBox(title, monoLabel(fmt.Sprintf("release %d", row.Release.ID)))
		if byline := ReleaseByline(row.Release); byline != "" {
			bl := widget.NewLabel(byline)
			bl.Wrapping = fyne.TextWrapWord
			entry.Add(bl)
		}
		entry.Add(widget.NewSeparator())
		entry.Add(trackRows)
		into.Add(container.NewVBox(entry, widget.NewSeparator()))
	}
	for _, note := range []string{core.SearchSyncUnknown, truncatedNote(out)} {
		if note == "" {
			continue
		}
		l := widget.NewLabel(note)
		l.Wrapping = fyne.TextWrapWord
		into.Add(l)
	}
	if sawGenerated {
		l := widget.NewLabel(core.GeneratedExplainer)
		l.Wrapping = fyne.TextWrapWord
		into.Add(l)
	}
	into.Refresh()
}

func truncatedNote(out core.SearchOutcome) string {
	if out.Truncated {
		return core.SearchTruncatedNote
	}
	return ""
}

func (u *appUI) searchTrackRowWidget(tr TrackRow) fyne.CanvasObject {
	head := container.NewHBox(widget.NewLabelWithStyle(tr.Track.Lang, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	if k := tr.Track.Kind; k != "" && k != "default" {
		head.Add(newChip(k, false))
	}
	if tr.Badge != "" {
		head.Add(newChip(tr.Badge, true))
	}
	sub := container.NewHBox()
	if tr.Credit != "" {
		sub.Add(monoLabel(tr.Credit))
	}
	sub.Add(monoLabel(fmt.Sprintf("%d downloads", tr.Track.Downloads)))
	sub.Add(monoLabel(voteCountsText(tr.Track.Up, tr.Track.Down)))
	dl := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() { u.downloadTrack(tr) })
	dl.Importance = widget.HighImportance
	return container.NewBorder(nil, nil, container.NewVBox(head, sub), dl)
}

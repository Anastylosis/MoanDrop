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
		u.list.Add(u.searchPanel(videoPath))
		u.list.Refresh()
	})
}

// searchPanel is the search box for videoPath; downloads land beside that file, not the window's current video.
func (u *appUI) searchPanel(videoPath string) fyne.CanvasObject {
	results := container.NewVBox()
	entry := widget.NewEntry()
	entry.SetText(core.CleanStem(videoPath))

	preferred := ""
	if langs, err := parseLangs(batchLangs(u.app.Preferences())); err == nil {
		preferred = langs[0]
	}
	lang := widget.NewSelect(core.SearchLangChoices(preferred), nil)
	if preferred != "" {
		lang.Selected = preferred
	} else {
		lang.Selected = core.SearchAnyLanguage
	}

	run := func() { u.runSearch(videoPath, entry.Text, core.SearchLangValue(lang.Selected), results) }
	entry.OnSubmitted = func(string) { run() }
	lang.OnChanged = func(string) { run() }
	button := widget.NewButtonWithIcon("Search", theme.SearchIcon(), run)

	prompt := widget.NewLabel(core.SearchPrompt)
	prompt.Wrapping = fyne.TextWrapWord
	return container.NewVBox(prompt, container.NewBorder(nil, nil, lang, button, entry), results)
}

func (u *appUI) runSearch(videoPath, query, lang string, results *fyne.Container) {
	if u.searchBusy || strings.TrimSpace(query) == "" {
		return
	}
	u.searchBusy = true
	gen := u.matchGen
	u.setBusy(true)
	u.setStatus("searching...")
	c := client.New(serverURL(u.app.Preferences()), "")
	go func() {
		out, err := core.Search(context.Background(), c, query, lang, u.busyNotice(gen))
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
			u.renderSearchResults(results, videoPath, out)
		})
	}()
}

func (u *appUI) renderSearchResults(into *fyne.Container, videoPath string, out core.SearchOutcome) {
	into.RemoveAll()
	if len(out.Hits) == 0 {
		into.Add(widget.NewLabel(core.NoSearchResultsMessage))
		if out.Weak > 0 {
			l := widget.NewLabel(core.SearchWeakNote(out.Weak))
			l.Wrapping = fyne.TextWrapWord
			into.Add(l)
		}
		into.Refresh()
		return
	}
	sawGenerated := false
	for _, row := range BuildSearchRows(out.Hits) {
		trackRows := container.NewVBox()
		for _, tr := range row.Tracks {
			trackRows.Add(u.searchTrackRowWidget(videoPath, tr))
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
	switch {
	case out.Truncated && out.Weak > 0:
		return core.SearchTruncatedNote + "\n" + core.SearchWeakNote(out.Weak)
	case out.Truncated:
		return core.SearchTruncatedNote
	case out.Weak > 0:
		return core.SearchWeakNote(out.Weak)
	}
	return ""
}

func (u *appUI) searchTrackRowWidget(videoPath string, tr TrackRow) fyne.CanvasObject {
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
	dl := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() { u.downloadTrackTo(videoPath, tr) })
	dl.Importance = widget.HighImportance
	return container.NewBorder(nil, nil, container.NewVBox(head, sub), dl)
}

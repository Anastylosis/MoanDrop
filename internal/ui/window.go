package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// appUI holds the one window's live widgets. MoanDrop is "a drop target and
// a candidate list", so there is exactly one of these per run — no
// multi-document sprawl to manage.
type appUI struct {
	app fyne.App
	win fyne.Window

	status *widget.Label
	busy   *widget.ProgressBarInfinite
	drop   *widget.Label
	list   *fyne.Container
	scroll *container.Scroll

	// All three below are touched only on the UI goroutine. matchGen
	// invalidates callbacks from a superseded run: a slow match for video A
	// must not repaint its results over video B's after the user moved on.
	videoPath    string
	matchGen     int
	downloadBusy bool
}

// build wires up the window's permanent chrome: the privacy line, the drop
// target, the busy indicator, the result list, and the menu. Called once,
// after the age gate (if any) has already been resolved.
func (u *appUI) build() {
	u.win.SetMaster()
	u.win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if len(uris) == 0 {
			return
		}
		u.startVideo(uris[0].Path())
	})

	privacy := widget.NewLabelWithStyle(
		"Fingerprints only — "+core.PrivacyLine+".",
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	privacy.Wrapping = fyne.TextWrapWord

	u.drop = widget.NewLabelWithStyle(
		"Drop a video here, or use File → Open Video…",
		fyne.TextAlignCenter, fyne.TextStyle{})
	u.drop.Wrapping = fyne.TextWrapWord
	dropArea := widget.NewCard("", "", container.NewCenter(u.drop))

	u.busy = widget.NewProgressBarInfinite()
	u.busy.Hide()

	u.status = widget.NewLabel("")
	u.status.Wrapping = fyne.TextWrapWord

	u.list = container.NewVBox()
	u.scroll = container.NewVScroll(u.list)

	content := container.NewBorder(
		container.NewVBox(privacy, dropArea, u.busy),
		u.status,
		nil, nil,
		u.scroll,
	)
	u.win.SetContent(content)
	u.win.Resize(fyne.NewSize(720, 560))

	u.win.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Video…", u.openVideoDialog),
			fyne.NewMenuItem("Server…", u.promptServerURL),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", u.app.Quit),
		),
	))
}

func (u *appUI) openVideoDialog() {
	fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			showError(u.win, err)
			return
		}
		if r == nil {
			return // user canceled
		}
		path := r.URI().Path()
		_ = r.Close()
		u.startVideo(path)
	}, u.win)
	fd.Show()
}

func (u *appUI) promptServerURL() {
	entry := widget.NewEntry()
	entry.SetText(serverURL(u.app.Preferences()))
	entry.SetPlaceHolder(core.DefaultServerURL)
	d := dialog.NewCustomConfirm("moansubs server", "Save", "Cancel", entry, func(ok bool) {
		if !ok {
			return
		}
		url := entry.Text
		if url == "" {
			url = core.DefaultServerURL
		}
		setServerURL(u.app.Preferences(), url)
	}, u.win)
	d.Show()
}

func (u *appUI) setStatus(msg string) {
	u.status.SetText(msg)
}

func (u *appUI) setBusy(busy bool) {
	if busy {
		u.busy.Show()
		u.busy.Start()
	} else {
		u.busy.Stop()
		u.busy.Hide()
	}
}

// renderCandidates replaces the result list with one card per candidate
// release: its confidence and evidence line, then one row per track with a
// download button and, on a generated track, the AI badge.
func (u *appUI) renderCandidates(rows []CandidateRow) {
	u.list.RemoveAll()
	sawGenerated := false
	for _, row := range rows {
		header := row.Confidence
		if row.Evidence != "" {
			header = fmt.Sprintf("%s — %s", row.Confidence, row.Evidence)
		}
		trackRows := container.NewVBox()
		for _, tr := range row.Tracks {
			trackRows.Add(u.trackRowWidget(tr))
			if tr.Badge != "" {
				sawGenerated = true
			}
		}
		card := widget.NewCard(fmt.Sprintf("release %d", row.Release.ID), header, trackRows)
		u.list.Add(card)
	}
	if sawGenerated {
		explainer := widget.NewLabel(core.GeneratedExplainer)
		explainer.Wrapping = fyne.TextWrapWord
		u.list.Add(explainer)
	}
	u.list.Refresh()
}

func (u *appUI) trackRowWidget(tr TrackRow) fyne.CanvasObject {
	made := core.LabelHuman
	if tr.Badge != "" {
		made = tr.Badge
	}
	// Same rule as the CLI: "default" is the unmarked case, every other
	// declared kind (cc/sdh/forced/other) distinguishes same-language tracks.
	kind := ""
	if k := tr.Track.Kind; k != "" && k != "default" {
		kind = "  " + k
	}
	label := widget.NewLabel(fmt.Sprintf("%s  %s%s  ↑%d ↓%d  %d downloads",
		tr.Track.Lang, made, kind, tr.Track.Up, tr.Track.Down, tr.Track.Downloads))

	items := []fyne.CanvasObject{label}
	if tr.Badge != "" {
		badge := widget.NewButtonWithIcon(tr.Badge, theme.InfoIcon(), func() {
			dialog.ShowInformation("AI-generated subtitle", tr.Tooltip, u.win)
		})
		badge.Importance = widget.WarningImportance
		items = append(items, badge)
	}
	dl := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
		u.downloadTrack(tr)
	})
	items = append(items, dl)
	return container.NewHBox(items...)
}

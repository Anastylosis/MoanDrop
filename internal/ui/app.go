// Package ui is MoanDrop's desktop window: a drop target and a candidate
// list. Matching, ranking and writing all stay in internal/core, so the
// window and the CLI can never disagree about what a result means.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
)

// Run launches the GUI; initialVideo, when non-empty, starts matching
// immediately instead of waiting for a drop (the CLI's `moandrop <video>`).
func Run(initialVideo string) error {
	a := app.NewWithID(appID)
	w := a.NewWindow("MoanDrop")

	u := &appUI{app: a, win: w}
	// Sized before Show: an empty window otherwise appears at its minimum
	// size, and some window managers ignore a resize after mapping.
	w.Resize(fyne.NewSize(720, 560))
	w.CenterOnScreen()
	w.Show()

	proceed := func() {
		u.build()
		if initialVideo != "" {
			u.startVideo(initialVideo)
		}
	}

	// Where a system tray exists, closing the window hides to it instead of
	// quitting, so the drop target stays a click away. Quit still works from
	// the File menu and the tray's own entry (fyne appends one).
	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayMenu(fyne.NewMenu("MoanDrop",
			fyne.NewMenuItem("Show", w.Show),
		))
		w.SetCloseIntercept(w.Hide)
	}

	if ageGateAccepted(a.Preferences()) {
		proceed()
	} else {
		showAgeGate(w, func(accepted bool) {
			if !accepted {
				a.Quit()
				return
			}
			acceptAgeGate(a.Preferences())
			proceed()
		})
	}

	a.Run()
	return nil
}

// newTestApp is a seam for tests: fyne's test driver needs no display, so
// widget-construction logic can be exercised in CI without X11.
func newTestApp(a fyne.App) *appUI {
	w := a.NewWindow("MoanDrop")
	u := &appUI{app: a, win: w}
	u.build()
	return u
}

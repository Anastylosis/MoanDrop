// Package ui is MoanDrop's desktop window: a drop target and a candidate
// list. Matching, ranking and writing all stay in internal/core, so the
// window and the CLI can never disagree about what a result means.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

// Run launches the GUI; initialVideo, when non-empty, starts matching
// immediately instead of waiting for a drop (the CLI's `moandrop <video>`).
func Run(initialVideo string) error {
	a := app.NewWithID(appID)
	w := a.NewWindow("MoanDrop")

	u := &appUI{app: a, win: w}
	w.Show()

	proceed := func() {
		u.build()
		if initialVideo != "" {
			u.startVideo(initialVideo)
		}
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

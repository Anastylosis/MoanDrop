package ui

import (
	"context"
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanSubs/client"
)

// offerFitPrompt shows "Did it fit?" after a sibling download — the one
// case where a human's verdict upgrades an estimated/unknown sync for the
// next user. The server is probed for the "fit" feature once per run so an
// older node never sees a request it would 404.
func (u *appUI) offerFitPrompt(tr TrackRow) {
	if tr.ForRelease == 0 {
		return
	}
	if u.fitFeature == nil {
		server := serverURL(u.app.Preferences())
		gen := u.matchGen
		go func() {
			v, err := client.New(server, "").Version(context.Background())
			supported := err == nil && slices.Contains(v.Features, "fit")
			fyne.Do(func() {
				u.fitFeature = &supported
				if gen == u.matchGen {
					u.offerFitPrompt(tr)
				}
			})
		}()
		return
	}
	if !*u.fitFeature {
		return
	}
	u.fitPrompt.RemoveAll()
	u.fitPrompt.Add(widget.NewLabel("Did the subtitle fit?"))
	u.fitPrompt.Add(widget.NewButton("fits", func() { u.onFitReport(tr, true) }))
	u.fitPrompt.Add(widget.NewButton("doesn't fit", func() { u.onFitReport(tr, false) }))
	u.fitPrompt.Add(widget.NewButton("dismiss", func() { u.hideFitPrompt() }))
	u.fitPrompt.Show()
	u.fitPrompt.Refresh()
}

func (u *appUI) hideFitPrompt() {
	u.fitPrompt.RemoveAll()
	u.fitPrompt.Hide()
	u.fitPrompt.Refresh()
}

// onFitReport reports the pairing as served — no offset values ever leave
// the client; the server only accepts a verdict on what it itself applied.
func (u *appUI) onFitReport(tr TrackRow, fits bool) {
	if token(u.app.Preferences()) == "" {
		u.promptToken(func() { u.onFitReport(tr, fits) })
		return
	}
	if u.fitBusy {
		return
	}
	u.fitBusy = true
	gen := u.matchGen
	server, tok := serverURL(u.app.Preferences()), token(u.app.Preferences())
	trackID, releaseID := tr.Track.ID, tr.ForRelease
	go func() {
		fitsN, misfits, err := client.New(server, tok).ReportFit(context.Background(), trackID, releaseID, fits)
		fyne.Do(func() {
			u.fitBusy = false
			if gen != u.matchGen {
				return
			}
			if err != nil {
				showError(u.win, err)
				return
			}
			u.hideFitPrompt()
			u.setStatus(fmt.Sprintf("recorded — %d fits, %d misfits for this pairing", fitsN, misfits))
		})
	}()
}

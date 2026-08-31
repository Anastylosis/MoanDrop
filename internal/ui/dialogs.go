package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// ageGateText is the one-time 18+ confirmation. The server age-gates every
// human-facing page (see moansubs.org); the desktop client asks the same
// question once so a node operator never has to wonder whether a client
// bypassed it.
const ageGateText = "moansubs.org indexes subtitles for adult video. By continuing you confirm you are 18 or older (or the age of majority where you live)."

// showAgeGate asks the one-time 18+ question. onDecision runs with true on
// acceptance, false on decline — the caller must quit the app on false, per
// the spec: decline means exit, not a degraded mode.
func showAgeGate(win fyne.Window, onDecision func(accepted bool)) {
	d := dialog.NewConfirm("Before you continue", ageGateText, onDecision, win)
	d.SetConfirmText("I'm 18 or older")
	d.SetDismissText("Exit")
	d.Show()
}

// showFFmpegMissing surfaces the resolver's own error text — the same
// guidance the CLI prints when EnsureFFmpeg can neither find nor download
// a build — and offers the CLI's --no-phash fallback as a button instead
// of a flag. onFallback runs only if the user picks it.
func showFFmpegMissing(win fyne.Window, err error, onFallback func()) {
	d := dialog.NewConfirm("ffmpeg not found", err.Error()+"\n\nYou can still match this file by its exact bytes, without ffmpeg — other encodes of the same video won't be found.", func(ok bool) {
		if ok {
			onFallback()
		}
	}, win)
	d.SetConfirmText("Match without ffmpeg")
	d.SetDismissText("Cancel")
	d.Show()
}

// showNoMatch renders the CLI's exit-2 guidance verbatim.
func showNoMatch(win fyne.Window, hadPHash bool) {
	msg := core.NoMatchMessage
	if !hadPHash {
		msg += "\n" + core.NoPhashOnlyHint
	}
	dialog.ShowInformation("No match", msg, win)
}

// confirmOverwrite asks before an existing sidecar is replaced — the file
// may be a hand-made subtitle, so silence is never an option here (see
// core.ErrSidecarExists).
func confirmOverwrite(win fyne.Window, path string, onConfirm func()) {
	d := dialog.NewConfirm("Subtitle already exists",
		fmt.Sprintf("%s already exists.\n\nReplace it?", path),
		func(ok bool) {
			if ok {
				onConfirm()
			}
		}, win)
	d.SetConfirmText("Replace")
	d.SetDismissText("Cancel")
	d.SetConfirmImportance(widget.DangerImportance)
	d.Show()
}

func showError(win fyne.Window, err error) {
	dialog.ShowError(err, win)
}

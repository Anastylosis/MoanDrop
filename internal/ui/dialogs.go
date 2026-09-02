package ui

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// ageGateText is the one-time 18+ confirmation, phrased to match the
// server's own age gate at moansubs.org.
const ageGateText = "moansubs.org indexes subtitles for adult video. By continuing you confirm you are 18 or older (or the age of majority where you live)."

// showAgeGate asks the one-time 18+ question; onDecision must quit the app
// on false — decline means exit, not a degraded mode.
func showAgeGate(win fyne.Window, onDecision func(accepted bool)) {
	d := dialog.NewConfirm("Before you continue", ageGateText, onDecision, win)
	d.SetConfirmText("I'm 18 or older")
	d.SetDismissText("Exit")
	d.Show()
}

// showFFmpegMissing surfaces the resolver's own error text — the CLI's
// guidance — and offers --no-phash as a button instead of a flag.
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
// may be a hand-made subtitle, so silence is never an option (core.ErrSidecarExists).
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

// showError routes every error through core.ExplainError, so a 429 reads
// as "try again in Ns" here exactly as it does on the CLI.
func showError(win fyne.Window, err error) {
	dialog.ShowError(core.ExplainError(err), win)
}

// shareOptionsConfirm is the share-options dialog's confirm button — not
// "Share", which the sidecar rows behind the dialog already use.
const shareOptionsConfirm = "Share now"

// askShareOptions asks what the uploader wants to say about the subtitle
// beyond its language: authorship (the server's closed vocabulary, worded
// by core.AuthorshipDescriptions) and the voluntary AI-generated
// declaration. defaultAuthorship preselects the remembered choice. onShare
// is never called on cancel.
func askShareOptions(win fyne.Window, subtitlePath, lang, defaultAuthorship string, onShare func(opts core.PushOptions)) {
	labels := make([]string, 0, len(core.AuthorshipOrder))
	for _, a := range core.AuthorshipOrder {
		labels = append(labels, core.AuthorshipDescriptions[a])
	}
	group := widget.NewRadioGroup(labels, nil)
	group.Required = true
	group.SetSelected(core.AuthorshipDescriptions[defaultAuthorship])

	generated := widget.NewCheck(core.GeneratedDeclarationLabel, nil)

	content := container.NewVBox(
		widget.NewLabelWithStyle("Who made it?", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		group,
		generated,
	)
	d := dialog.NewCustomConfirm(
		fmt.Sprintf("Share %s (%s)", filepath.Base(subtitlePath), lang),
		shareOptionsConfirm, "Cancel", content, func(ok bool) {
			if !ok {
				return
			}
			onShare(core.PushOptions{
				Authorship: authorshipFromLabel(group.Selected),
				Generated:  generated.Checked,
			})
		}, win)
	d.Show()
}

// authorshipFromLabel maps a radio label back to its vocabulary value;
// anything unexpected (there is nothing else in the group) reads as shared.
func authorshipFromLabel(label string) string {
	for _, a := range core.AuthorshipOrder {
		if core.AuthorshipDescriptions[a] == label {
			return a
		}
	}
	return core.AuthorshipShared
}

// downvoteReasonText is the one-sentence explainer shown alongside the
// entry: the server requires a reason on a down-vote (client.Vote's doc
// comment) so down-votes stay accountable rather than anonymous.
const downvoteReasonText = "A down-vote needs a reason — the server requires one so down-votes stay accountable."

// promptDownvoteReason asks for the reason a down-vote requires. onReason
// is never called on cancel or an empty entry — an empty submission is
// simply dropped, the same "no-op on blank input" the language dialog uses.
func promptDownvoteReason(win fyne.Window, onReason func(reason string)) {
	msg := widget.NewLabel(downvoteReasonText)
	msg.Wrapping = fyne.TextWrapWord
	entry := widget.NewEntry()
	entry.SetPlaceHolder("why is this subtitle bad?")
	content := container.NewVBox(msg, entry)
	d := dialog.NewCustomConfirm("Down-vote reason", "Vote", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		reason := entry.Text
		if reason == "" {
			return
		}
		onReason(reason)
	}, win)
	d.Show()
}

// askLanguage prompts for a subtitle's language when its filename carried
// no parseable tag — the multi-file drop and manual-picker paths, where the
// name is whatever the user happened to save it as. onLang is never called
// on cancel or an empty entry.
func askLanguage(win fyne.Window, subtitlePath string, onLang func(lang string)) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("e.g. en")
	d := dialog.NewCustomConfirm(
		fmt.Sprintf("Language for %s", filepath.Base(subtitlePath)),
		"Share", "Cancel", entry, func(ok bool) {
			if !ok {
				return
			}
			lang := entry.Text
			if lang == "" {
				return
			}
			onLang(lang)
		}, win)
	d.Show()
}

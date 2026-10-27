package ui

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// appUI holds the one window's live widgets — MoanDrop is a single-window
// app, so there is exactly one of these per run.
type appUI struct {
	app fyne.App
	win fyne.Window

	status   *widget.Label
	busy     *widget.ProgressBarInfinite
	drop     *dropZone
	batchBar *widget.ProgressBar
	list     *fyne.Container
	scroll   *container.Scroll
	shareBox *fyne.Container

	// All below are touched only on the UI goroutine. matchGen invalidates
	// callbacks from a superseded run: a slow match (or push) for video A
	// must not repaint its results over video B's after the user moved on.
	// downloadBusy/shareBusy each serialize their own action (a double-click
	// could otherwise race a WriteSidecar existence check, or fire two
	// concurrent uploads of the same file).
	videoPath    string
	matchGen     int
	downloadBusy bool
	fitPrompt    *fyne.Container
	fitBusy      bool
	detailsBusy  bool
	// features caches the once-per-run GET /api/v1/version feature probe
	// (see withFeature); featuresProbed distinguishes "not yet asked" from
	// "asked, and the node advertises nothing". UI goroutine only.
	features       []string
	featuresProbed bool
	shareBusy      bool
	searchBusy     bool

	// votes remembers this session's own cast votes by trackID (1 or -1) —
	// only enough to decide whether a row shows the +1/-1 buttons or the
	// "remove vote" affordance. Lookups are anonymous, so the window has no
	// way to learn a vote cast in a previous session; voteBusy serializes
	// vote calls the same way downloadBusy/shareBusy serialize theirs.
	votes    map[int64]int
	voteBusy bool

	batchCancel context.CancelFunc
}

// build wires up the window's permanent chrome. Called once, after the age
// gate (if any) has already been resolved.
func (u *appUI) build() {
	u.votes = make(map[int64]int)
	u.win.SetMaster()
	u.win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if len(uris) == 0 {
			return
		}
		paths := make([]string, len(uris))
		for i, uri := range uris {
			paths[i] = uri.Path()
		}
		if droppedFolder(paths) {
			u.startFolder(paths)
			return
		}
		if len(paths) == 1 {
			u.startVideo(paths[0])
			return
		}
		// A video plus one or more subtitle files is explicit share intent
		// (see splitVideoAndSubtitles): pair them even if the subtitle names
		// don't follow the sidecar convention FindSidecars looks for.
		video, subs := splitVideoAndSubtitles(paths)
		if video == "" || len(subs) == 0 {
			u.startVideo(paths[0])
			return
		}
		u.startVideo(video, subs...)
	})

	privacy := widget.NewLabelWithStyle(
		"Fingerprints only — "+core.PrivacyLine+".",
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	privacy.Wrapping = fyne.TextWrapWord

	u.drop = newDropZone()
	u.batchBar = widget.NewProgressBar()
	u.batchBar.Hide()

	u.busy = widget.NewProgressBarInfinite()
	u.busy.Hide()

	u.status = widget.NewLabel("")
	u.status.Wrapping = fyne.TextWrapWord

	u.list = container.NewVBox()
	u.scroll = container.NewVScroll(u.list)

	// Empty until a video loads (refreshShareSection populates it) — kept
	// visually subordinate to the results below by living in the fixed top
	// area, not the results' own scroll region, and by using plain,
	// unemphasized labels.
	u.shareBox = container.NewVBox()
	u.fitPrompt = container.NewHBox()
	u.fitPrompt.Hide()

	content := container.NewBorder(
		container.NewVBox(privacy, u.drop.Container, u.batchBar, u.busy, u.shareBox),
		container.NewVBox(u.fitPrompt, u.status),
		nil, nil,
		u.scroll,
	)
	u.win.SetContent(content)
	u.win.Resize(fyne.NewSize(720, 560))

	// No Quit item: fyne appends its own locale-aware one to the File menu,
	// and a hand-rolled second one shows up alongside it.
	u.win.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open Video...", u.openVideoDialog),
			fyne.NewMenuItem("Settings...", u.promptSettings),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("What's popular...", u.showPopular),
			fyne.NewMenuItem("About this node...", u.showNodeTotals),
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

// tokenDialogText matches CLAUDE.md's explainer: sharing needs a token,
// finding and downloading never do.
const tokenDialogText = "A token comes from a free account on the server and is only needed for sharing — finding and downloading stay anonymous."

// closeBehaviorHideLabel/closeBehaviorQuitLabel are Settings' wording for
// the close-behavior choice, kept separate from the pref's own stored
// values (closeBehaviorHide/closeBehaviorQuit) so a future rewording never
// touches the persisted preference.
const (
	closeBehaviorHideLabel = "Hide to the system tray"
	closeBehaviorQuitLabel = "Quit the app"
)

func closeBehaviorLabel(v string) string {
	if v == closeBehaviorQuit {
		return closeBehaviorQuitLabel
	}
	return closeBehaviorHideLabel
}

func closeBehaviorFromLabel(label string) string {
	if label == closeBehaviorQuitLabel {
		return closeBehaviorQuit
	}
	return closeBehaviorHide
}

// promptSettings is the File menu's single "Settings..." entry: server
// URL, account token, and the close-behavior choice, all in one dialog and
// all persisted together. The share flow's just-in-time token ask
// (promptToken) stays separate — this is the deliberate-configuration
// path, not the "you need one now" path — but both write prefToken.
func (u *appUI) promptSettings() {
	p := u.app.Preferences()

	serverEntry := widget.NewEntry()
	serverEntry.SetText(serverURL(p))
	serverEntry.SetPlaceHolder(core.DefaultServerURL)

	tokenMsg := widget.NewLabel(tokenDialogText)
	tokenMsg.Wrapping = fyne.TextWrapWord
	tokenEntry := widget.NewPasswordEntry()
	// Prefilled from the saved preference only: token() also falls back to
	// MOANDROP_TOKEN, and prefilling that here would silently persist an
	// env-sourced token on a Save made for unrelated reasons.
	tokenEntry.SetText(p.String(prefToken))
	if tokenEntry.Text == "" && token(p) != "" {
		tokenEntry.SetPlaceHolder("using MOANDROP_TOKEN from the environment")
	} else {
		tokenEntry.SetPlaceHolder("paste your account token")
	}

	accountButtons := container.NewHBox(
		widget.NewButtonWithIcon("Create account", theme.AccountIcon(), func() {
			u.openServerPage(core.RegisterURL, serverEntry.Text)
		}),
		widget.NewButtonWithIcon("My account / get token", theme.InfoIcon(), func() {
			u.openServerPage(core.AccountURL, serverEntry.Text)
		}),
	)

	closeGroup := widget.NewRadioGroup([]string{closeBehaviorHideLabel, closeBehaviorQuitLabel}, nil)
	closeGroup.SetSelected(closeBehaviorLabel(closeBehavior(p)))

	content := container.NewVBox(
		widget.NewLabelWithStyle("Server", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		serverEntry,
		widget.NewLabelWithStyle("Account token", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		tokenMsg,
		accountButtons,
		tokenEntry,
		widget.NewLabelWithStyle("When the window is closed", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		closeGroup,
	)

	d := dialog.NewCustomConfirm("Settings", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		url := serverEntry.Text
		if url == "" {
			url = core.DefaultServerURL
		}
		setServerURL(p, url)
		setToken(p, tokenEntry.Text)
		setCloseBehavior(p, closeBehaviorFromLabel(closeGroup.Selected))
		// Takes effect immediately rather than waiting for the next launch.
		applyCloseBehavior(u.app, u.win)
	}, u.win)
	d.Show()
}

func (u *appUI) openServerPage(page func(string) string, server string) {
	if strings.TrimSpace(server) == "" {
		server = core.DefaultServerURL
	}
	target, err := url.Parse(page(strings.TrimSpace(server)))
	if err == nil {
		err = u.app.OpenURL(target)
	}
	if err != nil {
		showError(u.win, err)
	}
}

// promptToken shows the account-token dialog; onSaved runs after a
// non-empty token is saved (never on cancel or an empty save), so a share
// click with no token configured can chain straight into the push once one exists.
func (u *appUI) promptToken(onSaved func()) {
	msg := widget.NewLabel(tokenDialogText)
	msg.Wrapping = fyne.TextWrapWord
	entry := widget.NewEntry()
	entry.SetText(token(u.app.Preferences()))
	entry.SetPlaceHolder("paste your account token")
	content := container.NewVBox(msg, entry)
	d := dialog.NewCustomConfirm("Account token", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		tok := entry.Text
		setToken(u.app.Preferences(), tok)
		if tok != "" && onSaved != nil {
			onSaved()
		}
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

func (u *appUI) renderCandidates(rows []CandidateRow) {
	u.list.RemoveAll()
	sawGenerated := false
	var probes []func()
	for _, row := range rows {
		trackRows := container.NewVBox()
		for _, tr := range row.Tracks {
			trackRows.Add(u.trackRowWidget(tr))
			if tr.Badge != "" {
				sawGenerated = true
			}
		}
		title := widget.NewLabelWithStyle(ReleaseLabel(row.Release), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		title.Wrapping = fyne.TextWrapWord
		verdict := widget.NewLabel(core.ConfidenceLabel(row.Confidence))
		if row.Confidence == core.ConfidenceExact {
			verdict.Importance = widget.SuccessImportance
		}
		sub := fmt.Sprintf("release %d", row.Release.ID)
		if row.Evidence != "" {
			sub = fmt.Sprintf("%s  %s", row.Evidence, sub)
		}
		entry := container.NewVBox(
			container.NewBorder(nil, nil, nil, verdict, title),
			monoLabel(sub),
		)
		if byline := ReleaseByline(row.Release); byline != "" {
			bl := widget.NewLabel(byline)
			bl.Wrapping = fyne.TextWrapWord
			entry.Add(bl)
		}
		if row.Confidence == core.ConfidenceExact && row.Release.Title == "" {
			probes = append(probes, u.offerDetails(entry, row.Release))
		}
		entry.Add(widget.NewSeparator())
		entry.Add(trackRows)
		u.list.Add(container.NewVBox(entry, widget.NewSeparator()))
	}
	if sawGenerated {
		explainer := widget.NewLabel(core.GeneratedExplainer)
		explainer.Wrapping = fyne.TextWrapWord
		u.list.Add(explainer)
	}
	u.list.Refresh()
	for _, probe := range probes {
		probe()
	}
}

func (u *appUI) trackRowWidget(tr TrackRow) fyne.CanvasObject {
	lang := widget.NewLabelWithStyle(tr.Track.Lang, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	head := container.NewHBox(lang)
	// "default" is the unmarked case; every other declared kind
	// (cc/sdh/forced/other) distinguishes same-language tracks.
	if k := tr.Track.Kind; k != "" && k != "default" {
		head.Add(newChip(k, false))
	}
	if rev := core.RevisionLabel(tr.Track.Revision); rev != "" {
		head.Add(newChip(rev, false))
	}
	if tr.Badge != "" {
		head.Add(newChip(tr.Badge, true))
	}

	// counts is a handle a cast vote's response (or a post-retract
	// VoteCounts) updates in place, so voting never needs a full
	// renderCandidates rebuild, which would also lose the scroll position.
	counts := monoLabel(voteCountsText(tr.Track.Up, tr.Track.Down))
	sub := container.NewHBox()
	if tr.Credit != "" {
		sub.Add(monoLabel(tr.Credit))
	}
	sub.Add(monoLabel(fmt.Sprintf("%d downloads", tr.Track.Downloads)))
	sub.Add(counts)

	dl := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
		u.downloadTrack(tr)
	})
	dl.Importance = widget.HighImportance
	acts := container.NewHBox(u.voteRowWidget(tr, counts))
	if token(u.app.Preferences()) != "" {
		acts.Add(widget.NewButton(reviseButtonText, func() {
			u.pickSubtitleToRevise(tr)
		}))
	}
	acts.Add(dl)
	return container.NewBorder(nil, nil, container.NewVBox(head, sub), acts)
}

// voteCountsText is the counts label's exact text, shared between the
// initial render and every in-place update after a vote/unvote.
func voteCountsText(up, down int) string {
	return fmt.Sprintf("+%d -%d", up, down)
}

// voteRowWidget builds one track row's vote affordance: +1/-1 buttons, or —
// once this session has cast a vote on it — a marker plus a "remove vote"
// button instead. It rebuilds its own contents in place after every
// vote/unvote rather than asking the caller to re-render the whole
// candidate list, which would also reset the user's scroll position.
func (u *appUI) voteRowWidget(tr TrackRow, counts *widget.Label) fyne.CanvasObject {
	trackID := tr.Track.ID
	box := container.NewHBox()

	var rebuild func()
	rebuild = func() {
		box.RemoveAll()
		if v, voted := u.votes[trackID]; voted {
			state := "+1"
			if v < 0 {
				state = "-1"
			}
			box.Add(widget.NewLabel("your vote: " + state))
			// The opposite direction stays clickable: casting again
			// replaces the vote server-side, no retract-first needed.
			if v < 0 {
				box.Add(widget.NewButton("+1", func() {
					u.onUpvote(trackID, counts, rebuild)
				}))
			} else {
				box.Add(widget.NewButton("-1", func() {
					u.onDownvote(trackID, counts, rebuild)
				}))
			}
			box.Add(widget.NewButton("remove vote", func() {
				u.castUnvote(trackID, counts, rebuild)
			}))
		} else {
			box.Add(widget.NewButton("+1", func() {
				u.onUpvote(trackID, counts, rebuild)
			}))
			box.Add(widget.NewButton("-1", func() {
				u.onDownvote(trackID, counts, rebuild)
			}))
		}
		box.Refresh()
	}
	rebuild()
	return box
}

// renderShareSection rebuilds the sharing area from sidecars (found beside
// the video, plus anything paired by an explicit multi-file drop). The
// manual picker button always shows once a video is loaded, whether or not
// any sidecar was found — it is the only share path when discovery comes up empty.
func (u *appUI) renderShareSection(sidecars []core.SidecarCandidate) {
	u.shareBox.RemoveAll()
	if len(sidecars) > 0 {
		header := widget.NewLabelWithStyle("Share what you have", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		u.shareBox.Add(header)
		for _, sc := range sidecars {
			u.shareBox.Add(u.sidecarRowWidget(sc))
		}
	}
	manual := widget.NewButtonWithIcon("Share a subtitle...", theme.UploadIcon(), u.pickSubtitleToShare)
	u.shareBox.Add(manual)
	u.shareBox.Refresh()
}

func (u *appUI) sidecarRowWidget(sc core.SidecarCandidate) fyne.CanvasObject {
	langText := sc.Lang
	if langText == "" {
		langText = "language unknown"
	}
	label := widget.NewLabel(fmt.Sprintf("%s (%s)", filepath.Base(sc.Path), langText))
	share := widget.NewButtonWithIcon("Share", theme.UploadIcon(), func() {
		u.shareSidecar(sc.Path, sc.Lang)
	})
	return container.NewHBox(label, share)
}

// pickSubtitleToShare is the manual share path (spec: reachable even when
// FindSidecars finds nothing): a file picker filtered to subtitle files,
// paired with the loaded video.
func (u *appUI) pickSubtitleToShare() {
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
		u.handlePickedSubtitle(path)
	}, u.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".srt", ".vtt"}))
	fd.Show()
}

const reviseButtonText = "Share a correction"

// The new file keeps the track's own language; the server requires a match.
func (u *appUI) pickSubtitleToRevise(tr TrackRow) {
	fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			showError(u.win, err)
			return
		}
		if r == nil {
			return
		}
		path := r.URI().Path()
		_ = r.Close()
		u.reviseTrack(path, tr)
	}, u.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".srt", ".vtt"}))
	fd.Show()
}

// handlePickedSubtitle is pickSubtitleToShare's callback body, split out so
// a test can drive it directly — fyne's headless test driver cannot open
// pickSubtitleToShare's native file dialog to tap through the picker itself.
func (u *appUI) handlePickedSubtitle(path string) {
	u.shareSidecar(path, core.InferSidecarLang(path))
}

// isSubtitleFile matches the extensions FindSidecars and the writer both recognize.
func isSubtitleFile(path string) bool {
	ext := filepath.Ext(path)
	return strings.EqualFold(ext, ".srt") || strings.EqualFold(ext, ".vtt")
}

// splitVideoAndSubtitles recognizes the explicit-share drop shape: exactly
// one non-subtitle file (the video) plus one or more subtitle files.
// Anything else (no video, no subtitle, two videos) returns "" so the
// caller falls back to the ordinary single-file drop convention.
func splitVideoAndSubtitles(paths []string) (video string, subs []string) {
	for _, p := range paths {
		if isSubtitleFile(p) {
			subs = append(subs, p)
			continue
		}
		if video != "" {
			return "", nil
		}
		video = p
	}
	return video, subs
}

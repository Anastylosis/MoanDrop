package ui

import (
	"context"
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// startVideo is the entry point for a drop, File > Open Video..., and the
// video named on the command line. extraSubs is the multi-file drop's
// explicit pairing (video + subtitle files dropped together) — offered for
// sharing alongside whatever FindSidecars turns up on its own.
func (u *appUI) startVideo(path string, extraSubs ...string) {
	if path == "" {
		return
	}
	u.videoPath = path
	u.matchGen++
	gen := u.matchGen
	u.hideFitPrompt()
	u.setStatus("")
	u.list.RemoveAll()
	u.list.Refresh()
	u.refreshShareSection(path, extraSubs...)

	ffmpeg, ffprobe, err := core.FindFFmpeg("", "")
	if err == nil {
		u.runMatch(gen, path, ffmpeg, ffprobe)
		return
	}

	// Not installed: EnsureFFmpeg may download the pinned build, so it
	// cannot run on the UI goroutine.
	u.setBusy(true)
	u.setStatus("fetching ffmpeg (downloaded once, then cached)...")
	go func() {
		ffmpeg, ffprobe, err := core.EnsureFFmpeg(context.Background(), "", "")
		fyne.Do(func() {
			if gen != u.matchGen {
				return
			}
			if err != nil {
				u.setBusy(false)
				u.setStatus("")
				showFFmpegMissing(u.win, err, func() {
					u.runMatch(gen, path, "", "")
				})
				return
			}
			u.runMatch(gen, path, ffmpeg, ffprobe)
		})
	}()
}

// runMatch mirrors match.go's runMatch without --write. It runs off the UI
// goroutine and wraps every widget touch in fyne.Do so results land safely.
func (u *appUI) runMatch(gen int, path, ffmpeg, ffprobe string) {
	if gen != u.matchGen {
		return
	}
	u.setBusy(true)
	u.setStatus(core.FingerprintingMessage)

	go func() {
		ctx := context.Background()
		fp, err := core.FingerprintFile(ctx, ffmpeg, ffprobe, path)
		if err != nil {
			fyne.Do(func() {
				if gen != u.matchGen {
					return
				}
				u.setBusy(false)
				u.setStatus("")
				showError(u.win, err)
			})
			return
		}

		fyne.Do(func() {
			if gen == u.matchGen {
				u.setStatus("looking up...")
			}
		})

		c := client.New(serverURL(u.app.Preferences()), "")
		releases, err := c.LookupBuckets(ctx, fp.OSHash, fp.PHash)
		if err != nil {
			fyne.Do(func() {
				if gen != u.matchGen {
					return
				}
				u.setBusy(false)
				u.setStatus("")
				showError(u.win, err)
			})
			return
		}

		candidates := core.RankCandidates(releases, fp, false)
		fyne.Do(func() {
			if gen != u.matchGen {
				return
			}
			u.setBusy(false)
			if len(candidates) == 0 {
				u.setStatus(core.NoMatchMessage)
				showNoMatch(u.win, fp.PHash != nil)
				return
			}
			u.setStatus(fmt.Sprintf("found %d candidate(s)", len(candidates)))
			u.renderCandidates(BuildCandidateRows(candidates))
		})
	}()
}

// downloadTrack writes tr's track beside the video, off the UI goroutine so
// a blocking download can't freeze the window. It never os.Stats first —
// core.WriteSidecar's own existence check is the sole, race-free source of truth.
func (u *appUI) downloadTrack(tr TrackRow) {
	// downloadBusy serializes downloads (touched only on the UI goroutine):
	// without it two quick clicks can both pass WriteSidecar's existence
	// check, and the second silently replaces the first with no confirm dialog.
	if u.downloadBusy {
		return
	}
	u.downloadBusy = true
	u.setStatus("downloading...")
	c := client.New(serverURL(u.app.Preferences()), "")
	u.attemptDownload(c, u.videoPath, tr, false)
}

func (u *appUI) attemptDownload(c *client.Client, videoPath string, tr TrackRow, overwrite bool) {
	gen := u.matchGen
	go func() {
		res, err := core.DownloadTrack(context.Background(), c, videoPath, tr.Track.ID, tr.ForRelease, tr.Track.Lang, overwrite)
		if err != nil {
			if errors.Is(err, core.ErrSidecarExists) {
				path := videoPath
				if lang, lerr := core.ResolveCaptionLang(tr.Track.Lang); lerr == nil {
					path = core.SidecarPath(videoPath, lang)
				}
				fyne.Do(func() {
					u.downloadBusy = false
					u.setStatus("")
					confirmOverwrite(u.win, path, func() {
						if u.downloadBusy {
							return
						}
						u.downloadBusy = true
						u.setStatus("downloading...")
						u.attemptDownload(c, videoPath, tr, true)
					})
				})
				return
			}
			fyne.Do(func() {
				u.downloadBusy = false
				u.setStatus("")
				showError(u.win, err)
			})
			return
		}
		fyne.Do(func() {
			u.downloadBusy = false
			u.setStatus(fmt.Sprintf("%s %s%s", res.Verb(), res.Path, res.Note()))
			if gen == u.matchGen {
				u.offerFitPrompt(tr)
			}
		})
	}()
}

// refreshShareSection rebuilds what the window offers for sharing: whatever
// core.FindSidecars turns up beside videoPath, plus extraSubs (a multi-file
// drop's explicit pairing) for names FindSidecars wouldn't recognize on its
// own. Discovery is best-effort — a directory read failure still leaves the
// manual "Share a subtitle..." button reachable via renderShareSection.
// Deliberately NOT re-run after a download: the file the node just handed
// over is nothing to share back, so offering it would only be noise.
func (u *appUI) refreshShareSection(videoPath string, extraSubs ...string) {
	sidecars, _ := core.FindSidecars(videoPath)
	seen := make(map[string]bool, len(sidecars)+len(extraSubs))
	for _, sc := range sidecars {
		seen[sc.Path] = true
	}
	for _, p := range extraSubs {
		if seen[p] {
			continue
		}
		seen[p] = true
		sidecars = append(sidecars, core.SidecarCandidate{Path: p, Lang: core.InferSidecarLang(p)})
	}
	u.renderShareSection(sidecars)
}

// shareSidecar is the common continuation for the discovered-sidecar Share
// button, the multi-file drop pairing, and the manual picker: resolve a
// language when the name didn't give one, make sure a token is configured,
// then push.
func (u *appUI) shareSidecar(subPath, lang string) {
	if lang == "" {
		askLanguage(u.win, subPath, func(lang string) {
			u.shareSidecar(subPath, lang)
		})
		return
	}
	if token(u.app.Preferences()) == "" {
		u.promptToken(func() {
			u.shareSidecar(subPath, lang)
		})
		return
	}
	// The authorship/declaration ask only makes sense on a node that
	// records the answer; an older one gets the plain push it always did
	// rather than a question whose answer would be silently dropped.
	u.withFeature("authorship", func(supported bool) {
		if !supported {
			u.pushSidecar(subPath, lang, core.PushOptions{})
			return
		}
		p := u.app.Preferences()
		askShareOptions(u.win, subPath, lang, authorship(p), func(opts core.PushOptions) {
			setAuthorship(p, opts.Authorship)
			u.pushSidecar(subPath, lang, opts)
		})
	})
}

// pushSidecar runs core.PushSidecar off the UI goroutine. shareBusy
// serializes pushes the same way downloadBusy serializes downloads; gen is
// captured up front so a push begun against one video can't paint its
// result over the window after the user has moved on to another (the same
// matchGen discipline runMatch uses).
func (u *appUI) pushSidecar(subPath, lang string, opts core.PushOptions) {
	if u.shareBusy {
		return
	}
	u.shareBusy = true
	u.setStatus("sharing...")
	gen := u.matchGen
	videoPath := u.videoPath
	server := serverURL(u.app.Preferences())
	tok := token(u.app.Preferences())

	// Size-capped read first, so an oversized file gets its own error
	// instead of one from the ffmpeg resolution that would follow.
	body, err := core.ReadSubtitle(subPath)
	if err != nil {
		u.shareBusy = false
		u.setStatus("")
		showError(u.win, err)
		return
	}

	u.resolveFFmpegForShare(func(ffmpeg, ffprobe string) {
		if gen != u.matchGen {
			u.shareBusy = false
			return
		}
		go func() {
			c := client.New(server, tok)
			res, err := core.PushSidecar(context.Background(), c, videoPath, lang, body, ffmpeg, ffprobe, opts)
			fyne.Do(func() {
				u.shareBusy = false
				if gen != u.matchGen {
					return
				}
				if err != nil {
					u.setStatus("")
					showError(u.win, err)
					return
				}
				u.setStatus(res.Message())
			})
		}()
	})
}

// onUpvote is the +1 button's handler: ensure a token first (the share
// flow's ensure-then-continue pattern), then cast straight away — an
// up-vote never carries a reason.
func (u *appUI) onUpvote(trackID int64, counts *widget.Label, rebuild func()) {
	if token(u.app.Preferences()) == "" {
		u.promptToken(func() {
			u.onUpvote(trackID, counts, rebuild)
		})
		return
	}
	u.castVote(trackID, 1, "", "", counts, rebuild)
}

// onDownvote is the -1 button's handler: ensure a token first, then ask for
// the reason the server requires on a down-vote.
func (u *appUI) onDownvote(trackID int64, counts *widget.Label, rebuild func()) {
	if token(u.app.Preferences()) == "" {
		u.promptToken(func() {
			u.onDownvote(trackID, counts, rebuild)
		})
		return
	}
	promptDownvoteReason(u.win, func(reason string) {
		u.castVote(trackID, -1, reason, "", counts, rebuild)
	})
}

// castVote runs client.Vote off the UI goroutine, serialized by voteBusy
// the same way downloadBusy/shareBusy serialize their own actions. gen is
// captured up front so a vote against a track from a superseded match
// can't paint over whatever the window has moved on to. counts and rebuild
// are the row's own widgets, updated in place rather than through a full
// renderCandidates rebuild.
func (u *appUI) castVote(trackID int64, value int, reason, note string, counts *widget.Label, rebuild func()) {
	if u.voteBusy {
		return
	}
	u.voteBusy = true
	gen := u.matchGen
	c := client.New(serverURL(u.app.Preferences()), token(u.app.Preferences()))

	go func() {
		up, down, err := c.Vote(context.Background(), trackID, value, reason, note)
		fyne.Do(func() {
			u.voteBusy = false
			if gen != u.matchGen {
				return
			}
			if err != nil {
				showError(u.win, err)
				return
			}
			u.votes[trackID] = value
			counts.SetText(voteCountsText(up, down))
			rebuild()
		})
	}()
}

// castUnvote runs client.Unvote then re-fetches the counts with
// VoteCounts, since Unvote's DELETE answers 204 with no body (see its doc
// comment) — this is how the row learns the post-retract tally.
func (u *appUI) castUnvote(trackID int64, counts *widget.Label, rebuild func()) {
	if u.voteBusy {
		return
	}
	u.voteBusy = true
	gen := u.matchGen
	c := client.New(serverURL(u.app.Preferences()), token(u.app.Preferences()))

	go func() {
		err := c.Unvote(context.Background(), trackID)
		if err != nil {
			fyne.Do(func() {
				u.voteBusy = false
				if gen != u.matchGen {
					return
				}
				showError(u.win, err)
			})
			return
		}
		up, down, cerr := c.VoteCounts(context.Background(), trackID)
		fyne.Do(func() {
			u.voteBusy = false
			if gen != u.matchGen {
				return
			}
			delete(u.votes, trackID)
			rebuild()
			if cerr != nil {
				showError(u.win, cerr)
				return
			}
			counts.SetText(voteCountsText(up, down))
		})
	}()
}

// resolveFFmpegForShare finds ffmpeg the same way startVideo does
// (core.FindFFmpeg first, else EnsureFFmpeg's pinned download) so a push
// phashes the video exactly like match already did. Unlike startVideo it
// has no further fallback dialog: a push still fingerprints and uploads on
// oshash alone when ffmpeg can't be had, the same as --no-phash.
func (u *appUI) resolveFFmpegForShare(onReady func(ffmpeg, ffprobe string)) {
	if ffmpeg, ffprobe, err := core.FindFFmpeg("", ""); err == nil {
		onReady(ffmpeg, ffprobe)
		return
	}
	u.setStatus("fetching ffmpeg (downloaded once, then cached)...")
	go func() {
		ffmpeg, ffprobe, err := core.EnsureFFmpeg(context.Background(), "", "")
		fyne.Do(func() {
			if err != nil {
				onReady("", "")
				return
			}
			onReady(ffmpeg, ffprobe)
		})
	}()
}

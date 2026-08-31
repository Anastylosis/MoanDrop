package ui

import (
	"context"
	"errors"
	"fmt"

	"fyne.io/fyne/v2"

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
		})
	}()
}

// refreshShareSection rebuilds what the window offers for sharing: whatever
// core.FindSidecars turns up beside videoPath, plus extraSubs (a multi-file
// drop's explicit pairing) for names FindSidecars wouldn't recognize on its
// own. Discovery is best-effort — a directory read failure still leaves the
// manual "Share a subtitle..." button reachable via renderShareSection.
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
			u.pushSidecar(subPath, lang)
		})
		return
	}
	u.pushSidecar(subPath, lang)
}

// pushSidecar runs core.PushSidecar off the UI goroutine. shareBusy
// serializes pushes the same way downloadBusy serializes downloads; gen is
// captured up front so a push begun against one video can't paint its
// result over the window after the user has moved on to another (the same
// matchGen discipline runMatch uses).
func (u *appUI) pushSidecar(subPath, lang string) {
	if u.shareBusy {
		return
	}
	u.shareBusy = true
	u.setStatus("sharing...")
	gen := u.matchGen
	videoPath := u.videoPath
	server := serverURL(u.app.Preferences())
	tok := token(u.app.Preferences())

	u.resolveFFmpegForShare(func(ffmpeg, ffprobe string) {
		if gen != u.matchGen {
			u.shareBusy = false
			return
		}
		go func() {
			c := client.New(server, tok)
			res, err := core.PushSidecar(context.Background(), c, videoPath, subPath, lang, ffmpeg, ffprobe)
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

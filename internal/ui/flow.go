package ui

import (
	"context"
	"errors"
	"fmt"

	"fyne.io/fyne/v2"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// startVideo is the entry point for both a drop and File → Open Video…, and
// for the video named on the command line. It always runs the pipeline
// with ffmpeg first; ffmpegMissing offers the --no-phash fallback the CLI
// exposes as a flag.
func (u *appUI) startVideo(path string) {
	if path == "" {
		return
	}
	u.videoPath = path
	u.matchGen++
	gen := u.matchGen
	u.setStatus("")
	u.list.RemoveAll()
	u.list.Refresh()

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

// runMatch fingerprints path and looks it up, mirroring match.go's runMatch
// without --write: the default bucketed lookup, ranked client-side. Network
// and CPU work runs off the UI goroutine; every widget touch is wrapped in
// fyne.Do so it lands safely on the driver's own goroutine.
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

// downloadTrack fetches tr's track and writes it beside the current video.
// It tries the no-overwrite path first, so core.WriteSidecar's own
// existence check is the single source of truth for "does a sidecar
// already exist" — this never races a separate os.Stat done here first.
// Runs off the UI goroutine like runMatch, for the same reason: a click
// handler that blocks on the network freezes the whole window.
func (u *appUI) downloadTrack(tr TrackRow) {
	// downloadBusy is only touched on the UI goroutine (click handlers and
	// fyne.Do callbacks), so a plain bool serializes downloads: without it,
	// two quick clicks that resolve to the same sidecar can both pass
	// WriteSidecar's existence check and the second silently replaces the
	// first with no confirm dialog.
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

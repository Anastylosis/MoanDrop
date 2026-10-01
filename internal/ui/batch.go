package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func droppedFolder(paths []string) bool {
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func parseLangs(text string) ([]string, error) {
	var out []string
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' }) {
		l, err := core.ResolveCaptionLang(f)
		if err != nil {
			return nil, err
		}
		out = append(out, l.Base)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("enter a language, e.g. en")
	}
	return out, nil
}

func (u *appUI) cancelBatch() {
	if u.batchCancel != nil {
		u.batchCancel()
		u.batchCancel = nil
	}
}

func (u *appUI) startFolder(paths []string) {
	p := u.app.Preferences()
	entry := widget.NewEntry()
	entry.SetText(batchLangs(p))
	entry.SetPlaceHolder("en")
	replace := widget.NewCheck("Replace subtitles that already exist", nil)
	content := container.NewVBox(
		widget.NewLabel("Confident matches are written beside each video; ambiguous ones are only listed."),
		widget.NewLabelWithStyle("Language(s), in preference order", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		entry,
		replace,
	)
	dialog.NewCustomConfirm("Subtitle a folder", "Start", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		langs, err := parseLangs(entry.Text)
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		setBatchLangs(p, strings.Join(langs, ","))
		u.beginBatch(paths, langs, replace.Checked)
	}, u.win).Show()
}

func (u *appUI) beginBatch(paths, langs []string, overwrite bool) {
	u.cancelBatch()
	u.videoPath = ""
	u.matchGen++
	gen := u.matchGen
	u.hideFitPrompt()
	u.list.RemoveAll()
	u.list.Refresh()
	u.shareBox.RemoveAll()

	ctx, cancel := context.WithCancel(context.Background())
	u.batchCancel = cancel
	stop := widget.NewButton("Stop", cancel)
	u.shareBox.Add(stop)
	u.shareBox.Refresh()
	u.setBusy(true)
	u.setStatus("looking for videos...")

	go func() {
		videos, err := core.CollectVideos(paths)
		if err == nil && len(videos) == 0 {
			err = fmt.Errorf("no video files found")
		}
		if err != nil {
			fyne.Do(func() { u.endBatch(gen, err, nil) })
			return
		}
		ffmpeg, ffprobe, err := core.EnsureFFmpeg(ctx, "", "")
		if err != nil {
			fyne.Do(func() {
				if gen != u.matchGen {
					return
				}
				u.endBatch(gen, nil, nil)
				showFFmpegMissing(u.win, err, func() {
					u.resumeBatch(videos, langs, overwrite, "", "")
				})
			})
			return
		}
		fyne.Do(func() { u.resumeBatch(videos, langs, overwrite, ffmpeg, ffprobe) })
	}()
}

func (u *appUI) resumeBatch(videos, langs []string, overwrite bool, ffmpeg, ffprobe string) {
	u.cancelBatch()
	u.matchGen++
	gen := u.matchGen
	ctx, cancel := context.WithCancel(context.Background())
	u.batchCancel = cancel
	u.shareBox.RemoveAll()
	u.shareBox.Add(widget.NewButton("Stop", cancel))
	u.shareBox.Refresh()
	u.setBusy(true)
	u.runFolder(ctx, gen, videos, core.BatchOptions{
		Langs: langs, Write: true, Overwrite: overwrite, Jobs: 2, FFmpeg: ffmpeg, FFprobe: ffprobe,
	})
}

func (u *appUI) runFolder(ctx context.Context, gen int, videos []string, opts core.BatchOptions) {
	c := client.New(serverURL(u.app.Preferences()), "")
	total := len(videos)
	done := 0
	u.setStatus(fmt.Sprintf("0 of %d videos — %s", total, core.FingerprintingMessage))
	opts.OnResult = func(r core.BatchResult) {
		fyne.Do(func() {
			if gen != u.matchGen {
				return
			}
			done++
			line := widget.NewLabel(r.Line())
			line.Wrapping = fyne.TextWrapWord
			u.list.Add(line)
			u.list.Refresh()
			u.scroll.ScrollToBottom()
			u.setStatus(fmt.Sprintf("%d of %d videos", done, total))
		})
	}
	go func() {
		sum := core.RunBatch(ctx, c, videos, opts)
		fyne.Do(func() { u.endBatch(gen, nil, &sum) })
	}()
}

func (u *appUI) endBatch(gen int, err error, sum *core.BatchSummary) {
	if gen != u.matchGen {
		return
	}
	u.batchCancel = nil
	u.setBusy(false)
	u.shareBox.RemoveAll()
	u.shareBox.Refresh()
	u.setStatus("")
	if err != nil {
		showError(u.win, err)
		return
	}
	if sum != nil {
		u.setStatus(sum.Line(true))
	}
}

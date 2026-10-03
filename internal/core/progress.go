package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Anastylosis/mediahash/videophash"
)

// PhashFrames is how many frames a phash samples.
const PhashFrames = videophash.FrameCount

// mediahash has no per-frame hook and runs one ffmpeg per frame, so the
// hook rides on a wrapper script that runs the real ffmpeg and appends a
// byte to a tick file after each success; the hash itself is untouched.
// Not wired on Windows (cmd quoting could mangle a path): no ticks there.
func generateCounting(ctx context.Context, ffmpegPath, videoPath string, duration float64, onFrame func(done int)) (uint64, error) {
	if onFrame == nil || runtime.GOOS == "windows" {
		return videophash.Generate(ctx, ffmpegPath, videoPath, duration)
	}
	dir, err := os.MkdirTemp("", "moandrop-frames-")
	if err != nil {
		return videophash.Generate(ctx, ffmpegPath, videoPath, duration)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	tick := filepath.Join(dir, "ticks")
	script := filepath.Join(dir, "ffmpeg")
	body := "#!/bin/sh\n" + shQuote(ffmpegPath) + " \"$@\"\nrc=$?\n" +
		"[ $rc -eq 0 ] && printf . >> " + shQuote(tick) + "\nexit $rc\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return videophash.Generate(ctx, ffmpegPath, videoPath, duration)
	}

	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		reported := 0
		poll := func() {
			fi, err := os.Stat(tick)
			if err != nil {
				return
			}
			for n := int(fi.Size()); reported < n && reported < PhashFrames; {
				reported++
				onFrame(reported)
			}
		}
		t := time.NewTicker(30 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				poll()
			case <-stop:
				poll()
				return
			}
		}
	}()
	p, err := videophash.Generate(ctx, script, videoPath, duration)
	close(stop)
	<-finished
	return p, err
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

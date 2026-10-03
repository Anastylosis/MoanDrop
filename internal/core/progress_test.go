package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeMedia writes an ffprobe that reports a 100s duration and an ffmpeg
// that emits a 2x2 24-bit BMP, enough for videophash to hash.
func fakeMedia(t *testing.T) (ffmpeg, ffprobe, video string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("frame progress is not wired on windows")
	}
	dir := t.TempDir()
	bmp := []byte{
		'B', 'M', 70, 0, 0, 0, 0, 0, 0, 0, 54, 0, 0, 0,
		40, 0, 0, 0, 2, 0, 0, 0, 2, 0, 0, 0, 1, 0, 24, 0, 0, 0, 0, 0, 16, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		10, 20, 30, 40, 50, 60, 0, 0,
		70, 80, 90, 100, 110, 120, 0, 0,
	}
	bmpPath := filepath.Join(dir, "f.bmp")
	if err := os.WriteFile(bmpPath, bmp, 0o644); err != nil {
		t.Fatal(err)
	}
	ffmpeg = filepath.Join(dir, "ffmpeg")
	ffprobe = filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\ncat '"+bmpPath+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffprobe, []byte("#!/bin/sh\necho '{\"format\":{\"duration\":\"100.0\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	video = filepath.Join(dir, "v.mp4")
	if err := os.WriteFile(video, make([]byte, 200000), 0o644); err != nil {
		t.Fatal(err)
	}
	return ffmpeg, ffprobe, video
}

func TestFingerprintFileProgress_CallsHookPerFrame(t *testing.T) {
	ffmpeg, ffprobe, video := fakeMedia(t)

	var got []int
	fp, err := FingerprintFileProgress(context.Background(), ffmpeg, ffprobe, video, func(done int) {
		got = append(got, done)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != PhashFrames {
		t.Fatalf("hook called %d times, want %d", len(got), PhashFrames)
	}
	for i, v := range got {
		if v != i+1 {
			t.Fatalf("call %d reported done=%d, want %d", i, v, i+1)
		}
	}

	plain, err := FingerprintFile(context.Background(), ffmpeg, ffprobe, video)
	if err != nil {
		t.Fatal(err)
	}
	if plain.PHash == nil || fp.PHash == nil || *plain.PHash != *fp.PHash {
		t.Errorf("the hook changed the phash: %v vs %v", fp.PHash, plain.PHash)
	}
}

func TestFingerprintFileProgress_FailedFrameStopsTicks(t *testing.T) {
	ffmpeg, ffprobe, video := fakeMedia(t)
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := FingerprintFileProgress(context.Background(), ffmpeg, ffprobe, video, func(int) { calls++ }); err == nil {
		t.Fatal("want an error from a failing ffmpeg")
	}
	if calls != 0 {
		t.Errorf("hook called %d times for a failed frame", calls)
	}
}

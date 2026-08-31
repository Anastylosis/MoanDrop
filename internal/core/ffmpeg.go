package core

import (
	"fmt"
	"os"
	"os/exec"
)

// FindFFmpeg resolves the ffmpeg and ffprobe binaries to run, in order:
// explicit paths (CLI flags), the MOANDROP_FFMPEG / MOANDROP_FFPROBE
// environment variables, then PATH lookup.
//
// Auto-downloading a pinned build the way Stash does is planned but not
// built — which build to pin is still an open decision, because different
// ffmpeg builds can decode different frames at the same seek and the
// pinned build must pass the bit-exactness harness first (mediahash's
// TestBitExact). Until then, ffmpeg is the one thing the user installs.
func FindFFmpeg(explicitFFmpeg, explicitFFprobe string) (ffmpeg, ffprobe string, err error) {
	resolve := func(explicit, envVar, name string) (string, error) {
		if explicit != "" {
			return explicit, nil
		}
		if v := os.Getenv(envVar); v != "" {
			return v, nil
		}
		p, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("%s not found: install ffmpeg (it provides both ffmpeg and ffprobe), or point --%s / %s at a binary", name, name, envVar)
		}
		return p, nil
	}

	ffmpeg, err = resolve(explicitFFmpeg, "MOANDROP_FFMPEG", "ffmpeg")
	if err != nil {
		return "", "", err
	}
	ffprobe, err = resolve(explicitFFprobe, "MOANDROP_FFPROBE", "ffprobe")
	if err != nil {
		return "", "", err
	}
	return ffmpeg, ffprobe, nil
}

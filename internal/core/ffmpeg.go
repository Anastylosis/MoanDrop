package core

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FindFFmpeg resolves ffmpeg and ffprobe, in order: explicit paths, then
// MOANDROP_FFMPEG/MOANDROP_FFPROBE, then PATH. It never downloads — use
// EnsureFFmpeg for that.
func FindFFmpeg(explicitFFmpeg, explicitFFprobe string) (ffmpeg, ffprobe string, err error) {
	ffmpeg, err = locate(explicitFFmpeg, "MOANDROP_FFMPEG", "ffmpeg")
	if err != nil {
		return "", "", err
	}
	ffprobe, err = locate(explicitFFprobe, "MOANDROP_FFPROBE", "ffprobe")
	if err != nil {
		return "", "", err
	}
	return ffmpeg, ffprobe, nil
}

func locate(explicit, envVar, name string) (string, error) {
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

// ffBuild pins one auto-downloadable ffmpeg+ffprobe pair for a GOOS/GOARCH.
// ffprobeURL is empty when ffprobe ships inside the ffmpeg zip (gyan.dev's
// essentials build); otherwise the two are separate zips.
type ffBuild struct {
	version                   string
	ffmpegURL, ffmpegSHA256   string
	ffprobeURL, ffprobeSHA256 string
	ffmpegName, ffprobeName   string // the binary's file name inside its zip
	host, approxSize          string // for the download notice
}

// Pinned builds, one per supported GOOS/GOARCH. gyan.dev only mirrors its
// most recent release under a versioned URL, so windows/amd64 is pinned to
// whatever it currently serves rather than the 6.1 used everywhere else.
var ffBuilds = map[string]ffBuild{
	"linux/amd64": {
		version:       "6.1",
		ffmpegURL:     "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffmpeg-6.1-linux-64.zip",
		ffmpegSHA256:  "8bb4a27f5fd02f3dd9a5e75c9eddf6ace1d50a08929ee0d20bbf17eb467fb711",
		ffprobeURL:    "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffprobe-6.1-linux-64.zip",
		ffprobeSHA256: "cb690c360042b51d9e901db2b0185c585330c1067b5c5edf0b6a5e26e0375e2a",
		ffmpegName:    "ffmpeg",
		ffprobeName:   "ffprobe",
		host:          "github.com",
		approxSize:    "~30 MB",
	},
	"linux/arm64": {
		version:       "6.1",
		ffmpegURL:     "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffmpeg-6.1-linux-arm-64.zip",
		ffmpegSHA256:  "0e3a60df317df0bbfbe25e8b777864b5407d166e5b5ac692fb1e477568a4c886",
		ffprobeURL:    "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffprobe-6.1-linux-arm-64.zip",
		ffprobeSHA256: "c99358a90d93ada454e2f4dd26cd25718d36b4032b0ad5539425eac6e66349d8",
		ffmpegName:    "ffmpeg",
		ffprobeName:   "ffprobe",
		host:          "github.com",
		approxSize:    "~25 MB",
	},
	// evermeet.cx ships amd64-only; Rosetta runs it fine on arm64 Macs, so
	// both darwin arches point at the same build.
	"darwin/amd64": {
		version:       "6.1",
		ffmpegURL:     "https://evermeet.cx/ffmpeg/ffmpeg-6.1.zip",
		ffmpegSHA256:  "a1d9289404b353619749d5d7108b8ded5c1be0d10d236ac13d2d4fc67f89b65b",
		ffprobeURL:    "https://evermeet.cx/ffmpeg/ffprobe-6.1.zip",
		ffprobeSHA256: "e292a8e401aa6a87bdb32feab8eef913f69320a14126ce667ea09964a40e20a4",
		ffmpegName:    "ffmpeg",
		ffprobeName:   "ffprobe",
		host:          "evermeet.cx",
		approxSize:    "~25 MB",
	},
	"windows/amd64": {
		version:      "8.1.2",
		ffmpegURL:    "https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-8.1.2-essentials_build.zip",
		ffmpegSHA256: "db580001caa24ac104c8cb856cd113a87b0a443f7bdf47d8c12b1d740584a2ec",
		ffmpegName:   "ffmpeg.exe",
		ffprobeName:  "ffprobe.exe",
		host:         "gyan.dev",
		approxSize:   "~105 MB",
	},
}

func init() {
	ffBuilds["darwin/arm64"] = ffBuilds["darwin/amd64"]
}

// EnsureFFmpeg extends FindFFmpeg with a cached, then a freshly pinned,
// download. MOANDROP_NO_DOWNLOAD=1 disables both.
func EnsureFFmpeg(ctx context.Context, explicitFFmpeg, explicitFFprobe string) (ffmpeg, ffprobe string, err error) {
	build, ok := ffBuilds[runtime.GOOS+"/"+runtime.GOARCH]
	return ensureFFmpeg(ctx, explicitFFmpeg, explicitFFprobe, build, ok)
}

func ensureFFmpeg(ctx context.Context, explicitFFmpeg, explicitFFprobe string, build ffBuild, haveBuild bool) (ffmpeg, ffprobe string, err error) {
	ffmpeg, ffmpegErr := locate(explicitFFmpeg, "MOANDROP_FFMPEG", "ffmpeg")
	ffprobe, ffprobeErr := locate(explicitFFprobe, "MOANDROP_FFPROBE", "ffprobe")
	if ffmpegErr == nil && ffprobeErr == nil {
		return ffmpeg, ffprobe, nil
	}

	firstErr := ffmpegErr
	if firstErr == nil {
		firstErr = ffprobeErr
	}
	if !haveBuild || os.Getenv("MOANDROP_NO_DOWNLOAD") == "1" {
		return "", "", firstErr
	}

	cacheDir, err := ffmpegCacheDir(build.version)
	if err != nil {
		return "", "", fmt.Errorf("locating ffmpeg cache dir: %w", err)
	}

	if ffmpegErr != nil {
		if ffmpeg, err = build.ensure(ctx, cacheDir, build.ffmpegName); err != nil {
			return "", "", err
		}
	}
	if ffprobeErr != nil {
		if ffprobe, err = build.ensure(ctx, cacheDir, build.ffprobeName); err != nil {
			return "", "", err
		}
	}
	return ffmpeg, ffprobe, nil
}

func ffmpegCacheDir(version string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "moandrop", "ffmpeg", version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ensure returns the cached path to name, downloading and extracting first
// if needed. When ffprobe ships in ffmpeg's own zip (ffprobeURL == ""), one
// download populates both names so the second call finds its target cached.
func (b ffBuild) ensure(ctx context.Context, cacheDir, name string) (string, error) {
	target := filepath.Join(cacheDir, name)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	url, sha256Hex, wanted, label := b.ffmpegURL, b.ffmpegSHA256, []string{b.ffmpegName}, "ffmpeg"
	switch {
	case name == b.ffprobeName && b.ffprobeURL != "":
		url, sha256Hex, wanted, label = b.ffprobeURL, b.ffprobeSHA256, []string{b.ffprobeName}, "ffprobe"
	case b.ffprobeURL == "":
		wanted, label = []string{b.ffmpegName, b.ffprobeName}, "ffmpeg and ffprobe"
	}

	fmt.Fprintf(os.Stderr, "downloading %s %s from %s (one-time, %s)\n", label, b.version, b.host, b.approxSize)
	if err := downloadAndExtract(ctx, cacheDir, url, sha256Hex, wanted); err != nil {
		return "", err
	}
	return target, nil
}

// downloadAndExtract fetches url, verifies its sha256 before touching the
// archive, then extracts each wanted entry (matched by base name, so a
// bin/ subdirectory is fine) via a same-directory temp file and rename, so
// a half-written binary is never observable at its final path.
func downloadAndExtract(ctx context.Context, cacheDir, url, wantSHA256 string, wanted []string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}

	zipFile, err := os.CreateTemp(cacheDir, "download-*.zip")
	if err != nil {
		return err
	}
	zipPath := zipFile.Name()
	defer func() { _ = os.Remove(zipPath) }() // no-op once extraction succeeds and callers move on

	h := sha256.New()
	_, copyErr := io.Copy(zipFile, io.TeeReader(resp.Body, h))
	closeErr := zipFile.Close()
	if copyErr != nil {
		return fmt.Errorf("downloading %s: %w", url, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}

	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA256 {
		return fmt.Errorf("%s: checksum mismatch (got %s, want %s) — refusing to run an unverified binary", url, got, wantSHA256)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	defer func() { _ = zr.Close() }()

	found := make(map[string]bool, len(wanted))
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if found[name] {
			continue
		}
		for _, w := range wanted {
			if name != w {
				continue
			}
			if err := extractZipEntry(f, filepath.Join(cacheDir, name)); err != nil {
				return fmt.Errorf("%s: extracting %s: %w", url, name, err)
			}
			found[name] = true
			break
		}
	}
	for _, w := range wanted {
		if !found[w] {
			return fmt.Errorf("%s: zip did not contain %s", url, w)
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dest), "extract-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

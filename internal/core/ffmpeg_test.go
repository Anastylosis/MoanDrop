package core

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// zipBytes builds an in-memory zip with the given name -> content entries.
func zipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// makePathBinary drops an executable script named `name` into a fresh
// directory and points PATH at it, so exec.LookPath(name) resolves.
func makePathBinary(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe" // LookPath only resolves PATHEXT extensions there
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

// setTempCache points os.UserCacheDir at dir. UserCacheDir reads a
// different variable per OS — XDG_CACHE_HOME only counts on linux, which
// let a test setting only that pass locally while quietly sharing the real cache on windows/macos runners.
func setTempCache(t *testing.T, dir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("XDG_CACHE_HOME", dir)
	}
}

func TestFindFFmpeg_ResolutionOrder(t *testing.T) {
	t.Run("explicit wins over everything", func(t *testing.T) {
		t.Setenv("MOANDROP_FFMPEG", "/env/ffmpeg")
		t.Setenv("MOANDROP_FFPROBE", "/env/ffprobe")
		ffmpeg, ffprobe, err := FindFFmpeg("/explicit/ffmpeg", "/explicit/ffprobe")
		if err != nil {
			t.Fatal(err)
		}
		if ffmpeg != "/explicit/ffmpeg" || ffprobe != "/explicit/ffprobe" {
			t.Fatalf("got (%q, %q)", ffmpeg, ffprobe)
		}
	})

	t.Run("env wins over PATH", func(t *testing.T) {
		makePathBinary(t, "ffmpeg")
		makePathBinary(t, "ffprobe")
		t.Setenv("MOANDROP_FFMPEG", "/env/ffmpeg")
		t.Setenv("MOANDROP_FFPROBE", "/env/ffprobe")
		ffmpeg, ffprobe, err := FindFFmpeg("", "")
		if err != nil {
			t.Fatal(err)
		}
		if ffmpeg != "/env/ffmpeg" || ffprobe != "/env/ffprobe" {
			t.Fatalf("got (%q, %q)", ffmpeg, ffprobe)
		}
	})

	t.Run("falls back to PATH", func(t *testing.T) {
		wantFFmpeg := makePathBinary(t, "ffmpeg")
		wantFFprobe := makePathBinary(t, "ffprobe")
		ffmpeg, ffprobe, err := FindFFmpeg("", "")
		if err != nil {
			t.Fatal(err)
		}
		if ffmpeg != wantFFmpeg || ffprobe != wantFFprobe {
			t.Fatalf("got (%q, %q), want (%q, %q)", ffmpeg, ffprobe, wantFFmpeg, wantFFprobe)
		}
	})

	t.Run("not found anywhere errors", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // empty dir, nothing resolves
		if _, _, err := FindFFmpeg("", ""); err == nil {
			t.Fatal("want error when ffmpeg is nowhere to be found")
		}
	})
}

func TestEnsureFFmpeg_NoDownloadHonored(t *testing.T) {
	t.Setenv("MOANDROP_NO_DOWNLOAD", "1")
	t.Setenv("PATH", t.TempDir()) // nothing on PATH
	setTempCache(t, t.TempDir())

	build := ffBuild{version: "6.1"} // would panic if ever dereferenced for a download
	_, _, err := ensureFFmpeg(context.Background(), "", "", build, true)
	if err == nil {
		t.Fatal("want error: MOANDROP_NO_DOWNLOAD=1 must skip the download step")
	}
}

func TestEnsureFFmpeg_UnsupportedPlatformKeepsLocateError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, _, err := ensureFFmpeg(context.Background(), "", "", ffBuild{}, false)
	if err == nil {
		t.Fatal("want the plain locate error for an unsupported GOOS/GOARCH")
	}
}

func TestEnsureFFmpeg_CachedPriorDownload(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cacheBase := t.TempDir()
	setTempCache(t, cacheBase)

	build := ffBuild{version: "6.1", ffmpegName: "ffmpeg", ffprobeName: "ffprobe"}
	dir, err := ffmpegCacheDir(build.version)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("cached"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// URLs left empty: a real download here would panic the test server-less
	// http.DefaultClient into an error, proving the cache hit short-circuits it.
	ffmpeg, ffprobe, err := ensureFFmpeg(context.Background(), "", "", build, true)
	if err != nil {
		t.Fatalf("ensureFFmpeg: %v", err)
	}
	if ffmpeg != filepath.Join(dir, "ffmpeg") || ffprobe != filepath.Join(dir, "ffprobe") {
		t.Fatalf("got (%q, %q)", ffmpeg, ffprobe)
	}
}

func TestEnsureFFmpeg_DownloadsAndExtracts(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	setTempCache(t, t.TempDir())

	ffmpegZip := zipBytes(t, map[string]string{"ffmpeg": "fake-ffmpeg-bytes"})
	ffprobeZip := zipBytes(t, map[string]string{"ffprobe": "fake-ffprobe-bytes"})

	mux := http.NewServeMux()
	mux.HandleFunc("/ffmpeg.zip", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(ffmpegZip) })
	mux.HandleFunc("/ffprobe.zip", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(ffprobeZip) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	build := ffBuild{
		version:       "test",
		ffmpegURL:     srv.URL + "/ffmpeg.zip",
		ffmpegSHA256:  sha256Hex(ffmpegZip),
		ffprobeURL:    srv.URL + "/ffprobe.zip",
		ffprobeSHA256: sha256Hex(ffprobeZip),
		ffmpegName:    "ffmpeg",
		ffprobeName:   "ffprobe",
		host:          "test-host",
		approxSize:    "~1 KB",
	}

	ffmpeg, ffprobe, err := ensureFFmpeg(context.Background(), "", "", build, true)
	if err != nil {
		t.Fatalf("ensureFFmpeg: %v", err)
	}
	gotFFmpeg, err := os.ReadFile(ffmpeg)
	if err != nil || string(gotFFmpeg) != "fake-ffmpeg-bytes" {
		t.Fatalf("ffmpeg content = %q, %v", gotFFmpeg, err)
	}
	gotFFprobe, err := os.ReadFile(ffprobe)
	if err != nil || string(gotFFprobe) != "fake-ffprobe-bytes" {
		t.Fatalf("ffprobe content = %q, %v", gotFFprobe, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(ffmpeg)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("extracted binary is not executable: %v", info.Mode())
		}
	}

	// A second call must hit the cache, not the server: swap in a build
	// whose URLs would 500 to prove the cache path is what's taken.
	brokenBuild := build
	brokenBuild.ffmpegURL, brokenBuild.ffprobeURL = srv.URL+"/missing", srv.URL+"/missing"
	ffmpeg2, ffprobe2, err := ensureFFmpeg(context.Background(), "", "", brokenBuild, true)
	if err != nil {
		t.Fatalf("second ensureFFmpeg (should be cached): %v", err)
	}
	if ffmpeg2 != ffmpeg || ffprobe2 != ffprobe {
		t.Fatalf("got (%q, %q), want cached (%q, %q)", ffmpeg2, ffprobe2, ffmpeg, ffprobe)
	}
}

func TestEnsureFFmpeg_SharedZipForBothBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	setTempCache(t, t.TempDir())

	// Mirrors gyan.dev's essentials build layout: both binaries in one zip,
	// nested under a bin/ subdirectory, matched by base name.
	combined := zipBytes(t, map[string]string{
		"ffmpeg-8.1.2-essentials_build/bin/ffmpeg.exe":  "fake-ffmpeg-exe",
		"ffmpeg-8.1.2-essentials_build/bin/ffprobe.exe": "fake-ffprobe-exe",
	})

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(combined)
	}))
	defer srv.Close()

	build := ffBuild{
		version:      "test",
		ffmpegURL:    srv.URL + "/combined.zip",
		ffmpegSHA256: sha256Hex(combined),
		// ffprobeURL left empty: ffprobe ships inside the ffmpeg zip.
		ffmpegName:  "ffmpeg.exe",
		ffprobeName: "ffprobe.exe",
		host:        "test-host",
		approxSize:  "~1 KB",
	}

	ffmpeg, ffprobe, err := ensureFFmpeg(context.Background(), "", "", build, true)
	if err != nil {
		t.Fatalf("ensureFFmpeg: %v", err)
	}
	if hits != 1 {
		t.Fatalf("server hit %d times, want 1 (one zip covers both binaries)", hits)
	}
	got, _ := os.ReadFile(ffmpeg)
	if string(got) != "fake-ffmpeg-exe" {
		t.Fatalf("ffmpeg content = %q", got)
	}
	got, _ = os.ReadFile(ffprobe)
	if string(got) != "fake-ffprobe-exe" {
		t.Fatalf("ffprobe content = %q", got)
	}
}

func TestEnsureFFmpeg_ChecksumMismatchRejected(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	setTempCache(t, t.TempDir())

	ffmpegZip := zipBytes(t, map[string]string{"ffmpeg": "fake-ffmpeg-bytes"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(ffmpegZip)
	}))
	defer srv.Close()

	build := ffBuild{
		version:       "test",
		ffmpegURL:     srv.URL + "/ffmpeg.zip",
		ffmpegSHA256:  "0000000000000000000000000000000000000000000000000000000000000000000000000000",
		ffprobeURL:    srv.URL + "/ffmpeg.zip",
		ffprobeSHA256: "0000000000000000000000000000000000000000000000000000000000000000000000000000",
		ffmpegName:    "ffmpeg",
		ffprobeName:   "ffprobe",
		host:          "test-host",
		approxSize:    "~1 KB",
	}

	if _, _, err := ensureFFmpeg(context.Background(), "", "", build, true); err == nil {
		t.Fatal("want checksum mismatch to be rejected")
	}

	dir, err := ffmpegCacheDir(build.version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ffmpeg")); !os.IsNotExist(err) {
		t.Fatalf("extracted binary should not exist after a checksum mismatch, stat err = %v", err)
	}
}

func TestExtractZipEntry_UnwritableDestDirLeavesNoFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("directory permissions do not block writes for this user/OS")
	}
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(zipPath, zipBytes(t, map[string]string{"ffmpeg": "data"}), 0o644); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()

	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })

	dest := filepath.Join(ro, "ffmpeg")
	if err := extractZipEntry(zr.File[0], dest); err == nil {
		t.Fatal("want error extracting into an unwritable directory")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("a failed extract must leave no file at dest, stat err = %v", err)
	}
}

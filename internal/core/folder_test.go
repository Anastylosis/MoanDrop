package core

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectVideos_WalksRecursivelyAndFilters(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.mp4"))
	touch(t, filepath.Join(dir, "sub", "b.MKV"))
	touch(t, filepath.Join(dir, "sub", "b.en.srt"))
	touch(t, filepath.Join(dir, "notes.txt"))
	loose := filepath.Join(t.TempDir(), "named.bin")
	touch(t, loose)

	got, err := CollectVideos([]string{dir, loose, dir})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.mp4"), filepath.Join(dir, "sub", "b.MKV"), loose}
	if !reflect.DeepEqual(got, sortedCopy(want)) {
		t.Fatalf("got %v, want %v", got, sortedCopy(want))
	}
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestCollectVideos_SkipsSymlinkedDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	outside := t.TempDir()
	touch(t, filepath.Join(outside, "hidden.mp4"))
	touch(t, filepath.Join(dir, "real.mp4"))
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := CollectVideos([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "real.mp4" {
		t.Fatalf("got %v, want only real.mp4", got)
	}
}

func TestCollectVideos_MissingPath(t *testing.T) {
	if _, err := CollectVideos([]string{filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Fatal("want an error for a missing path")
	}
}

func TestHasSidecarFor(t *testing.T) {
	dir := t.TempDir()
	v := filepath.Join(dir, "scene.mp4")
	touch(t, v)
	touch(t, filepath.Join(dir, "scene.en.srt"))
	touch(t, filepath.Join(dir, "scene.de.vtt"))

	cases := []struct {
		langs []string
		want  bool
	}{
		{nil, false},
		{[]string{"en"}, true},
		{[]string{"en", "de"}, true},
		{[]string{"en", "fr"}, false},
		{[]string{"fr"}, false},
	}
	for _, c := range cases {
		if got := HasSidecarFor(v, c.langs); got != c.want {
			t.Errorf("HasSidecarFor(%v) = %v, want %v", c.langs, got, c.want)
		}
	}
}

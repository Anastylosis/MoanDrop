package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSidecarPath(t *testing.T) {
	lang := CaptionLang{Base: "en"}
	if got, want := SidecarPath("/v/Scene.Title.1080p.mp4", lang), "/v/Scene.Title.1080p.en.srt"; got != want {
		t.Errorf("SidecarPath = %q, want %q", got, want)
	}
}

func TestWriteSidecar(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	lang := CaptionLang{Base: "en"}

	path, created, err := WriteSidecar(video, lang, "1\n00:00:01,000 --> 00:00:02,000\nhi\n", false)
	if err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}
	if !created || path != filepath.Join(dir, "scene.en.srt") {
		t.Fatalf("WriteSidecar = (%q, %v)", path, created)
	}

	// Never destroy an existing caption without being told to.
	if _, _, err := WriteSidecar(video, lang, "other", false); err == nil {
		t.Fatal("overwrite without --overwrite: want error")
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "hi") {
		t.Fatalf("refused overwrite must leave the original intact, got %q", body)
	}

	path2, created, err := WriteSidecar(video, lang, "replaced", true)
	if err != nil || created || path2 != path {
		t.Fatalf("overwrite: (%q, %v, %v)", path2, created, err)
	}
	body, _ = os.ReadFile(path)
	if string(body) != "replaced" {
		t.Fatalf("overwrite body = %q", body)
	}

	// A temp file must never survive a completed write.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestWriteSidecar_OversizeBody(t *testing.T) {
	video := filepath.Join(t.TempDir(), "scene.mp4")
	huge := strings.Repeat("x", MaxTrackBytes+1)
	if _, _, err := WriteSidecar(video, CaptionLang{Base: "en"}, huge, false); err == nil {
		t.Fatal("oversize body: want error")
	}
}

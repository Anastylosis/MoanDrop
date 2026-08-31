package core

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFindSidecars_LangSuffixedAndBare(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	writeFiles(t, dir, "scene.mp4", "scene.en.srt", "scene.srt")

	got, err := FindSidecars(video)
	if err != nil {
		t.Fatalf("FindSidecars: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
	}
	byPath := map[string]string{}
	for _, c := range got {
		byPath[c.Path] = c.Lang
	}
	if lang := byPath[filepath.Join(dir, "scene.en.srt")]; lang != "en" {
		t.Errorf("scene.en.srt lang = %q, want en", lang)
	}
	if lang, ok := byPath[filepath.Join(dir, "scene.srt")]; !ok || lang != "" {
		t.Errorf("scene.srt (bare) lang = %q, ok=%v, want \"\" true", lang, ok)
	}
}

func TestFindSidecars_MultipleLanguages(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	writeFiles(t, dir, "scene.mp4", "scene.en.srt", "scene.de.srt", "scene.fr.vtt")

	got, err := FindSidecars(video)
	if err != nil {
		t.Fatalf("FindSidecars: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d candidates, want 3: %+v", len(got), got)
	}
	langs := map[string]bool{}
	for _, c := range got {
		langs[c.Lang] = true
	}
	for _, want := range []string{"en", "de", "fr"} {
		if !langs[want] {
			t.Errorf("missing language %q among %+v", want, got)
		}
	}
}

func TestFindSidecars_SpacesInStem(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "My Scene (1080p).mp4")
	writeFiles(t, dir, "My Scene (1080p).mp4", "My Scene (1080p).en.srt")

	got, err := FindSidecars(video)
	if err != nil {
		t.Fatalf("FindSidecars: %v", err)
	}
	if len(got) != 1 || got[0].Lang != "en" {
		t.Fatalf("got %+v, want one en candidate", got)
	}
}

func TestFindSidecars_IgnoresUnrelatedAndVideoItself(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	writeFiles(t, dir,
		"scene.mp4",       // the video itself
		"other.en.srt",    // unrelated stem
		"scene.jpg",       // wrong extension
		"scene.2019.srt",  // extra segment isn't a language tag
		"scene.1080p.srt", // extra segment isn't a language tag
		"scene.notes.txt", // wrong extension, wrong stem-suffix shape
	)

	got, err := FindSidecars(video)
	if err != nil {
		t.Fatalf("FindSidecars: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no candidates", got)
	}
}

func TestFindSidecars_MissingDirectory(t *testing.T) {
	if _, err := FindSidecars(filepath.Join(t.TempDir(), "gone", "scene.mp4")); err == nil {
		t.Fatal("want an error when the video's directory does not exist")
	}
}

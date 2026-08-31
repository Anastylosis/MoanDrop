package core

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestSidecarExistsError(t *testing.T) {
	err := &sidecarExistsError{path: "/v/scene.en.srt"}
	if got, want := err.Error(), "caption /v/scene.en.srt already exists; pass --overwrite to replace it"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	// The concrete error must unwrap to the sentinel, so a caller can catch
	// just this case via errors.Is, in either call order.
	if !errors.Is(err, ErrSidecarExists) {
		t.Error("errors.Is(err, ErrSidecarExists) = false, want true")
	}
	if errors.Is(ErrSidecarExists, err) {
		t.Error("errors.Is(ErrSidecarExists, err) = true, want false: the sentinel does not unwrap to a concrete instance")
	}
	if !errors.Is(error(err), ErrSidecarExists) {
		t.Error("errors.Is must still hold once err is boxed as a plain error")
	}
}

func TestDownloadResult_Verb(t *testing.T) {
	if got := (DownloadResult{Created: true}).Verb(); got != "wrote" {
		t.Errorf("Created: Verb() = %q, want %q", got, "wrote")
	}
	if got := (DownloadResult{Created: false}).Verb(); got != "replaced" {
		t.Errorf("not Created: Verb() = %q, want %q", got, "replaced")
	}
}

func TestDownloadResult_Note(t *testing.T) {
	if got := (DownloadResult{}).Note(); got != "" {
		t.Errorf("plain result: Note() = %q, want empty", got)
	}
	generated := DownloadResult{Generated: true}
	if got := generated.Note(); !strings.Contains(got, "(AI-generated subtitle)") {
		t.Errorf("Generated: Note() = %q, want the AI-generated caveat", got)
	}
	normalized := DownloadResult{Lang: CaptionLang{Base: "pt", Normalized: true, Original: "pt-BR"}}
	if got := normalized.Note(); got != "  (pt-BR stored as pt — sidecar names take bare language codes)" {
		t.Errorf("Normalized: Note() = %q", got)
	}
	both := DownloadResult{Generated: true, Lang: CaptionLang{Base: "pt", Normalized: true, Original: "pt-BR"}}
	note := both.Note()
	if !strings.Contains(note, "AI-generated") || !strings.Contains(note, "pt-BR stored as pt") {
		t.Errorf("both caveats: Note() = %q, want both present", note)
	}
}

func TestWriteFileAtomic_UnwritableDirLeavesNoFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("directory permissions do not block writes for this user/OS")
	}
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) }) // let TempDir's cleanup remove it

	target := filepath.Join(ro, "scene.en.srt")
	if err := writeFileAtomic(target, []byte("data"), 0o644); err == nil {
		t.Fatal("want error writing into an unwritable directory")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("a failed write must leave no file at the target path, stat err = %v", err)
	}
}

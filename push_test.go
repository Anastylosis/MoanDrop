package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// whatever it wrote — runPush's success output is a bare fmt.Println, and
// this is the CLI's only way to see it (the golden case this test guards:
// core.PushSidecar's extraction must not change what that line says).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestRunPush_UploadedWording(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.UploadResult{TrackID: 3, ReleaseID: 4})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sub := filepath.Join(dir, "scene.en.srt")
	if err := os.WriteFile(video, bytes.Repeat([]byte{0xAA}, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sub, []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	out := captureStdout(t, func() {
		if err := runPush(context.Background(), video, sub, "", true); err != nil {
			t.Fatalf("runPush: %v", err)
		}
	})
	if got, want := out, "uploaded as track 3 (release 4)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunPush_DuplicateWording(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.UploadResult{TrackID: 3, ReleaseID: 4, Duplicate: true})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sub := filepath.Join(dir, "scene.en.srt")
	_ = os.WriteFile(video, bytes.Repeat([]byte{0xAA}, 4096), 0o644)
	_ = os.WriteFile(sub, []byte("body"), 0o644)

	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	out := captureStdout(t, func() {
		if err := runPush(context.Background(), video, sub, "", true); err != nil {
			t.Fatalf("runPush: %v", err)
		}
	})
	if got, want := out, "already on the node: track 3 (release 4) — nothing new to share\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunPush_UnresolvableLangErrorsBeforeUpload(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sub := filepath.Join(dir, "scene.srt") // no language segment
	_ = os.WriteFile(video, bytes.Repeat([]byte{0xAA}, 4096), 0o644)
	_ = os.WriteFile(sub, []byte("body"), 0o644)

	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	if err := runPush(context.Background(), video, sub, "", true); err == nil {
		t.Fatal("want an error when the language can't be inferred and --lang wasn't passed")
	}
	if hit {
		t.Error("an unresolvable language must fail before ever reaching the network")
	}
}

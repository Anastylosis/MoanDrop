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
	"strings"
	"testing"

	"github.com/Anastylosis/MoanDrop/internal/core"
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
		if err := runPush(context.Background(), video, sub, "", true, core.PushOptions{}); err != nil {
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
		if err := runPush(context.Background(), video, sub, "", true, core.PushOptions{}); err != nil {
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

	if err := runPush(context.Background(), video, sub, "", true, core.PushOptions{}); err == nil {
		t.Fatal("want an error when the language can't be inferred and --lang wasn't passed")
	}
	if hit {
		t.Error("an unresolvable language must fail before ever reaching the network")
	}
}

func TestRunPush_OversizeFailsBeforeFFmpegResolution(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sub := filepath.Join(dir, "scene.en.srt")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sub, bytes.Repeat([]byte{'x'}, core.MaxTrackBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	// Nothing on PATH and downloads off: were ffmpeg resolved before the
	// size check, its error would mask the real one (the master-era order
	// this test freezes).
	t.Setenv("PATH", t.TempDir())
	t.Setenv("MOANDROP_NO_DOWNLOAD", "1")

	err := runPush(context.Background(), video, sub, "en", false, core.PushOptions{})
	if err == nil {
		t.Fatal("want the size-cap error")
	}
	if !strings.Contains(err.Error(), "byte cap") {
		t.Fatalf("err = %v, want the size-cap error, not an ffmpeg one", err)
	}
}

func TestRunPush_AuthorshipAndDeclarationReachTheServer(t *testing.T) {
	var got client.UploadRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.UploadResult{TrackID: 3, ReleaseID: 4, Generated: true, GeneratedSource: core.GeneratedSourceDeclared})
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
		if err := runPush(context.Background(), video, sub, "", true, core.PushOptions{Authorship: core.AuthorshipUncredited, Generated: true}); err != nil {
			t.Fatalf("runPush: %v", err)
		}
	})
	if got.Authorship != core.AuthorshipUncredited || !got.Generated {
		t.Errorf("server saw authorship=%q generated=%v, want uncredited/true", got.Authorship, got.Generated)
	}
	if want := "uploaded as track 3 (release 4), labelled AI-generated as you declared\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRunPush_BadAuthorshipFailsBeforeAnythingElse(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sub := filepath.Join(dir, "scene.srt") // not even a resolvable language
	_ = os.WriteFile(video, bytes.Repeat([]byte{0xAA}, 4096), 0o644)
	_ = os.WriteFile(sub, []byte("body"), 0o644)

	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	err := runPush(context.Background(), video, sub, "", true, core.PushOptions{Authorship: "author"})
	if err == nil || !strings.Contains(err.Error(), "authorship") {
		t.Fatalf("err = %v, want the authorship vocabulary error, ahead of the language one", err)
	}
	if hit {
		t.Error("a bad authorship value must fail before ever reaching the network")
	}
}

func TestRunPush_NoTokenNamesAccountPage(t *testing.T) {
	flagServer, flagToken = "https://node.example/", ""
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	dir := t.TempDir()
	sub := filepath.Join(dir, "v.en.srt")
	if err := os.WriteFile(sub, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runPush(context.Background(), filepath.Join(dir, "v.mp4"), sub, "", true, core.PushOptions{})
	if err == nil || !strings.Contains(err.Error(), "https://node.example/me") {
		t.Fatalf("err = %v, want it to name https://node.example/me", err)
	}
}

func revisionNode(t *testing.T, features []string, result client.UploadResult, status int) (*httptest.Server, *client.UploadRequest) {
	t.Helper()
	var got client.UploadRequest
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "test", "features": features})
	})
	mux.HandleFunc("POST /api/v1/subtitles", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &got
}

func revisionFiles(t *testing.T) (video, sub string) {
	t.Helper()
	dir := t.TempDir()
	video = filepath.Join(dir, "scene.mp4")
	sub = filepath.Join(dir, "scene.en.srt")
	_ = os.WriteFile(video, bytes.Repeat([]byte{0xAA}, 4096), 0o644)
	_ = os.WriteFile(sub, []byte("body"), 0o644)
	return video, sub
}

func TestRunPush_RevisesAccepted(t *testing.T) {
	srv, got := revisionNode(t, []string{"revisions"}, client.UploadResult{TrackID: 9, ReleaseID: 4, Revision: 3, Supersedes: 5}, http.StatusCreated)
	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })
	video, sub := revisionFiles(t)

	out := captureStdout(t, func() {
		if err := runPush(context.Background(), video, sub, "", true, core.PushOptions{Supersedes: 5}); err != nil {
			t.Fatalf("runPush: %v", err)
		}
	})
	if got.Supersedes != 5 {
		t.Errorf("server saw supersedes=%d", got.Supersedes)
	}
	if want := "uploaded as track 9 (release 4) — revision 3 of track 5\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRunPush_RevisesOnOldNodeFailsBeforeUpload(t *testing.T) {
	srv, got := revisionNode(t, []string{"lookup"}, client.UploadResult{}, http.StatusCreated)
	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })
	video, sub := revisionFiles(t)

	err := runPush(context.Background(), video, sub, "", true, core.PushOptions{Supersedes: 5})
	if err == nil || err.Error() != core.RevisionsUnsupportedMessage {
		t.Fatalf("err = %v, want the unsupported message", err)
	}
	if got.Lang != "" {
		t.Error("an old node must not receive the upload")
	}
}

func TestRunPush_RevisesConflictReadsInPlainWords(t *testing.T) {
	srv, _ := revisionNode(t, []string{"revisions"}, client.UploadResult{}, http.StatusConflict)
	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })
	video, sub := revisionFiles(t)

	err := runPush(context.Background(), video, sub, "", true, core.PushOptions{Supersedes: 5})
	if err == nil || !strings.Contains(core.ExplainError(err).Error(), "no longer the latest version") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunPush_SceneDetailsReachTheServer(t *testing.T) {
	var got client.UploadRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.UploadResult{TrackID: 3, ReleaseID: 4})
	}))
	t.Cleanup(srv.Close)

	video, sub := revisionFiles(t)
	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	scene := core.SceneDetails{Title: "A Title", Date: "2024-03-01", Studio: "Studio", Performers: []string{"Alice", "Bob"}}
	captureStdout(t, func() {
		if err := runPush(context.Background(), video, sub, "en", true, core.PushOptions{SceneDetails: scene}); err != nil {
			t.Fatalf("runPush: %v", err)
		}
	})
	if got.Title != "A Title" || got.Date != "2024-03-01" || got.Studio != "Studio" || len(got.Performers) != 2 {
		t.Errorf("server saw %+v", got)
	}
}

func TestRunPush_BadDateFailsBeforeNetwork(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	t.Cleanup(srv.Close)
	video, sub := revisionFiles(t)
	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	err := runPush(context.Background(), video, sub, "en", true, core.PushOptions{SceneDetails: core.SceneDetails{Date: "03/01/2024"}})
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("err = %v, want a date format error", err)
	}
	if hit {
		t.Error("a bad date must fail before the network")
	}
}

func TestRunDetails_SendsOshashAndReportsOutcome(t *testing.T) {
	var got struct {
		Entries []client.MetadataEntry `json:"entries"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/version" {
			_, _ = w.Write([]byte(`{"version":"t","features":["metadata"]}`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"results":[{"release_id":4,"known":true,"recorded":true}]}`))
	}))
	t.Cleanup(srv.Close)
	video, _ := revisionFiles(t)
	flagServer, flagToken = srv.URL, "tok"
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	out := captureStdout(t, func() {
		if err := runDetails(context.Background(), video, core.SceneDetails{Title: "T"}); err != nil {
			t.Fatalf("runDetails: %v", err)
		}
	})
	if len(got.Entries) != 1 || got.Entries[0].OSHash == "" || got.Entries[0].Title != "T" {
		t.Errorf("server saw %+v", got)
	}
	if !strings.Contains(out, "recorded scene details for release 4") {
		t.Errorf("stdout = %q", out)
	}
}

func TestRunDetails_OldNodeAndNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"t","features":["lookup"]}`))
	}))
	t.Cleanup(srv.Close)
	video, _ := revisionFiles(t)
	flagServer = srv.URL
	t.Cleanup(func() { flagServer, flagToken = "", "" })

	flagToken = ""
	if err := runDetails(context.Background(), video, core.SceneDetails{Title: "T"}); err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("err = %v, want the no-token message", err)
	}
	flagToken = "tok"
	if err := runDetails(context.Background(), video, core.SceneDetails{Title: "T"}); err == nil || err.Error() != core.DetailsUnsupportedMessage {
		t.Errorf("err = %v, want unsupported", err)
	}
	if err := runDetails(context.Background(), video, core.SceneDetails{}); err == nil || err.Error() != core.DetailsEmptyMessage {
		t.Errorf("err = %v, want empty", err)
	}
}

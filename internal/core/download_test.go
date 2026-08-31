package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

// trackServer answers every GET /api/v1/subtitles/{id} with track, recording
// the request path and query the client actually sent.
func trackServer(t *testing.T, track client.Track) (*httptest.Server, *string) {
	t.Helper()
	var lastURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(track)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastURL
}

func TestDownloadTrack_FreshWrite(t *testing.T) {
	srv, _ := trackServer(t, client.Track{ID: 5, Lang: "en", Body: "1\n00:00:01,000 --> 00:00:02,000\nhi\n"})
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	c := client.New(srv.URL, "")

	res, err := DownloadTrack(context.Background(), c, video, 5, 0, "en", false)
	if err != nil {
		t.Fatalf("DownloadTrack: %v", err)
	}
	wantPath := filepath.Join(dir, "scene.en.srt")
	if !res.Created || res.Path != wantPath {
		t.Fatalf("res = %+v, want Created=true Path=%q", res, wantPath)
	}
	body, err := os.ReadFile(res.Path)
	if err != nil || !strings.Contains(string(body), "hi") {
		t.Fatalf("sidecar body = %q, %v", body, err)
	}
}

func TestDownloadTrack_RefusesExistingSidecar(t *testing.T) {
	srv, _ := trackServer(t, client.Track{ID: 5, Lang: "en", Body: "server body"})
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sidecar := filepath.Join(dir, "scene.en.srt")
	if err := os.WriteFile(sidecar, []byte("hand-made"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := client.New(srv.URL, "")

	if _, err := DownloadTrack(context.Background(), c, video, 5, 0, "en", false); !errors.Is(err, ErrSidecarExists) {
		t.Fatalf("err = %v, want ErrSidecarExists", err)
	}
	body, _ := os.ReadFile(sidecar)
	if string(body) != "hand-made" {
		t.Fatalf("refused download must leave the existing sidecar untouched, got %q", body)
	}
}

func TestDownloadTrack_OverwriteReplaces(t *testing.T) {
	srv, _ := trackServer(t, client.Track{ID: 5, Lang: "en", Body: "new body"})
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	sidecar := filepath.Join(dir, "scene.en.srt")
	if err := os.WriteFile(sidecar, []byte("old body"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := client.New(srv.URL, "")

	res, err := DownloadTrack(context.Background(), c, video, 5, 0, "en", true)
	if err != nil {
		t.Fatalf("DownloadTrack: %v", err)
	}
	if res.Created {
		t.Errorf("overwrite: Created = true, want false (existing sidecar was replaced)")
	}
	body, _ := os.ReadFile(sidecar)
	if string(body) != "new body" {
		t.Fatalf("sidecar body = %q, want the fresh download", body)
	}
}

func TestDownloadTrack_RetimedSiblingSendsForRelease(t *testing.T) {
	srv, gotURL := trackServer(t, client.Track{ID: 5, Lang: "en", Body: "retimed"})
	video := filepath.Join(t.TempDir(), "scene.mp4")
	c := client.New(srv.URL, "")

	if _, err := DownloadTrack(context.Background(), c, video, 5, 42, "en", false); err != nil {
		t.Fatalf("DownloadTrack: %v", err)
	}
	if !strings.HasSuffix(*gotURL, "/api/v1/subtitles/5?for_release=42") {
		t.Errorf("server saw request %q, want the sibling retime query for_release=42", *gotURL)
	}
}

func TestDownloadTrack_ZeroForReleaseOmitsQuery(t *testing.T) {
	srv, gotURL := trackServer(t, client.Track{ID: 5, Lang: "en", Body: "as-authored"})
	video := filepath.Join(t.TempDir(), "scene.mp4")
	c := client.New(srv.URL, "")

	if _, err := DownloadTrack(context.Background(), c, video, 5, 0, "en", false); err != nil {
		t.Fatalf("DownloadTrack: %v", err)
	}
	if *gotURL != "/api/v1/subtitles/5" {
		t.Errorf("server saw request %q, want no for_release query", *gotURL)
	}
}

func TestDownloadTrack_ServerErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL, "")

	_, err := DownloadTrack(context.Background(), c, filepath.Join(t.TempDir(), "scene.mp4"), 5, 0, "en", false)
	if err == nil {
		t.Fatal("want error on a server 500")
	}
	if !strings.Contains(err.Error(), "downloading track 5") {
		t.Errorf("err = %v, want it wrapped with the track id", err)
	}
}

func TestDownloadTrack_InvalidLangRejectedBeforeNetworkCall(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL, "")

	if _, err := DownloadTrack(context.Background(), c, filepath.Join(t.TempDir(), "scene.mp4"), 5, 0, "", false); err == nil {
		t.Fatal("want error for a track with no language tag")
	}
	if hit {
		t.Error("an unresolvable language must fail before ever reaching the network")
	}
}

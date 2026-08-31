package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
	"github.com/Anastylosis/mediahash/oshash"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// noFFmpegEnv makes core.FindFFmpeg fail deterministically (empty PATH, no
// env overrides) and core.EnsureFFmpeg fail fast rather than attempt a
// download, so startVideo takes the "match without ffmpeg" (oshash-only)
// path every time — the only fingerprinting that works on an arbitrary
// fixture file with no real ffmpeg present.
func noFFmpegEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("MOANDROP_FFMPEG", "")
	t.Setenv("MOANDROP_FFPROBE", "")
	t.Setenv("MOANDROP_NO_DOWNLOAD", "1")
}

// tempVideo writes a small fixture file distinguishable by seed — oshash
// depends on file size and content, so a different seed reliably yields a
// different oshash.
func tempVideo(t *testing.T, name string, seed byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, bytes.Repeat([]byte{seed}, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func fileOSHash(t *testing.T, path string) string {
	t.Helper()
	h, err := oshash.FromFile(path)
	if err != nil {
		t.Fatalf("oshash.FromFile(%q): %v", path, err)
	}
	return h
}

// lookupBatchHandler answers POST /api/v1/lookup/batch by echoing every
// requested oshash bucket key back with release (or with nothing, when
// release is nil) — enough to drive RankCandidates without reimplementing
// the server's real bucketing.
func lookupBatchHandler(release *client.Release) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OshashPrefixes []string `json:"oshash_prefixes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		results := map[string][]client.Release{}
		if release != nil {
			for _, p := range req.OshashPrefixes {
				results["oshash:"+p] = []client.Release{*release}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Results map[string][]client.Release `json:"results"`
		}{results})
	}
}

// waitReq blocks until the server reports request number want on ch, with
// a deadline.
func waitReq(t *testing.T, ch <-chan int, want int) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("server saw request #%d, want #%d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for request #%d", want)
	}
}

func TestStartVideo_NoMatchRendersStatus(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)

	srv := httptest.NewServer(lookupBatchHandler(nil))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	u.startVideo(tempVideo(t, "scene.mp4", 0xAA))

	waitDo(t, doneCh) // ffmpeg-missing dialog shown
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // "looking up..." status
	waitDo(t, doneCh) // final: no match

	if u.status.Text != core.NoMatchMessage {
		t.Errorf("status = %q, want %q", u.status.Text, core.NoMatchMessage)
	}
	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), core.NoMatchMessage) {
		t.Error("the no-match dialog does not show the expected message")
	}
}

func TestStartVideo_CandidatesRenderEvidenceKindAndBadge(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)

	video := tempVideo(t, "scene.mp4", 0xBB)
	release := client.Release{
		ID: 100, OSHash: fileOSHash(t, video), DurationMs: 600_000,
		Tracks: []client.TrackSummary{
			{ID: 1, Lang: "en", Kind: "sdh", Downloads: 5, Up: 2},
			{ID: 2, Lang: "de", Generated: true},
		},
		Siblings: []client.Sibling{{ID: 3, Lang: "fr", ReleaseID: 7}},
	}
	srv := httptest.NewServer(lookupBatchHandler(&release))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	u.startVideo(video)
	waitDo(t, doneCh) // ffmpeg-missing dialog
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // "looking up..."
	waitDo(t, doneCh) // final render

	if len(u.list.Objects) != 3 { // exact card + sibling-offer card + generated explainer
		t.Fatalf("list has %d children, want 3: %+v", len(u.list.Objects), u.list.Objects)
	}

	all := strings.Join(collectTexts(u.win.Content()), "\n")
	for _, want := range []string{
		"byte-identical file",                      // exact-match evidence
		"another cut of this video — sync unknown", // sibling offer evidence
		"sdh", // non-default kind label on the human-made track
		core.GeneratedExplainer,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("rendered content missing %q; got:\n%s", want, all)
		}
	}
	if findButton(u.win.Content(), core.LabelGenerated) == nil {
		t.Error("no AI badge button rendered for the generated track")
	}
}

func TestDownloadTrack_ClickWritesSidecar(t *testing.T) {
	u, doneCh := newFlowApp(t)
	video := tempVideo(t, "scene.mp4", 0xCC)
	u.videoPath = video

	const body = "1\n00:00:01,000 --> 00:00:02,000\nhi\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.Track{ID: 5, Lang: "en", Body: body})
	}))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	rows := BuildCandidateRows([]core.Candidate{{
		Confidence: core.ConfidenceExact,
		Release:    client.Release{ID: 1, Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}}},
	}})
	u.renderCandidates(rows)

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh) // download finished, status updated

	sidecar := core.SidecarPath(video, core.CaptionLang{Base: "en"})
	got, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar: %v", err)
	}
	if string(got) != body {
		t.Errorf("sidecar body = %q, want %q", got, body)
	}
	if !strings.Contains(u.status.Text, "wrote") {
		t.Errorf("status = %q, want it to report a fresh write", u.status.Text)
	}
}

func TestDownloadTrack_SecondDownloadPopsOverwriteConfirm(t *testing.T) {
	u, doneCh := newFlowApp(t)
	video := tempVideo(t, "scene.mp4", 0xDD)
	u.videoPath = video

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.Track{ID: 5, Lang: "en", Body: "first download"})
	}))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	rows := BuildCandidateRows([]core.Candidate{{
		Confidence: core.ConfidenceExact,
		Release:    client.Release{ID: 1, Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}}},
	}})
	u.renderCandidates(rows)

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh) // first download completes

	sidecar := core.SidecarPath(video, core.CaptionLang{Base: "en"})
	first, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh) // WriteSidecar's existence check refuses; confirm dialog shown

	if findButton(topOverlay(u.win), "Replace") == nil {
		t.Fatal("a second download of an existing sidecar must pop the overwrite confirm")
	}
	again, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(first) {
		t.Errorf("sidecar changed before the overwrite was confirmed: %q -> %q", first, again)
	}
}

func TestDownloadTrack_ConfirmedOverwriteReplaces(t *testing.T) {
	u, doneCh := newFlowApp(t)
	video := tempVideo(t, "scene.mp4", 0xEE)
	u.videoPath = video

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "first"
		if atomic.AddInt32(&calls, 1) > 1 {
			body = "second"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.Track{ID: 5, Lang: "en", Body: body})
	}))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	rows := BuildCandidateRows([]core.Candidate{{
		Confidence: core.ConfidenceExact,
		Release:    client.Release{ID: 1, Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}}},
	}})
	u.renderCandidates(rows)

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh) // first download

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh) // overwrite confirm shown

	tapButtonOn(t, u.win, "Replace")
	waitDo(t, doneCh) // replace completes

	sidecar := core.SidecarPath(video, core.CaptionLang{Base: "en"})
	got, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("sidecar body = %q, want the replaced download", got)
	}
	if !strings.Contains(u.status.Text, "replaced") {
		t.Errorf("status = %q, want it to report a replace", u.status.Text)
	}
}

func TestDownloadTrack_ServerErrorShowsError(t *testing.T) {
	u, doneCh := newFlowApp(t)
	video := tempVideo(t, "scene.mp4", 0xFF)
	u.videoPath = video

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	rows := BuildCandidateRows([]core.Candidate{{
		Confidence: core.ConfidenceExact,
		Release:    client.Release{ID: 1, Tracks: []client.TrackSummary{{ID: 5, Lang: "en"}}},
	}})
	u.renderCandidates(rows)

	tapButtonOn(t, u.win, "Download")
	waitDo(t, doneCh) // error surfaced

	dialogText := strings.ToLower(strings.Join(collectTexts(topOverlay(u.win)), "\n"))
	if !strings.Contains(dialogText, "downloading track 5") {
		t.Errorf("a download failure must surface the wrapped server error in a dialog, got %q", dialogText)
	}
	sidecar := core.SidecarPath(video, core.CaptionLang{Base: "en"})
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Errorf("no sidecar should exist after a failed download, stat err = %v", err)
	}
}

func TestStartVideo_FingerprintErrorShowsError(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)

	missing := filepath.Join(t.TempDir(), "does-not-exist.mp4")
	u.startVideo(missing)

	waitDo(t, doneCh) // ffmpeg-missing dialog
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // FingerprintFile fails reading a nonexistent file

	dialogText := strings.ToLower(strings.Join(collectTexts(topOverlay(u.win)), "\n"))
	if !strings.Contains(dialogText, "oshash:") || !strings.Contains(dialogText, strings.ToLower(filepath.Base(missing))) {
		t.Errorf("dialog text = %q, want the oshash open error surfaced", dialogText)
	}
}

// TestStartVideo_MatchGenInvalidation drives two overlapping matches: video
// A's lookup is stalled server-side until video B's has fully rendered, so
// A's eventual (stale) result must be discarded by the matchGen check
// rather than clobbering B's already-displayed candidates.
func TestStartVideo_MatchGenInvalidation(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)

	videoA := tempVideo(t, "a.mp4", 0x11)
	videoB := tempVideo(t, "b.mp4", 0x22)
	releaseB := client.Release{ID: 200, OSHash: fileOSHash(t, videoB), Tracks: []client.TrackSummary{{ID: 9, Lang: "en"}}}

	received := make(chan int, 4)
	releaseA := make(chan struct{})
	var reqNum int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&reqNum, 1))
		received <- n

		var req struct {
			OshashPrefixes []string `json:"oshash_prefixes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if n == 1 {
			<-releaseA // stall video A's request until the test says go
		}

		results := map[string][]client.Release{}
		if n != 1 { // only B's (and any later) request finds a match
			for _, p := range req.OshashPrefixes {
				results["oshash:"+p] = []client.Release{releaseB}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Results map[string][]client.Release `json:"results"`
		}{results})
	}))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	u.startVideo(videoA)
	waitDo(t, doneCh) // A's ffmpeg-missing dialog
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // A's "looking up..."
	waitReq(t, received, 1)

	u.startVideo(videoB)
	waitDo(t, doneCh) // B's ffmpeg-missing dialog
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // B's "looking up..."
	waitReq(t, received, 2)
	waitDo(t, doneCh) // B's final render

	if len(u.list.Objects) != 1 {
		t.Fatalf("after B: %d list children, want 1 (B's card)", len(u.list.Objects))
	}

	close(releaseA)   // let A's stalled response through
	waitDo(t, doneCh) // A's final fyne.Do — a no-op under the stale gen

	if len(u.list.Objects) != 1 {
		t.Fatalf("after releasing A: %d list children, want B's card still standing untouched", len(u.list.Objects))
	}
}

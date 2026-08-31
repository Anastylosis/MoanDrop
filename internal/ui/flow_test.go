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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

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

// uploadServer answers POST /api/v1/subtitles with result, recording the
// request body the client actually sent.
func uploadServer(t *testing.T, result client.UploadResult) (*httptest.Server, *client.UploadRequest) {
	t.Helper()
	var got client.UploadRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// videoWithSidecar writes a video and one sidecar beside it in the same
// directory (unlike tempVideo, which gets a fresh directory per call and so
// can't produce a video/sidecar pair).
func videoWithSidecar(t *testing.T, videoName, sidecarName string) (video, sidecar string) {
	t.Helper()
	dir := t.TempDir()
	video = filepath.Join(dir, videoName)
	sidecar = filepath.Join(dir, sidecarName)
	if err := os.WriteFile(video, bytes.Repeat([]byte{0x77}, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return video, sidecar
}

func TestRefreshShareSection_RendersDiscoveredSidecar(t *testing.T) {
	u := newTestApp(test.NewApp())
	video, _ := videoWithSidecar(t, "scene.mp4", "scene.en.srt")

	u.refreshShareSection(video)

	if findButton(u.win.Content(), "Share") == nil {
		t.Error("no per-sidecar Share button rendered")
	}
	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), "scene.en.srt (en)") {
		t.Error("share section does not list the discovered sidecar with its language")
	}
	if findButton(u.win.Content(), "Share a subtitle...") == nil {
		t.Error("the manual share button must always be present once a video is loaded")
	}
}

// TestRefreshShareSection_NoSidecarsStillShowsManualButton covers the
// maintainer addendum: sharing must stay reachable even when discovery
// finds nothing.
func TestRefreshShareSection_NoSidecarsStillShowsManualButton(t *testing.T) {
	u := newTestApp(test.NewApp())
	video := tempVideo(t, "scene.mp4", 0x01)

	u.refreshShareSection(video)

	if findButton(u.win.Content(), "Share a subtitle...") == nil {
		t.Fatal("no sidecars found: the manual share button must still render")
	}
	if findButton(u.win.Content(), "Share") != nil {
		t.Error("no sidecars found: no per-sidecar Share button should exist")
	}
}

func TestShareSidecar_WithTokenPushesAndReportsLanguage(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)
	video, _ := videoWithSidecar(t, "scene.mp4", "scene.en.srt")

	srv, gotReq := uploadServer(t, client.UploadResult{TrackID: 8, ReleaseID: 9})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.videoPath = video
	u.refreshShareSection(video)

	tapButtonOn(t, u.win, "Share")
	waitDo(t, doneCh) // ffmpeg resolution fails fast (no download), dispatches the push
	waitDo(t, doneCh) // push completes, status updated

	if gotReq.Lang != "en" {
		t.Errorf("server saw lang %q, want en", gotReq.Lang)
	}
	if !strings.Contains(gotReq.Body, "hi") {
		t.Errorf("server saw body %q, want the sidecar's contents", gotReq.Body)
	}
	if u.status.Text != "uploaded as track 8 (release 9)" {
		t.Errorf("status = %q, want the upload wording", u.status.Text)
	}
}

func TestShareSidecar_DuplicateReadsAsCalmOutcome(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)
	video, _ := videoWithSidecar(t, "scene.mp4", "scene.en.srt")

	srv, _ := uploadServer(t, client.UploadResult{TrackID: 8, ReleaseID: 9, Duplicate: true})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.videoPath = video
	u.refreshShareSection(video)

	tapButtonOn(t, u.win, "Share")
	waitDo(t, doneCh)
	waitDo(t, doneCh)

	if u.status.Text != "already on the node: track 8 (release 9) — nothing new to share" {
		t.Errorf("status = %q, want the calm duplicate wording", u.status.Text)
	}
	if topOverlay(u.win) != nil {
		t.Error("a duplicate push must not surface as an error dialog")
	}
}

func TestShareSidecar_NoTokenOpensTokenDialogThenPushes(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)
	video, _ := videoWithSidecar(t, "scene.mp4", "scene.en.srt")

	srv, gotReq := uploadServer(t, client.UploadResult{TrackID: 1, ReleaseID: 2})
	setServerURL(u.app.Preferences(), srv.URL)
	// No token configured.

	u.videoPath = video
	u.refreshShareSection(video)

	tapButtonOn(t, u.win, "Share")

	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), tokenDialogText) {
		t.Fatal("no token configured: expected the account-token dialog to open")
	}

	entry := findEntry(topOverlay(u.win))
	if entry == nil {
		t.Fatal("token dialog has no entry field")
	}
	test.Type(entry, "fresh-token")
	tapButtonOn(t, u.win, "Save")

	waitDo(t, doneCh) // ffmpeg resolution
	waitDo(t, doneCh) // push completes

	if gotReq.Lang != "en" {
		t.Errorf("server saw lang %q, want en", gotReq.Lang)
	}
	if token(u.app.Preferences()) != "fresh-token" {
		t.Errorf("token() = %q, want the just-saved token to persist", token(u.app.Preferences()))
	}
}

func TestHandlePickedSubtitle_InfersLanguageAndPushes(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)
	video := tempVideo(t, "scene.mp4", 0x02)
	sub := filepath.Join(t.TempDir(), "Some Scene.en.srt")
	if err := os.WriteFile(sub, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, gotReq := uploadServer(t, client.UploadResult{TrackID: 4, ReleaseID: 5})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.videoPath = video
	u.refreshShareSection(video) // no sidecars found; only the manual button
	if findButton(u.win.Content(), "Share a subtitle...") == nil {
		t.Fatal("manual share button must render even with no discovered sidecars")
	}

	// fyne's headless test driver cannot open pickSubtitleToShare's native
	// file dialog, so the picker's callback is driven directly (see
	// handlePickedSubtitle's doc comment).
	u.handlePickedSubtitle(sub)
	waitDo(t, doneCh)
	waitDo(t, doneCh)

	if gotReq.Lang != "en" {
		t.Errorf("server saw lang %q, want en (inferred from the picked file's name)", gotReq.Lang)
	}
}

func TestHandlePickedSubtitle_UnknownLanguageAsksThenPushes(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)
	video := tempVideo(t, "scene.mp4", 0x03)
	sub := filepath.Join(t.TempDir(), "my-subtitle-file.srt") // no parseable language
	if err := os.WriteFile(sub, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, gotReq := uploadServer(t, client.UploadResult{TrackID: 6, ReleaseID: 7})
	setServerURL(u.app.Preferences(), srv.URL)
	setToken(u.app.Preferences(), "tok")

	u.videoPath = video
	u.handlePickedSubtitle(sub)

	all := strings.Join(collectTexts(topOverlay(u.win)), "\n")
	if !strings.Contains(all, filepath.Base(sub)) {
		t.Fatalf("expected a language-ask dialog naming %q, got %q", sub, all)
	}
	entry := findEntry(topOverlay(u.win))
	if entry == nil {
		t.Fatal("language dialog has no entry field")
	}
	test.Type(entry, "de")
	tapButtonOn(t, u.win, "Share")

	waitDo(t, doneCh)
	waitDo(t, doneCh)

	if gotReq.Lang != "de" {
		t.Errorf("server saw lang %q, want de", gotReq.Lang)
	}
}

func TestStartVideo_MultiFileDropPairsVideoAndSubtitle(t *testing.T) {
	noFFmpegEnv(t)
	u, doneCh := newFlowApp(t)

	srv := httptest.NewServer(lookupBatchHandler(nil))
	t.Cleanup(srv.Close)
	setServerURL(u.app.Preferences(), srv.URL)

	video := tempVideo(t, "scene.mp4", 0x04)
	oddlyNamed := filepath.Join(t.TempDir(), "whatever-i-called-it.srt") // unconventional name, explicit drop pairing

	u.startVideo(video, oddlyNamed)
	waitDo(t, doneCh) // ffmpeg-missing dialog
	tapButtonOn(t, u.win, "Match without ffmpeg")
	waitDo(t, doneCh) // "looking up..."
	waitDo(t, doneCh) // final: no match

	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), "whatever-i-called-it.srt") {
		t.Error("explicitly dropped subtitle must be offered for sharing even with an unconventional name")
	}
}

func TestSplitVideoAndSubtitles(t *testing.T) {
	video, subs := splitVideoAndSubtitles([]string{"/x/scene.mp4", "/x/scene.en.srt", "/x/scene.de.vtt"})
	if video != "/x/scene.mp4" || len(subs) != 2 {
		t.Fatalf("video=%q subs=%v", video, subs)
	}

	if v, s := splitVideoAndSubtitles([]string{"/x/a.mp4", "/x/b.mp4", "/x/c.srt"}); v != "" || s != nil {
		t.Errorf("two videos: got video=%q subs=%v, want the explicit shape rejected", v, s)
	}

	if v, s := splitVideoAndSubtitles([]string{"/x/a.srt", "/x/b.srt"}); v != "" || s == nil {
		t.Errorf("no video: got video=%q subs=%v", v, s)
	}
}

// findEntry finds the first *widget.Entry reachable from o.
func findEntry(o fyne.CanvasObject) *widget.Entry {
	var found *widget.Entry
	walkCanvas(o, func(c fyne.CanvasObject) {
		if found != nil {
			return
		}
		if e, ok := c.(*widget.Entry); ok {
			found = e
		}
	})
	return found
}

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

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

func writeSub(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPushSidecar_SendsFingerprintAndBody(t *testing.T) {
	srv, gotReq := uploadServer(t, client.UploadResult{TrackID: 5, ReleaseID: 9})
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	if err := os.WriteFile(video, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := writeSub(t, dir, "scene.en.srt", "1\n00:00:01,000 --> 00:00:02,000\nhi\n")
	c := client.New(srv.URL, "tok")

	res, err := PushSidecar(context.Background(), c, video, "en", mustReadSubtitle(t, sub), "", "", PushOptions{})
	if err != nil {
		t.Fatalf("PushSidecar: %v", err)
	}
	if res.TrackID != 5 || res.ReleaseID != 9 {
		t.Fatalf("res = %+v", res)
	}
	if gotReq.Lang != "en" || gotReq.Body != "1\n00:00:01,000 --> 00:00:02,000\nhi\n" {
		t.Fatalf("server saw req = %+v", gotReq)
	}
	if gotReq.Stem != "scene" {
		t.Errorf("Stem = %q, want %q", gotReq.Stem, "scene")
	}
	if gotReq.OSHash == "" {
		t.Error("server saw no oshash")
	}
}

func TestPushSidecar_EmptyLangRejectedBeforeNetworkCall(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	_ = os.WriteFile(video, []byte("v"), 0o644)
	sub := writeSub(t, dir, "scene.srt", "body")
	c := client.New(srv.URL, "tok")

	if _, err := PushSidecar(context.Background(), c, video, "", mustReadSubtitle(t, sub), "", "", PushOptions{}); err == nil {
		t.Fatal("want error for an empty language")
	}
	if hit {
		t.Error("an unresolved language must fail before ever reaching the network")
	}
}

func TestReadSubtitle_OversizeBodyRejected(t *testing.T) {
	dir := t.TempDir()
	sub := writeSub(t, dir, "scene.en.srt", strings.Repeat("x", MaxTrackBytes+1))

	if _, err := ReadSubtitle(sub); err == nil {
		t.Fatal("want error for an oversize body")
	}
}

func TestPushSidecar_NoTokenSurfacesClientError(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	_ = os.WriteFile(video, []byte("v"), 0o644)
	sub := writeSub(t, dir, "scene.en.srt", "body")
	c := client.New("http://127.0.0.1:0", "") // no token, never actually dialed

	if _, err := PushSidecar(context.Background(), c, video, "en", mustReadSubtitle(t, sub), "", "", PushOptions{}); err == nil {
		t.Fatal("want the client's own no-token error")
	}
}

func TestPushResult_Message(t *testing.T) {
	cases := []struct {
		res  PushResult
		want string
	}{
		{PushResult{TrackID: 1, ReleaseID: 2}, "uploaded as track 1 (release 2)"},
		{PushResult{TrackID: 1, ReleaseID: 2, Generated: true}, "uploaded as track 1 (release 2), detected as AI-generated"},
		{PushResult{TrackID: 1, ReleaseID: 2, Generated: true, GeneratedSource: GeneratedSourceProvenance}, "uploaded as track 1 (release 2), detected as AI-generated"},
		{PushResult{TrackID: 1, ReleaseID: 2, Generated: true, GeneratedSource: GeneratedSourceDeclared}, "uploaded as track 1 (release 2), labelled AI-generated as you declared"},
		// A duplicate stays a duplicate whatever the server says about generation.
		{PushResult{TrackID: 1, ReleaseID: 2, Generated: true, GeneratedSource: GeneratedSourceDeclared, Duplicate: true}, "already on the node: track 1 (release 2) — nothing new to share"},
		{PushResult{TrackID: 1, ReleaseID: 2, Duplicate: true}, "already on the node: track 1 (release 2) — nothing new to share"},
	}
	for _, c := range cases {
		if got := c.res.Message(); got != c.want {
			t.Errorf("Message() = %q, want %q", got, c.want)
		}
	}
}

func mustReadSubtitle(t *testing.T, path string) []byte {
	t.Helper()
	body, err := ReadSubtitle(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestPushSidecar_SendsAuthorshipAndDeclaration(t *testing.T) {
	srv, gotReq := uploadServer(t, client.UploadResult{TrackID: 5, ReleaseID: 9, Generated: true, GeneratedSource: GeneratedSourceDeclared})
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	if err := os.WriteFile(video, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := writeSub(t, dir, "scene.en.srt", "body")
	c := client.New(srv.URL, "tok")

	res, err := PushSidecar(context.Background(), c, video, "en", mustReadSubtitle(t, sub), "", "", PushOptions{Authorship: AuthorshipCredited, Generated: true})
	if err != nil {
		t.Fatalf("PushSidecar: %v", err)
	}
	if gotReq.Authorship != AuthorshipCredited || !gotReq.Generated {
		t.Fatalf("server saw authorship=%q generated=%v, want credited/true", gotReq.Authorship, gotReq.Generated)
	}
	if res.GeneratedSource != GeneratedSourceDeclared {
		t.Errorf("GeneratedSource = %q, want the server's declared answer carried through", res.GeneratedSource)
	}
}

// TestPushSidecar_ZeroOptionsSendNoFields freezes the compatibility rule:
// a server predating the authorship feature must see the request it always
// did — no authorship key, no generated key.
func TestPushSidecar_ZeroOptionsSendNoFields(t *testing.T) {
	var raw map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.UploadResult{TrackID: 1, ReleaseID: 2})
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	if err := os.WriteFile(video, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := writeSub(t, dir, "scene.en.srt", "body")

	if _, err := PushSidecar(context.Background(), client.New(srv.URL, "tok"), video, "en", mustReadSubtitle(t, sub), "", "", PushOptions{}); err != nil {
		t.Fatalf("PushSidecar: %v", err)
	}
	for _, key := range []string{"authorship", "generated"} {
		if _, present := raw[key]; present {
			t.Errorf("request carried %q with zero options; an older server must see no such field", key)
		}
	}
}

func TestPushSidecar_BadAuthorshipRejectedBeforeNetworkCall(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	video := filepath.Join(dir, "scene.mp4")
	if err := os.WriteFile(video, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := writeSub(t, dir, "scene.en.srt", "body")

	_, err := PushSidecar(context.Background(), client.New(srv.URL, "tok"), video, "en", mustReadSubtitle(t, sub), "", "", PushOptions{Authorship: "mine"})
	if err == nil || !strings.Contains(err.Error(), "authorship") {
		t.Fatalf("err = %v, want the authorship vocabulary error", err)
	}
	if hit {
		t.Error("a bad authorship value must fail before reaching the network")
	}
}

func TestValidateAuthorship(t *testing.T) {
	for _, ok := range append([]string{""}, AuthorshipOrder...) {
		if err := ValidateAuthorship(ok); err != nil {
			t.Errorf("ValidateAuthorship(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"Shared", "mine", "credited "} {
		if err := ValidateAuthorship(bad); err == nil {
			t.Errorf("ValidateAuthorship(%q) = nil, want an error", bad)
		}
	}
	for _, a := range AuthorshipOrder {
		if AuthorshipDescriptions[a] == "" {
			t.Errorf("no description for %q", a)
		}
	}
}

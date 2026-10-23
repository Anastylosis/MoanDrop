package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Anastylosis/MoanSubs/client"
)

func TestSceneDetails_Validate(t *testing.T) {
	if err := (SceneDetails{Date: "2024-03-01"}).Validate(); err != nil {
		t.Errorf("valid date: %v", err)
	}
	if err := (SceneDetails{}).Validate(); err != nil {
		t.Errorf("empty: %v", err)
	}
	for _, bad := range []string{"2024-3-1", "01/03/2024", "2024-13-01", "yesterday"} {
		if err := (SceneDetails{Date: bad}).Validate(); err == nil {
			t.Errorf("date %q accepted", bad)
		}
	}
}

func TestParsePerformers(t *testing.T) {
	got := ParsePerformers(" Alice, Bob ,, ")
	if want := []string{"Alice", "Bob"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if ParsePerformers("") != nil {
		t.Error("blank entry must yield no performers")
	}
}

func TestPushSidecar_OmitsZeroSceneDetails(t *testing.T) {
	var raw map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&raw)
		_, _ = w.Write([]byte(`{"track_id":1,"release_id":2}`))
	}))
	t.Cleanup(srv.Close)
	video := filepath.Join(t.TempDir(), "scene.mp4")
	_ = os.WriteFile(video, make([]byte, 4096), 0o644)

	if _, err := PushSidecar(context.Background(), client.New(srv.URL, "tok"), video, "en", []byte("b"), "", "", PushOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"title", "date", "studio", "performers"} {
		if _, ok := raw[k]; ok {
			t.Errorf("%q sent with no details given", k)
		}
	}
}

func TestPushSidecar_BadDateRejectedBeforeFingerprint(t *testing.T) {
	_, err := PushSidecar(context.Background(), client.New("http://unused", "tok"), "/nonexistent.mp4", "en", []byte("b"), "", "",
		PushOptions{SceneDetails: SceneDetails{Date: "nope"}})
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("err = %v, want the date error ahead of the missing file", err)
	}
}

func TestDetailsResult_Message(t *testing.T) {
	cases := []struct {
		r    DetailsResult
		want string
	}{
		{DetailsResult{ReleaseID: 4, Known: true, Recorded: true}, "recorded scene details for release 4 — thank you"},
		{DetailsResult{ReleaseID: 4, Known: true}, "release 4: the node took nothing from those details"},
		{DetailsResult{}, DetailsUnknownMessage},
		{DetailsResult{Known: true, Error: "release withdrawn"}, "the node refused the details: release withdrawn"},
	}
	for _, c := range cases {
		if got := c.r.Message(); got != c.want {
			t.Errorf("Message() = %q, want %q", got, c.want)
		}
	}
}

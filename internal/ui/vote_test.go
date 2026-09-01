package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Anastylosis/MoanSubs/client"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// voteFake fakes the three vote endpoints client.Vote/Unvote/VoteCounts
// hit (PUT cast, DELETE retract, GET counts), tracking every cast request
// body so a test can assert what the client actually sent, and every
// retract call.
type voteFake struct {
	mu   sync.Mutex
	up   int
	down int
	puts []struct {
		Value  int    `json:"value"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	deletes int
}

func newVoteFake(up, down int) *voteFake {
	return &voteFake{up: up, down: down}
}

func (f *voteFake) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/subtitles/{id}/vote", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Value  int    `json:"value"`
			Reason string `json:"reason"`
			Note   string `json:"note"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.puts = append(f.puts, body)
		if body.Value > 0 {
			f.up++
		} else {
			f.down++
		}
		up, down := f.up, f.down
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Up   int `json:"up"`
			Down int `json:"down"`
		}{up, down})
	})
	mux.HandleFunc("DELETE /api/v1/subtitles/{id}/vote", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.deletes++
		// The tests here only ever unvote after an up-vote.
		if f.up > 0 {
			f.up--
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/subtitles/{id}/votes", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		up, down := f.up, f.down
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Up   int `json:"up"`
			Down int `json:"down"`
		}{up, down})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (f *voteFake) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.puts)
}

func (f *voteFake) lastPut() (value int, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.puts) == 0 {
		return 0, ""
	}
	last := f.puts[len(f.puts)-1]
	return last.Value, last.Reason
}

// renderOneTrack sets up a window showing a single track row (with the
// given seed up/down counts) and returns the track's server-side fake.
func renderOneTrack(t *testing.T, up, down int) (u *appUI, doneCh chan struct{}, fake *voteFake) {
	t.Helper()
	u, doneCh = newFlowApp(t)
	fake = newVoteFake(up, down)
	srv := fake.server(t)
	setServerURL(u.app.Preferences(), srv.URL)

	rows := BuildCandidateRows([]core.Candidate{{
		Confidence: core.ConfidenceExact,
		Release: client.Release{
			ID:     1,
			Tracks: []client.TrackSummary{{ID: 42, Lang: "en", Up: up, Down: down}},
		},
	}})
	u.renderCandidates(rows)
	return u, doneCh, fake
}

func TestVote_UpvoteSendsValue1WithEmptyReason(t *testing.T) {
	u, doneCh, fake := renderOneTrack(t, 2, 1)
	setToken(u.app.Preferences(), "tok")

	tapButtonOn(t, u.win, "+1")
	waitDo(t, doneCh)

	if fake.putCount() != 1 {
		t.Fatalf("server saw %d PUT vote requests, want 1", fake.putCount())
	}
	value, reason := fake.lastPut()
	if value != 1 || reason != "" {
		t.Errorf("server saw value=%d reason=%q, want value=1 reason=\"\"", value, reason)
	}
	if u.votes[42] != 1 {
		t.Errorf("u.votes[42] = %d, want 1 (this session's own up-vote remembered)", u.votes[42])
	}
	if findButton(u.win.Content(), "remove vote") == nil {
		t.Error("after voting, the row must offer a remove-vote affordance")
	}
}

func TestVote_DownvoteWithoutReasonIsBlockedClientSide(t *testing.T) {
	u, doneCh, fake := renderOneTrack(t, 0, 0)
	setToken(u.app.Preferences(), "tok")
	_ = doneCh

	tapButtonOn(t, u.win, "-1")
	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), downvoteReasonText) {
		t.Fatal("expected the down-vote reason dialog to open")
	}
	tapButtonOn(t, u.win, "Vote") // entry left blank

	if fake.putCount() != 0 {
		t.Errorf("server saw %d PUT vote requests, want 0 (empty reason must be blocked client-side)", fake.putCount())
	}
	if _, voted := u.votes[42]; voted {
		t.Error("no vote should be remembered when the reason was blank")
	}
}

func TestVote_DownvoteWithReasonSendsReason(t *testing.T) {
	u, doneCh, fake := renderOneTrack(t, 0, 0)
	setToken(u.app.Preferences(), "tok")

	tapButtonOn(t, u.win, "-1")
	entry := findEntry(topOverlay(u.win))
	if entry == nil {
		t.Fatal("down-vote dialog has no entry field")
	}
	test.Type(entry, "out of sync")
	tapButtonOn(t, u.win, "Vote")
	waitDo(t, doneCh)

	if fake.putCount() != 1 {
		t.Fatalf("server saw %d PUT vote requests, want 1", fake.putCount())
	}
	value, reason := fake.lastPut()
	if value != -1 || reason != "out of sync" {
		t.Errorf("server saw value=%d reason=%q, want value=-1 reason=%q", value, reason, "out of sync")
	}
	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), "+0 -1") {
		t.Error("counts label was not updated from the server's response")
	}
}

func TestVote_RemoveVoteCallsUnvoteAndRefreshesCounts(t *testing.T) {
	u, doneCh, fake := renderOneTrack(t, 0, 0)
	setToken(u.app.Preferences(), "tok")

	tapButtonOn(t, u.win, "+1")
	waitDo(t, doneCh)
	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), "+1 -0") {
		t.Fatal("counts did not reflect the up-vote before testing the retract")
	}

	tapButtonOn(t, u.win, "remove vote")
	waitDo(t, doneCh) // Unvote + the VoteCounts refresh land in one fyne.Do

	if fake.deletes != 1 {
		t.Errorf("server saw %d DELETE requests, want 1", fake.deletes)
	}
	if _, voted := u.votes[42]; voted {
		t.Error("u.votes must no longer remember a retracted vote")
	}
	if !strings.Contains(strings.Join(collectTexts(u.win.Content()), "\n"), "+0 -0") {
		t.Error("counts were not refreshed via VoteCounts after the retract")
	}
	if findButton(u.win.Content(), "+1") == nil {
		t.Error("after removing the vote, the +1/-1 buttons must reappear")
	}
}

func TestVote_NoTokenOpensTokenDialogFirstThenVotes(t *testing.T) {
	u, doneCh, fake := renderOneTrack(t, 0, 0)
	// No token configured.

	tapButtonOn(t, u.win, "+1")

	if !strings.Contains(strings.Join(collectTexts(topOverlay(u.win)), "\n"), tokenDialogText) {
		t.Fatal("no token configured: expected the account-token dialog to open before voting")
	}
	if fake.putCount() != 0 {
		t.Fatal("no vote should be sent before a token is supplied")
	}

	entry := findEntry(topOverlay(u.win))
	if entry == nil {
		t.Fatal("token dialog has no entry field")
	}
	test.Type(entry, "fresh-token")
	tapButtonOn(t, u.win, "Save")
	waitDo(t, doneCh) // vote completes once the token flow chains into it

	if fake.putCount() != 1 {
		t.Fatalf("server saw %d PUT vote requests after supplying a token, want 1", fake.putCount())
	}
	if token(u.app.Preferences()) != "fresh-token" {
		t.Errorf("token() = %q, want the just-saved token to persist", token(u.app.Preferences()))
	}
}

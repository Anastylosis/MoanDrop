package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// dialogWindow returns a fresh window with a shown dialog attached to it —
// each case gets its own so a tapped/dismissed dialog from one case can
// never leak an overlay into the next.
func dialogWindow() fyne.Window {
	return test.NewWindow(widget.NewLabel(""))
}

func TestShowAgeGate_Accept(t *testing.T) {
	win := dialogWindow()
	var got *bool
	showAgeGate(win, func(accepted bool) { got = &accepted })

	tapButtonOn(t, win, "I'm 18 or older")
	if got == nil || !*got {
		t.Fatalf("onDecision = %v, want true", got)
	}
}

func TestShowAgeGate_Decline(t *testing.T) {
	win := dialogWindow()
	var got *bool
	showAgeGate(win, func(accepted bool) { got = &accepted })

	tapButtonOn(t, win, "Exit")
	if got == nil || *got {
		t.Fatalf("onDecision = %v, want false — decline must exit, not degrade", got)
	}
}

func TestConfirmOverwrite_Confirm(t *testing.T) {
	win := dialogWindow()
	called := false
	confirmOverwrite(win, "/videos/scene.en.srt", func() { called = true })

	if !strings.Contains(strings.Join(collectTexts(topOverlay(win)), "\n"), "/videos/scene.en.srt") {
		t.Error("confirm dialog never shows the sidecar path")
	}

	tapButtonOn(t, win, "Replace")
	if !called {
		t.Error("onConfirm was not called after tapping Replace")
	}
}

func TestConfirmOverwrite_Cancel(t *testing.T) {
	win := dialogWindow()
	called := false
	confirmOverwrite(win, "/videos/scene.en.srt", func() { called = true })

	tapButtonOn(t, win, "Cancel")
	if called {
		t.Error("onConfirm was called after tapping Cancel, want no callback")
	}
}

func TestShowNoMatch_WithPHashOmitsHint(t *testing.T) {
	win := dialogWindow()
	showNoMatch(win, true)

	all := strings.Join(collectTexts(topOverlay(win)), "\n")
	if !strings.Contains(all, core.NoMatchMessage) {
		t.Errorf("dialog text = %q, want core.NoMatchMessage", all)
	}
	if strings.Contains(all, core.NoPhashOnlyHint) {
		t.Errorf("dialog text = %q, a phash search must not show the oshash-only hint", all)
	}
}

func TestShowNoMatch_WithoutPHashAddsHint(t *testing.T) {
	win := dialogWindow()
	showNoMatch(win, false)

	all := strings.Join(collectTexts(topOverlay(win)), "\n")
	if !strings.Contains(all, core.NoMatchMessage) || !strings.Contains(all, core.NoPhashOnlyHint) {
		t.Errorf("dialog text = %q, want both the message and the oshash-only hint", all)
	}
}

func TestShowFFmpegMissing_FallbackAndWording(t *testing.T) {
	win := dialogWindow()
	wantErr := errors.New("ffmpeg not found: install it")
	called := false
	showFFmpegMissing(win, wantErr, func() { called = true })

	if !strings.Contains(strings.Join(collectTexts(topOverlay(win)), "\n"), wantErr.Error()) {
		t.Errorf("dialog does not render the resolver's own error text %q", wantErr.Error())
	}

	tapButtonOn(t, win, "Match without ffmpeg")
	if !called {
		t.Error("onFallback was not called after tapping Match without ffmpeg")
	}
}

func TestShowFFmpegMissing_Cancel(t *testing.T) {
	win := dialogWindow()
	called := false
	showFFmpegMissing(win, errors.New("nope"), func() { called = true })

	tapButtonOn(t, win, "Cancel")
	if called {
		t.Error("onFallback was called after Cancel, want no callback")
	}
}

func TestPromptDownvoteReason_EmptySubmissionIsDropped(t *testing.T) {
	win := dialogWindow()
	called := false
	promptDownvoteReason(win, func(reason string) { called = true })

	if !strings.Contains(strings.Join(collectTexts(topOverlay(win)), "\n"), downvoteReasonText) {
		t.Error("dialog does not explain why a reason is required")
	}

	tapButtonOn(t, win, "Vote") // entry left blank
	if called {
		t.Error("onReason was called with an empty reason, want it blocked client-side")
	}
}

func TestPromptDownvoteReason_NonEmptySubmissionSendsReason(t *testing.T) {
	win := dialogWindow()
	var got string
	called := false
	promptDownvoteReason(win, func(reason string) {
		called = true
		got = reason
	})

	entry := findEntry(topOverlay(win))
	if entry == nil {
		t.Fatal("down-vote dialog has no entry field")
	}
	test.Type(entry, "bad sync")
	tapButtonOn(t, win, "Vote")

	if !called {
		t.Fatal("onReason was not called for a non-empty reason")
	}
	if got != "bad sync" {
		t.Errorf("reason = %q, want %q", got, "bad sync")
	}
}

func TestPromptDownvoteReason_Cancel(t *testing.T) {
	win := dialogWindow()
	called := false
	promptDownvoteReason(win, func(reason string) { called = true })

	tapButtonOn(t, win, "Cancel")
	if called {
		t.Error("onReason was called after Cancel, want no callback")
	}
}

func TestAskShareOptions_DefaultsToRememberedAuthorshipAndNoDeclaration(t *testing.T) {
	win := dialogWindow()
	var got *core.PushOptions
	askShareOptions(win, "/videos/scene.en.srt", "en", core.AuthorshipUncredited, func(opts core.PushOptions) { got = &opts })

	ov := topOverlay(win)
	if all := strings.Join(collectTexts(ov), "\n"); !strings.Contains(all, "scene.en.srt") {
		t.Errorf("dialog text = %q, want the file name", all)
	}
	// Radio options and a check's text are widget state, not labels, so the
	// text walker can't see them — read the widgets themselves.
	group := findRadioGroup(ov)
	if group == nil {
		t.Fatal("dialog has no authorship choice")
	}
	for _, a := range core.AuthorshipOrder {
		if !slices.Contains(group.Options, core.AuthorshipDescriptions[a]) {
			t.Errorf("dialog never offers %q", a)
		}
	}
	if check := findCheck(ov); check == nil || check.Text != core.GeneratedDeclarationLabel {
		t.Errorf("declaration checkbox = %v, want %q", check, core.GeneratedDeclarationLabel)
	}

	tapButtonOn(t, win, shareOptionsConfirm)
	if got == nil {
		t.Fatal("onShare was not called after confirming")
	}
	if got.Authorship != core.AuthorshipUncredited || got.Generated {
		t.Errorf("opts = %+v, want the remembered authorship and no declaration", *got)
	}
}

func TestAskShareOptions_Cancel(t *testing.T) {
	win := dialogWindow()
	called := false
	askShareOptions(win, "/videos/scene.en.srt", "en", core.AuthorshipShared, func(core.PushOptions) { called = true })

	tapButtonOn(t, win, "Cancel")
	if called {
		t.Error("onShare was called after Cancel, want no callback")
	}
}

func TestShowError_RewordsRateLimit(t *testing.T) {
	win := dialogWindow()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	_, err := client.New(srv.URL, "").Version(context.Background())
	if err == nil {
		t.Fatal("want a 429 error")
	}

	showError(win, err)
	all := strings.Join(collectTexts(topOverlay(win)), "\n")
	if !strings.Contains(all, "30s") {
		t.Errorf("dialog text = %q, want the server's Retry-After wait", all)
	}
}

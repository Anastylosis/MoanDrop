package ui

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
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

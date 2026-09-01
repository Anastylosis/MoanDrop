package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// captureWindow wraps a real test window, recording whatever function
// SetCloseIntercept was last given — the seam applyCloseBehavior's tests
// drive, since a real intercept only ever runs when the OS closes the
// window — and whether its own Hide was called, since a method value
// taken through the fyne.Window interface (w.Hide passed to
// SetCloseIntercept) still dispatches to this override.
type captureWindow struct {
	fyne.Window
	intercept func()
	hidden    bool
}

func (w *captureWindow) SetCloseIntercept(f func()) {
	w.intercept = f
}

func (w *captureWindow) Hide() {
	w.hidden = true
	w.Window.Hide()
}

// quitTrackingApp wraps a test app, recording whether Quit was called —
// test.App's own Quit is a no-op, so this is the only way to observe that
// applyCloseBehavior wired the intercept to the app's Quit.
type quitTrackingApp struct {
	fyne.App
	quit bool
}

func (a *quitTrackingApp) Quit() { a.quit = true }

// fakeDesktopApp satisfies desktop.App on top of a plain test app, since
// fyne's test driver has no system tray of its own — needed to exercise
// applyCloseBehavior's Hide branch, which is gated on desktop.App support.
type fakeDesktopApp struct {
	fyne.App
}

func (fakeDesktopApp) SetSystemTrayMenu(*fyne.Menu)    {}
func (fakeDesktopApp) SetSystemTrayIcon(fyne.Resource) {}
func (fakeDesktopApp) SetSystemTrayWindow(fyne.Window) {}

func TestApplyCloseBehavior_QuitPrefWiresAppQuit(t *testing.T) {
	base := test.NewApp()
	a := &quitTrackingApp{App: base}
	setCloseBehavior(a.Preferences(), closeBehaviorQuit)

	cw := &captureWindow{Window: test.NewWindow(nil)}
	applyCloseBehavior(a, cw)

	if cw.intercept == nil {
		t.Fatal("applyCloseBehavior did not set a close intercept")
	}
	cw.intercept()
	if !a.quit {
		t.Error("the quit preference's intercept did not call the app's Quit")
	}
}

func TestApplyCloseBehavior_QuitPrefWorksWithoutSystemTray(t *testing.T) {
	// Plain test.App does not implement desktop.App — the quit intercept
	// must still be wired, since it's offered precisely for desktops where
	// the tray is unusable.
	base := test.NewApp()
	a := &quitTrackingApp{App: base}
	setCloseBehavior(a.Preferences(), closeBehaviorQuit)

	cw := &captureWindow{Window: test.NewWindow(nil)}
	applyCloseBehavior(a, cw)

	if cw.intercept == nil {
		t.Fatal("quit must be wired even without desktop.App support")
	}
}

func TestApplyCloseBehavior_HidePrefWiresWindowHide(t *testing.T) {
	a := fakeDesktopApp{App: test.NewApp()}

	cw := &captureWindow{Window: test.NewWindow(nil)}
	applyCloseBehavior(a, cw)

	if cw.intercept == nil {
		t.Fatal("applyCloseBehavior did not set a close intercept for the hide (default) preference")
	}
	cw.intercept()
	if !cw.hidden {
		t.Error("the hide preference's intercept did not call the window's Hide")
	}
}

func TestApplyCloseBehavior_HidePrefWithoutTrayLeavesDefaultBehavior(t *testing.T) {
	// No desktop.App support: hiding would strand the window with no way
	// back, so applyCloseBehavior must not wire an intercept at all.
	base := test.NewApp()
	cw := &captureWindow{Window: test.NewWindow(nil)}
	applyCloseBehavior(base, cw)

	if cw.intercept != nil {
		t.Error("the hide preference must not install an intercept when there is no system tray")
	}
}

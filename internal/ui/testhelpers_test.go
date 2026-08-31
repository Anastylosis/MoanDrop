package ui

import (
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// doTrackingDriver wraps the real test driver, serializing every fyne.Do
// call and reporting each one's completion on doneCh. The test driver runs
// DoFromGoroutine inline on whatever goroutine calls it — unlike a real
// desktop driver, which queues to one UI thread — so two concurrent match
// runs would otherwise race each other's widget writes with no lock at
// all; the mutex here restores that single-threaded guarantee, and the
// channel is the only way a test can observe a background goroutine's
// widget write without racing the write itself.
type doTrackingDriver struct {
	fyne.Driver
	mu     sync.Mutex
	doneCh chan struct{}
}

func (d *doTrackingDriver) DoFromGoroutine(fn func(), wait bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Driver.DoFromGoroutine(fn, wait)
	d.doneCh <- struct{}{}
}

type doTrackingApp struct {
	fyne.App
	driver *doTrackingDriver
}

func (a *doTrackingApp) Driver() fyne.Driver { return a.driver }

// newFlowApp builds a window-backed appUI wired to a doTrackingDriver, for
// tests that drive startVideo/runMatch/attemptDownload through their real
// goroutines. The channel reports one value per completed fyne.Do call.
func newFlowApp(t *testing.T) (*appUI, chan struct{}) {
	t.Helper()
	base := test.NewApp()
	doneCh := make(chan struct{}, 32)
	wrapped := &doTrackingApp{App: base, driver: &doTrackingDriver{Driver: base.Driver(), doneCh: doneCh}}
	fyne.SetCurrentApp(wrapped)
	return newTestApp(wrapped), doneCh
}

// waitDo blocks for one fyne.Do call to finish, with a deadline — never a
// bare sleep — so the caller can then safely read whatever it wrote.
func waitDo(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a fyne.Do call to complete")
	}
}

// topOverlay returns the most recently shown dialog/popup on win, or nil if
// none is showing.
func topOverlay(win fyne.Window) fyne.CanvasObject {
	return win.Canvas().Overlays().Top()
}

// canvasChildren returns o's visible children. dialog.Show wraps its popup
// in an unexported overlay container (fyne.io/fyne/v2/internal/widget),
// unreachable by type switch from outside the module — the generic
// fyne.Widget/renderer fallback is what lets a walk actually reach a
// dialog's buttons and labels.
func canvasChildren(o fyne.CanvasObject) []fyne.CanvasObject {
	switch v := o.(type) {
	case *fyne.Container:
		return v.Objects
	case *widget.Card:
		return []fyne.CanvasObject{v.Content}
	case *widget.PopUp:
		return []fyne.CanvasObject{v.Content}
	case *container.Scroll:
		return []fyne.CanvasObject{v.Content}
	case fyne.Widget:
		if r := test.WidgetRenderer(v); r != nil {
			return r.Objects()
		}
	}
	return nil
}

// walkCanvas visits o and every descendant canvasChildren can reach.
func walkCanvas(o fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	visit(o)
	for _, c := range canvasChildren(o) {
		walkCanvas(c, visit)
	}
}

// findButton searches o for a *widget.Button with the given text.
func findButton(o fyne.CanvasObject, text string) *widget.Button {
	var found *widget.Button
	walkCanvas(o, func(c fyne.CanvasObject) {
		if found != nil {
			return
		}
		if b, ok := c.(*widget.Button); ok && b.Text == text {
			found = b
		}
	})
	return found
}

// collectTexts gathers every label/button/card text reachable from o, so a
// test can assert on wording without hard-coding a dialog's or window's layout.
func collectTexts(o fyne.CanvasObject) []string {
	var out []string
	walkCanvas(o, func(c fyne.CanvasObject) {
		switch v := c.(type) {
		case *widget.Label:
			out = append(out, v.Text)
		case *widget.Button:
			out = append(out, v.Text)
		case *widget.Card:
			out = append(out, v.Title, v.Subtitle)
		}
	})
	return out
}

// tapButtonOn finds text among win's main content and its overlays (a shown
// dialog lives in the overlay stack, not the content tree) and taps it,
// failing the test if no such button is visible.
func tapButtonOn(t *testing.T, win fyne.Window, text string) {
	t.Helper()
	if b := findButton(win.Content(), text); b != nil {
		test.Tap(b)
		return
	}
	for _, ov := range win.Canvas().Overlays().List() {
		if b := findButton(ov, text); b != nil {
			test.Tap(b)
			return
		}
	}
	t.Fatalf("no visible button %q", text)
}

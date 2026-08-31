package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

func TestServerURL_DefaultsToCoreDefault(t *testing.T) {
	p := test.NewApp().Preferences()
	if got := serverURL(p); got != core.DefaultServerURL {
		t.Errorf("serverURL() = %q, want core.DefaultServerURL %q", got, core.DefaultServerURL)
	}
}

func TestServerURL_PersistsOverride(t *testing.T) {
	p := test.NewApp().Preferences()
	setServerURL(p, "https://example.invalid")
	if got := serverURL(p); got != "https://example.invalid" {
		t.Errorf("serverURL() = %q, want the saved override", got)
	}
}

func TestAgeGate_DefaultsToNotAccepted(t *testing.T) {
	p := test.NewApp().Preferences()
	if ageGateAccepted(p) {
		t.Error("a fresh install must not start pre-accepted")
	}
	acceptAgeGate(p)
	if !ageGateAccepted(p) {
		t.Error("acceptAgeGate did not persist")
	}
}

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

func TestToken_PreferenceBeforeEnv(t *testing.T) {
	t.Setenv("MOANDROP_TOKEN", "from-env")
	p := test.NewApp().Preferences()
	if got := token(p); got != "from-env" {
		t.Errorf("token() with no preference = %q, want the env fallback", got)
	}
	setToken(p, "from-pref")
	if got := token(p); got != "from-pref" {
		t.Errorf("token() = %q, want the saved preference to win over env", got)
	}
}

func TestToken_EmptyWithNoPreferenceOrEnv(t *testing.T) {
	t.Setenv("MOANDROP_TOKEN", "")
	p := test.NewApp().Preferences()
	if got := token(p); got != "" {
		t.Errorf("token() = %q, want empty", got)
	}
}

package ui

import (
	"os"

	"fyne.io/fyne/v2"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

// appID namespaces Fyne's Preferences storage on disk; changing it would
// orphan every existing user's saved server URL and age-gate acceptance.
const appID = "org.moansubs.moandrop"

const (
	prefServerURL   = "server-url"
	prefAgeAccepted = "age-gate-accepted"
	prefToken       = "account-token"
)

// serverURL falls back to the CLI's own default so the two never diverge.
func serverURL(p fyne.Preferences) string {
	v := p.StringWithFallback(prefServerURL, core.DefaultServerURL)
	if v == "" {
		return core.DefaultServerURL
	}
	return v
}

func setServerURL(p fyne.Preferences, url string) {
	p.SetString(prefServerURL, url)
}

// ageGateAccepted reports whether this install already passed the one-time
// 18+ confirmation (see showAgeGate).
func ageGateAccepted(p fyne.Preferences) bool {
	return p.Bool(prefAgeAccepted)
}

func acceptAgeGate(p fyne.Preferences) {
	p.SetBool(prefAgeAccepted, true)
}

// token resolves the GUI's account token: the saved preference first, then
// MOANDROP_TOKEN as a fallback so a CLI user's env just works in the window
// too. Finding and downloading never call this — only sharing needs one.
func token(p fyne.Preferences) string {
	if v := p.String(prefToken); v != "" {
		return v
	}
	return os.Getenv("MOANDROP_TOKEN")
}

func setToken(p fyne.Preferences, tok string) {
	p.SetString(prefToken, tok)
}

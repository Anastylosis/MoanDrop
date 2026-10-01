package core

import (
	"fmt"
	"strings"
)

// User-facing strings shared by the CLI and the GUI window, so the two
// surfaces cannot drift onto different wording for the same event.

// DefaultServerURL is the moansubs node both surfaces talk to absent an
// override — the CLI's --server default and the GUI's Preferences fallback.
const DefaultServerURL = "https://moansubs.org"

// PrivacyLine is the claim both surfaces make about fingerprinting: it
// happens locally, nothing about the file itself is sent anywhere.
const PrivacyLine = "the video never leaves this machine"

// FingerprintingMessage is printed (CLI) or shown as a busy status (GUI)
// while FingerprintFile runs — it can take several seconds on a large file.
const FingerprintingMessage = "fingerprinting (" + PrivacyLine + ")..."

// NoMatchMessage and NoPhashOnlyHint are the exit-2 "nothing found"
// guidance; NoPhashOnlyHint only applies to an oshash-only lookup (no phash).
const (
	NoMatchMessage  = "no match on the node for this file"
	NoPhashOnlyHint = "(searched by exact file hash only — without --no-phash, other encodes of the same video would also be found)"
)

// GeneratedExplainer is the AI-track disclaimer, shown once under a result
// list (CLI) or as a badge tooltip (GUI).
const GeneratedExplainer = "AI = machine-transcribed, unreviewed. Human-made tracks are listed first;\nan AI track is usually accurate but may mishear names and slang.\nAI (declared) = the uploader said it was AI-made; nothing confirms that, unlike a detected one."

// LabelHuman, LabelGenerated and LabelDeclaredGenerated are the track
// provenance labels a candidate list renders per track. The declared form
// is the server's generated_source "declared": the only AI signal is the
// uploader's own upload-time checkbox, which is worth noting but confirms
// nothing — the marker-detected kind (LabelGenerated) is stronger evidence,
// and the two must never read the same.
const (
	LabelHuman             = "human-made"
	LabelGenerated         = "AI"
	LabelDeclaredGenerated = "AI (declared)"
)

// GeneratedSourceProvenance and GeneratedSourceDeclared mirror the server's
// generated_source vocabulary (feature "authorship"). Absent on a server
// predating it, in which case generated alone means detection.
const (
	GeneratedSourceProvenance = "provenance"
	GeneratedSourceDeclared   = "declared"
)

// GeneratedLabel picks the provenance label for one track from the wire
// fields, so the CLI and the GUI can never label the same track differently.
func GeneratedLabel(generated bool, source string) string {
	switch {
	case !generated:
		return LabelHuman
	case source == GeneratedSourceDeclared:
		return LabelDeclaredGenerated
	default:
		return LabelGenerated
	}
}

// CreditLine words a track's public credit (the server's credited_to, sent
// only for a track its uploader chose to be credited for) the way the
// node's own release page does; empty when there is none.
func CreditLine(creditedTo string) string {
	if creditedTo == "" {
		return ""
	}
	return "by " + creditedTo
}

// RegisterURL and AccountURL are the node's sign-up page and the account
// page that shows the API token; sign-up happens in a browser (the site
// sits behind a challenge), never inside the app.
func RegisterURL(server string) string { return strings.TrimRight(server, "/") + "/register" }

// AccountURL is where a signed-in user finds their token.
func AccountURL(server string) string { return strings.TrimRight(server, "/") + "/me" }

// NoTokenMessage is the push error when no account token is configured.
func NoTokenMessage(server string) string {
	return fmt.Sprintf("no account token — get yours at %s (create an account at %s first), then pass --token or set MOANDROP_TOKEN", AccountURL(server), RegisterURL(server))
}

// FeatureRevisions is the GET /api/v1/version feature that advertises
// supersede-style uploads; RevisionsUnsupportedMessage is what both
// surfaces say on a node without it.
const (
	FeatureRevisions            = "revisions"
	RevisionsUnsupportedMessage = "this node does not support revising a track yet — share it as a new track instead"
)

// The server's revision_declined vocabulary.
const (
	RevisionDeclinedRetime       = "retime"
	RevisionDeclinedTooDifferent = "too_different"
)

// RevisionLabel is the "rev N" tag a track row carries once it has been
// revised; empty for a first version or a node predating revisions.
func RevisionLabel(revision int) string {
	if revision <= 1 {
		return ""
	}
	return fmt.Sprintf("rev %d", revision)
}

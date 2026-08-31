package core

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
const GeneratedExplainer = "AI = machine-transcribed, unreviewed. Human-made tracks are listed first;\nan AI track is usually accurate but may mishear names and slang."

// LabelHuman and LabelGenerated are the two track-provenance labels a
// candidate list renders per track.
const (
	LabelHuman     = "human-made"
	LabelGenerated = "AI"
)

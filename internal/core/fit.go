package core

import "fmt"

// FitRecordedMessage acknowledges a fit report with the pairing's tally.
func FitRecordedMessage(fits, misfits int) string {
	return fmt.Sprintf("recorded — %d fits, %d misfits for this pairing", fits, misfits)
}

// FitRetractedMessage acknowledges a withdrawn fit report.
const FitRetractedMessage = "fit report withdrawn"

// FitAnswerLabel says which verdict was reported, shown beside the undo action.
func FitAnswerLabel(fits bool) string {
	if fits {
		return "You said it fits."
	}
	return "You said it doesn't fit."
}

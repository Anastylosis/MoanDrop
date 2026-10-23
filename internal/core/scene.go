package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Anastylosis/MoanSubs/client"
)

// FeatureMetadata is the node feature that stores uploader-supplied scene
// details and accepts them without a subtitle.
const FeatureMetadata = "metadata"

// DateLayout is the only date shape the server accepts.
const DateLayout = "2006-01-02"

// SceneDetails is what an uploader says a video is. Every field is optional
// and a zero value is left out of the request entirely.
type SceneDetails struct {
	Title      string
	Date       string
	Studio     string
	Performers []string
}

// Validate fails on a malformed date so a typo costs no fingerprint.
func (d SceneDetails) Validate() error {
	if d.Date == "" {
		return nil
	}
	if _, err := time.Parse(DateLayout, d.Date); err != nil {
		return fmt.Errorf("date %q: want YYYY-MM-DD", d.Date)
	}
	return nil
}

// Empty reports whether there is nothing to send.
func (d SceneDetails) Empty() bool {
	return d.Title == "" && d.Date == "" && d.Studio == "" && len(d.Performers) == 0
}

// ParsePerformers splits a comma-separated entry, dropping blanks.
func ParsePerformers(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Trim returns d with surrounding whitespace removed and blank performers dropped.
func (d SceneDetails) Trim() SceneDetails {
	d.Title = strings.TrimSpace(d.Title)
	d.Date = strings.TrimSpace(d.Date)
	d.Studio = strings.TrimSpace(d.Studio)
	var perfs []string
	for _, p := range d.Performers {
		if p = strings.TrimSpace(p); p != "" {
			perfs = append(perfs, p)
		}
	}
	d.Performers = perfs
	return d
}

// DetailsUnsupportedMessage and friends are the details outcome wording.
const (
	DetailsUnsupportedMessage = "this node does not accept scene details yet"
	DetailsEmptyMessage       = "nothing to send — give at least a title, date, studio or performer"
	DetailsUnknownMessage     = "the node does not know this video, so it has nothing to attach details to — share a subtitle for it first"
)

// DetailsResult is the server's per-scene answer to a metadata contribution.
type DetailsResult struct {
	ReleaseID int64
	Known     bool
	Recorded  bool
	Error     string
}

// Message is the one-line outcome both surfaces show verbatim.
func (r DetailsResult) Message() string {
	switch {
	case r.Error != "":
		return fmt.Sprintf("the node refused the details: %s", r.Error)
	case !r.Known:
		return DetailsUnknownMessage
	case !r.Recorded:
		return fmt.Sprintf("release %d: the node took nothing from those details", r.ReleaseID)
	default:
		return fmt.Sprintf("recorded scene details for release %d — thank you", r.ReleaseID)
	}
}

// ContributeDetails sends details for the exact file at videoPath. Only the
// byte hash is computed: the node never creates a release from this, so a
// perceptual hash would add nothing.
func ContributeDetails(ctx context.Context, c *client.Client, videoPath string, d SceneDetails) (DetailsResult, error) {
	d = d.Trim()
	if d.Empty() {
		return DetailsResult{}, errors.New(DetailsEmptyMessage)
	}
	if err := d.Validate(); err != nil {
		return DetailsResult{}, err
	}
	fp, err := FingerprintFile(ctx, "", "", videoPath)
	if err != nil {
		return DetailsResult{}, err
	}
	return ContributeDetailsFor(ctx, c, fp.OSHash.String(), d)
}

// ContributeDetailsFor is ContributeDetails for a caller that already holds
// the file's oshash (the GUI, after a match).
func ContributeDetailsFor(ctx context.Context, c *client.Client, oshash string, d SceneDetails) (DetailsResult, error) {
	d = d.Trim()
	if d.Empty() {
		return DetailsResult{}, errors.New(DetailsEmptyMessage)
	}
	if err := d.Validate(); err != nil {
		return DetailsResult{}, err
	}
	res, err := c.ContributeMetadata(ctx, []client.MetadataEntry{{
		OSHash: oshash, Title: d.Title, Date: d.Date, Studio: d.Studio, Performers: d.Performers,
	}})
	if err != nil {
		return DetailsResult{}, err
	}
	if len(res) != 1 {
		return DetailsResult{}, fmt.Errorf("node answered %d results for one scene", len(res))
	}
	return DetailsResult{ReleaseID: res[0].ReleaseID, Known: res[0].Known, Recorded: res[0].Recorded, Error: res[0].Error}, nil
}

// Wording for the optional scene-details fields and the "add details" action.
const (
	SceneDetailsHeading   = "About this scene (optional)"
	AddSceneDetailsLabel  = "Add scene details"
	SceneDetailsHelp      = "This release has no title yet. Tell the node what it is so others can find it by name."
	DateHintText          = "YYYY-MM-DD"
	PerformersPlaceholder = "performers, comma-separated"
)

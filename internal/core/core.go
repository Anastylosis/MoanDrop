// Package core is MoanDrop's engine: fingerprint a local video the way
// Stash would, look it up on a moansubs node, rank the candidates, and
// write sidecar subtitle files a media player picks up on its own.
//
// It is a headless library on purpose — the CLI at the module root and the
// desktop window both wrap this package, so behavior can never diverge
// between them. The matching logic is ported from MoanSubs' Stash plugin
// (plugin/match.go, plugin/sidecar.go, plugin/download.go), which is the
// reference implementation; when in doubt, diverge from the plugin only
// deliberately.
package core

import (
	"context"
	"fmt"
	"math"

	"github.com/Anastylosis/MoanSubs/hash"
	"github.com/Anastylosis/mediahash/oshash"
	"github.com/Anastylosis/mediahash/videophash"
)

// Fingerprint identifies one local video file the way Stash records it:
// oshash always, phash and duration when ffmpeg was available to compute
// them. PHash nil means "not computed" — lookups still work on oshash
// alone, they just cannot see other encodes of the same video.
type Fingerprint struct {
	OSHash     hash.OSHash
	PHash      *hash.PHash
	DurationMs int64
}

// FingerprintFile hashes videoPath. ffmpegPath/ffprobePath run mediahash's
// videophash pipeline (bit-exact with Stash, which is what makes phash
// matches against the node trustworthy); pass both empty to skip phash and
// duration entirely — oshash needs no external tools.
func FingerprintFile(ctx context.Context, ffmpegPath, ffprobePath, videoPath string) (Fingerprint, error) {
	osh, err := oshash.FromFile(videoPath)
	if err != nil {
		return Fingerprint{}, err
	}
	oh, err := hash.ParseOSHash(osh)
	if err != nil {
		return Fingerprint{}, fmt.Errorf("core: %w", err)
	}
	fp := Fingerprint{OSHash: oh}

	if ffmpegPath == "" && ffprobePath == "" {
		return fp, nil
	}

	d, err := videophash.Duration(ctx, ffprobePath, videoPath)
	if err != nil {
		return Fingerprint{}, err
	}
	p, err := videophash.Generate(ctx, ffmpegPath, videoPath, d)
	if err != nil {
		return Fingerprint{}, err
	}
	ph := hash.PHash(p)
	fp.PHash = &ph
	fp.DurationMs = int64(math.Round(d * 1000))
	return fp, nil
}

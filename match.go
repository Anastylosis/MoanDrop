package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

// exit code 2 means "worked, but no match" — distinct from 1 (error) so a
// shell integration can tell "nothing found" from "something broke".
const exitNoMatch = 2

func matchCmd() *cobra.Command {
	var (
		langs     []string
		write     bool
		overwrite bool
		allLangs  bool
		exact     bool
		noPhash   bool
	)
	cmd := &cobra.Command{
		Use:   "match <video>",
		Short: "Look a video up on the node and list (or write) matching subtitles",
		Long: `Fingerprints the video (oshash, duration, perceptual hash — computed
locally, the file itself is never uploaded), queries the server's bucketed
lookup, and ranks candidates client-side, so the server never learns which
candidate matched. With --write, the best track per requested language is
downloaded and written beside the video as <stem>.<lang>.srt.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMatch(cmd.Context(), args[0], langs, write, overwrite, allLangs, exact, noPhash)
		},
	}
	cmd.Flags().StringSliceVar(&langs, "lang", nil, "language(s) to download, in preference order (e.g. --lang en,de)")
	cmd.Flags().BoolVar(&write, "write", false, "write the best matching subtitle(s) beside the video")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace an existing sidecar (default: refuse)")
	cmd.Flags().BoolVar(&allLangs, "all-languages", false, "with --write: write every language the match has")
	cmd.Flags().BoolVar(&exact, "exact", false, "send full fingerprints for a wider fuzzy search (trades the private bucketed lookup for more reach)")
	cmd.Flags().BoolVar(&noPhash, "no-phash", false, "skip ffmpeg entirely; byte-identical (oshash) matches only")
	return cmd
}

func runMatch(ctx context.Context, videoPath string, langs []string, write, overwrite, allLangs, exact, noPhash bool) error {
	var ffmpeg, ffprobe string
	if !noPhash {
		var err error
		ffmpeg, ffprobe, err = core.EnsureFFmpeg(ctx, flagFFmpeg, flagFFprobe)
		if err != nil {
			return fmt.Errorf("%w (or pass --no-phash for exact-file matches only)", err)
		}
	}

	fmt.Fprintln(os.Stderr, "fingerprinting (the video never leaves this machine)...")
	fp, err := core.FingerprintFile(ctx, ffmpeg, ffprobe, videoPath)
	if err != nil {
		return err
	}

	c := client.New(flagServer, flagToken)
	var releases []client.Release
	if exact {
		releases, err = c.LookupExact(ctx, fp.OSHash, fp.PHash, 8)
	} else {
		releases, err = c.LookupBuckets(ctx, fp.OSHash, fp.PHash)
	}
	if err != nil {
		return err
	}

	candidates := core.RankCandidates(releases, fp, exact)
	if len(candidates) == 0 {
		if flagJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("no match on the node for this file")
			if fp.PHash == nil {
				fmt.Println("(searched by exact file hash only — without --no-phash, other encodes of the same video would also be found)")
			}
		}
		os.Exit(exitNoMatch)
	}

	if flagJSON && !write {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(candidates)
	}

	if !write {
		printCandidates(candidates)
		return nil
	}

	if len(langs) == 0 && !allLangs {
		return fmt.Errorf("--write needs --lang (e.g. --lang en) or --all-languages")
	}
	return writeMatches(ctx, c, videoPath, candidates, langs, allLangs, overwrite)
}

func printCandidates(candidates []core.Candidate) {
	sawGenerated := false
	for _, cand := range candidates {
		fmt.Printf("release %d  %s%s\n", cand.Release.ID, cand.Confidence, evidenceNote(cand))
		for _, t := range cand.Release.Tracks {
			madeBy := "human-made"
			if t.Generated {
				madeBy = "AI"
				sawGenerated = true
			}
			kind := ""
			if t.Kind != "" && t.Kind != "default" {
				kind = " " + t.Kind
			}
			fmt.Printf("  track %-6d %-4s %-10s%s  ↑%d ↓%d  %d downloads\n",
				t.ID, t.Lang, madeBy, kind, t.Up, t.Down, t.Downloads)
		}
	}
	if sawGenerated {
		fmt.Println("\nAI = machine-transcribed, unreviewed. Human-made tracks are listed first;")
		fmt.Println("an AI track is usually accurate but may mishear names and slang.")
	}
}

func evidenceNote(c core.Candidate) string {
	var parts []string
	switch {
	case c.SiblingOf != 0 && c.SiblingSyncKnown && c.SiblingOffsetSource != "duration-delta":
		parts = append(parts, fmt.Sprintf("another cut of this video, verified shift %+dms", c.SiblingOffsetMs))
	case c.SiblingOf != 0 && c.SiblingSyncKnown:
		parts = append(parts, fmt.Sprintf("another cut of this video, estimated shift %+dms — sync unverified", c.SiblingOffsetMs))
	case c.SiblingOf != 0:
		parts = append(parts, "another cut of this video — sync unknown")
	case c.Confidence == core.ConfidenceExact:
		parts = append(parts, "byte-identical file")
	case c.CrossRelease:
		parts = append(parts, fmt.Sprintf("same video, different encode (distance %d, Δ%+dms) — sync usually fine", c.HammingDistance, c.DurationDeltaMs))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, "; ") + ")"
}

func writeMatches(ctx context.Context, c *client.Client, videoPath string, candidates []core.Candidate, langs []string, allLangs, overwrite bool) error {
	top := candidates[0]
	tracks := append([]client.TrackSummary(nil), top.Release.Tracks...)
	core.SortTracksByPreference(tracks, langs)
	selected := core.SelectTracks(tracks, langs, allLangs)
	if len(selected) == 0 {
		fmt.Printf("matched release %d, but it has no track in %s\n", top.Release.ID, strings.Join(langs, ", "))
		os.Exit(exitNoMatch)
	}

	// Only a true sibling grouping asks the server to retime; an ordinary
	// cross-release phash match is served exactly as authored.
	var forRelease int64
	if top.SiblingOf != 0 {
		forRelease = top.Release.ID
	}

	for _, t := range selected {
		lang, err := core.ResolveCaptionLang(t.Lang)
		if err != nil {
			return err
		}
		track, err := c.GetTrackFor(ctx, t.ID, forRelease)
		if err != nil {
			return fmt.Errorf("downloading track %d: %w", t.ID, err)
		}
		path, created, err := core.WriteSidecar(videoPath, lang, track.Body, overwrite)
		if err != nil {
			return err
		}
		verb := "wrote"
		if !created {
			verb = "replaced"
		}
		note := ""
		if track.Generated {
			note = "  (AI-generated subtitle)"
		}
		if lang.Normalized {
			note += fmt.Sprintf("  (%s stored as %s — sidecar names take bare language codes)", lang.Original, lang.Base)
		}
		fmt.Printf("%s %s%s\n", verb, path, note)
	}
	return nil
}

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

	fmt.Fprintln(os.Stderr, core.FingerprintingMessage)
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
			fmt.Println(core.NoMatchMessage)
			if fp.PHash == nil {
				fmt.Println(core.NoPhashOnlyHint)
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
			madeBy := core.GeneratedLabel(t.Generated, t.GeneratedSource)
			if t.Generated {
				sawGenerated = true
			}
			kind := ""
			if t.Kind != "" && t.Kind != "default" {
				kind = " " + t.Kind
			}
			credit := ""
			if line := core.CreditLine(t.CreditedTo); line != "" {
				credit = "  " + line
			}
			fmt.Printf("  track %-6d %-4s %-13s%s  ↑%d ↓%d  %d downloads%s\n",
				t.ID, t.Lang, madeBy, kind, t.Up, t.Down, t.Downloads, credit)
		}
	}
	if sawGenerated {
		fmt.Println("\n" + core.GeneratedExplainer)
	}
}

func evidenceNote(c core.Candidate) string {
	parts := core.EvidenceParts(c)
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

	forRelease := core.ForRelease(top)
	for _, t := range selected {
		res, err := core.DownloadTrack(ctx, c, videoPath, t.ID, forRelease, t.Lang, overwrite)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s%s\n", res.Verb(), res.Path, res.Note())
	}
	return nil
}

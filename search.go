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

func searchCmd() *cobra.Command {
	var (
		lang      string
		writeTo   string
		trackID   int64
		overwrite bool
	)
	cmd := &cobra.Command{
		Use:   "search <query...>",
		Short: "Search the catalogue by title, studio or performer",
		Long: `For a video that matched nothing by fingerprint. Lists releases whose
title, studio or performer matches the query, with their subtitle tracks.

With --write <video>, downloads one track beside that video: name it with
--track <id>, or give --lang when exactly one listed track fits. A track
found this way was timed against some other file, so its sync is unknown.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd.Context(), strings.Join(args, " "), lang, writeTo, trackID, overwrite)
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "", "only releases with a track in this language")
	cmd.Flags().StringVar(&writeTo, "write", "", "download a track beside this video")
	cmd.Flags().Int64Var(&trackID, "track", 0, "with --write: the track id to download")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "with --write: replace an existing sidecar (default: refuse)")
	return cmd
}

func runSearch(ctx context.Context, query, lang, writeTo string, trackID int64, overwrite bool) error {
	if writeTo == "" && (trackID != 0 || overwrite) {
		return fmt.Errorf("--track and --overwrite only apply with --write <video>")
	}
	c := client.New(flagServer, flagToken)
	if ok, err := core.HasFeature(ctx, c, core.FeatureSearch); err == nil && !ok {
		return fmt.Errorf("%s", core.SearchUnsupportedNotice)
	}
	out, err := core.Search(ctx, c, query, lang, printBusy)
	if err != nil {
		return err
	}
	if flagJSON && writeTo == "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(out.Hits) == 0 {
		if !flagJSON {
			fmt.Println(core.NoSearchResultsMessage)
		}
		os.Exit(exitNoMatch)
	}

	if writeTo == "" {
		printSearch(out)
		return nil
	}
	t, err := core.ChooseSearchTrack(out, trackID, lang)
	if err != nil {
		return err
	}
	var res core.DownloadResult
	err = core.WithRetry429(ctx, printBusy, func() (derr error) {
		res, derr = core.DownloadTrack(ctx, c, writeTo, t.ID, 0, t.Lang, overwrite)
		return derr
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s %s%s\n", res.Verb(), res.Path, res.Note())
	fmt.Println(core.SearchSyncUnknown)
	return nil
}

func printSearch(out core.SearchOutcome) {
	sawGenerated := false
	for _, h := range out.Hits {
		fmt.Printf("release %d  %s\n", h.Release.ID, core.ReleaseLabel(h.Release))
		if byline := core.ReleaseByline(h.Release); byline != "" {
			fmt.Printf("  %s\n", byline)
		}
		for _, t := range h.Tracks {
			if t.Generated {
				sawGenerated = true
			}
			fmt.Println(trackLine(t))
		}
	}
	if out.Truncated {
		fmt.Println("\n" + core.SearchTruncatedNote)
	}
	fmt.Println("\n" + core.SearchSyncUnknown)
	if sawGenerated {
		fmt.Println("\n" + core.GeneratedExplainer)
	}
}

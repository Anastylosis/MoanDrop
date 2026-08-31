package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func pushCmd() *cobra.Command {
	var (
		lang    string
		noPhash bool
	)
	cmd := &cobra.Command{
		Use:   "push <video> <subtitle>",
		Short: "Share a subtitle you already have: fingerprint the video and upload the subtitle",
		Long: `Fingerprints the video locally (the video itself is never uploaded — only
its hashes and the subtitle file), then pushes the subtitle to the server so
everyone else with the same video can find it. Needs an account token
(--token or MOANDROP_TOKEN); create an account on the server to get one.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(cmd.Context(), args[0], args[1], lang, noPhash)
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "", "subtitle language (default: read from a <stem>.<lang>.srt filename)")
	cmd.Flags().BoolVar(&noPhash, "no-phash", false, "skip ffmpeg; upload with the exact file hash only")
	return cmd
}

func runPush(ctx context.Context, videoPath, subPath, lang string, noPhash bool) error {
	if lang == "" {
		lang = core.InferSidecarLang(subPath)
		if lang == "" {
			return fmt.Errorf("cannot tell the subtitle's language from %q — pass --lang (e.g. --lang en)", filepath.Base(subPath))
		}
		fmt.Fprintf(os.Stderr, "language %s (from the filename; --lang overrides)\n", lang)
	}

	body, err := os.ReadFile(subPath)
	if err != nil {
		return err
	}
	if len(body) > core.MaxTrackBytes {
		return fmt.Errorf("%s is %d bytes, over the server's %d byte cap for a subtitle", subPath, len(body), core.MaxTrackBytes)
	}

	var ffmpeg, ffprobe string
	if !noPhash {
		ffmpeg, ffprobe, err = core.FindFFmpeg(flagFFmpeg, flagFFprobe)
		if err != nil {
			return fmt.Errorf("%w (or pass --no-phash to upload with the exact file hash only)", err)
		}
	}
	fmt.Fprintln(os.Stderr, "fingerprinting (the video never leaves this machine)...")
	fp, err := core.FingerprintFile(ctx, ffmpeg, ffprobe, videoPath)
	if err != nil {
		return err
	}

	req := client.UploadRequest{
		OSHash:     fp.OSHash.String(),
		DurationMs: fp.DurationMs,
		Lang:       lang,
		Body:       string(body),
		// The filename stem feeds the server's name-based fallback matching;
		// it is the one piece of metadata a non-Stash user reliably has.
		Stem: strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)),
	}
	if fp.PHash != nil {
		req.PHash = fp.PHash.String()
	}

	res, err := client.New(flagServer, flagToken).Upload(ctx, req)
	if err != nil {
		return err
	}
	switch {
	case res.Duplicate:
		fmt.Printf("already on the node: track %d (release %d) — nothing new to share\n", res.TrackID, res.ReleaseID)
	case res.Generated:
		fmt.Printf("uploaded as track %d (release %d), detected as AI-generated\n", res.TrackID, res.ReleaseID)
	default:
		fmt.Printf("uploaded as track %d (release %d)\n", res.TrackID, res.ReleaseID)
	}
	return nil
}

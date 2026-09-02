// MoanDrop is a desktop subtitle finder for the moansubs database, for
// people who do not run Stash: drop (or name) a video file, get a matching
// subtitle written beside it as `<stem>.<lang>.srt`, which Plex, Jellyfin,
// Kodi and VLC pick up with no scan step.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanDrop/internal/ui"
)

// version is stamped by the release build (-ldflags "-X main.version=...").
var version = "dev"

// Global flags. Lookups and downloads are anonymous by design — the token
// is only ever needed for push.
var (
	flagServer  string
	flagToken   string
	flagFFmpeg  string
	flagFFprobe string
	flagJSON    bool
)

func main() {
	// Cobra's Windows mousetrap guard rejects Explorer double-clicks with
	// "This is a command line tool..." before RunE ever runs — invisible
	// in a windowsgui build, so the app just never appears. Double-click
	// IS this app's front door; empty text disables the guard.
	cobra.MousetrapHelpText = ""

	root := &cobra.Command{
		Use:           "moandrop",
		Short:         "Find and share subtitles for your videos by fingerprint, not filename",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// No args: launch the GUI empty. One path: launch it preloaded and start
		// matching immediately, for a file manager's "Open with" (`moandrop "%f"`).
		// Cobra only reaches this RunE when args[0], if any, doesn't name a subcommand.
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var video string
			if len(args) == 1 {
				video = args[0]
			}
			return ui.Run(video)
		},
	}
	root.PersistentFlags().StringVar(&flagServer, "server", envOr("MOANDROP_SERVER", core.DefaultServerURL), "moansubs server URL")
	root.PersistentFlags().StringVar(&flagToken, "token", os.Getenv("MOANDROP_TOKEN"), "account token (only needed for push)")
	root.PersistentFlags().StringVar(&flagFFmpeg, "ffmpeg", "", "path to ffmpeg (default: $MOANDROP_FFMPEG, then PATH)")
	root.PersistentFlags().StringVar(&flagFFprobe, "ffprobe", "", "path to ffprobe (default: $MOANDROP_FFPROBE, then PATH)")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable output")

	root.AddCommand(matchCmd(), pushCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "moandrop:", core.ExplainError(err))
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

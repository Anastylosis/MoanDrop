package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func addSceneFlags(cmd *cobra.Command, d *core.SceneDetails) {
	cmd.Flags().StringVar(&d.Title, "title", "", "scene title, so others can find it by name")
	cmd.Flags().StringVar(&d.Date, "date", "", "release date, YYYY-MM-DD")
	cmd.Flags().StringVar(&d.Studio, "studio", "", "studio")
	cmd.Flags().StringArrayVar(&d.Performers, "performer", nil, "performer name (repeatable)")
}

func detailsCmd() *cobra.Command {
	var scene core.SceneDetails
	cmd := &cobra.Command{
		Use:   "details <video> [--title ...] [--date ...] [--studio ...] [--performer ...]",
		Short: "Tell the node what a video is, without sharing a subtitle",
		Long: `Sends scene details (title, date, studio, performers) for a video the node
already knows. Only the video's exact-file hash is computed and sent; the
node never creates a release from this. Needs an account token (--token or
MOANDROP_TOKEN).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetails(cmd.Context(), args[0], scene)
		},
	}
	addSceneFlags(cmd, &scene)
	return cmd
}

func runDetails(ctx context.Context, videoPath string, scene core.SceneDetails) error {
	scene = scene.Trim()
	if scene.Empty() {
		return errors.New(core.DetailsEmptyMessage)
	}
	if err := scene.Validate(); err != nil {
		return fmt.Errorf("--date: %w", err)
	}
	if flagToken == "" {
		return errors.New(core.NoTokenMessage(flagServer))
	}
	ok, err := core.HasFeature(ctx, client.New(flagServer, ""), core.FeatureMetadata)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(core.DetailsUnsupportedMessage)
	}
	res, err := core.ContributeDetails(ctx, client.New(flagServer, flagToken), videoPath, scene)
	if err != nil {
		return err
	}
	fmt.Println(res.Message())
	return nil
}

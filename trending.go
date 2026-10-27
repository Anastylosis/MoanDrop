package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Anastylosis/MoanDrop/internal/core"
	"github.com/Anastylosis/MoanSubs/client"
)

func trendingCmd() *cobra.Command {
	var days, limit int
	cmd := &cobra.Command{
		Use:   "trending",
		Short: "List the releases people have downloaded most lately",
		Long: `Lists the node's most downloaded releases over a trailing window (the
server's default window and length unless --days / --limit say otherwise;
it clamps out-of-range values). Anonymous, and the counts are aggregate-only.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if days < 0 || limit < 0 {
				return errors.New("--days and --limit must not be negative")
			}
			return runTrending(cmd.Context(), days, limit)
		},
	}
	cmd.Flags().IntVar(&days, "days", 0, "trailing window in days (default: the server's)")
	cmd.Flags().IntVar(&limit, "limit", 0, "how many releases to list (default: the server's)")
	return cmd
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show the node's totals",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStats(cmd.Context())
		},
	}
}

func requireFeature(ctx context.Context, feature, unsupported string) (*client.Client, error) {
	c := client.New(flagServer, "")
	ok, err := core.HasFeature(ctx, c, feature)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New(unsupported)
	}
	return c, nil
}

func runTrending(ctx context.Context, days, limit int) error {
	c, err := requireFeature(ctx, core.FeatureTrending, core.TrendingUnsupportedMessage)
	if err != nil {
		return err
	}
	items, err := c.Trending(ctx, days, limit)
	if err != nil {
		return err
	}
	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	if len(items) == 0 {
		fmt.Println(core.TrendingEmptyMessage)
		return nil
	}
	fmt.Println(core.TrendingHeading(days))
	for i, row := range core.TrendingRows(items) {
		fmt.Printf("%2d. %s\n", i+1, row.Label)
		fmt.Printf("    %s\n", row.Detail())
		fmt.Printf("    %s\n", core.ReleaseURL(flagServer, row.ReleaseID))
	}
	return nil
}

func runStats(ctx context.Context) error {
	c, err := requireFeature(ctx, core.FeatureStats, core.StatsUnsupportedMessage)
	if err != nil {
		return err
	}
	s, err := c.Stats(ctx)
	if err != nil {
		return err
	}
	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	for _, line := range core.StatsLines(s) {
		fmt.Println(line)
	}
	return nil
}

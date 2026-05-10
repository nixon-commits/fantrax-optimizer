package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/nixon-commits/rosterbot/internal/config"
	"github.com/nixon-commits/rosterbot/internal/waivers"
	"github.com/spf13/cobra"
)

var (
	waiversTopN        int
	waiversPositions   string
	waiversProjections string
)

var waiversCmd = &cobra.Command{
	Use:   "waivers",
	Short: "Identify Statcast-driven waiver wire pickups",
	RunE:  runWaivers,
}

func init() {
	waiversCmd.Flags().IntVar(&waiversTopN, "top", 15, "max number of candidates to surface")
	waiversCmd.Flags().StringVar(&waiversPositions, "positions", "", "comma-separated position filter (e.g. \"OF,1B,SP\")")
	waiversCmd.Flags().StringVar(&waiversProjections, "projections", "depthcharts", "projection system: steamer, depthcharts, thebatx, steamer-ros, depthcharts-ros, thebatx-ros")
	rootCmd.AddCommand(waiversCmd)
}

func runWaivers(cmd *cobra.Command, args []string) error {
	today := todayET()
	cfg, err := config.Load(dryRun, []time.Time{today})
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	platform, err := initWaiversPlatform(cfg)
	if err != nil {
		return err
	}

	var positions []string
	if waiversPositions != "" {
		for _, s := range strings.Split(waiversPositions, ",") {
			if t := strings.TrimSpace(s); t != "" {
				positions = append(positions, t)
			}
		}
	}

	opts := waivers.Options{
		TopN:             waiversTopN,
		Positions:        positions,
		ProjectionSystem: waiversProjections,
		NoCache:          noCache,
		DryRun:           cfg.DryRun,
		PushoverUserKey:  cfg.PushoverUserKey,
		PushoverAPIToken: cfg.PushoverAPIToken,
	}
	return waivers.Run(platform, today, opts)
}

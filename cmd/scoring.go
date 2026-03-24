package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var scoringCmd = &cobra.Command{
	Use:   "scoring",
	Short: "Print hitting and pitching scoring weights for this league",
	RunE:  runScoring,
}

func init() {
	rootCmd.AddCommand(scoringCmd)
}

func runScoring(cmd *cobra.Command, args []string) error {
	_, ft, err := initApp(nil)
	if err != nil {
		return err
	}

	hitterWeights, err := ft.GetScoringWeights()
	if err != nil {
		return fmt.Errorf("hitting weights: %w", err)
	}

	pitcherWeights, err := ft.GetPitcherScoringWeights()
	if err != nil {
		return fmt.Errorf("pitching weights: %w", err)
	}

	printWeights("Hitting", hitterWeights)
	fmt.Println()
	printWeights("Pitching", pitcherWeights)
	return nil
}

func printWeights(label string, weights map[string]float64) {
	stats := make([]string, 0, len(weights))
	for k := range weights {
		stats = append(stats, k)
	}
	sort.Strings(stats)

	fmt.Printf("%s scoring weights:\n", label)
	for _, stat := range stats {
		fmt.Printf("  %-6s %+.2f\n", stat, weights[stat])
	}
}

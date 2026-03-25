package cmd

import (
	"time"

	"github.com/nixon-commits/rosterbot/internal/prospects"
	"github.com/spf13/cobra"
)

var prospectsCmd = &cobra.Command{
	Use:   "prospects",
	Short: "Run minor league prospect report",
	RunE:  runProspects,
}

func init() {
	rootCmd.AddCommand(prospectsCmd)
}

func runProspects(cmd *cobra.Command, args []string) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	cfg, ft, err := initApp([]time.Time{today})
	if err != nil {
		return err
	}

	return prospects.RunProspectReport(ft, *cfg, today)
}

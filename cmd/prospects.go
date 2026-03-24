package cmd

import (
	"time"

	"github.com/nixon-commits/fantrax-optimizer/internal/prospects"
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
	today := time.Now().Truncate(24 * time.Hour)

	cfg, ft, err := initApp([]time.Time{today})
	if err != nil {
		return err
	}

	return prospects.RunProspectReport(ft, *cfg, today)
}

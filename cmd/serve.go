package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/nixon-commits/rosterbot/internal/server"
	"github.com/spf13/cobra"
)

var (
	servePort int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web GUI server for lineup visualization",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "HTTP server port")
	serveCmd.Flags().StringVar(&projectionSystem, "projections", "depthcharts", "projection system: steamer, depthcharts, thebatx")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Printf("starting web GUI on http://localhost:%d", servePort)
	log.Printf("projection system: %s", projectionSystem)

	srv := server.New(servePort, projectionSystem)
	return srv.Start(ctx)
}

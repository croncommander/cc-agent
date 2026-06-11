package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set by main.go from ldflags injection
var version = "dev"

// SetVersion sets the agent version from main (injected at build time).
func SetVersion(v string) {
	version = v
	rootCmd.Version = v
}

var rootCmd = &cobra.Command{
	Use:   "cc-agent",
	Short: "CronCommander Agent",
	Long: `CronCommander Agent is a lightweight daemon that connects cron-based systems
to the CronCommander control plane.

It operates in two modes:
  daemon   - Runs as a service, polls the HTTPS gateway
  discover - Scans existing cron entries for import review
  exec     - Wraps a command, captures output, reports to daemon`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

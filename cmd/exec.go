package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/spf13/cobra"
)

var (
	execJobID string
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] -- command [args...]",
	Short: "Execute a command and report results",
	Long: `Execute a command, capture its output and timing, then report the results
to the daemon process via Unix socket.

This command is designed to be called from cron, not by humans directly.

Example:
  cc-agent exec --job-id abc123 -- /path/to/script.sh arg1 arg2`,
	Run:               runExec,
	DisableFlagParsing: false,
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringVarP(&execJobID, "job-id", "j", "", "Job ID for this execution")
}

func runExec(cmd *cobra.Command, args []string) {
	// Find the command after "--"
	commandArgs := args
	if len(commandArgs) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No command specified")
		os.Exit(1)
	}

	// Execute the command
	startTime := time.Now()
	
	// Capture output with size limit to prevent DoS/OOM
	stdout := newLimitedBuffer(0) // 0 uses default limit (256KB)
	stderr := newLimitedBuffer(0)

	execCmd := exec.Command(commandArgs[0], commandArgs[1:]...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	
	err := execCmd.Run()
	
	duration := time.Since(startTime)
	exitCode := 0
	
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
			// Write error to stderr if not already truncated
			stderr.Write([]byte(fmt.Sprintf("\nExecution error: %v", err)))
		}
	}

	// Create execution report
	report := protocol.ExecutionReportPayload{
		JobID:      execJobID,
		Command:    strings.Join(commandArgs, " "),
		ExitCode:   exitCode,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartTime:  startTime.Format(time.RFC3339),
		DurationMs: int(duration.Milliseconds()),
	}

	// Send to daemon via Unix socket
	if err := sendToDaemon(report); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to send report to daemon: %v\n", err)
	}

	// Exit with the same code as the wrapped command
	os.Exit(exitCode)
}

func sendToDaemon(report protocol.ExecutionReportPayload) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon socket: %w", err)
	}
	defer conn.Close()

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}

	return nil
}

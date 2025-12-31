package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/spf13/cobra"
)

var (
	execJobID      string
	execSocketPath string
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] -- command [args...]",
	Short: "Execute a command and report results",
	Long: `Execute a command, capture its output and timing, then report the results
to the daemon process via Unix socket.

This command is designed to be called from cron, not by humans directly.

Example:
  cc-agent exec --job-id abc123 --socket-path /var/lib/croncommander/cc-agent.sock -- /path/to/script.sh arg1 arg2`,
	Run:                runExec,
	DisableFlagParsing: false,
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringVarP(&execJobID, "job-id", "j", "", "Job ID for this execution")
	execCmd.Flags().StringVar(&execSocketPath, "socket-path", "", "Path to daemon socket")
}

func runExec(cmd *cobra.Command, args []string) {
	// Find the command after "--"
	commandArgs := args
	if len(commandArgs) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No command specified")
		os.Exit(1)
	}

	// SECURITY: Collect execution context for audit logging
	executingUID := os.Geteuid()
	executingUser := "unknown"
	var securityWarning string

	currentUser, err := user.Current()
	if err == nil {
		executingUser = currentUser.Username
	}

	// SECURITY: Warn if running as root, but allow it for System Mode.
	// In System Mode, jobs may legitimately run as root.
	if executingUID == 0 {
		securityWarning = "Running as root (UID 0). Ensure this is intentional (System Mode)."
		log.Printf("Warning: %s", securityWarning)
	}

	// SECURITY: Warn if not running as an expected user.
	// Configurable pool allows flexibility for different deployment environments.
	allowedUsers := []string{"cc-agent-user", "root"}
	isAllowedUser := false
	for _, u := range allowedUsers {
		if executingUser == u {
			isAllowedUser = true
			break
		}
	}
	if !isAllowedUser {
		msg := fmt.Sprintf("Running as unexpected user '%s' (expected one of: %v)", executingUser, allowedUsers)
		if securityWarning != "" {
			securityWarning += " | " + msg
		} else {
			securityWarning = msg
		}
		log.Printf("Warning: %s", msg)
	}

	// SECURITY: Set PR_SET_NO_NEW_PRIVS to prevent privilege escalation via setuid binaries.
	// This is Linux-specific (kernel 3.5+); silently skip on other platforms.
	setNoNewPrivs()

	// SECURITY: Prepare minimal execution environment.
	// Do not inherit arbitrary environment variables from parent process.
	// This limits what an attacker can exploit via environment manipulation.
	minimalEnv := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/var/lib/croncommander",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
	}

	// SECURITY: Use a controlled working directory.
	// Jobs execute in a known location with restrictive permissions.
	workDir := "/var/lib/croncommander"

	// Execute the command
	startTime := time.Now()

	stdout := newLimitedBuffer()
	stderr := newLimitedBuffer()
	execCmd := exec.Command(commandArgs[0], commandArgs[1:]...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	execCmd.Env = minimalEnv
	execCmd.Dir = workDir

	err = execCmd.Run()

	duration := time.Since(startTime)
	exitCode := 0

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
			stderr.WriteString(fmt.Sprintf("\nExecution error: %v", err))
		}
	}

	// Create execution report with full audit information.
	// SECURITY: Log exact command, timestamp, UID, and exit status for auditability.
	// Commands are NOT redacted or rewritten.
	report := protocol.ExecutionReportPayload{
		JobID:         execJobID,
		Command:       strings.Join(commandArgs, " "),
		ExitCode:      exitCode,
		ExecutingUID:  executingUID,
		ExecutingUser: executingUser,
		Warning:       securityWarning,
		Stdout:        stdout.String(),
		Stderr:        stderr.String(),
		StartTime:     startTime.Format(time.RFC3339),
		DurationMs:    int(duration.Milliseconds()),
	}

	// Log for local audit trail
	log.Printf("Job executed: job=%s user=%s uid=%d exit=%d cmd=%q",
		execJobID, executingUser, executingUID, exitCode, report.Command)

	// Send to daemon via Unix socket
	if err := sendToDaemon(report); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to send report to daemon: %v\n", err)
	}

	// Exit with the same code as the wrapped command
	os.Exit(exitCode)
}

func sendToDaemon(report protocol.ExecutionReportPayload) error {
	path := socketPath
	if execSocketPath != "" {
		path = execSocketPath
	}
	conn, err := net.Dial("unix", path)
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

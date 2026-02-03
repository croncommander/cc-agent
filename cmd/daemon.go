package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	// secureSocketDir is the directory where the socket should be in production
	secureSocketDir   = "/var/lib/croncommander"
	cronFilePath      = "/etc/cron.d/croncommander"
	heartbeatInterval = 60 * time.Second
	reconnectDelay    = 5 * time.Second
	maxReconnectDelay = 60 * time.Second
)

var (
	daemonKey        string
	daemonServer     string
	daemonConfigFile string
	// socketPath is determined at runtime to support both prod (secure) and dev (tmp) environments.
	socketPath = getSocketPath()
	// socketReadTimeout prevents Slowloris-style DoS attacks on the unix socket.
	// It is a variable to allow overriding in tests.
	socketReadTimeout = 5 * time.Second
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run as a background daemon",
	Long: `Run the cc-agent as a background daemon that:
  - Maintains a WebSocket connection to the CronCommander server
  - Receives job synchronization commands
  - Updates cron configuration (User crontab or System /etc/cron.d)
  - Listens for execution reports from exec mode`,
	Run: runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().StringVarP(&daemonKey, "key", "k", "", "Workspace API key")
	daemonCmd.Flags().StringVarP(&daemonServer, "server", "s", "ws://localhost:8081/agent", "WebSocket server URL")
	daemonCmd.Flags().StringVarP(&daemonConfigFile, "config", "c", "/etc/croncommander/config.yaml", "Path to config file")
}

// getSocketPath determines the socket path based on environment.
func getSocketPath() string {
	// In System Mode (root), use global secure dir.
	// In User Mode, use user's runtime dir or tmp.
	if os.Geteuid() == 0 {
		return filepath.Join(secureSocketDir, "cc-agent.sock")
	}
	// Fallback for non-root: use XDG_RUNTIME_DIR or tmp
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir != "" {
		return filepath.Join(runtimeDir, "cc-agent.sock")
	}
	// Secure fallback: use a private subdirectory in temp
	return filepath.Join(os.TempDir(), fmt.Sprintf("cc-agent-%d", os.Geteuid()), "cc-agent.sock")
}

// getSocketPathWithBase returns the socket path within the given base directory.
// This is primarily exposed for testing to verify path construction logic.
func getSocketPathWithBase(baseDir string) string {
	return filepath.Join(baseDir, "cc-agent.sock")
}

// Config represents the agent configuration
type Config struct {
	ApiKey        string `yaml:"api_key"`
	ServerURL     string `yaml:"server_url"`
	ExecutionMode string `yaml:"execution_mode"` // "user" (default) or "system"
}

func runDaemon(cmd *cobra.Command, args []string) {
	// Load config
	config := loadConfig()

	apiKey := daemonKey
	serverURL := daemonServer
	executionMode := "user"

	if config != nil {
		if apiKey == "" {
			apiKey = config.ApiKey
		}
		if serverURL == "ws://localhost:8081/agent" && config.ServerURL != "" {
			serverURL = config.ServerURL
		}
		if config.ExecutionMode != "" {
			executionMode = config.ExecutionMode
		}
	}

	if apiKey == "" {
		log.Fatal("API key is required. Use --key flag or set api_key in config file")
	}

	// Validation: System mode requires root
	isRoot := os.Geteuid() == 0
	if executionMode == "system" && !isRoot {
		log.Fatal("Execution mode 'system' requires root privileges. Please run as root or switch to 'user' mode.")
	}

	log.Printf("CronCommander Agent starting...")
	log.Printf("Server: %s", serverURL)
	log.Printf("Mode: %s (Root: %v)", executionMode, isRoot)

	// Create daemon instance
	d := &daemon{
		apiKey:        apiKey,
		serverURL:     serverURL,
		hostname:      getHostname(),
		osType:        getOsInfo(),
		executionMode: executionMode,
		isRoot:        isRoot,
	}

	// Start Unix socket listener for exec mode reports
	go d.startSocketListener()

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		d.shutdown()
		os.Exit(0)
	}()

	// Main loop - maintain WebSocket connection
	d.run()
}

func loadConfig() *Config {
	// Try config file
	configPaths := []string{
		daemonConfigFile,
		"/etc/croncommander/config.yaml",
		"/etc/croncommander/config.yml",
		filepath.Join(os.Getenv("HOME"), ".croncommander/config.yaml"),
	}

	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			var config Config
			if err := yaml.Unmarshal(data, &config); err == nil {
				log.Printf("Loaded config from %s", path)
				return &config
			}
		}
	}

	return nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// getOsInfo returns a descriptive OS string.
// On Linux, it reads /etc/os-release to get the distro name and version.
// Falls back to runtime.GOOS if the file is not available.
func getOsInfo() string {
	if runtime.GOOS != "linux" {
		return runtime.GOOS
	}

	// Try to read /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}

	var name, version string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "NAME=") {
			name = parseOsReleaseValue(line[5:])
		} else if strings.HasPrefix(line, "VERSION=") {
			version = parseOsReleaseValue(line[8:])
		}
	}

	if name == "" {
		return runtime.GOOS
	}

	if version != "" {
		return name + " " + version
	}
	return name
}

// parseOsReleaseValue removes quotes from /etc/os-release values
func parseOsReleaseValue(s string) string {
	s = strings.TrimSpace(s)
	// Remove surrounding quotes if present
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		s = s[1 : len(s)-1]
	}
	return s
}

type daemon struct {
	apiKey        string
	serverURL     string
	hostname      string
	osType        string
	executionMode string
	isRoot        bool
	agentID       string
	conn          *websocket.Conn
	connMu        sync.Mutex
	shutdown      func()
}

func (d *daemon) run() {
	currentDelay := reconnectDelay

	for {
		err := d.connect()
		if err != nil {
			log.Printf("Connection failed: %v. Reconnecting in %v...", err, currentDelay)
			time.Sleep(currentDelay)

			// Exponential backoff
			currentDelay *= 2
			if currentDelay > maxReconnectDelay {
				currentDelay = maxReconnectDelay
			}
			continue
		}

		// Reset delay on successful connection
		currentDelay = reconnectDelay

		// Run message loop
		d.messageLoop()

		log.Println("Connection lost. Reconnecting...")
		time.Sleep(currentDelay)
	}
}

func (d *daemon) connect() error {
	u, err := url.Parse(d.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	log.Printf("Connecting to %s...", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}

	d.connMu.Lock()
	d.conn = conn
	d.connMu.Unlock()

	// Send registration
	regMsg := protocol.RegisterMessage{
		Type:          "register",
		ApiKey:        d.apiKey,
		Hostname:      d.hostname,
		Os:            d.osType,
		ExecutionMode: d.executionMode,
		IsRoot:        d.isRoot,
	}

	if err := d.sendMessage(regMsg); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send register message: %w", err)
	}

	log.Println("Connected, waiting for registration response...")
	return nil
}

func (d *daemon) messageLoop() {
	// Start heartbeat goroutine
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)

	go func() {
		for {
			select {
			case <-heartbeatTicker.C:
				if err := d.sendMessage(protocol.HeartbeatMessage{Type: "heartbeat"}); err != nil {
					log.Printf("Failed to send heartbeat: %v", err)
					return
				}
			case <-stopHeartbeat:
				return
			}
		}
	}()

	// Message receive loop
	for {
		_, message, err := d.conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		d.handleMessage(message)
	}
}

func (d *daemon) handleMessage(data []byte) {
	var msg UnifiedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	switch msg.Type {
	case "register_ack":
		if msg.Status == "success" {
			d.agentID = msg.AgentID
			log.Printf("Registration successful. Agent ID: %s", d.agentID)
		} else {
			log.Printf("Registration failed: %s", msg.Reason)
		}

	case "heartbeat_ack":
		log.Println("Heartbeat acknowledged")

	case "sync_jobs":
		log.Printf("Received sync_jobs with %d jobs", len(msg.Jobs))
		d.syncCron(msg.Jobs)

	case "error":
		log.Printf("Server error: %s", msg.Reason)

	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (d *daemon) syncCron(jobs []protocol.JobDefinition) {
	if d.executionMode == "system" {
		d.syncSystemCron(jobs)
	} else {
		d.syncUserCron(jobs)
	}
}

func (d *daemon) syncSystemCron(jobs []protocol.JobDefinition) {
	content := generateCronContent(jobs, true)

	// Write atomically to /etc/cron.d/croncommander
	tmpFile := cronFilePath + ".tmp"
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		log.Printf("Failed to write cron file: %v", err)
		return
	}

	if err := os.Rename(tmpFile, cronFilePath); err != nil {
		log.Printf("Failed to rename cron file: %v", err)
		os.Remove(tmpFile)
		return
	}
	log.Printf("System cron file updated with %d jobs", len(jobs))
}

func (d *daemon) syncUserCron(jobs []protocol.JobDefinition) {
	content := generateCronContent(jobs, false)

	// Use 'crontab -' to install
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = bytes.NewReader(content)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to update user crontab: %v. Output: %s", err, output)
		return
	}
	log.Printf("User crontab updated with %d jobs", len(jobs))
}

func generateCronContent(jobs []protocol.JobDefinition, systemMode bool) []byte {
	var buf bytes.Buffer
	buf.Grow(len(jobs) * 100)

	buf.WriteString("# CronCommander managed cron jobs\n")
	buf.WriteString("# Do not edit this file manually\n")
	buf.WriteString("SHELL=/bin/bash\n")
	buf.WriteString("PATH=/usr/local/bin:/usr/bin:/bin\n\n")

	for _, job := range jobs {
		if containsNewline(job.CronExpression) || containsNewline(job.JobID) || containsNewline(job.Command) {
			log.Printf("Skipping job %q: contains invalid characters", job.JobID)
			continue
		}

		// User mode: <cron> command
		// System mode: <cron> <user> command

		buf.WriteString(job.CronExpression)
		buf.WriteByte(' ')

		if systemMode {
			// In system mode, run jobs as root (for this MVP) since we don't have per-job user config.
			buf.WriteString("root ")
		}

		// Self-executable path
		execPath, err := os.Executable()
		if err != nil {
			execPath = "/usr/local/bin/cc-agent"
		}

		buf.WriteString(execPath)
		buf.WriteString(" exec --job-id ")
		writeShellQuote(&buf, job.JobID)

		// Always pass the socket path explicitly to ensure the job finds the daemon
		// regardless of the user execution context (e.g. non-root job -> root daemon).
		buf.WriteString(" --socket-path ")
		writeShellQuote(&buf, socketPath)

		buf.WriteString(" -- /bin/sh -c ")
		writeShellQuote(&buf, job.Command)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func containsNewline(s string) bool {
	return strings.ContainsAny(s, "\n\r")
}

func writeShellQuote(buf *bytes.Buffer, s string) {
	if s == "" {
		buf.WriteString("''")
		return
	}
	buf.WriteByte('\'')
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			buf.WriteString(s[last:i])
			buf.WriteString("'\\''")
			last = i + 1
		}
	}
	buf.WriteString(s[last:])
	buf.WriteByte('\'')
}

func (d *daemon) sendMessage(msg interface{}) error {
	d.connMu.Lock()
	defer d.connMu.Unlock()

	if d.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return d.conn.WriteMessage(websocket.TextMessage, data)
}

func (d *daemon) startSocketListener() {
	if err := ensureSocketDir(filepath.Dir(socketPath)); err != nil {
		log.Printf("Failed to ensure socket directory: %v", err)
		return
	}

	os.Remove(socketPath)

	oldUmask := syscall.Umask(0117)
	listener, err := net.Listen("unix", socketPath)
	syscall.Umask(oldUmask)

	if err != nil {
		log.Printf("Failed to create socket listener: %v", err)
		return
	}
	defer listener.Close()

	// In user mode, 0660 is fine for user/group access.
	// In system mode, it's in /var/lib/croncommander.
	os.Chmod(socketPath, 0660)

	log.Printf("Listening on %s", socketPath)

	d.shutdown = func() {
		listener.Close()
		d.connMu.Lock()
		if d.conn != nil {
			d.conn.Close()
		}
		d.connMu.Unlock()
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Socket accept error: %v", err)
			continue
		}

		go d.handleSocketConnection(conn)
	}
}

func (d *daemon) handleSocketConnection(conn net.Conn) {
	defer conn.Close()

	// Read execution report from exec mode

	// SECURITY: Set a read deadline to prevent indefinite blocking (Slowloris DoS).
	// If a client connects but sends data too slowly (or not at all), we must timeout
	// to free up resources (goroutines, file descriptors).
	if err := conn.SetReadDeadline(time.Now().Add(socketReadTimeout)); err != nil {
		log.Printf("Failed to set read deadline: %v", err)
		return
	}

	// SECURITY: Limit the size of the request to prevent DoS (OOM) from a local attacker.
	// 1MB is sufficient for legitimate reports (256KB stdout + 256KB stderr + metadata).
	const maxReportSize = 1024 * 1024 // 1MB
	limitReader := io.LimitReader(conn, maxReportSize)

	decoder := json.NewDecoder(limitReader)
	var report protocol.ExecutionReportPayload
	if err := decoder.Decode(&report); err != nil {
		log.Printf("Failed to decode execution report: %v", err)
		return
	}

	log.Printf("Received execution report: job=%s, exitCode=%d", report.JobID, report.ExitCode)

	msg := protocol.ExecutionReportMessage{
		Type:    "execution_report",
		Payload: report,
	}

	if err := d.sendMessage(msg); err != nil {
		log.Printf("Failed to forward execution report: %v", err)
	}
}

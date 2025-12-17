package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	socketPath         = "/tmp/croncommander.sock"
	cronFilePath       = "/etc/cron.d/croncommander"
	heartbeatInterval  = 60 * time.Second
	reconnectDelay     = 5 * time.Second
	maxReconnectDelay  = 60 * time.Second
)

var (
	daemonKey        string
	daemonServer     string
	daemonConfigFile string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run as a background daemon",
	Long: `Run the cc-agent as a background daemon that:
  - Maintains a WebSocket connection to the CronCommander server
  - Receives job synchronization commands
  - Updates /etc/cron.d/croncommander with managed jobs
  - Listens for execution reports from exec mode`,
	Run: runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().StringVarP(&daemonKey, "key", "k", "", "Workspace API key")
	daemonCmd.Flags().StringVarP(&daemonServer, "server", "s", "ws://localhost:8081/agent", "WebSocket server URL")
	daemonCmd.Flags().StringVarP(&daemonConfigFile, "config", "c", "/etc/croncommander/config.yaml", "Path to config file")
}

// Config represents the agent configuration
type Config struct {
	ApiKey    string `yaml:"api_key"`
	ServerURL string `yaml:"server_url"`
}

func runDaemon(cmd *cobra.Command, args []string) {
	// Load config
	config := loadConfig()
	
	apiKey := daemonKey
	serverURL := daemonServer
	
	if config != nil {
		if apiKey == "" {
			apiKey = config.ApiKey
		}
		if serverURL == "ws://localhost:8081/agent" && config.ServerURL != "" {
			serverURL = config.ServerURL
		}
	}

	if apiKey == "" {
		log.Fatal("API key is required. Use --key flag or set api_key in config file")
	}

	log.Printf("CronCommander Agent starting...")
	log.Printf("Server: %s", serverURL)

	// Create daemon instance
	d := &daemon{
		apiKey:    apiKey,
		serverURL: serverURL,
		hostname:  getHostname(),
		osType:    runtime.GOOS,
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

type daemon struct {
	apiKey    string
	serverURL string
	hostname  string
	osType    string
	agentID   string
	conn      *websocket.Conn
	connMu    sync.Mutex
	shutdown  func()
}

func (d *daemon) run() {
	reconnectDelay := reconnectDelay

	for {
		err := d.connect()
		if err != nil {
			log.Printf("Connection failed: %v. Reconnecting in %v...", err, reconnectDelay)
			time.Sleep(reconnectDelay)
			
			// Exponential backoff
			reconnectDelay *= 2
			if reconnectDelay > maxReconnectDelay {
				reconnectDelay = maxReconnectDelay
			}
			continue
		}

		// Reset delay on successful connection
		reconnectDelay = reconnectDelay

		// Run message loop
		d.messageLoop()

		log.Println("Connection lost. Reconnecting...")
		time.Sleep(reconnectDelay)
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
		Type:     "register",
		ApiKey:   d.apiKey,
		Hostname: d.hostname,
		Os:       d.osType,
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

	go func() {
		for range heartbeatTicker.C {
			if err := d.sendMessage(protocol.HeartbeatMessage{Type: "heartbeat"}); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
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
	var baseMsg protocol.Message
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	switch baseMsg.Type {
	case "register_ack":
		var msg protocol.RegisterAckMessage
		json.Unmarshal(data, &msg)
		if msg.Status == "success" {
			d.agentID = msg.AgentID
			log.Printf("Registration successful. Agent ID: %s", d.agentID)
		} else {
			log.Printf("Registration failed: %s", msg.Reason)
		}

	case "heartbeat_ack":
		log.Println("Heartbeat acknowledged")

	case "sync_jobs":
		var msg protocol.SyncJobsMessage
		json.Unmarshal(data, &msg)
		log.Printf("Received sync_jobs with %d jobs", len(msg.Jobs))
		d.syncCronFile(msg.Jobs)

	case "error":
		var msg protocol.ErrorMessage
		json.Unmarshal(data, &msg)
		log.Printf("Server error: %s", msg.Reason)

	default:
		log.Printf("Unknown message type: %s", baseMsg.Type)
	}
}

func generateCronContent(jobs []protocol.JobDefinition) []byte {
	var buf bytes.Buffer
	buf.WriteString("# CronCommander managed cron jobs\n")
	buf.WriteString("# Do not edit this file manually\n")
	buf.WriteString("SHELL=/bin/bash\n")
	buf.WriteString("PATH=/usr/local/bin:/usr/bin:/bin\n\n")

	for _, job := range jobs {
		// Format: <cronExpression> ccrunner /usr/local/bin/cc-agent exec --job-id <jobId> -- <command>
		fmt.Fprintf(&buf, "%s ccrunner /usr/local/bin/cc-agent exec --job-id %s -- %s\n",
			job.CronExpression, job.JobID, job.Command)
	}
	return buf.Bytes()
}

func (d *daemon) syncCronFile(jobs []protocol.JobDefinition) {
	// Generate cron file content
	content := generateCronContent(jobs)

	// Write atomically
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

	log.Printf("Cron file updated with %d jobs", len(jobs))
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
	// Remove existing socket
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Printf("Failed to create socket listener: %v", err)
		return
	}
	defer listener.Close()

	// Make socket writable by ccrunner group
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
	decoder := json.NewDecoder(conn)
	var report protocol.ExecutionReportPayload
	if err := decoder.Decode(&report); err != nil {
		log.Printf("Failed to decode execution report: %v", err)
		return
	}

	log.Printf("Received execution report: job=%s, exitCode=%d", report.JobID, report.ExitCode)

	// Forward to WebSocket
	msg := protocol.ExecutionReportMessage{
		Type:    "execution_report",
		Payload: report,
	}

	if err := d.sendMessage(msg); err != nil {
		log.Printf("Failed to forward execution report: %v", err)
	}
}

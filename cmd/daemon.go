package cmd

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
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
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	secureSocketDir   = "/var/lib/croncommander"
	cronFilePath      = "/etc/cron.d/croncommander"
	defaultServerURL  = "https://gateway.croncommander.com"
	defaultPoll       = 60 * time.Second
	defaultPollJitter = 30 * time.Second
	initialJitter     = 30 * time.Second
	retryDelay        = 5 * time.Second
	maxRetryDelay     = 60 * time.Second
	maxResponseSize   = 1024 * 1024
)

var (
	daemonKey         string
	daemonServer      string
	daemonConfigFile  string
	socketPath        = getSocketPath()
	socketReadTimeout = 5 * time.Second
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run as a background daemon",
	Long: `Run the cc-agent as a background daemon that:
  - Registers and polls the CronCommander gateway over HTTPS
  - Receives versioned job manifests
  - Updates cron configuration (User crontab or System /etc/cron.d)
  - Durably spools execution reports received from exec mode`,
	Run: runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().StringVarP(&daemonKey, "key", "k", "", "Workspace API key")
	daemonCmd.Flags().StringVarP(&daemonServer, "server", "s", defaultServerURL, "HTTPS gateway base URL")
	daemonCmd.Flags().StringVarP(&daemonConfigFile, "config", "c", "/etc/croncommander/config.yaml", "Path to config file")
}

func getSocketPath() string {
	if info, err := os.Stat(secureSocketDir); err == nil && info.IsDir() {
		return filepath.Join(secureSocketDir, "cc-agent.sock")
	}
	if os.Geteuid() == 0 {
		return filepath.Join(secureSocketDir, "cc-agent.sock")
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "cc-agent.sock")
	}
	return filepath.Join(os.TempDir(), "cc-agent-"+os.Getenv("USER")+".sock")
}

func getSocketPathWithBase(baseDir string) string {
	return filepath.Join(baseDir, "cc-agent.sock")
}

type Config struct {
	ApiKey            string `yaml:"api_key"`
	ServerURL         string `yaml:"server_url"`
	ExecutionMode     string `yaml:"execution_mode"`
	StateFile         string `yaml:"state_file"`
	SpoolDir          string `yaml:"spool_dir"`
	AllowInsecureHTTP bool   `yaml:"allow_insecure_http"`
}

type agentState struct {
	AgentID         string `json:"agentId"`
	AgentToken      string `json:"agentToken"`
	ManifestVersion string `json:"manifestVersion,omitempty"`
}

type daemon struct {
	apiKey        string
	serverURL     string
	hostname      string
	osType        string
	executionMode string
	isRoot        bool
	stateFile     string
	spoolDir      string
	state         agentState
	httpClient    *http.Client
	pollInterval  time.Duration
	pollJitter    time.Duration
	stop          chan struct{}
	wake          chan struct{}
	stopOnce      sync.Once
	listenerMu    sync.Mutex
	listener      net.Listener
}

func runDaemon(cmd *cobra.Command, args []string) {
	config, configPath := loadConfig()

	apiKey := daemonKey
	serverURL := daemonServer
	executionMode := "user"
	stateFile, spoolDir := defaultRuntimePaths(configPath)
	allowInsecureHTTP := false

	if config != nil {
		if apiKey == "" {
			apiKey = config.ApiKey
		}
		if serverURL == defaultServerURL && config.ServerURL != "" {
			serverURL = config.ServerURL
		}
		if config.ExecutionMode != "" {
			executionMode = config.ExecutionMode
		}
		if config.StateFile != "" {
			stateFile = config.StateFile
		}
		if config.SpoolDir != "" {
			spoolDir = config.SpoolDir
		}
		allowInsecureHTTP = config.AllowInsecureHTTP
	}

	if apiKey == "" {
		log.Fatal("API key is required. Use --key or set api_key in the config file")
	}

	isRoot := os.Geteuid() == 0
	if executionMode == "system" && !isRoot {
		log.Fatal("Execution mode 'system' requires root privileges")
	}

	normalizedURL, err := normalizeServerURL(serverURL, allowInsecureHTTP)
	if err != nil {
		log.Fatalf("Invalid gateway URL: %v", err)
	}

	d := &daemon{
		apiKey:        apiKey,
		serverURL:     normalizedURL,
		hostname:      getHostname(),
		osType:        getOsInfo(),
		executionMode: executionMode,
		isRoot:        isRoot,
		stateFile:     stateFile,
		spoolDir:      spoolDir,
		httpClient:    newHTTPClient(),
		pollInterval:  defaultPoll,
		pollJitter:    defaultPollJitter,
		stop:          make(chan struct{}),
		wake:          make(chan struct{}, 1),
	}

	if err := d.loadState(); err != nil {
		log.Fatalf("Failed to load agent state: %v", err)
	}

	log.Printf("CronCommander Agent starting")
	log.Printf("Gateway: %s", d.serverURL)
	log.Printf("Mode: %s (Root: %v)", executionMode, isRoot)

	go d.startSocketListener()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			log.Println("Shutting down")
			d.shutdown()
		case <-d.stop:
		}
	}()

	d.run()
}

func loadConfig() (*Config, string) {
	configPaths := []string{
		daemonConfigFile,
		"/etc/croncommander/config.yaml",
		"/etc/croncommander/config.yml",
		filepath.Join(os.Getenv("HOME"), ".croncommander/config.yaml"),
	}

	seen := make(map[string]bool)
	for _, path := range configPaths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var config Config
		if err := yaml.Unmarshal(data, &config); err != nil {
			log.Printf("Ignoring invalid config %s: %v", path, err)
			continue
		}
		log.Printf("Loaded config from %s", path)
		return &config, path
	}
	return nil, ""
}

func defaultRuntimePaths(configPath string) (string, string) {
	baseDir := ""
	if configPath != "" {
		baseDir = filepath.Dir(configPath)
	} else if home, err := os.UserHomeDir(); err == nil {
		baseDir = filepath.Join(home, ".croncommander")
	} else {
		baseDir = os.TempDir()
	}
	return filepath.Join(baseDir, "agent-state.json"), filepath.Join(baseDir, "spool")
}

func normalizeServerURL(rawURL string, allowInsecureHTTP bool) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("scheme must be https")
	}
	if parsed.Scheme == "http" && !allowInsecureHTTP {
		return "", fmt.Errorf("plain HTTP requires allow_insecure_http: true and is only for local development")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("must be an origin URL without credentials, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("must not include an API path")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (d *daemon) run() {
	if !d.sleep(randomDuration(0, initialJitter)) {
		return
	}

	currentRetry := retryDelay
	for {
		err := d.cycle()
		if err == nil {
			currentRetry = retryDelay
			if !d.sleep(randomPollDelay(d.pollInterval, d.pollJitter)) {
				return
			}
			continue
		}

		if status := statusCode(err); status == http.StatusUnauthorized {
			if clearErr := d.clearCredentials(); clearErr != nil {
				log.Printf("Failed to clear rejected credentials: %v", clearErr)
			}
		}

		delay := jitteredRetry(currentRetry)
		log.Printf("Gateway request failed: %v; retrying in %v", err, delay.Round(time.Second))
		if !d.sleep(delay) {
			return
		}
		currentRetry *= 2
		if currentRetry > maxRetryDelay {
			currentRetry = maxRetryDelay
		}
	}
}

func (d *daemon) cycle() error {
	if d.state.AgentID == "" || d.state.AgentToken == "" {
		if err := d.register(); err != nil {
			return err
		}
	}
	if err := d.flushSpool(); err != nil {
		return err
	}
	return d.poll()
}

func (d *daemon) register() error {
	request := protocol.RegisterRequest{
		Hostname:      d.hostname,
		Os:            d.osType,
		ExecutionMode: d.executionMode,
		IsRoot:        d.isRoot,
		Version:       version,
	}
	var response protocol.RegisterResponse
	err := d.postJSON("/api/v2/agents/register", map[string]string{
		"X-CC-API-Key": d.apiKey,
	}, request, &response)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	if response.AgentID == "" || response.AgentToken == "" {
		return errors.New("registration response omitted agent credentials")
	}

	d.state.AgentID = response.AgentID
	d.state.AgentToken = response.AgentToken
	if response.PollIntervalSeconds > 0 {
		d.pollInterval = time.Duration(response.PollIntervalSeconds) * time.Second
	}
	if response.PollJitterSeconds >= 0 {
		d.pollJitter = time.Duration(response.PollJitterSeconds) * time.Second
	}
	if err := d.saveState(); err != nil {
		d.state.AgentID = ""
		d.state.AgentToken = ""
		return fmt.Errorf("persist registration credentials: %w", err)
	}
	log.Printf("Registration successful. Agent ID: %s", d.state.AgentID)
	return nil
}

func (d *daemon) poll() error {
	request := protocol.PollRequest{
		ManifestVersion: d.state.ManifestVersion,
		Version:         version,
	}
	var response protocol.PollResponse
	path := fmt.Sprintf("/api/v2/agents/%s/poll", url.PathEscape(d.state.AgentID))
	err := d.postJSON(path, d.authorizationHeaders(), request, &response)
	if err != nil {
		return fmt.Errorf("poll failed: %w", err)
	}
	if response.ManifestVersion == "" {
		return errors.New("poll response omitted manifest version")
	}
	if !response.Changed {
		return nil
	}
	if err := d.syncCron(response.Jobs); err != nil {
		return fmt.Errorf("apply job manifest: %w", err)
	}
	d.state.ManifestVersion = response.ManifestVersion
	if err := d.saveState(); err != nil {
		return fmt.Errorf("persist manifest version: %w", err)
	}
	log.Printf("Applied job manifest %s with %d jobs", response.ManifestVersion, len(response.Jobs))
	return nil
}

func (d *daemon) syncCron(jobs []protocol.JobDefinition) error {
	if d.executionMode == "system" {
		return d.syncSystemCron(jobs)
	}
	return d.syncUserCron(jobs)
}

func (d *daemon) syncSystemCron(jobs []protocol.JobDefinition) error {
	content := generateCronContentWithSpool(jobs, true, d.spoolDir)
	tmpFile := cronFilePath + ".tmp"
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		return fmt.Errorf("write cron file: %w", err)
	}
	if err := os.Rename(tmpFile, cronFilePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("replace cron file: %w", err)
	}
	return nil
}

func (d *daemon) syncUserCron(jobs []protocol.JobDefinition) error {
	content := generateCronContentWithSpool(jobs, false, d.spoolDir)
	command := exec.Command("crontab", "-")
	command.Stdin = bytes.NewReader(content)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update user crontab: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func generateCronContent(jobs []protocol.JobDefinition, systemMode bool) []byte {
	return generateCronContentWithSpool(
		jobs,
		systemMode,
		filepath.Join(filepath.Dir(socketPath), "spool"),
	)
}

func generateCronContentWithSpool(jobs []protocol.JobDefinition, systemMode bool, spoolDir string) []byte {
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

		buf.WriteString(job.CronExpression)
		buf.WriteByte(' ')
		if systemMode {
			buf.WriteString("root ")
		}

		execPath, err := os.Executable()
		if err != nil {
			execPath = "/usr/local/bin/cc-agent"
		}
		buf.WriteString(execPath)
		buf.WriteString(" exec --job-id ")
		writeShellQuote(&buf, job.JobID)
		buf.WriteString(" --socket-path ")
		writeShellQuote(&buf, socketPath)
		buf.WriteString(" --spool-dir ")
		writeShellQuote(&buf, spoolDir)
		buf.WriteString(" -- /bin/sh -c ")
		writeShellQuote(&buf, job.Command)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func containsNewline(value string) bool {
	return strings.ContainsAny(value, "\n\r")
}

func writeShellQuote(buf *bytes.Buffer, value string) {
	if value == "" {
		buf.WriteString("''")
		return
	}
	buf.WriteByte('\'')
	last := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\'' {
			buf.WriteString(value[last:i])
			buf.WriteString("'\\''")
			last = i + 1
		}
	}
	buf.WriteString(value[last:])
	buf.WriteByte('\'')
}

func (d *daemon) startSocketListener() {
	if err := prepareSocketPath(socketPath); err != nil {
		log.Printf("Failed to prepare socket: %v", err)
		d.shutdown()
		return
	}

	oldUmask := syscall.Umask(0117)
	listener, err := net.Listen("unix", socketPath)
	syscall.Umask(oldUmask)
	if err != nil {
		log.Printf("Failed to create socket listener: %v", err)
		d.shutdown()
		return
	}

	d.listenerMu.Lock()
	d.listener = listener
	d.listenerMu.Unlock()
	defer listener.Close()

	select {
	case <-d.stop:
		return
	default:
	}

	if err := os.Chmod(socketPath, 0660); err != nil {
		log.Printf("Failed to secure socket permissions: %v", err)
		d.shutdown()
		return
	}
	log.Printf("Listening on %s", socketPath)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-d.stop:
				return
			default:
				log.Printf("Socket accept error: %v", err)
				continue
			}
		}
		go d.handleSocketConnection(conn)
	}
}

func prepareSocketPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to replace non-socket path %s", path)
	}
	return os.Remove(path)
}

func (d *daemon) handleSocketConnection(conn net.Conn) {
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(socketReadTimeout)); err != nil {
		log.Printf("Failed to set socket read deadline: %v", err)
		return
	}

	decoder := json.NewDecoder(io.LimitReader(conn, 1024*1024))
	var report protocol.LocalExecutionReport
	if err := decoder.Decode(&report); err != nil || !validEventID(report.EventID) {
		d.writeSocketAck(conn, false, "invalid execution report")
		log.Printf("Failed to decode execution report: %v", err)
		return
	}

	if err := writeSpoolRecord(d.spoolDir, report); err != nil {
		d.writeSocketAck(conn, false, "failed to persist execution report")
		log.Printf("Failed to spool execution report: %v", err)
		return
	}

	d.writeSocketAck(conn, true, "")
	log.Printf("Execution report spooled: event=%s job=%s exitCode=%d",
		report.EventID, report.Payload.JobID, report.Payload.ExitCode)
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *daemon) writeSocketAck(conn net.Conn, accepted bool, message string) {
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(conn).Encode(protocol.LocalReportAck{Accepted: accepted, Error: message}); err != nil {
		log.Printf("Failed to acknowledge local execution report: %v", err)
	}
}

func (d *daemon) spoolReport(payload protocol.ExecutionReportPayload) (string, error) {
	eventID, err := newEventID()
	if err != nil {
		return "", err
	}
	record := protocol.SpooledReport{EventID: eventID, Payload: payload}
	if err := writeSpoolRecord(d.spoolDir, record); err != nil {
		return "", err
	}
	return eventID, nil
}

func writeSpoolRecord(spoolDir string, record protocol.SpooledReport) error {
	if !validEventID(record.EventID) {
		return errors.New("execution report event ID is not a UUID")
	}
	if err := ensurePrivateDir(spoolDir); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(spoolDir, record.EventID+".json"), record)
}

func (d *daemon) flushSpool() error {
	if err := ensurePrivateDir(d.spoolDir); err != nil {
		return fmt.Errorf("prepare report spool: %w", err)
	}
	entries, err := os.ReadDir(d.spoolDir)
	if err != nil {
		return fmt.Errorf("read report spool: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(d.spoolDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect spooled report: %w", err)
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxResponseSize {
			log.Printf("Rejecting unsafe spool file %s", entry.Name())
			if rejectErr := d.rejectSpoolFile(path); rejectErr != nil {
				return rejectErr
			}
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read spooled report: %w", err)
		}
		var record protocol.SpooledReport
		if err := json.Unmarshal(data, &record); err != nil || record.EventID == "" {
			log.Printf("Rejecting malformed spool file %s", entry.Name())
			if rejectErr := d.rejectSpoolFile(path); rejectErr != nil {
				return rejectErr
			}
			continue
		}

		err = d.sendReport(record)
		if err == nil {
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("remove acknowledged report: %w", removeErr)
			}
			continue
		}
		status := statusCode(err)
		if status >= 400 && status < 500 && status != 401 && status != 408 && status != 429 {
			log.Printf("Gateway permanently rejected report %s: %v", record.EventID, err)
			if rejectErr := d.rejectSpoolFile(path); rejectErr != nil {
				return rejectErr
			}
			continue
		}
		return err
	}
	return nil
}

func (d *daemon) sendReport(record protocol.SpooledReport) error {
	path := fmt.Sprintf("/api/v2/agents/%s/execution-reports", url.PathEscape(d.state.AgentID))
	headers := d.authorizationHeaders()
	headers["Idempotency-Key"] = record.EventID
	var response protocol.ReportResponse
	if err := d.postJSON(path, headers, record.Payload, &response); err != nil {
		return err
	}
	if response.ExecutionID == "" {
		return errors.New("report response omitted execution ID")
	}
	return nil
}

func (d *daemon) rejectSpoolFile(path string) error {
	rejectedDir := filepath.Join(d.spoolDir, "rejected")
	if err := ensurePrivateDir(rejectedDir); err != nil {
		return fmt.Errorf("prepare rejected spool: %w", err)
	}
	if err := os.Rename(path, filepath.Join(rejectedDir, filepath.Base(path))); err != nil {
		return fmt.Errorf("quarantine rejected report: %w", err)
	}
	return nil
}

func (d *daemon) authorizationHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer " + d.state.AgentToken}
}

func (d *daemon) postJSON(path string, headers map[string]string, requestBody any, responseBody any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, d.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "cc-agent/"+version)
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := d.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	responseData, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return err
	}
	if len(responseData) > maxResponseSize {
		return errors.New("gateway response exceeded 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &httpStatusError{
			status: response.StatusCode,
			body:   strings.TrimSpace(string(responseData)),
		}
	}
	if responseBody != nil && len(responseData) > 0 {
		if err := json.Unmarshal(responseData, responseBody); err != nil {
			return fmt.Errorf("decode gateway response: %w", err)
		}
	}
	return nil
}

func (d *daemon) loadState() error {
	data, err := os.ReadFile(d.stateFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &d.state); err != nil {
		return fmt.Errorf("decode %s: %w", d.stateFile, err)
	}
	return nil
}

func (d *daemon) saveState() error {
	return writeJSONAtomic(d.stateFile, d.state)
}

func (d *daemon) clearCredentials() error {
	d.state.AgentID = ""
	d.state.AgentToken = ""
	return d.saveState()
}

func writeJSONAtomic(path string, value any) error {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func ensurePrivateDir(path string) error {
	if path == "" || path == "." {
		return errors.New("runtime directory is not configured")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func newEventID() (string, error) {
	var value [16]byte
	if _, err := cryptorand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func validEventID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func (d *daemon) sleep(duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-d.wake:
		return true
	case <-d.stop:
		return false
	}
}

func (d *daemon) shutdown() {
	d.stopOnce.Do(func() {
		close(d.stop)
		d.listenerMu.Lock()
		if d.listener != nil {
			_ = d.listener.Close()
		}
		d.listenerMu.Unlock()
	})
}

func randomPollDelay(interval, jitter time.Duration) time.Duration {
	minimum := interval - jitter
	if minimum < 0 {
		minimum = 0
	}
	return randomDuration(minimum, interval+jitter)
}

func randomDuration(minimum, maximum time.Duration) time.Duration {
	if maximum <= minimum {
		return minimum
	}
	return minimum + time.Duration(rand.Int63n(int64(maximum-minimum)+1))
}

func jitteredRetry(base time.Duration) time.Duration {
	jitter := base / 5
	return randomDuration(base-jitter, base+jitter)
}

type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("gateway returned HTTP %d", e.status)
	}
	return fmt.Sprintf("gateway returned HTTP %d: %s", e.status, e.body)
}

func statusCode(err error) int {
	var statusError *httpStatusError
	if errors.As(err, &statusError) {
		return statusError.status
	}
	return 0
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func getOsInfo() string {
	if runtime.GOOS != "linux" {
		return runtime.GOOS
	}
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

func parseOsReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
		value = value[1 : len(value)-1]
	}
	return value
}

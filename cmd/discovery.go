package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/spf13/cobra"
)

const (
	defaultDiscoveryInterval = 24 * time.Hour
	minDiscoveryInterval     = 5 * time.Minute
	maxDiscoveryJobs         = 1_000
	maxDiscoveryBody         = 1024 * 1024
	maxCronFileSize          = 1024 * 1024
	maxCronLineSize          = 8 * 1024
	maxCronCommand           = 4 * 1024
)

var (
	discoverKey        string
	discoverServer     string
	discoverConfigFile string
	userCronPattern    = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+)$`)
	systemCronPattern  = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+)$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Scan host cron entries and submit them for import review",
	RunE:  runDiscover,
}

func init() {
	rootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().StringVarP(&discoverKey, "key", "k", "", "Workspace API key")
	discoverCmd.Flags().StringVarP(&discoverServer, "server", "s", defaultServerURL, "HTTPS gateway base URL")
	discoverCmd.Flags().StringVarP(&discoverConfigFile, "config", "c", "/etc/croncommander/config.yaml", "Path to config file")
}

type discoveryScanner struct {
	executionMode string
	systemCrontab string
	cronDir       string
	listCrontab   func() ([]byte, error)
	currentUser   func() string
}

type pendingDiscovery struct {
	EventID string                          `json:"eventId"`
	Payload protocol.DiscoveryReportPayload `json:"payload"`
}

func defaultDiscoveryScanner() discoveryScanner {
	return discoveryScanner{
		systemCrontab: "/etc/crontab",
		cronDir:       "/etc/cron.d",
		listCrontab: func() ([]byte, error) {
			output, err := exec.Command("crontab", "-l").CombinedOutput()
			if err != nil {
				if strings.Contains(strings.ToLower(string(output)), "no crontab") {
					return nil, nil
				}
				return nil, fmt.Errorf("list user crontab: %w: %s", err, strings.TrimSpace(string(output)))
			}
			return output, nil
		},
		currentUser: func() string {
			if current, err := user.Current(); err == nil && current.Username != "" {
				return current.Username
			}
			return fmt.Sprintf("uid-%d", os.Geteuid())
		},
	}
}

func runDiscover(cmd *cobra.Command, args []string) error {
	config, configPath := loadConfigWithPrimary(discoverConfigFile)
	apiKey := discoverKey
	serverURL := discoverServer
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
	normalizedURL, err := normalizeServerURL(serverURL, allowInsecureHTTP)
	if err != nil {
		return fmt.Errorf("invalid gateway URL: %w", err)
	}
	d := &daemon{
		apiKey:        apiKey,
		serverURL:     normalizedURL,
		hostname:      getHostname(),
		osType:        getOsInfo(),
		executionMode: executionMode,
		isRoot:        os.Geteuid() == 0,
		stateFile:     stateFile,
		spoolDir:      spoolDir,
		httpClient:    newHTTPClient(),
	}
	if err := d.loadState(); err != nil {
		return err
	}
	if d.state.AgentID == "" || d.state.AgentToken == "" {
		if apiKey == "" {
			return errors.New("agent is not registered and no workspace API key is configured")
		}
		if err := d.register(); err != nil {
			return err
		}
	}
	scanner := defaultDiscoveryScanner()
	scanner.executionMode = executionMode
	if err := d.discoverAndReport(scanner); err != nil {
		return err
	}
	fmt.Printf("Submitted cron discovery for agent %s\n", d.state.AgentID)
	return nil
}

func (d *daemon) discoveryDue(now time.Time) bool {
	if d.state.PendingDiscovery != nil {
		return true
	}
	if d.state.LastDiscoveryAt == "" {
		return true
	}
	interval := d.discoveryEvery
	if interval <= 0 {
		interval = defaultDiscoveryInterval
	}
	last, err := time.Parse(time.RFC3339, d.state.LastDiscoveryAt)
	return err != nil || now.Sub(last) >= interval
}

func parseDiscoveryInterval(value string) (time.Duration, error) {
	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if interval < minDiscoveryInterval {
		return 0, fmt.Errorf("must be at least %s", minDiscoveryInterval)
	}
	return interval, nil
}

func (d *daemon) discoverAndReport(scanner discoveryScanner) error {
	pending := d.state.PendingDiscovery
	if pending == nil {
		scanner.executionMode = d.executionMode
		jobs, scannedAt, err := scanner.scan()
		if err != nil {
			return err
		}
		eventID, err := newEventID()
		if err != nil {
			return err
		}
		payload := protocol.DiscoveryReportPayload{
			ScannedAt: scannedAt.UTC().Format(time.RFC3339),
			Jobs:      jobs,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode discovery report: %w", err)
		}
		if len(encoded) > maxDiscoveryBody {
			return fmt.Errorf("discovery report exceeds %d bytes", maxDiscoveryBody)
		}
		pending = &pendingDiscovery{
			EventID: eventID,
			Payload: payload,
		}
		d.state.PendingDiscovery = pending
		if err := d.saveState(); err != nil {
			d.state.PendingDiscovery = nil
			return fmt.Errorf("persist pending discovery report: %w", err)
		}
	} else if !validEventID(pending.EventID) {
		return errors.New("pending discovery event ID is invalid")
	}

	path := fmt.Sprintf("/api/v2/agents/%s/discovery-reports", url.PathEscape(d.state.AgentID))
	headers := d.authorizationHeaders()
	headers["Idempotency-Key"] = pending.EventID
	var response protocol.DiscoveryReportResponse
	if err := d.postJSON(path, headers, pending.Payload, &response); err != nil {
		return fmt.Errorf("submit discovery report: %w", err)
	}
	if response.DiscoveryReportID == "" {
		return errors.New("discovery response omitted report ID")
	}
	previousLastDiscoveryAt := d.state.LastDiscoveryAt
	d.state.LastDiscoveryAt = pending.Payload.ScannedAt
	d.state.PendingDiscovery = nil
	if err := d.saveState(); err != nil {
		d.state.LastDiscoveryAt = previousLastDiscoveryAt
		d.state.PendingDiscovery = pending
		return fmt.Errorf("persist discovery timestamp: %w", err)
	}
	log.Printf(
		"Cron discovery submitted: report=%s jobs=%d",
		response.DiscoveryReportID,
		len(pending.Payload.Jobs),
	)
	return nil
}

func (scanner discoveryScanner) scan() ([]protocol.DiscoveredJob, time.Time, error) {
	var jobs []protocol.DiscoveredJob
	currentUser := scanner.currentUser()
	if scanner.listCrontab != nil {
		content, err := scanner.listCrontab()
		if err != nil {
			return nil, time.Time{}, err
		}
		parsed, err := parseCronContent(
			content, "crontab:"+currentUser, currentUser, false)
		if err != nil {
			return nil, time.Time{}, err
		}
		jobs = append(jobs, parsed...)
	}
	if scanner.executionMode == "system" {
		if content, err := readBoundedRegularFile(scanner.systemCrontab); err == nil {
			parsed, parseErr := parseCronContent(content, scanner.systemCrontab, "", true)
			if parseErr != nil {
				return nil, time.Time{}, parseErr
			}
			jobs = append(jobs, parsed...)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, time.Time{}, err
		}
		entries, err := os.ReadDir(scanner.cronDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, time.Time{}, fmt.Errorf("read %s: %w", scanner.cronDir, err)
		}
		for _, entry := range entries {
			if len(jobs) >= maxDiscoveryJobs {
				break
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Name() == "croncommander" {
				continue
			}
			path := filepath.Join(scanner.cronDir, entry.Name())
			content, err := readBoundedRegularFile(path)
			if err != nil {
				return nil, time.Time{}, err
			}
			parsed, err := parseCronContent(content, path, "", true)
			if err != nil {
				return nil, time.Time{}, err
			}
			jobs = append(jobs, parsed...)
		}
	}
	if len(jobs) > maxDiscoveryJobs {
		jobs = jobs[:maxDiscoveryJobs]
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].SourceFile != jobs[j].SourceFile {
			return jobs[i].SourceFile < jobs[j].SourceFile
		}
		return jobs[i].LineNumber < jobs[j].LineNumber
	})
	return jobs, time.Now(), nil
}

func readBoundedRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxCronFileSize {
		return nil, fmt.Errorf("refusing unsafe cron file %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxCronFileSize+1))
}

func parseCronContent(
	content []byte,
	sourceFile string,
	defaultUser string,
	systemFormat bool,
) ([]protocol.DiscoveredJob, error) {
	var jobs []protocol.DiscoveredJob
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 1024), maxCronLineSize)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || environmentPattern.MatchString(line) {
			continue
		}
		pattern := userCronPattern
		if systemFormat {
			pattern = systemCronPattern
		}
		match := pattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		commandIndex := 6
		runAsUser := defaultUser
		if systemFormat {
			runAsUser = match[6]
			commandIndex = 7
		}
		command := strings.TrimSpace(match[commandIndex])
		if command == "" || isManagedCronCommand(command) {
			continue
		}
		if len(command) > maxCronCommand {
			return nil, fmt.Errorf("%s:%d command exceeds %d bytes", sourceFile, lineNumber, maxCronCommand)
		}
		jobs = append(jobs, protocol.DiscoveredJob{
			CronExpression: strings.Join(match[1:6], " "),
			Command:        command,
			RunAsUser:      runAsUser,
			SourceFile:     sourceFile,
			LineNumber:     lineNumber,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", sourceFile, err)
	}
	return jobs, nil
}

func isManagedCronCommand(command string) bool {
	return strings.Contains(command, "cc-agent exec") ||
		strings.Contains(command, "/cc-agent") && strings.Contains(command, " exec ")
}

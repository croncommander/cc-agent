package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseUserCronContent(t *testing.T) {
	content := []byte(`
# application tasks
SHELL=/bin/bash
*/5 * * * * /usr/local/bin/wp cron event run --due-now
0 0 * * * /usr/local/sbin/wordpress-db-backup
1 2 3 4 5 /usr/local/bin/cc-agent exec --job-id managed -- true
`)
	jobs, err := parseCronContent(content, "crontab:www-data", "www-data", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	if jobs[0].CronExpression != "*/5 * * * *" || jobs[0].RunAsUser != "www-data" {
		t.Fatalf("unexpected first job: %#v", jobs[0])
	}
	if jobs[1].Command != "/usr/local/sbin/wordpress-db-backup" {
		t.Fatalf("unexpected command: %q", jobs[1].Command)
	}
}

func TestParseCronContentRejectsOversizedLines(t *testing.T) {
	content := []byte("* * * * * " + strings.Repeat("x", maxCronLineSize))
	if _, err := parseCronContent(content, "crontab:root", "root", false); err == nil {
		t.Fatal("expected oversized cron line to be rejected")
	}
}

func TestSystemDiscoveryScansCronFilesAndExcludesManagedFile(t *testing.T) {
	root := t.TempDir()
	cronDir := filepath.Join(root, "cron.d")
	if err := os.Mkdir(cronDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "crontab"), []byte(
		"17 * * * * root /usr/local/sbin/hourly-health\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cronDir, "erp"), []byte(
		"0 2 * * * erp /opt/erp/bin/inventory-reconcile\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cronDir, "croncommander"), []byte(
		"* * * * * root /usr/local/bin/cc-agent exec -- true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	scanner := discoveryScanner{
		executionMode: "system",
		systemCrontab: filepath.Join(root, "crontab"),
		cronDir:       cronDir,
		listCrontab:   func() ([]byte, error) { return nil, nil },
		currentUser:   func() string { return "root" },
	}
	jobs, _, err := scanner.scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2: %#v", len(jobs), jobs)
	}
	var erpJobFound bool
	for _, job := range jobs {
		if job.Command == "/opt/erp/bin/inventory-reconcile" {
			erpJobFound = job.RunAsUser == "erp" &&
				job.SourceFile == filepath.Join(cronDir, "erp")
		}
	}
	if !erpJobFound {
		t.Fatalf("ERP job not discovered correctly: %#v", jobs)
	}
}

func TestDiscoveryDue(t *testing.T) {
	d := &daemon{}
	now := mustParseTime(t, "2026-06-11T10:00:00Z")
	if !d.discoveryDue(now) {
		t.Fatal("new agent should be due for discovery")
	}
	d.state.LastDiscoveryAt = "2026-06-11T09:00:00Z"
	if d.discoveryDue(now) {
		t.Fatal("recent discovery should not be due")
	}
	d.state.PendingDiscovery = &pendingDiscovery{EventID: "11111111-1111-4111-8111-111111111111"}
	if !d.discoveryDue(now) {
		t.Fatal("pending discovery should be retried immediately")
	}
	d.state.PendingDiscovery = nil
	d.discoveryEvery = 30 * time.Minute
	d.state.LastDiscoveryAt = "2026-06-11T09:20:00Z"
	if !d.discoveryDue(now) {
		t.Fatal("configured discovery interval should be honored")
	}
	d.state.LastDiscoveryAt = "2026-06-10T09:00:00Z"
	if !d.discoveryDue(now) {
		t.Fatal("daily discovery should be due")
	}
}

func TestParseDiscoveryInterval(t *testing.T) {
	interval, err := parseDiscoveryInterval("12h")
	if err != nil || interval != 12*time.Hour {
		t.Fatalf("interval = %v, err = %v", interval, err)
	}
	if _, err := parseDiscoveryInterval("1m"); err == nil {
		t.Fatal("expected too-frequent discovery interval to be rejected")
	}
}

func TestDiscoveryRetryReusesDurableEvent(t *testing.T) {
	var eventIDs []string
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		eventIDs = append(eventIDs, request.Header.Get("Idempotency-Key"))
		if attempts == 1 {
			http.Error(writer, "try later", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"discoveryReportId": "44444444-4444-4444-8444-444444444444",
			"duplicate":         true,
			"discoveredCount":   1,
		})
	}))
	defer server.Close()

	scans := 0
	d := &daemon{
		serverURL:  server.URL,
		stateFile:  filepath.Join(t.TempDir(), "state.json"),
		httpClient: server.Client(),
		state: agentState{
			AgentID:    "11111111-1111-4111-8111-111111111111",
			AgentToken: "agent-token",
		},
	}
	scanner := discoveryScanner{
		listCrontab: func() ([]byte, error) {
			scans++
			return []byte("0 0 * * * /usr/local/sbin/backup\n"), nil
		},
		currentUser: func() string { return "root" },
	}

	if err := d.discoverAndReport(scanner); err == nil {
		t.Fatal("expected first discovery attempt to fail")
	}
	if d.state.PendingDiscovery == nil {
		t.Fatal("failed discovery was not retained")
	}
	if err := d.discoverAndReport(scanner); err != nil {
		t.Fatalf("discovery retry failed: %v", err)
	}
	if scans != 1 {
		t.Fatalf("scanner ran %d times, want 1", scans)
	}
	if len(eventIDs) != 2 || eventIDs[0] == "" || eventIDs[0] != eventIDs[1] {
		t.Fatalf("discovery event IDs were not stable: %v", eventIDs)
	}
	if d.state.PendingDiscovery != nil {
		t.Fatal("acknowledged discovery remained pending")
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

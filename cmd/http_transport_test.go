package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
)

func TestNormalizeServerURLRequiresExplicitInsecureDevelopmentMode(t *testing.T) {
	if _, err := normalizeServerURL("http://listener:8081", false); err == nil {
		t.Fatal("Expected plain HTTP to be rejected by default")
	}
	got, err := normalizeServerURL("http://listener:8081/", true)
	if err != nil {
		t.Fatalf("Expected explicitly allowed local HTTP URL: %v", err)
	}
	if got != "http://listener:8081" {
		t.Fatalf("Normalized URL = %q", got)
	}
	if _, err := normalizeServerURL("https://gateway.example/agent", false); err == nil {
		t.Fatal("Expected legacy API path to be rejected")
	}
}

func TestHTTPClientDoesNotFollowCredentialRedirects(t *testing.T) {
	redirectTargetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirectTargetCalled = true
		writer.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	d := &daemon{
		serverURL:  redirect.URL,
		httpClient: newHTTPClient(),
	}
	err := d.postJSON("/register", map[string]string{
		"X-CC-API-Key": "must-not-be-forwarded",
	}, map[string]string{}, nil)
	if statusCode(err) != http.StatusTemporaryRedirect {
		t.Fatalf("Redirect status = %d, error = %v", statusCode(err), err)
	}
	if redirectTargetCalled {
		t.Fatal("HTTP client followed a redirect with credentials")
	}
}

func TestHTTPRegistrationPollingAndDurableReportDelivery(t *testing.T) {
	var mutex sync.Mutex
	var reportAttempts int
	var receivedEventID string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v2/agents/register":
			if request.Header.Get("X-CC-API-Key") != "workspace-key" {
				http.Error(writer, "missing workspace key", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(writer).Encode(protocol.RegisterResponse{
				AgentID:             "11111111-1111-4111-8111-111111111111",
				AgentToken:          "agent-token",
				PollIntervalSeconds: 60,
				PollJitterSeconds:   30,
			})
		case "/api/v2/agents/11111111-1111-4111-8111-111111111111/poll":
			if request.Header.Get("Authorization") != "Bearer agent-token" {
				http.Error(writer, "invalid token", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(writer).Encode(protocol.PollResponse{
				ManifestVersion: "manifest-1",
				Changed:         false,
				Jobs:            []protocol.JobDefinition{},
			})
		case "/api/v2/agents/11111111-1111-4111-8111-111111111111/execution-reports":
			mutex.Lock()
			reportAttempts++
			receivedEventID = request.Header.Get("Idempotency-Key")
			mutex.Unlock()
			if request.Header.Get("Authorization") != "Bearer agent-token" {
				http.Error(writer, "invalid token", http.StatusUnauthorized)
				return
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(protocol.ReportResponse{
				ExecutionID: "22222222-2222-4222-8222-222222222222",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	runtimeDir := t.TempDir()
	d := &daemon{
		apiKey:        "workspace-key",
		serverURL:     server.URL,
		hostname:      "test-host",
		osType:        "Linux",
		executionMode: "user",
		stateFile:     filepath.Join(runtimeDir, "agent-state.json"),
		spoolDir:      filepath.Join(runtimeDir, "spool"),
		httpClient:    server.Client(),
		pollInterval:  defaultPoll,
		pollJitter:    defaultPollJitter,
		wake:          make(chan struct{}, 1),
		stop:          make(chan struct{}),
	}

	if err := d.register(); err != nil {
		t.Fatalf("Registration failed: %v", err)
	}
	if d.state.AgentToken != "agent-token" {
		t.Fatalf("Agent token = %q", d.state.AgentToken)
	}
	stateInfo, err := os.Stat(d.stateFile)
	if err != nil {
		t.Fatalf("State file was not created: %v", err)
	}
	if stateInfo.Mode().Perm() != 0600 {
		t.Fatalf("State file permissions = %o", stateInfo.Mode().Perm())
	}

	eventID, err := d.spoolReport(protocol.ExecutionReportPayload{
		JobID:      "33333333-3333-4333-8333-333333333333",
		Command:    "true",
		ExitCode:   0,
		Stdout:     "",
		Stderr:     "",
		StartTime:  time.Now().UTC().Format(time.RFC3339),
		DurationMs: 1,
	})
	if err != nil {
		t.Fatalf("Failed to spool report: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.spoolDir, eventID+".json")); err != nil {
		t.Fatalf("Spool file was not created: %v", err)
	}

	if err := d.flushSpool(); err != nil {
		t.Fatalf("Failed to flush report: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.spoolDir, eventID+".json")); !os.IsNotExist(err) {
		t.Fatalf("Acknowledged spool file still exists: %v", err)
	}
	if err := d.poll(); err != nil {
		t.Fatalf("Poll failed: %v", err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if reportAttempts != 1 {
		t.Fatalf("Report attempts = %d", reportAttempts)
	}
	if receivedEventID != eventID {
		t.Fatalf("Idempotency-Key = %q, want %q", receivedEventID, eventID)
	}
}

func TestTransientReportFailureKeepsSpoolFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "try later", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	d := &daemon{
		serverURL: server.URL,
		state: agentState{
			AgentID:    "11111111-1111-4111-8111-111111111111",
			AgentToken: "agent-token",
		},
		spoolDir:   t.TempDir(),
		httpClient: server.Client(),
	}
	eventID, err := d.spoolReport(protocol.ExecutionReportPayload{
		Command:   "false",
		StartTime: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Failed to spool report: %v", err)
	}

	if err := d.flushSpool(); err == nil {
		t.Fatal("Expected transient gateway failure")
	}
	if _, err := os.Stat(filepath.Join(d.spoolDir, eventID+".json")); err != nil {
		t.Fatalf("Transient failure removed spool file: %v", err)
	}
}

func TestExecFallbackPreservesStableEventID(t *testing.T) {
	spoolDir := t.TempDir()
	record := protocol.LocalExecutionReport{
		EventID: "11111111-1111-4111-8111-111111111111",
		Payload: protocol.ExecutionReportPayload{
			Command:   "true",
			StartTime: time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := writeSpoolRecord(spoolDir, record); err != nil {
		t.Fatalf("Failed to write fallback spool record: %v", err)
	}
	if err := writeSpoolRecord(spoolDir, record); err != nil {
		t.Fatalf("Failed to safely rewrite the same event: %v", err)
	}

	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		t.Fatalf("Failed to read spool: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != record.EventID+".json" {
		t.Fatalf("Stable event ID did not map to one spool file: %v", entries)
	}
}

func TestEventIDAndPollJitterBounds(t *testing.T) {
	eventID, err := newEventID()
	if err != nil {
		t.Fatalf("Failed to generate event ID: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(eventID) {
		t.Fatalf("Invalid UUID v4 event ID: %s", eventID)
	}

	for range 100 {
		delay := randomPollDelay(60*time.Second, 30*time.Second)
		if delay < 30*time.Second || delay > 90*time.Second {
			t.Fatalf("Poll delay outside 30-90 seconds: %v", delay)
		}
	}
}

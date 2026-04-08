package cmd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/gorilla/websocket"
)

// TestSocketConcurrencyLimit verifies that the daemon limits the number of
// concurrent socket connections to prevent resource exhaustion (DoS).
func TestSocketConcurrencyLimit(t *testing.T) {
	// Channel to receive job IDs sent to the server
	receivedJobs := make(chan string, 10)

	// Setup WebSocket server (daemon target)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Try to parse as ExecutionReportMessage
			var m protocol.ExecutionReportMessage
			if err := json.Unmarshal(msg, &m); err == nil && m.Type == "execution_report" {
				receivedJobs <- m.Payload.JobID
			}
		}
	}))
	defer wsServer.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	// Setup daemon
	d := &daemon{
		apiKey:        "test-key",
		serverURL:     wsURL,
		hostname:      "test-host",
		osType:        "linux",
		executionMode: "user",
	}

	// Connect daemon to WS
	if err := d.connect(); err != nil {
		t.Fatalf("Failed to connect daemon: %v", err)
	}
	// Note: We don't need a full shutdown here because we'll just close the listener
	// and the test ends. But we can call it.
	defer func() {
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	// Override socket path to a temp file
	tempDir, err := os.MkdirTemp("", "cc-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testSocketPath := filepath.Join(tempDir, "test.sock")
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	// Override concurrency limit to 1 for this test
	originalLimit := socketConcurrencyLimit
	socketConcurrencyLimit = 1
	defer func() { socketConcurrencyLimit = originalLimit }()

	// Start listener in background
	go d.startSocketListener()

	// Wait for socket to appear
	deadline := time.Now().Add(2 * time.Second)
	socketReady := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(testSocketPath); err == nil {
			socketReady = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !socketReady {
		t.Fatal("Timeout waiting for socket file")
	}

	// Helper to connect and send report
	// If 'hold' is true, it sends partial data and keeps connection open
	connectAndSend := func(jobID string, hold bool) (net.Conn, error) {
		conn, err := net.Dial("unix", testSocketPath)
		if err != nil {
			return nil, err
		}

		payload := protocol.ExecutionReportPayload{
			JobID: jobID,
			ExitCode: 0,
		}
		data, _ := json.Marshal(payload)

		if hold {
			// Send partial data (missing last byte)
			conn.Write(data[:len(data)-1])
		} else {
			// Send full data
			conn.Write(data)
			// Close write side to ensure daemon sees EOF if it reads everything?
			// Actually daemon uses LimitReader + Decoder.
			// Decoder needs valid JSON. If we send valid JSON, it should return.
		}
		return conn, nil
	}

	// 1. Connect Client A (Hold)
	// This should consume the only available slot (limit=1)
	connA, err := connectAndSend("job-A", true)
	if err != nil {
		t.Fatalf("Failed to connect client A: %v", err)
	}
	defer connA.Close()

	// Give the daemon time to accept Client A and block on reading
	time.Sleep(100 * time.Millisecond)

	// 2. Connect Client B (Fast)
	// This connection should be accepted by OS (backlog), but the daemon
	// should block on the semaphore before processing it.
	connB, err := connectAndSend("job-B", false)
	if err != nil {
		t.Fatalf("Failed to connect client B: %v", err)
	}
	defer connB.Close()

	// 3. Verify that Job B is NOT processed immediately
	select {
	case id := <-receivedJobs:
		t.Fatalf("Concurrency limit failed: Received job %s while slot was occupied", id)
	case <-time.After(500 * time.Millisecond):
		// This is expected: No job received yet
	}

	// 4. Close Client A
	// This should cause handleSocketConnection to fail/return for A, releasing the semaphore.
	connA.Close()

	// 5. Verify that Job B IS processed now
	select {
	case id := <-receivedJobs:
		if id != "job-B" {
			t.Errorf("Expected job-B, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for job-B after slot released")
	}
}

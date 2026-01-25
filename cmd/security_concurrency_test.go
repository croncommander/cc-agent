package cmd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/gorilla/websocket"
)

// TestConcurrentConnectionLimit verifies that the daemon limits the number of
// active concurrent socket connections to prevent resource exhaustion.
func TestConcurrentConnectionLimit(t *testing.T) {
	// Setup temporary socket path
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	// Override global variables
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	originalTimeout := socketReadTimeout
	socketReadTimeout = 2 * time.Second
	defer func() { socketReadTimeout = originalTimeout }()

	originalMax := maxConcurrentConnections
	maxConcurrentConnections = 1
	defer func() { maxConcurrentConnections = originalMax }()

	// Mock WebSocket Server
	upgrader := websocket.Upgrader{}
	receivedMessages := make(chan string, 10)

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				break
			}
			var m map[string]interface{}
			if err := json.Unmarshal(msg, &m); err == nil {
				if t, ok := m["type"].(string); ok && t == "execution_report" {
					if payload, ok := m["payload"].(map[string]interface{}); ok {
						if jobID, ok := payload["jobId"].(string); ok {
							receivedMessages <- jobID
						}
					}
				}
			}
		}
	}))
	defer wsServer.Close()

	// Start Daemon
	d := &daemon{
		apiKey:    "test-key",
		serverURL: "ws://" + wsServer.Listener.Addr().String(),
	}
	// We need to connect first so sendMessage doesn't fail
	if err := d.connect(); err != nil {
		t.Fatalf("Failed to connect to mock WS: %v", err)
	}

	// Start Listener in goroutine
	go d.startSocketListener()

	// Wait for socket
	waitForSocket(t, testSocketPath)

	// Client 1: Connect and hold
	conn1, err := net.Dial("unix", testSocketPath)
	if err != nil {
		t.Fatalf("Client 1 failed to connect: %v", err)
	}
	defer conn1.Close()
	// Client 1 sends nothing, keeping the handler blocked on Read()

	// Client 2: Connect and send data
	conn2, err := net.Dial("unix", testSocketPath)
	if err != nil {
		t.Fatalf("Client 2 failed to connect: %v", err)
	}
	defer conn2.Close()

	report2 := protocol.ExecutionReportPayload{
		JobID:    "job2",
		ExitCode: 0,
	}
	json.NewEncoder(conn2).Encode(report2)

	// Verification:
	// Since Limit=1, Client 1 is occupying the slot.
	// Client 2 is connected (backlog) but its handler hasn't started.
	// So WS should NOT receive "job2".

	select {
	case msg := <-receivedMessages:
		t.Fatalf("Received message '%s' unexpectedly early! Concurrency limit failed.", msg)
	case <-time.After(500 * time.Millisecond):
		// Good, no message yet.
	}

	// Now release Client 1
	conn1.Close() // This causes Read() to fail/return in Handler 1.

	// Handler 1 finishes, releasing semaphore.
	// Handler 2 starts, reads Client 2 data, sends to WS.

	select {
	case msg := <-receivedMessages:
		if msg != "job2" {
			t.Errorf("Expected 'job2', got '%s'", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for job2 after releasing client 1")
	}
}

func waitForSocket(t *testing.T, path string) {
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Timed out waiting for socket file")
}

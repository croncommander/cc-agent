package cmd

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestSocketConcurrencyLimit verifies that the daemon limits the number of concurrent
// socket handlers to prevent resource exhaustion (DoS).
func TestSocketConcurrencyLimit(t *testing.T) {
	// Setup secure temp directory for socket
	tmpDir, err := os.MkdirTemp("", "cc-agent-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override socket path
	originalSocketPath := socketPath
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	// Override limit for testing
	originalLimit := maxConcurrentSocketConnections
	maxConcurrentSocketConnections = 5 // Low limit for testing
	defer func() { maxConcurrentSocketConnections = originalLimit }()

	// Setup daemon
	d := &daemon{
		// Mock dependencies if needed
	}

	// Start listener in a goroutine
	go d.startSocketListener()

	// Wait for socket to be created
	ready := false
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(testSocketPath); err == nil {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Socket was not created in time")
	}

	// Measure baseline goroutines
	time.Sleep(100 * time.Millisecond) // Let listener settle
	baselineGoroutines := runtime.NumGoroutine()

	// Connect N clients (where N > limit)
	const numClients = 20
	var conns []net.Conn
	for i := 0; i < numClients; i++ {
		c, err := net.Dial("unix", testSocketPath)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns = append(conns, c)
		// We do NOT write data, so handleSocketConnection will block on LimitReader/Decode
		// until socketReadTimeout (default 5s, we didn't change it).
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// Wait for handlers to spawn (or block)
	time.Sleep(200 * time.Millisecond)

	currentGoroutines := runtime.NumGoroutine()
	delta := currentGoroutines - baselineGoroutines

	t.Logf("Baseline goroutines: %d", baselineGoroutines)
	t.Logf("Current goroutines: %d", currentGoroutines)
	t.Logf("Delta: %d", delta)

	// Without fix: Delta should be around 20 (plus overhead)
	// With fix: Delta should be around 5 (limit) + 1 (blocked main loop) = 6
	// We allow some slack.

	// If the delta is close to numClients (20), the protection is missing.
	// We expect this to fail initially.
	if delta > maxConcurrentSocketConnections+5 {
		t.Fatalf("Goroutine leak detected! Delta %d is much higher than limit %d. Concurrency limit is NOT working.", delta, maxConcurrentSocketConnections)
	} else {
		t.Log("Goroutine count is within expected limits.")
	}
}

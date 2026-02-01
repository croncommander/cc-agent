package cmd

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBoundedConcurrency(t *testing.T) {
	// Setup temporary socket path
	tmpDir := t.TempDir()
	originalSocketPath := socketPath
	socketPath = filepath.Join(tmpDir, "test.sock")
	defer func() { socketPath = originalSocketPath }()

	// Override concurrency limit
	originalLimit := maxConcurrentConnections
	maxConcurrentConnections = 5
	defer func() { maxConcurrentConnections = originalLimit }()

	// Setup daemon
	d := &daemon{
		// minimal setup
		shutdown: func() {}, // prevent nil panic
	}

	// Start listener in background
	go d.startSocketListener()

	// Wait for socket to be created
	ready := false
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Socket file was not created")
	}

	// Baseline goroutines
	// We wait a bit for the listener to settle
	time.Sleep(100 * time.Millisecond)
	baseline := runtime.NumGoroutine()
	t.Logf("Baseline goroutines: %d", baseline)

	// Launch more clients than the limit
	clientCount := 20
	clients := make([]net.Conn, clientCount)

	for i := 0; i < clientCount; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			// If the backlog is full, Dial might fail. That's fine, it proves backpressure.
			// But with 20 and default backlog, it should succeed.
			t.Logf("Client %d failed to connect (backpressure?): %v", i, err)
			continue
		}
		clients[i] = conn
	}
	defer func() {
		for _, c := range clients {
			if c != nil {
				c.Close()
			}
		}
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	// Allow scheduler to settle
	time.Sleep(200 * time.Millisecond)

	current := runtime.NumGoroutine()
	t.Logf("Current goroutines: %d", current)

	// Expected: baseline + maxConcurrentConnections + safety buffer
	// If unbounded, it would be baseline + ~20.
	// With limit 5, it should be baseline + 5.
	// We'll be generous with buffer.
	expectedMax := baseline + maxConcurrentConnections + 5

	if current > expectedMax {
		t.Errorf("Goroutine count %d exceeded expected max %d (limit %d). Concurrency likely unbounded.",
			current, expectedMax, maxConcurrentConnections)
	} else {
		t.Logf("Concurrency check passed: %d <= %d", current, expectedMax)
	}
}

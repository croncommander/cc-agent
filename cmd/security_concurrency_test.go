package cmd

import (
	"net"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestSocketConcurrencyLimit verifies that the daemon's socket listener
// limits the number of concurrent connections.
func TestSocketConcurrencyLimit(t *testing.T) {
	// Create a temporary socket path
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	// Override the package-level socketPath variable
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() {
		socketPath = originalSocketPath
	}()

	d := &daemon{
		connLimit: make(chan struct{}, 50),
		// Initialize shutdown to avoid panic in defer
		shutdown: func() {},
	}

	// Start the listener in a goroutine
	// We need to give it a moment to start listening
	ready := make(chan struct{})
	go func() {
		close(ready)
		d.startSocketListener()
	}()
	<-ready
	time.Sleep(100 * time.Millisecond) // Give it time to bind

	// Number of connections to attempt
	const numConns = 100
	conns := make([]net.Conn, numConns)

	// Connect loop
	var connected int
	for i := 0; i < numConns; i++ {
		// Set a short timeout for dialing because if the backlog is full, it might block
		dialer := net.Dialer{Timeout: 100 * time.Millisecond}
		conn, err := dialer.Dial("unix", testSocketPath)
		if err != nil {
			t.Logf("Connection %d failed: %v", i, err)
			continue
		}
		conns[i] = conn
		connected++
	}
	defer func() {
		d.shutdown()
		for _, c := range conns {
			if c != nil {
				c.Close()
			}
		}
	}()

	t.Logf("Connected %d clients", connected)

	// Allow some time for goroutines to spawn
	time.Sleep(200 * time.Millisecond)

	numGoroutines := runtime.NumGoroutine()
	t.Logf("Number of goroutines: %d", numGoroutines)

	// Expect around 50 handlers + overhead (approx 5-10).
	// Should be strictly less than 100 (which would be the case if unbounded).
	if numGoroutines > 80 {
		t.Errorf("Too many goroutines: %d. Expected approx 50.", numGoroutines)
	}
	if numGoroutines < 40 {
		t.Errorf("Too few goroutines: %d. Expected approx 50. (Did the clients disconnect?)", numGoroutines)
	}
}

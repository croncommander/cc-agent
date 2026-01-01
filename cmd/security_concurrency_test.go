package cmd

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestSocketConcurrencyLimit verifies that the daemon limits the number of concurrent
// socket connections to prevent resource exhaustion (DoS).
func TestSocketConcurrencyLimit(t *testing.T) {
	// Reduce limit for testing
	originalLimit := maxConcurrentConnections
	maxConcurrentConnections = 5
	defer func() { maxConcurrentConnections = originalLimit }()

	// Create temp dir for socket
	tmpDir, err := os.MkdirTemp("", "cc-agent-limit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override socketPath
	origSocketPath := socketPath
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	socketPath = testSocketPath
	defer func() { socketPath = origSocketPath }()

	d := &daemon{
		connMu: sync.Mutex{},
	}
	// Start listener
	go d.startSocketListener()
	time.Sleep(100 * time.Millisecond)

	// We want to verify that we can't spawn significantly more goroutines than the limit.
	// Since the semaphore blocks the Accept loop, connections will queue in the OS backlog.
	// We can't easily detect "blocked in backlog" vs "blocked in handler" from the outside.
	// But we CAN verify that the number of active handlers (and thus goroutines) is capped.

	startGoroutines := runtime.NumGoroutine()
	numConns := 20
	var conns []net.Conn
	var mu sync.Mutex

	for i := 0; i < numConns; i++ {
		go func() {
			c, err := net.Dial("unix", testSocketPath)
			if err == nil {
				mu.Lock()
				conns = append(conns, c)
				mu.Unlock()
			}
		}()
	}

	// Wait enough time for all dials to succeed (filling backlog)
	time.Sleep(500 * time.Millisecond)

	// Even if 20 dials succeeded (due to backlog), the daemon should only have
	// spawned 'maxConcurrentConnections' handlers.
	// Each handler is a goroutine.
	// The Accept loop is blocked on the semaphore.

	currentGoroutines := runtime.NumGoroutine()
	delta := currentGoroutines - startGoroutines

	// Expected delta:
	// +1 for the startSocketListener goroutine (blocked on sem)
	// +maxConcurrentConnections for the handlers (blocked on Read)
	// +some overhead from test goroutines if they are still alive (they finish after Dial)

	// Wait, the test goroutines finish immediately after Dial.
	// So currentGoroutines should be primarily server goroutines.

	// The Listener loop is 1 goroutine.
	// The Handlers are 'maxConcurrentConnections' goroutines.
	// So delta should be close to maxConcurrentConnections + 1.

	t.Logf("Limit: %d. Delta Goroutines: %d", maxConcurrentConnections, delta)

	if delta > maxConcurrentConnections + 5 { // Allow small buffer for test runner overhead
		t.Errorf("Goroutine leak detected. Expected ~%d, got %d", maxConcurrentConnections, delta)
	}

	// Clean up
	mu.Lock()
	for _, c := range conns {
		c.Close()
	}
	mu.Unlock()
	if d.shutdown != nil {
		d.shutdown()
	}
}

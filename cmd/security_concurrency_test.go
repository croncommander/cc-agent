package cmd

import (
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestSocketConcurrencyLimit verifies that the daemon limits the number of concurrent
// socket connections to prevent resource exhaustion DoS.
func TestSocketConcurrencyLimit(t *testing.T) {
	// Setup temporary socket path
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	// Save original socket path and restore after test
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	// Save original timeout and restore
	originalTimeout := socketReadTimeout
	socketReadTimeout = 200 * time.Millisecond // Short timeout for test speed
	defer func() { socketReadTimeout = originalTimeout }()

	// Override the concurrency limit for testing
	originalLimit := socketConcurrencyLimit
	socketConcurrencyLimit = 5
	defer func() { socketConcurrencyLimit = originalLimit }()

	d := &daemon{
		connMu: sync.Mutex{},
	}

	// Start listener
	go d.startSocketListener()

	// Wait for listener to start
	for i := 0; i < 10; i++ {
		if _, err := net.Dial("unix", testSocketPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Measure baseline goroutines
	// We need to wait a bit for stability
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()

	// Launch N concurrent connections (N > limit)
	const attempts = 50
	var conns []net.Conn

	for i := 0; i < attempts; i++ {
		conn, err := net.Dial("unix", testSocketPath)
		if err != nil {
			// Eventually the backlog fills and Dial fails or blocks.
			// This is expected and good.
			continue
		}
		conns = append(conns, conn)
		// We hold the connection open.
	}

	// Wait for things to settle
	time.Sleep(200 * time.Millisecond)

	currentGoroutines := runtime.NumGoroutine()

	diff := currentGoroutines - baselineGoroutines

	// We expect the increase to be close to the limit (5).
	// It might be 5 + 1 (listener loop blocked) or so.
	// But it should NOT be close to 'attempts' (50) or 'conns' (likely > 20).

	if diff > socketConcurrencyLimit+5 {
		t.Errorf("Too many goroutines spawned! Expected around %d, got increase of %d", socketConcurrencyLimit, diff)
	}

	if len(conns) < socketConcurrencyLimit {
		t.Errorf("Should have been able to connect at least %d times, got %d", socketConcurrencyLimit, len(conns))
	}

	defer func() {
		for _, c := range conns {
			c.Close()
		}
		if d.shutdown != nil {
			d.shutdown()
		}
	}()
}

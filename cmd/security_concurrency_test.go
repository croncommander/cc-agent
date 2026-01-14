package cmd

import (
	"net"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSocketConcurrencyLimit(t *testing.T) {
	// Setup temporary socket path
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test_concurrency.sock")

	// Override global socketPath variable for this test
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	d := &daemon{
		apiKey: "test",
	}

	// Start listener in a goroutine
	go d.startSocketListener()

	// Wait for listener to be ready
	time.Sleep(100 * time.Millisecond)

	// Cleanup at end
	defer func() {
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	startGoroutines := runtime.NumGoroutine()
	t.Logf("Starting goroutines: %d", startGoroutines)

	// Try to open many connections
	const numConns = 100
	const maxLimit = 50
	var conns []net.Conn

	for i := 0; i < numConns; i++ {
		// Use a timeout for Dial because if the backlog fills up, Dial might block.
		// We want to fill the backlog and the app limit.
		conn, err := net.DialTimeout("unix", testSocketPath, 100*time.Millisecond)
		if err != nil {
			// It is acceptable for Dial to fail if the backlog is full.
			// We just want to ensure we don't spawn too many goroutines.
			t.Logf("Connection %d rejected/timed out (expected behavior under load): %v", i, err)
			break
		}
		conns = append(conns, conn)
		// Don't send anything, just hold the connection open.
	}

	t.Logf("Successfully opened %d connections", len(conns))

	// Give time for server to accept and launch handlers
	time.Sleep(200 * time.Millisecond)

	currentGoroutines := runtime.NumGoroutine()
	t.Logf("Current goroutines: %d", currentGoroutines)

	diff := currentGoroutines - startGoroutines
	t.Logf("Goroutine increase: %d", diff)

	// We expect the increase to be capped around maxLimit (50).
	// Allow a small margin (e.g., +5) for auxiliary goroutines or timing.
	if diff > maxLimit+10 {
		t.Errorf("Security check failed: Goroutine count increased by %d, expected cap around %d", diff, maxLimit)
	} else {
		t.Logf("Security check passed: Goroutine increase %d is within limit %d", diff, maxLimit)
	}

	// Cleanup connections
	for _, c := range conns {
		c.Close()
	}
}

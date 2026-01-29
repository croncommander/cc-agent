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

// TestUnboundedConcurrency demonstrates that the daemon accepts unlimited connections,
// causing unbounded goroutine growth.
func TestUnboundedConcurrency(t *testing.T) {
	// Setup temp socket path
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test_concurrency.sock")

	// Override global socketPath
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	// Override concurrency limit for testing
	originalMax := maxConcurrentConnections
	maxConcurrentConnections = 10
	defer func() { maxConcurrentConnections = originalMax }()

	d := &daemon{
		connMu: sync.Mutex{},
	}

	// Initialize shutdown to avoid nil panic if called early
	d.shutdown = func() {}

	// Start listener
	go d.startSocketListener()

	// Wait for socket to appear
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(testSocketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Socket file not created in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Baseline goroutine count
	baseCount := runtime.NumGoroutine()

	// Number of concurrent connections to attempt
	// A safe number that clearly shows growth but doesn't crash the test runner
	const concurrency = 100

	var conns []net.Conn
	var mu sync.Mutex

	// Launch clients
	for i := 0; i < concurrency; i++ {
		go func() {
			conn, err := net.Dial("unix", testSocketPath)
			if err != nil {
				// It's possible to hit system limits or race conditions, ignore for this test
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			// Hold connection open
			time.Sleep(1 * time.Second)
		}()
	}

	// Wait a bit for connections to be established and handled
	time.Sleep(500 * time.Millisecond)

	// Check goroutine count
	currentCount := runtime.NumGoroutine()
	totalGrowth := currentCount - baseCount

	// Subtract the goroutines we spawned for the clients
	daemonGrowth := totalGrowth - concurrency

	t.Logf("Base: %d, Current: %d, Total Growth: %d, Client Goroutines: %d, Daemon Growth: %d, Limit: %d",
		baseCount, currentCount, totalGrowth, concurrency, daemonGrowth, maxConcurrentConnections)

	// Clean up
	mu.Lock()
	for _, c := range conns {
		c.Close()
	}
	mu.Unlock()
	d.shutdown()

	// Verify that daemon growth is capped near maxConcurrentConnections
	// Expected: ~10 (limit) + small overhead. Definitely < 20.
	// If it was unbounded, daemonGrowth would be ~100.
	if daemonGrowth > maxConcurrentConnections+5 {
		t.Errorf("FAIL: Daemon goroutine growth %d exceeded limit %d significantly", daemonGrowth, maxConcurrentConnections)
	} else {
		t.Log("SUCCESS: Goroutine growth correctly limited.")
	}
}

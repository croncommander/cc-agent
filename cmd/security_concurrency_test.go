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

// TestBoundedConcurrency verifies that the daemon limits the number of concurrent
// socket connections to prevent resource exhaustion (DoS).
func TestBoundedConcurrency(t *testing.T) {
	// Setup temp socket path
	tmpDir := t.TempDir()
	testSocket := filepath.Join(tmpDir, "test.sock")
	originalSocketPath := socketPath
	socketPath = testSocket

	// Reduce concurrency limit for testing
	originalMax := maxConcurrentConnections
	maxConcurrentConnections = 5

	defer func() {
		socketPath = originalSocketPath
		maxConcurrentConnections = originalMax
	}()

	// Start listener
	d := &daemon{}
	// We need to stop the listener at end of test.

	go d.startSocketListener()

	// Wait for listener to start (poll for file)
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(testSocket); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	startGoroutines := runtime.NumGoroutine()
	// Launch more clients than the limit
	const numClients = 50
	const limit = 5

	var wg sync.WaitGroup
	wg.Add(numClients)

	// Launch clients
	for i := 0; i < numClients; i++ {
		go func() {
			defer wg.Done()
			conn, err := net.Dial("unix", testSocket)
			if err != nil {
				// With backpressure, Dial might eventually fail or timeout if backlog is full,
				// but for 50 clients it usually succeeds (OS backlog is often 128).
				return
			}
			defer conn.Close()
			// Keep connection open to occupy a slot
			time.Sleep(500 * time.Millisecond)
		}()
	}

	// Give clients time to connect and saturate the semaphore
	time.Sleep(200 * time.Millisecond)

	currentGoroutines := runtime.NumGoroutine()
	diff := currentGoroutines - startGoroutines

	t.Logf("Started with %d goroutines, now %d (diff: %d)", startGoroutines, currentGoroutines, diff)

	// We expect:
	// - numClients goroutines for the test clients (50)
	// - 5 goroutines for the active handlers (limit)
	// - 1 goroutine for the main loop (blocked on semaphore)
	// - Maybe a few runtime goroutines?

	// Unbounded behavior would be: numClients (clients) + numClients (handlers) = 100
	// Bounded behavior should be: numClients (clients) + limit (handlers) = 55

	expectedMax := numClients + limit + 10

	if diff > expectedMax {
		t.Fatalf("Concurrency limit failed! Expected <= %d active goroutines (clients + handlers), got increase of %d", expectedMax, diff)
	} else {
		t.Logf("Success: Concurrency limited. Goroutine count increased by %d (expected ~%d).", diff, numClients+limit)
	}

	// Wait for clients to finish
	wg.Wait()

	// Cleanup
	if d.shutdown != nil {
		d.shutdown()
	}
}

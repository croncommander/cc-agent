package cmd

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestUnboundedConcurrency demonstrates that without a semaphore,
// we can open a large number of concurrent connections.
// This test acts as a regression test to ensure the system remains stable under load,
// and after the fix, it ensures we don't *block* valid connections unnecessarily (up to the limit).
func TestUnboundedConcurrency(t *testing.T) {
	// Create a temp directory for the socket
	tmpDir, err := os.MkdirTemp("", "socket-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override the global socketPath for this test
	oldSocketPath := socketPath
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	socketPath = testSocketPath
	defer func() { socketPath = oldSocketPath }()

	d := &daemon{
		connMu: sync.Mutex{},
	}
	// Initial no-op shutdown
	d.shutdown = func() {}

	// Start listener in a goroutine
	go d.startSocketListener()

	// Wait for socket to be ready
	time.Sleep(200 * time.Millisecond)

	// Cleanup at end
	defer func() {
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	concurrency := 50
	activeConnections := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(concurrency)

	// Launch clients
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			// Connect
			conn, err := net.Dial("unix", testSocketPath)
			if err != nil {
				t.Errorf("Connection failed: %v", err)
				return
			}
			defer conn.Close()

			mu.Lock()
			activeConnections++
			mu.Unlock()

			// Hold connection open to consume a "slot"
			time.Sleep(500 * time.Millisecond)
		}()
	}

	wg.Wait()

	t.Logf("Opened %d concurrent connections", activeConnections)
	if activeConnections < concurrency {
		// Note: This might happen if the OS limit is low or we implemented the fix.
		// For the reproduction, we expect this to succeed (activeConnections == concurrency).
		t.Log("Note: Fewer connections succeeded than attempted.")
	}
}

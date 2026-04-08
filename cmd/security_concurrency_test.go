package cmd

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUnboundedConcurrency(t *testing.T) {
	// Setup a temporary socket path
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test_concurrency.sock")

	// Override the global socketPath
	originalSocketPath := socketPath
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	// Override maxConcurrentConnections to a small number
	originalMaxConns := maxConcurrentConnections
	maxConcurrentConnections = 5
	defer func() { maxConcurrentConnections = originalMaxConns }()

	d := &daemon{}

	// Start listener in a goroutine
	// We need to wait for it to be ready.
	ready := make(chan struct{})
	go func() {
		// startSocketListener removes the file, sets up listener, then loops.
		// We can't easily signal readiness from inside startSocketListener without modifying it.
		// So we'll poll for the file existence.
		d.startSocketListener()
	}()

	// Wait for socket to appear
	timeout := time.After(2 * time.Second)
	for {
		if _, err := os.Stat(testSocketPath); err == nil {
			close(ready)
			break
		}
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for socket to be created")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	<-ready

	// Try to open 100 connections.
	// If we wanted to prove it's unbounded, we'd go higher, but 100 is enough to show
	// we can establish them.
	// The fix will involve setting a limit (e.g., 50), so checking we can do 100 proves
	// we are currently over that limit.

	// We set limit to 5.
	// We want to verify that we can still open connections (at TCP level)
	// even if the application is processing 5.
	// The 6th connection will be accepted by Accept() but the loop will block on `sem <- struct{}{}`.
	// This means the 7th connection will sit in the OS backlog.
	// This test simply verifies that the application doesn't crash and eventually handles them if we were to release.
	// Since we don't release in this test (we hold them), we just verify we can "dial" successfully.
	// Dialing succeeds because of the OS backlog.

	const targetConnections = 20
	conns := make([]net.Conn, targetConnections)
	var connsMu sync.Mutex

	defer func() {
		connsMu.Lock()
		for _, c := range conns {
			if c != nil {
				c.Close()
			}
		}
		connsMu.Unlock()
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	var wg sync.WaitGroup
	wg.Add(targetConnections)

	// Launch clients
	for i := 0; i < targetConnections; i++ {
		go func(idx int) {
			defer wg.Done()
			conn, err := net.Dial("unix", testSocketPath)
			if err != nil {
				// It is possible that eventually Dial fails if backlog is full,
				// but 20 should fit in default backlog (usually 128).
				t.Errorf("Failed to connect %d: %v", idx, err)
				return
			}
			connsMu.Lock()
			conns[idx] = conn
			connsMu.Unlock()
		}(i)
	}

	// wait with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for connections to be established")
	}

	connsMu.Lock()
	count := 0
	for _, c := range conns {
		if c != nil {
			count++
		}
	}
	connsMu.Unlock()

	if count != targetConnections {
		t.Errorf("Expected %d connections, got %d", targetConnections, count)
	}
}

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

// TestConcurrentConnectionLimit verifies that the daemon limits the number of
// concurrent socket connections to prevent DoS.
func TestConcurrentConnectionLimit(t *testing.T) {
	// Setup temporary socket path
	tmpDir, err := os.MkdirTemp("", "cc-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldSocketPath := socketPath
	socketPath = filepath.Join(tmpDir, "cc-agent.sock")
	defer func() { socketPath = oldSocketPath }()

	// Override limit to a small number for testing
	oldLimit := maxConcurrentConnections
	maxConcurrentConnections = 2
	defer func() { maxConcurrentConnections = oldLimit }()

	// Increase read timeout so connections stay open during test
	oldTimeout := socketReadTimeout
	socketReadTimeout = 30 * time.Second
	defer func() { socketReadTimeout = oldTimeout }()

	d := &daemon{
		shutdown: func() {},
	}

	// Start listener
	go d.startSocketListener()

	// Wait for socket to be ready
	ready := false
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Socket file not created in time")
	}

	// Helper to connect and keep open
	var conns []net.Conn
	var mu sync.Mutex
	defer func() {
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
		// Trigger shutdown to clean up listener
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	connect := func() {
		conn, err := net.DialTimeout("unix", socketPath, 1*time.Second)
		if err != nil {
			t.Errorf("Failed to connect: %v", err)
			return
		}
		mu.Lock()
		conns = append(conns, conn)
		mu.Unlock()
	}

	// Get baseline goroutine count
	// Give time for listener to settle
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Connect 1 (should succeed)
	connect()
	time.Sleep(50 * time.Millisecond)
	g1 := runtime.NumGoroutine()
	t.Logf("Goroutines after 1 conn: %d", g1)
	if g1 <= initialGoroutines {
		t.Errorf("Expected goroutine count to increase after 1st connection, got %d vs %d", g1, initialGoroutines)
	}

	// Connect 2 (should succeed, hitting limit)
	connect()
	time.Sleep(50 * time.Millisecond)
	g2 := runtime.NumGoroutine()
	t.Logf("Goroutines after 2 conns: %d", g2)
	if g2 <= g1 {
		t.Errorf("Expected goroutine count to increase after 2nd connection, got %d vs %d", g2, g1)
	}

	// Connect 3 (should be blocked at semaphore, no new goroutine spawned)
	connect()
	time.Sleep(50 * time.Millisecond)
	g3 := runtime.NumGoroutine()
	t.Logf("Goroutines after 3 conns: %d", g3)

	// WITH THE BUG (Unbounded): g3 > g2
	// WITH THE FIX (Bounded): g3 == g2 (or maybe slight fluctuation, but handler not spawned)
	// We assert that it DOES NOT increase significantly (allowing for variance).
	// Ideally g3 == g2.

	if g3 > g2 {
		t.Errorf("Security Flaw: Goroutine count increased for 3rd connection (limit 2). Unbounded concurrency detected.")
	}
}

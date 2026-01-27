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
// concurrent socket connections to prevent DoS (resource exhaustion).
func TestConcurrentConnectionLimit(t *testing.T) {
	// Setup temp directory for socket
	tmpDir := t.TempDir()
	socketName := filepath.Join(tmpDir, "cc-agent-test.sock")

	// Override globals
	originalSocketPath := socketPath
	originalMaxConns := maxConcurrentConnections
	socketPath = socketName
	maxConcurrentConnections = 2 // Strict limit for this test
	defer func() {
		socketPath = originalSocketPath
		maxConcurrentConnections = originalMaxConns
	}()

	// Initialize daemon
	d := &daemon{
		// No need for real backend connection
	}

	// Start listener
	ready := make(chan struct{})
	go func() {
		close(ready)
		d.startSocketListener()
	}()
	<-ready

	// Wait for socket file to appear
	socketReady := false
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketName); err == nil {
			socketReady = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !socketReady {
		t.Fatal("Socket file not created in time")
	}

	// Ensure cleanup
	defer func() {
		if d.shutdown != nil {
			d.shutdown()
		}
	}()

	// Helper to count goroutines
	getGoroutines := func() int {
		time.Sleep(50 * time.Millisecond) // Allow scheduler to settle
		return runtime.NumGoroutine()
	}

	initialGo := getGoroutines()
	t.Logf("Initial goroutines: %d", initialGo)

	var wg sync.WaitGroup
	// Channel to signal when we are done with connections
	doneCh := make(chan struct{})
	defer close(doneCh)

	// Helper to connect and hold
	connectAndHold := func(id string) net.Conn {
		conn, err := net.Dial("unix", socketName)
		if err != nil {
			t.Logf("[%s] Dial failed: %v", id, err)
			return nil
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			// Send partial JSON to trigger handler but block on Decode
			conn.Write([]byte(`{"jobId": "` + id + `"`))

			// Wait until test is done
			<-doneCh
			conn.Close()
		}()
		return conn
	}

	// 1. Fill the slots (2 connections)
	t.Log("Connecting Client 1...")
	c1 := connectAndHold("1")
	if c1 == nil { t.Fatal("Failed to connect client 1") }

	t.Log("Connecting Client 2...")
	c2 := connectAndHold("2")
	if c2 == nil { t.Fatal("Failed to connect client 2") }

	// Allow handlers to start
	time.Sleep(100 * time.Millisecond)

	currentGo := getGoroutines()
	t.Logf("Goroutines after 2 conns: %d (Delta: %d)", currentGo, currentGo-initialGo)

	// We expect roughly 2 new goroutines (handlers)
	if currentGo < initialGo+2 {
		t.Logf("Warning: Expected goroutine count to increase by at least 2")
	}

	// 2. Try 3rd connection
	// This should be Accepted by OS but blocked by application semaphore
	t.Log("Connecting Client 3 (should be queued)...")

	// We do this in a goroutine because if the backlog is full, Dial might block?
	// But backlog is usually > 0.
	conn3, err := net.Dial("unix", socketName)
	if err != nil {
		t.Fatalf("Failed to dial 3rd connection: %v", err)
	}

	// Write data to ensure it would be processed if accepted
	conn3.Write([]byte(`{"jobId": "3"`))

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	go3 := getGoroutines()
	t.Logf("Goroutines after 3 conns: %d (Delta from 2 conns: %d)", go3, go3-currentGo)

	// If properly limited, go3 should be equal to currentGo (or very close, NO new handler goroutine).
	// If unbounded, go3 should be roughly currentGo + 1.

	if go3 > currentGo {
		// This assertion checks if the limit is working.
		// If go3 > currentGo, it means a new handler was spawned -> Unbounded.
		// We expect this to be false AFTER the fix.
		t.Errorf("Concurrency limit exceeded! New goroutine spawned for 3rd connection.")
	} else {
		t.Log("Success: No new goroutine spawned for 3rd connection.")
	}

	// 3. Close Client 1 to free a slot
	t.Log("Closing Client 1...")
	// We need to signal the specific client to close.
	// But our helper waits on doneCh.
	// We'll just close the connection directly from here?
	// The helper writes to it? No, helper reads? Helper waits.
	// Let's just close c1.
	c1.Close() // This breaks the Read/Write in handler?
	// The handler reads. Closing c1 from client side causes EOF in handler.
	// Handler exits. Release semaphore.

	// Wait for handler to exit and new one to start
	time.Sleep(200 * time.Millisecond)

	goFinal := getGoroutines()
	t.Logf("Goroutines after closing 1: %d", goFinal)

	// We expect goFinal to be roughly equal to currentGo
	// (one died, one picked up).

	// Clean up 3rd conn
	conn3.Close()
}

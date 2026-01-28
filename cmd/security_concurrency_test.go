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

func TestConcurrencyLimit(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "cc-agent-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override socket path
	originalSocketPath := socketPath
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	socketPath = testSocketPath

	// Override concurrency limit
	originalMaxConnections := maxConcurrentConnections
	maxConcurrentConnections = 10

	defer func() {
		socketPath = originalSocketPath
		maxConcurrentConnections = originalMaxConnections
	}()

	d := &daemon{
		apiKey: "test-key",
	}

	// Start listener in background
	ready := make(chan struct{})
	go func() {
		close(ready)
		d.startSocketListener()
	}()

	<-ready
	// Wait for socket creation
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(testSocketPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	initialGoroutines := runtime.NumGoroutine()
	connectionCount := 50 // Try to open 5x the limit

	var wg sync.WaitGroup
	// Open many connections and keep them open
	for i := 0; i < connectionCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("unix", testSocketPath)
			if err != nil {
				// Dial might fail if backlog is full, which is fine.
				return
			}
			defer conn.Close()
			// Hold connection open
			time.Sleep(2 * time.Second)
		}()
	}

	// Wait a bit for connections to be established and handled
	time.Sleep(1 * time.Second)

	currentGoroutines := runtime.NumGoroutine()
	delta := currentGoroutines - initialGoroutines

	t.Logf("Initial Goroutines: %d", initialGoroutines)
	t.Logf("Current Goroutines: %d", currentGoroutines)
	t.Logf("Delta: %d", delta)
	t.Logf("Limit: %d", maxConcurrentConnections)

	// Analysis:
	// We have 'connectionCount' client goroutines (50)
	// We expect 'maxConcurrentConnections' server handler goroutines (10)
	// Total expected increase ≈ 60
	//
	// If unbounded, we would expect 50 server handlers -> Total ≈ 100

	// We allow a small margin for test runtime overhead
	expectedMaxDelta := connectionCount + maxConcurrentConnections + 15

	if delta > expectedMaxDelta {
		t.Errorf("Goroutine leak detected! Expected max delta ~%d, got %d. It seems concurrency limit is not working.", expectedMaxDelta, delta)
	} else {
		t.Logf("PASS: Goroutine delta within expected limits (%d <= %d)", delta, expectedMaxDelta)
	}
}

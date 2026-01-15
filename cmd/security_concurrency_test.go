package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSocketConcurrencyLimit verifies that the daemon actively rejects connections
// when the concurrency limit is reached.
func TestSocketConcurrencyLimit(t *testing.T) {
	// 1. Setup
	// Override limit to a small number
	originalLimit := socketConcurrencyLimit
	socketConcurrencyLimit = 2
	defer func() { socketConcurrencyLimit = originalLimit }()

	// Use a temp directory for the socket
	tmpDir, err := os.MkdirTemp("", "cc-test-socket")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override socketPath
	originalSocketPath := socketPath
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	socketPath = testSocketPath
	defer func() { socketPath = originalSocketPath }()

	// Mock daemon
	d := &daemon{
		shutdown: func() {}, // No-op shutdown
	}

	// 2. Start Listener
	// We need to run startSocketListener in a goroutine.
	// However, startSocketListener blocks forever.
	// We need a way to stop it. Ideally startSocketListener should check context or d.shutdown.
	// But the current implementation just loops on listener.Accept().
	// When we close the listener, Accept returns error.

	// Create a done channel to signal when the listener exits
	listenerDone := make(chan struct{})

	go func() {
		defer close(listenerDone)
		d.startSocketListener()
	}()

	// Wait for socket to appear
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(testSocketPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 3. Connect Clients

	// Helper to connect and hold
	connectAndHold := func(id int) (net.Conn, error) {
		conn, err := net.Dial("unix", testSocketPath)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}

	// Connect Client 1 (Should succeed)
	conn1, err := connectAndHold(1)
	if err != nil {
		t.Fatalf("Client 1 failed to connect: %v", err)
	}
	defer conn1.Close()

	// Connect Client 2 (Should succeed)
	conn2, err := connectAndHold(2)
	if err != nil {
		t.Fatalf("Client 2 failed to connect: %v", err)
	}
	defer conn2.Close()

	// Give the server a moment to process the connections and decrement the semaphore
	// (Wait, we want the semaphore to be incremented/held)
	// The server acquires semaphore, then launches goroutine.
	// The goroutine handles connection.
	// We haven't sent anything, so handleSocketConnection is blocked on Read/Decode.
	// So the semaphore is held.
	time.Sleep(100 * time.Millisecond)

	// Connect Client 3 (Should be rejected)
	conn3, err := connectAndHold(3)
	if err != nil {
		// If Dial fails directly, that's also a form of rejection (though likely OS level)
		// But usually Dial succeeds on Unix sockets even if Accept hasn't returned yet, due to backlog.
		// However, if we accept and close, Dial succeeds, then Read returns EOF.
		t.Logf("Client 3 Dial error (unexpected if accepted-then-closed): %v", err)
	} else {
		defer conn3.Close()

		// Try to read from conn3. It should be closed immediately.
		conn3.SetReadDeadline(time.Now().Add(1 * time.Second))
		buf := make([]byte, 1)
		n, err := conn3.Read(buf)

		// We expect EOF (0, io.EOF) or similar
		if n > 0 {
			t.Fatalf("Client 3 should have been rejected, but read %d bytes", n)
		}
		if err == nil {
			t.Fatalf("Client 3 should have been rejected (closed), but Read returned nil error")
		}

		// If the error is a timeout, it means the connection was accepted and kept open (FAIL)
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			t.Fatalf("Client 3 connection timed out (was accepted and held open), expected immediate rejection/close")
		}
		// t.Logf("Client 3 rejected as expected: %v", err)
	}

	// 4. Cleanup
	// Close connections to free up slots (verifying release)
	conn1.Close()
	conn2.Close()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Client 4 (Should succeed now)
	conn4, err := connectAndHold(4)
	if err != nil {
		t.Fatalf("Client 4 failed to connect after slots freed: %v", err)
	}
	conn4.Close()

	// Stop listener by calling the shutdown hook created inside startSocketListener
	// Wait, startSocketListener sets d.shutdown. We can call it.
	// But d.shutdown is set INSIDE startSocketListener.
	// We need to wait until it's set.
	// It's racey to read d.shutdown from here.

	// A better way to stop is to dial and send a special command, but we don't have that.
	// Or just close the listener if we could access it.
	// Since d.shutdown wraps listener.Close(), calling it is the right way, IF it's set.

	// We'll just rely on the test ending. The goroutine will leak until the process exits, which is fine for `go test`.
	// But to be clean:
	if d.shutdown != nil {
		d.shutdown()
	}
	// Wait for listener to exit
	select {
	case <-listenerDone:
	case <-time.After(1 * time.Second):
		// t.Log("Listener didn't exit cleanly, likely due to blocking Accept")
	}
}

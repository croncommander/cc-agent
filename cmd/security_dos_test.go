package cmd

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestSocketReadTimeout verifies that the daemon's socket listener correctly times out
// clients that send data too slowly (Slowloris attack prevention).
func TestSocketReadTimeout(t *testing.T) {
	// Reduce timeout for testing speed
	originalTimeout := socketReadTimeout
	socketReadTimeout = 100 * time.Millisecond
	defer func() { socketReadTimeout = originalTimeout }()

	// Use net.Pipe to simulate a connection without actual sockets
	client, server := net.Pipe()

	// Start the handler in a goroutine
	d := &daemon{} // Mock daemon
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.handleSocketConnection(server)
	}()

	// Simulate a Slowloris attack: write a partial JSON object and wait
	go func() {
		// Write start of JSON
		client.Write([]byte(`{"jobId": "test"`))
		// Do NOT write the rest immediately
	}()

	// Wait for longer than the timeout
	time.Sleep(200 * time.Millisecond)

	// Attempt to read from client. Since server should have closed the connection
	// due to timeout, we expect an error (EOF or "closed pipe").
	buf := make([]byte, 10)
	n, err := client.Read(buf)

	if err == nil && n > 0 {
		t.Fatalf("Expected connection to be closed by server, but read %d bytes: %s", n, buf[:n])
	}
	// Check specifically for EOF or pipe closed
	if err != io.EOF && !strings.Contains(err.Error(), "closed") {
		t.Logf("Got expected read error: %v", err)
	}

	// Ensure handler finished
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("handleSocketConnection did not return after timeout")
	}

	client.Close()
}

// TestSocketReadSuccess verifies that a fast client is handled correctly
func TestSocketReadSuccess(t *testing.T) {
	// Standard timeout
	originalTimeout := socketReadTimeout
	socketReadTimeout = 1 * time.Second
	defer func() { socketReadTimeout = originalTimeout }()

	client, server := net.Pipe()

	d := &daemon{}
	// Mock sendMessage to avoid error log or panic if it tries to use d.conn (which is nil)
	// d.handleSocketConnection calls d.sendMessage.
	// d.sendMessage checks if d.conn is nil and returns error "not connected".
	// This error is logged but not fatal.

	done := make(chan struct{})
	go func() {
		defer close(done)
		d.handleSocketConnection(server)
	}()

	// Send valid JSON quickly
	go func() {
		payload := `{"jobId": "fast", "exitCode": 0, "stdout": "", "stderr": ""}`
		client.Write([]byte(payload))
		client.Close() // Close write side
	}()

	// Wait for handler to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("handleSocketConnection blocked despite valid data")
	}
}

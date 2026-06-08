package cmd

import (
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
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

	var acknowledgement protocol.LocalReportAck
	if err := json.NewDecoder(client).Decode(&acknowledgement); err != nil && err != io.EOF {
		t.Fatalf("Failed to read rejection acknowledgement: %v", err)
	}
	if acknowledgement.Accepted {
		t.Fatal("Expected timed-out partial report to be rejected")
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

	d := &daemon{spoolDir: t.TempDir(), wake: make(chan struct{}, 1)}

	done := make(chan struct{})
	go func() {
		defer close(done)
		d.handleSocketConnection(server)
	}()

	// Send valid JSON quickly
	acknowledgement := make(chan protocol.LocalReportAck, 1)
	go func() {
		payload := `{"eventId":"11111111-1111-4111-8111-111111111111","payload":{"jobId":"fast","exitCode":0,"stdout":"","stderr":""}}`
		_, _ = client.Write([]byte(payload))
		var ack protocol.LocalReportAck
		_ = json.NewDecoder(client).Decode(&ack)
		acknowledgement <- ack
		_ = client.Close()
	}()

	// Wait for handler to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("handleSocketConnection blocked despite valid data")
	}

	ack := <-acknowledgement
	if !ack.Accepted {
		t.Fatalf("Expected valid report to be accepted: %s", ack.Error)
	}
}

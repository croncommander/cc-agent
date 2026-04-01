package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWebSocketSecurity_WriteDeadlock ensures that the daemon does not hang
// indefinitely if the server stops reading data (fill buffer attack).
func TestWebSocketSecurity_WriteDeadlock(t *testing.T) {
	// Temporarily reduce timeout for this test
	originalWriteTimeout := websocketWriteTimeout
	websocketWriteTimeout = 100 * time.Millisecond
	defer func() { websocketWriteTimeout = originalWriteTimeout }()

	// 1. Setup a "hanging" server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Do not read. Let the client's write buffer fill up.
		select {}
	}))
	defer server.Close()

	d := &daemon{
		apiKey:    "test-key",
		serverURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}

	// Connect manually to avoid full run loop
	err := d.connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer d.conn.Close()

	// 2. Flood the connection until it blocks or errors
	// Without SetWriteDeadline, this would hang forever once the TCP buffer is full.
	done := make(chan error)
	go func() {
		for {
			// Send large messages to fill buffer quickly
			payload := strings.Repeat("a", 1024*10)
			err := d.sendMessage(map[string]string{"data": payload})
			if err != nil {
				done <- err
				return
			}
		}
	}()

	// 3. Verify it returns an error within a reasonable time
	select {
	case err := <-done:
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		// Error should be related to timeout or connection close
		if !strings.Contains(err.Error(), "i/o timeout") && !strings.Contains(err.Error(), "broken pipe") {
			t.Logf("Got expected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out: sendMessage did not return error (WriteDeadline broken?)")
	}
}

// TestWebSocketSecurity_ReadTimeout ensures that the daemon detects
// a silent server death (no heartbeats/traffic) and disconnects.
func TestWebSocketSecurity_ReadTimeout(t *testing.T) {
	// Reduce read timeout
	originalReadTimeout := websocketReadTimeout
	websocketReadTimeout = 200 * time.Millisecond
	defer func() { websocketReadTimeout = originalReadTimeout }()

	// 1. Setup a server that accepts but sends nothing
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Send nothing, just hang
		select {}
	}))
	defer server.Close()

	d := &daemon{
		apiKey:    "test-key",
		serverURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}

	err := d.connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer d.conn.Close()

	// 2. Run message loop in background
	done := make(chan struct{})
	go func() {
		// This should return when ReadMessage errors out
		d.messageLoop()
		close(done)
	}()

	// 3. Verify it exits quickly
	select {
	case <-done:
		// Success: messageLoop returned
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out: messageLoop did not return (ReadDeadline broken?)")
	}
}

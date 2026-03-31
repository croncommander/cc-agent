package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/croncommander/cc-agent/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestWebSocketWriteDeadline(t *testing.T) {
	// Start a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Just read loop to keep connection open
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	d := &daemon{
		serverURL: wsURL,
		apiKey:    "test-key",
	}

	// Connect
	err := d.connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() {
		d.connMu.Lock()
		if d.conn != nil {
			d.conn.Close()
		}
		d.connMu.Unlock()
	}()

	// 1. Verify normal send works
	msg := protocol.HeartbeatMessage{Type: "heartbeat"}
	if err := d.sendMessage(msg); err != nil {
		t.Fatalf("Normal sendMessage failed: %v", err)
	}

	// 2. Verify timeout works (by setting timeout to past)
	originalTimeout := websocketWriteTimeout
	defer func() { websocketWriteTimeout = originalTimeout }()

	// Set timeout to a negative value to force immediate timeout
	websocketWriteTimeout = -1 * time.Second

	err = d.sendMessage(msg)
	if err == nil {
		t.Fatal("Expected sendMessage to fail with timeout, but it succeeded")
	}

	// Check error message
	if !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("Expected 'i/o timeout' error, got: %v", err)
	}
}

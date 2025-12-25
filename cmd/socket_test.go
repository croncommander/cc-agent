package cmd

import (
	"os"
	"testing"
)

func TestGetSocketPath(t *testing.T) {
	// 1. Default (Fallback) behavior
	// In the sandbox, /var/lib/croncommander likely doesn't exist, so it should return /tmp
	path := getSocketPath()
	if path != "/tmp/croncommander.sock" {
		// Wait, if /var/lib/croncommander DOES exist (created by me or system), this might fail.
		// Let's check existence first.
		if _, err := os.Stat("/var/lib/croncommander"); err == nil {
			if path != "/var/lib/croncommander/cc-agent.sock" {
				t.Errorf("Expected /var/lib/croncommander/cc-agent.sock, got %s", path)
			}
		} else {
			if path != "/tmp/croncommander.sock" {
				t.Errorf("Expected /tmp/croncommander.sock, got %s", path)
			}
		}
	}
}

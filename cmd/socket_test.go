package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetSocketPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix socket test on Windows")
	}

	// Case 1: Secure directory does not exist. Should fallback to /tmp.
	path := getSocketPath()
	if path == "" {
		t.Error("getSocketPath returned empty string")
	}

	// In this sandbox, likely /var/lib/croncommander does not exist.
	// So it should default to /tmp/croncommander.sock

	secureDir := "/var/lib/croncommander"
	fallback := "/tmp/croncommander.sock"

	info, err := os.Stat(secureDir)
	exists := err == nil && info.IsDir()

	expected := fallback
	if exists {
		expected = filepath.Join(secureDir, "cc-agent.sock")
	}

	if path != expected {
		t.Errorf("Expected path %s, got %s (exists=%v)", expected, path, exists)
	}
}

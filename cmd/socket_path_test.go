package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSocketPathLogic verifies the logic for determining the socket path.
// It mocks the "secure directory" location for testing purposes.
func TestSocketPathLogic(t *testing.T) {
	// Create a temp directory to simulate /var/lib/croncommander
	tmpDir := t.TempDir()
	secureDir := filepath.Join(tmpDir, "secure")

	// Case 1: Secure directory does not exist. Should NOT fallback.
	// We need to inject the secure directory path into the function we are testing.
	// The agent must fail secure (fail to start) rather than degrade to insecure /tmp.

	path := getSocketPathWithBase(secureDir)
	expectedSecure := filepath.Join(secureDir, "cc-agent.sock")
	if path != expectedSecure {
		t.Errorf("Expected secure path %s, got %s", expectedSecure, path)
	}

	// Case 2: Secure directory exists. Should use it.
	if err := os.Mkdir(secureDir, 0750); err != nil {
		t.Fatalf("Failed to create secure dir: %v", err)
	}

	path = getSocketPathWithBase(secureDir)
	// Re-use expectedSecure variable if needed, or just use the same logic
	expectedSecure2 := filepath.Join(secureDir, "cc-agent.sock")
	if path != expectedSecure2 {
		t.Errorf("Expected secure path %s, got %s", expectedSecure2, path)
	}
}

// TestDetermineSocketPath_SecureFallback verifies that we create a secure directory
// when XDG_RUNTIME_DIR is not available.
func TestDetermineSocketPath_SecureFallback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Skipping test as root")
	}

	// Unset XDG_RUNTIME_DIR
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Unsetenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	path, cleanup, err := determineSocketPath()
	if err != nil {
		t.Fatalf("determineSocketPath failed: %v", err)
	}
	defer cleanup()

	// Verify path structure
	if !strings.HasSuffix(path, "cc-agent.sock") {
		t.Errorf("Unexpected socket name in path: %s", path)
	}

	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat created dir: %v", err)
	}

	// Verify permissions are 0700 (drwx------)
	mode := info.Mode().Perm()
	if mode != 0700 {
		// On some systems/temp dirs, sgid bit might be set, or restrictive umask.
		// But MkdirTemp usually creates 0700.
		t.Errorf("Directory permissions insecure: %v (expected 0700)", mode)
	}

	// Verify it is not directly in /tmp (which is world writable)
	// It should be a subdirectory of temp dir.
	if dir == os.TempDir() {
		t.Errorf("Created socket directly in os.TempDir(), which is insecure")
	}
}

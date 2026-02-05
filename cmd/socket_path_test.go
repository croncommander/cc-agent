package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSocketPathLogic verifies the logic for determining the socket path.
// It mocks the "secure directory" location for testing purposes.
func TestSocketPathLogic(t *testing.T) {
	// Create a temp directory to simulate /var/lib/croncommander
	tmpDir := t.TempDir()
	secureDir := filepath.Join(tmpDir, "secure")

	// Case 1: Secure directory does not exist. Should NOT fallback.
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
	expectedSecure2 := filepath.Join(secureDir, "cc-agent.sock")
	if path != expectedSecure2 {
		t.Errorf("Expected secure path %s, got %s", expectedSecure2, path)
	}
}

func TestGetSocketPath_SecureFallback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Skipping test as root")
	}
	// Ensure XDG_RUNTIME_DIR is unset for this test
	origXdg := os.Getenv("XDG_RUNTIME_DIR")
	os.Unsetenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXdg)

	path := getSocketPath()
	expected := filepath.Join(os.TempDir(), fmt.Sprintf("cc-agent-%d", os.Geteuid()), "cc-agent.sock")

	if path != expected {
		t.Errorf("Expected path %s, got %s", expected, path)
	}

	// Check that the directory is private (path structure)
	dir := filepath.Dir(path)
	expectedDir := filepath.Join(os.TempDir(), fmt.Sprintf("cc-agent-%d", os.Geteuid()))
	if dir != expectedDir {
		t.Errorf("Expected socket dir %s, got %s", expectedDir, dir)
	}
}

func TestEnsureSocketDir(t *testing.T) {
	tmpDir := t.TempDir()
	socketDir := filepath.Join(tmpDir, "private-socket-dir")
	socketPath := filepath.Join(socketDir, "socket.sock")

	// 1. Verify directory creation with correct permissions
	if err := ensureSocketDir(socketPath); err != nil {
		t.Fatalf("ensureSocketDir failed: %v", err)
	}

	info, err := os.Stat(socketDir)
	if err != nil {
		t.Fatalf("Failed to stat created dir: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("Expected permissions 0700, got %v", info.Mode().Perm())
	}

	// 2. Verify it fixes incorrect permissions (assuming we own it)
	if err := os.Chmod(socketDir, 0777); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	if err := ensureSocketDir(socketPath); err != nil {
		t.Fatalf("ensureSocketDir fix failed: %v", err)
	}

	info, err = os.Stat(socketDir)
	if err != nil {
		t.Fatalf("Failed to stat fixed dir: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("Expected fixed permissions 0700, got %v", info.Mode().Perm())
	}
}

func TestEnsureSocketDir_SymlinkAttack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	tmpDir := t.TempDir()
	socketDir := filepath.Join(tmpDir, "symlink-dir")
	targetDir := filepath.Join(tmpDir, "target-dir")
	socketPath := filepath.Join(socketDir, "socket.sock")

	// Create target directory
	if err := os.Mkdir(targetDir, 0700); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}

	// Create symlink pointing to target
	if err := os.Symlink(targetDir, socketDir); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// ensureSocketDir should fail because it's a symlink
	err := ensureSocketDir(socketPath)
	if err == nil {
		t.Error("Expected ensureSocketDir to fail on symlink, but it succeeded")
	}
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureSocketDir_Creation(t *testing.T) {
	tmpDir := t.TempDir()
	socketDir := filepath.Join(tmpDir, "socket-dir")

	// 1. Creation
	err := ensureSocketDir(socketDir)
	if err != nil {
		t.Fatalf("ensureSocketDir failed to create dir: %v", err)
	}

	info, err := os.Stat(socketDir)
	if err != nil {
		t.Fatalf("Failed to stat created dir: %v", err)
	}

	if !info.IsDir() {
		t.Errorf("Created path is not a directory")
	}

	// Check permissions (0700)
	perm := info.Mode().Perm()
	if perm != 0700 {
		if runtime.GOOS != "windows" {
			t.Errorf("Expected 0700 permissions, got %o", perm)
		}
	}
}

func TestEnsureSocketDir_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	socketDir := filepath.Join(tmpDir, "existing-dir")

	// Create with correct permissions
	if err := os.Mkdir(socketDir, 0700); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(socketDir, 0700); err != nil {
			t.Fatalf("Failed to chmod: %v", err)
		}
	}

	if err := ensureSocketDir(socketDir); err != nil {
		t.Errorf("ensureSocketDir failed on valid existing dir: %v", err)
	}
}

func TestEnsureSocketDir_BadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission check on Windows")
	}

	tmpDir := t.TempDir()
	socketDir := filepath.Join(tmpDir, "bad-perm-dir")

	// Create with 0755
	if err := os.Mkdir(socketDir, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}
	// Explicitly set permissions to ensure they are incorrect
	if err := os.Chmod(socketDir, 0755); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	if err := ensureSocketDir(socketDir); err == nil {
		t.Errorf("ensureSocketDir should fail on 0755 permissions")
	}
}

func TestGetSocketPath_Fallback(t *testing.T) {
	// Only run if not root, otherwise it returns secure dir
	if os.Geteuid() == 0 {
		t.Skip("Skipping fallback test when running as root")
	}

	originalXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Unsetenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", originalXDG)

	path := getSocketPath()
	expectedPart := fmt.Sprintf("cc-agent-%d", os.Geteuid())

	if !filepath.IsAbs(path) {
		t.Errorf("Expected absolute path, got %s", path)
	}

	// filepath.Base(filepath.Dir(path)) should be cc-agent-<uid>
	dir := filepath.Base(filepath.Dir(path))
	if dir != expectedPart {
		t.Errorf("Expected directory %s, got %s (full path: %s)", expectedPart, dir, path)
	}
}

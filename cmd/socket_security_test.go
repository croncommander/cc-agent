package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureSocketDir(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "mysocketdir", "socket.sock")

	// 1. Test creation of directory
	err := ensureSocketDir(socketPath)
	if err != nil {
		t.Fatalf("ensureSocketDir failed: %v", err)
	}

	dir := filepath.Dir(socketPath)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat failed: %v", err)
	}

	if !info.IsDir() {
		t.Errorf("Expected directory, got file")
	}

	// Windows permissions are tricky, so we skip strict checking there
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0700 {
			t.Errorf("Expected permissions 0700, got %v", info.Mode().Perm())
		}
	}

	// 2. Test fixing permissions
	// Manually set bad permissions
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0777); err != nil {
			t.Fatalf("Failed to chmod: %v", err)
		}

		info, _ = os.Stat(dir)
		if info.Mode().Perm() == 0700 {
			t.Fatalf("Failed to set bad permissions for test")
		}

		// Run ensureSocketDir again
		if err := ensureSocketDir(socketPath); err != nil {
			t.Fatalf("ensureSocketDir failed on fix: %v", err)
		}

		info, _ = os.Stat(dir)
		if info.Mode().Perm() != 0700 {
			t.Errorf("Failed to fix permissions. Expected 0700, got %v", info.Mode().Perm())
		}
	}
}

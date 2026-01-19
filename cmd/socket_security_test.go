package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSocketDir(t *testing.T) {
	// 1. Test in a temp directory (safe)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "mysock", "socket.sock")

	// Should create dir with 0700
	err := ensureSocketDir(socketPath)
	if err != nil {
		t.Fatalf("Failed to ensure socket dir: %v", err)
	}

	dir := filepath.Dir(socketPath)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat dir: %v", err)
	}

	// Check permissions (only if in actual temp dir, which t.TempDir is)
	// But ensureSocketDir checks "if strings.HasPrefix(dir, os.TempDir())"
	// t.TempDir() is inside os.TempDir().
	if strings.HasPrefix(dir, os.TempDir()) {
		mode := info.Mode().Perm()
		if mode != 0700 {
			t.Errorf("Expected 0700, got %o", mode)
		}
	}

	// 2. Test insecure permissions (pre-existing)
	insecureDir := filepath.Join(tmpDir, "insecure")
	if err := os.Mkdir(insecureDir, 0777); err != nil {
		t.Fatal(err)
	}

	insecureSocketPath := filepath.Join(insecureDir, "socket.sock")
	err = ensureSocketDir(insecureSocketPath)
	if err == nil {
		t.Error("Expected error for insecure directory permissions, got nil")
	} else if !strings.Contains(err.Error(), "expected 0700") {
		t.Errorf("Expected permission error, got: %v", err)
	}
}

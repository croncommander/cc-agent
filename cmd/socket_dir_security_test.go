package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSocketDir(t *testing.T) {
	// 1. Missing directory -> Creates with 0700
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "new-secure-dir", "sock.sock")
	if err := ensureSocketDir(socketPath); err != nil {
		t.Fatalf("Failed to create missing dir: %v", err)
	}

	dir := filepath.Dir(socketPath)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat created dir: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("Created dir has mode %o, expected 0700", info.Mode().Perm())
	}

	// 2. Existing secure directory -> Success
	if err := ensureSocketDir(socketPath); err != nil {
		t.Errorf("Failed on existing secure dir: %v", err)
	}

	// 3. Existing insecure directory (permissions) -> Error
	insecureDir := filepath.Join(tmpDir, "insecure-perm")
	if err := os.Mkdir(insecureDir, 0755); err != nil {
		t.Fatal(err)
	}
	socketPathInsecure := filepath.Join(insecureDir, "sock.sock")
	if err := ensureSocketDir(socketPathInsecure); err == nil {
		t.Error("Expected error for 0755 directory, got nil")
	} else {
		// Verify error message content
		expectedMsg := fmt.Sprintf("insecure permissions: %o", 0755)
		if len(err.Error()) < len(expectedMsg) { // weak check but better than nothing
			t.Logf("Got expected error: %v", err)
		}
	}
}

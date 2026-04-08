package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSocketDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Case 1: Directory does not exist, should be created with 0700
	socketDir := filepath.Join(tmpDir, "secure-socket-dir")
	if err := ensureSocketDir(socketDir); err != nil {
		t.Fatalf("ensureSocketDir failed to create dir: %v", err)
	}

	info, err := os.Stat(socketDir)
	if err != nil {
		t.Fatalf("Failed to stat created dir: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("Expected permissions 0700, got %v", info.Mode().Perm())
	}

	// Case 2: Directory exists with correct permissions, should pass
	if err := ensureSocketDir(socketDir); err != nil {
		t.Errorf("ensureSocketDir failed on existing valid dir: %v", err)
	}

	// Case 3: Directory exists with wrong permissions, should fail
	insecureDir := filepath.Join(tmpDir, "insecure-dir")
	if err := os.Mkdir(insecureDir, 0755); err != nil {
		t.Fatalf("Failed to create insecure dir: %v", err)
	}

	if err := ensureSocketDir(insecureDir); err == nil {
		t.Errorf("ensureSocketDir should have failed on 0755 directory")
	} else {
		t.Logf("Correctly rejected insecure dir: %v", err)
	}
}

func TestGetSocketPath_Structure(t *testing.T) {
	// Simulate non-root environment with no XDG_RUNTIME_DIR
	// We can't easily mock geteuid, so we assume this test runs as non-root (common in CI/sandbox).
	// If it runs as root, it will return the secure global path, which is also fine but skips the logic we changed.
	if os.Geteuid() == 0 {
		t.Skip("Skipping UID-based path test when running as root")
	}

	t.Setenv("XDG_RUNTIME_DIR", "")

	path := getSocketPath()
	base := filepath.Base(path)
	dir := filepath.Dir(path)

	if base != "cc-agent.sock" {
		t.Errorf("Expected socket filename 'cc-agent.sock', got '%s'", base)
	}

	// Verify it's in a subdirectory of TempDir (or at least looks like our pattern)
	// We expect: /tmp/cc-agent-<uid>/cc-agent.sock
	// Note: os.TempDir() might return /tmp or /var/folders/...

	// We just check that the parent dir ends with "cc-agent-<uid>"
	uid := os.Geteuid()
	expectedDirName := fmt.Sprintf("cc-agent-%d", uid)

	if filepath.Base(dir) != expectedDirName {
		t.Errorf("Expected directory name '%s', got '%s'", expectedDirName, filepath.Base(dir))
	}
}

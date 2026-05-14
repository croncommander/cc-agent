package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSocketPathSecurity verifies that the socket path is located in a secure directory.
func TestSocketPathSecurity(t *testing.T) {
	// Simulate non-root environment without XDG_RUNTIME_DIR
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("USER", "testuser")

	// Skip if root, as root uses a fixed secure path
	if os.Geteuid() == 0 {
		t.Skip("Skipping non-root socket test when running as root")
	}

	// This function mimics the new logic we want to implement.
	// For now, let's call the OLD getSocketPath and assert it is insecure
	// to demonstrate the issue, or use this to TDD the new logic.
	// Since we can't easily change getSocketPath logic in a test without changing code,
	// We will write the test to verify the NEW behavior we expect.

	// Verify that determineSocketPath creates a secure directory
	securePath, cleanup, err := determineSocketPath(false)
	if err != nil {
		t.Fatalf("determineSocketPath failed: %v", err)
	}
	defer cleanup()

	t.Logf("New secure path: %s", securePath)

	// Check that it is NOT directly in /tmp (it should be in a subdir)
	dir := filepath.Dir(securePath)
	if dir == os.TempDir() {
		t.Errorf("New path is directly in temp dir, expected subdir: %s", securePath)
	}

	// Verify permissions of the parent directory
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat new secure dir: %v", err)
	}

	mode := info.Mode()
	// Check strictly for 0700 (drwx------)
	// Go's mode includes type bits, so verify permission bits
	perm := mode & os.ModePerm
	if perm != 0700 {
		t.Errorf("Insecure permissions on socket dir %s: expected 0700, got %o", dir, perm)
	} else {
		t.Logf("Verified secure permissions 0700 on %s", dir)
	}

	// Verify that cleanup works
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Cleanup failed to remove directory %s", dir)
	}
}

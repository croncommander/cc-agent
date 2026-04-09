package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSocketDir(t *testing.T) {
	// Create a temp base dir
	tmpDir, err := os.MkdirTemp("", "cc-agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Case 1: Directory does not exist, should be created with 0700
	socketDir := filepath.Join(tmpDir, "socket-dir")
	if err := ensureSocketDir(socketDir); err != nil {
		t.Errorf("ensureSocketDir failed to create missing dir: %v", err)
	}

	info, err := os.Stat(socketDir)
	if err != nil {
		t.Fatalf("Failed to stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Created path is not a directory")
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("Created dir permissions = %04o, expected 0700", info.Mode().Perm())
	}

	// Case 2: Directory exists and is correct, should pass
	if err := ensureSocketDir(socketDir); err != nil {
		t.Errorf("ensureSocketDir failed on existing secure dir: %v", err)
	}

	// Case 3: Directory exists with insecure permissions (0755), should be fixed to 0700
	if err := os.Chmod(socketDir, 0755); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}
	if err := ensureSocketDir(socketDir); err != nil {
		t.Errorf("ensureSocketDir failed to fix insecure dir: %v", err)
	}
	info, _ = os.Stat(socketDir)
	if info.Mode().Perm() != 0700 {
		t.Errorf("ensureSocketDir failed to fix permissions. Got %04o", info.Mode().Perm())
	}

	// Case 4: verifySocketDir fails on insecure permissions (no fix)
	if err := os.Chmod(socketDir, 0755); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}
	if err := verifySocketDir(socketDir); err == nil {
		t.Errorf("verifySocketDir should fail on 0755")
	}
}

func TestGetSocketPath_Fallback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Skipping fallback test when running as root")
	}

	// Force fallback by clearing XDG_RUNTIME_DIR
	t.Setenv("XDG_RUNTIME_DIR", "")

	path := getSocketPath()
	uid := os.Getuid()
	expectedPart := fmt.Sprintf("cc-agent-%d", uid)

	// Check if path contains the UID subdirectory
	// Path should be like /tmp/cc-agent-<uid>/cc-agent.sock
	dir := filepath.Dir(path)
	if filepath.Base(dir) != expectedPart {
		t.Errorf("Expected socket directory name %q, got %q (Full path: %s)", expectedPart, filepath.Base(dir), path)
	}

	if filepath.Base(path) != "cc-agent.sock" {
		t.Errorf("Expected socket filename 'cc-agent.sock', got %q", filepath.Base(path))
	}
}

func TestVerifySocketDir_WrongOwner(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping wrong-owner test as root")
	}

	// /tmp is usually owned by root or another user, and/or has 1777 perms.
	// Either way, verifySocketDir should reject it (not 0700, and likely not owned by us, or if owned by us, wrong perms).
	// Ideally we find a dir not owned by us.
	// We can try root of filesystem "/"? Owned by root.
	err := verifySocketDir("/")
	if err == nil {
		t.Errorf("verifySocketDir should fail on / (wrong owner/perms)")
	}
}

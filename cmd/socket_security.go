package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ensureSocketDir checks if the directory for the socket exists and has secure permissions.
// It creates the directory if it doesn't exist.
//
// Security checks:
// 1. Directory must have mode 0700 (drwx------).
// 2. Directory must be owned by the current user.
//
// This prevents:
// - Pre-creation attacks in shared directories like /tmp.
// - Information disclosure (other users reading socket).
// - DoS (other users blocking socket creation).
func ensureSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Create with 0700 permissions
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create socket directory: %w", err)
		}
		// Re-stat to verify creation
		info, err = os.Stat(dir)
		if err != nil {
			return fmt.Errorf("failed to stat socket directory after creation: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to stat socket directory: %w", err)
	}

	// Verify permissions are restricted (0700)
	mode := info.Mode().Perm()
	if mode != 0700 {
		return fmt.Errorf("socket directory %s has insecure permissions: %o (expected 0700)", dir, mode)
	}

	// Verify ownership
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		currentUID := os.Getuid()
		// Handle potential type mismatches across platforms (uint32 vs int)
		if uint64(stat.Uid) != uint64(currentUID) {
			return fmt.Errorf("socket directory %s is owned by uid %d (expected %d)", dir, stat.Uid, currentUID)
		}
	}

	return nil
}

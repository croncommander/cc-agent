package cmd

import (
	"fmt"
	"os"
	"syscall"
)

// ensureSocketDir ensures that the directory exists, is owned by the current user,
// and has 0700 permissions. It creates the directory if it doesn't exist.
func ensureSocketDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Create with 0700
		return os.MkdirAll(dir, 0700)
	}
	if err != nil {
		return fmt.Errorf("failed to stat socket directory: %w", err)
	}

	return checkSocketDirSecurity(dir, info, true)
}

// verifySocketDir verifies that the directory exists and is secure (owned by user, 0700).
// It does not attempt to create or fix permissions.
func verifySocketDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("failed to access socket directory: %w", err)
	}
	return checkSocketDirSecurity(dir, info, false)
}

func checkSocketDirSecurity(dir string, info os.FileInfo, fixPermissions bool) error {
	if !info.IsDir() {
		return fmt.Errorf("socket path %s is not a directory", dir)
	}

	// Check ownership first
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != os.Getuid() {
			return fmt.Errorf("socket directory %s is owned by uid %d, expected %d", dir, stat.Uid, os.Getuid())
		}
	} else {
		return fmt.Errorf("unable to verify ownership of %s (system not supported)", dir)
	}

	// Check permissions (0700)
	// We want strict 0700 (rwx------).
	mode := info.Mode().Perm()
	if mode != 0700 {
		if fixPermissions {
			// We own it, so we can fix it.
			if err := os.Chmod(dir, 0700); err != nil {
				return fmt.Errorf("failed to secure socket directory permissions: %w", err)
			}
		} else {
			return fmt.Errorf("socket directory %s has insecure permissions %04o (expected 0700)", dir, mode)
		}
	}

	return nil
}

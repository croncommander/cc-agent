package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ensureSocketDir ensures the parent directory of the socket exists
// and has secure permissions (0700) to prevent access by other users.
func ensureSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create socket dir: %w", err)
	}

	// Windows doesn't strictly adhere to Unix permissions in the same way,
	// so we skip the strict check.
	if runtime.GOOS == "windows" {
		return nil
	}

	// Verify permissions
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("failed to stat socket dir: %w", err)
	}

	// Enforce 0700 (rwx------)
	if info.Mode().Perm() != 0700 {
		// Try to fix it. This will fail if we don't own the directory,
		// which is the desired behavior (fail secure).
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("insecure socket dir permissions %v and failed to fix: %w", info.Mode().Perm(), err)
		}
	}

	return nil
}

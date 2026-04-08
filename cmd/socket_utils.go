package cmd

import (
	"fmt"
	"os"
	"runtime"
)

// ensureSocketDir ensures that the directory for the socket exists,
// has 0700 permissions, and is owned by the current user.
func ensureSocketDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Create with strict permissions
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("failed to create socket directory: %w", err)
			}
			// Verify again to be sure
			info, err = os.Stat(dir)
			if err != nil {
				return fmt.Errorf("failed to stat created socket directory: %w", err)
			}
		} else {
			return fmt.Errorf("failed to stat socket directory: %w", err)
		}
	}

	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory: %s", dir)
	}

	// Verify permissions are exactly 0700 (Linux/Unix only)
	// On Windows, permissions work differently and os.Mkdir/Stat don't map 1:1 to mode bits in the same way.
	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		if perm != 0700 {
			return fmt.Errorf("insecure socket directory permissions: %o (expected 0700)", perm)
		}
	}

	// Verify ownership
	if err := verifyFileOwner(info); err != nil {
		return fmt.Errorf("insecure socket directory ownership: %w", err)
	}

	return nil
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ensureSocketDir ensures that the directory for the socket exists and is secure.
// For directories in /tmp, it enforces 0700 permissions and ownership by the current user.
func ensureSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)

	// Check if directory exists
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Create with 0700
		// We use MkdirAll, but we really expect this to be a single level in tmp.
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create socket directory: %w", err)
		}
		// Refresh info
		info, err = os.Stat(dir)
		if err != nil {
			return fmt.Errorf("failed to stat created directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to stat socket directory: %w", err)
	}

	// Security checks
	// We only strictly enforce 0700 for temporary directories to avoid breaking
	// system paths like /var/lib/croncommander which might have different permissions.
	// However, we MUST check ownership to prevent hijacking.

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get system stat for %s", dir)
	}

	uid := uint32(os.Getuid())

	// If running as root (system mode), we might be accessing a directory owned by root.
	// If running as user, it must be owned by user.
	// Note: os.Getuid() returns the real user ID.
	if stat.Uid != uid {
		return fmt.Errorf("insecure socket directory: %s is owned by uid %d, expected %d", dir, stat.Uid, uid)
	}

	// If in temp dir, enforce strict permissions
	// This protects against pre-creation attacks in shared directories.
	if strings.HasPrefix(dir, os.TempDir()) {
		mode := info.Mode().Perm()
		if mode != 0700 {
			return fmt.Errorf("insecure socket directory: %s has mode %o, expected 0700", dir, mode)
		}
	}

	return nil
}

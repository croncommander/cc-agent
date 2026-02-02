package cmd

import (
	"fmt"
	"os"
	"syscall"
)

// ensureSocketDir ensures the socket directory exists with secure permissions.
// It enforces 0700 permissions and ownership by the current user.
// SECURITY: This prevents race conditions and pre-creation attacks in shared directories like /tmp.
func ensureSocketDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Create with 0700 (rwx------)
		// We use MkdirAll to ensure parents exist, but typically this is just one level in /tmp
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create socket directory: %w", err)
		}
		// Re-stat to verify (and handle potential race if someone else created it in between)
		info, err = os.Stat(dir)
		if err != nil {
			return fmt.Errorf("failed to stat created directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to stat socket directory: %w", err)
	}

	// Verify permissions
	// Note: We check the least significant 9 bits (permissions).
	mode := info.Mode().Perm()
	if mode != 0700 {
		return fmt.Errorf("insecure socket directory permissions: %v (expected -rwx------ / 0700)", mode)
	}

	// Verify ownership
	// We use syscall.Stat_t to access the UID, which is not available in os.FileInfo in a portable way.
	// This works on Linux/Unix.
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		currentUID := uint32(os.Geteuid())
		if stat.Uid != currentUID {
			return fmt.Errorf("insecure socket directory ownership: uid %d (expected %d)", stat.Uid, currentUID)
		}
	}

	return nil
}

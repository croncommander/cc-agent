// +build !windows

package cmd

import (
	"fmt"
	"os"
	"syscall"
)

// verifyFileOwner checks if the file is owned by the current effective user.
func verifyFileOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get system file info")
	}

	uid := uint32(os.Geteuid())
	if stat.Uid != uid {
		return fmt.Errorf("file owner mismatch: expected uid %d, got %d", uid, stat.Uid)
	}

	return nil
}

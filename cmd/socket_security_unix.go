// +build !windows

package cmd

import (
	"fmt"
	"os"
	"syscall"
)

// verifySocketDirOwnership ensures that the directory is owned by the current effective user.
// This prevents the "confused deputy" problem where we might secure a directory owned by an attacker.
func verifySocketDirOwnership(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to retrieve file ownership information")
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("socket directory is not owned by the current user (uid=%d, owner=%d)", os.Geteuid(), stat.Uid)
	}
	return nil
}

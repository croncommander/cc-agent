// +build windows

package cmd

import "os"

// verifySocketDirOwnership is a no-op on Windows as file ownership semantics differ.
func verifySocketDirOwnership(info os.FileInfo) error {
	return nil
}

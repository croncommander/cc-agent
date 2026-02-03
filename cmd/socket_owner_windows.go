// +build windows

package cmd

import "os"

// verifyFileOwner is a no-op on Windows.
func verifyFileOwner(info os.FileInfo) error {
	return nil
}

// +build !linux

package cmd

// setNoNewPrivs is a no-op on non-Linux platforms.
// PR_SET_NO_NEW_PRIVS is a Linux-specific feature (kernel 3.5+).
// On BSD, macOS, and other platforms this function does nothing.
func setNoNewPrivs() {
	// No-op: PR_SET_NO_NEW_PRIVS not available on this platform
}

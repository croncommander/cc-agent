//go:build linux
// +build linux

package cmd

import (
	"log"
	"syscall"
)

// setNoNewPrivs sets PR_SET_NO_NEW_PRIVS to prevent privilege escalation.
// This prevents the process and its children from gaining new privileges
// via setuid/setgid binaries or file capabilities.
// SECURITY: This is a defense-in-depth measure to limit RCE impact.
func setNoNewPrivs() {
	// PR_SET_NO_NEW_PRIVS = 38, value = 1 to enable
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, 38, 1, 0)
	if errno != 0 {
		log.Printf("Warning: Failed to set PR_SET_NO_NEW_PRIVS: %v", errno)
	}
}

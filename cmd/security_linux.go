// +build linux

package cmd

import (
	"fmt"
	"log"
	"net"
	"syscall"
)

// verifySocketPeer checks that the unix socket peer is either the current user or root.
// This prevents connecting to a spoofed socket created by another user in a shared directory.
func verifySocketPeer(conn *net.UnixConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get syscall connection: %w", err)
	}

	var ucred *syscall.Ucred
	var sysErr error

	// Control allows us to use the raw file descriptor without interfering with the runtime's poller.
	err = raw.Control(func(fd uintptr) {
		ucred, sysErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return fmt.Errorf("control failed: %w", err)
	}
	if sysErr != nil {
		return fmt.Errorf("getsockopt failed: %w", sysErr)
	}

	myUid := uint32(getCurrentUid())
	// Allow if peer is me OR peer is root (uid 0)
	if ucred.Uid != myUid && ucred.Uid != 0 {
		return fmt.Errorf("socket peer uid %d does not match current uid %d or root", ucred.Uid, myUid)
	}

	return nil
}

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

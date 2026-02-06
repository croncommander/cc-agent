// +build linux

package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
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

var getCurrentUid = os.Geteuid

// verifySocketPeer checks that the remote peer of the Unix socket
// has the same UID as the current process (or is root).
// This prevents connecting to a spoofed socket created by another user in /tmp.
func verifySocketPeer(conn *net.UnixConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var cred *syscall.Ucred
	var sysErr error

	err = raw.Control(func(fd uintptr) {
		cred, sysErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return err
	}
	if sysErr != nil {
		return sysErr
	}

	uid := getCurrentUid()
	// Allow if peer is self OR peer is root
	if cred.Uid != uint32(uid) && cred.Uid != 0 {
		return fmt.Errorf("security: connected to socket owned by UID %d, expected %d or 0", cred.Uid, uid)
	}
	return nil
}

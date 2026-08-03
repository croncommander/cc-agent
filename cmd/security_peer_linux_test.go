// +build linux

package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifySocketPeer(t *testing.T) {
	// 1. Create a socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	// 2. Dial it
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		t.Fatalf("Not a unix conn")
	}

	// 3. Test Valid Case: Mock UID == Real UID
	originalGetUid := getCurrentUid
	defer func() { getCurrentUid = originalGetUid }()

	realUid := os.Geteuid()
	getCurrentUid = func() int { return realUid }

	if err := verifySocketPeer(unixConn); err != nil {
		t.Errorf("Expected success for matching UID, got error: %v", err)
	}

	// 4. Test Invalid Case: Mock UID != Real UID
	// Only possible if realUid (the peer's UID) is NOT 0.
	// Because if peer is 0, we always allow it.
	if realUid == 0 {
		t.Log("Running as root (UID 0), skipping failure test case because root peers are always trusted")
		return
	}

	fakeUid := realUid + 1
	getCurrentUid = func() int { return fakeUid }

	if err := verifySocketPeer(unixConn); err == nil {
		t.Errorf("Expected error for mismatched UID (peer=%d, expected=%d), got nil", realUid, fakeUid)
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

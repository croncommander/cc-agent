package cmd

import (
	"os"
	"path/filepath"
)

// getSocketPath determines the appropriate socket path.
// It prioritizes the secure directory /var/lib/croncommander if available and writable.
// Otherwise, it falls back to /tmp/croncommander.sock.
//
// SECURITY: Using a fixed path in /tmp is insecure because it allows for local DoS
// and potential hijacking. /var/lib/croncommander is restricted to the ccrunner user.
func getSocketPath() string {
	secureDir := "/var/lib/croncommander"
	secureSocket := filepath.Join(secureDir, "cc-agent.sock")
	fallbackSocket := "/tmp/croncommander.sock"

	// Check if secure directory exists
	info, err := os.Stat(secureDir)
	if err != nil || !info.IsDir() {
		return fallbackSocket
	}

	// We only check if the directory exists.
	// We do NOT check for write permissions because:
	// 1. The client (running as a user) might not have write permission to the directory,
	//    but needs to connect to the socket therein.
	// 2. The daemon (running as ccrunner) is responsible for creating the socket.
	//
	// If the directory exists, we assume the system is configured for secure IPC.
	return secureSocket
}

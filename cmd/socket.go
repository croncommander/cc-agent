package cmd

import (
	"os"
)

// getSocketPath determines the safest available socket path.
// It prioritizes a secure directory (/var/lib/croncommander) if it exists,
// falling back to /tmp/croncommander.sock (legacy/insecure) if not.
func getSocketPath() string {
	securePath := "/var/lib/croncommander"
	if _, err := os.Stat(securePath); err == nil {
		return securePath + "/cc-agent.sock"
	}
	// Fallback to legacy path
	return "/tmp/croncommander.sock"
}

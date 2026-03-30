package main

import (
	"github.com/croncommander/cc-agent/cmd"
)

// Version is injected at build time via -ldflags "-X main.Version=..."
var Version = "dev"

func main() {
	cmd.SetVersion(Version)
	cmd.Execute()
}

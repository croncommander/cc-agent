package cmd

import "github.com/croncommander/cc-agent/internal/protocol"

// UnifiedMessage combines fields from all message types to allow single-pass unmarshalling.
// This significantly improves performance by avoiding double JSON parsing.
type UnifiedMessage struct {
	Type string `json:"type"`

	// RegisterAck + Error fields
	Status  string `json:"status"`
	AgentID string `json:"agentId"`
	Reason  string `json:"reason"`

	// SyncJobs fields
	Jobs []protocol.JobDefinition `json:"jobs"`

	// Payload field (future proofing if we receive wrapped payloads)
	// Currently not used for incoming messages but good practice
	// Payload json.RawMessage `json:"payload"`
}

package protocol

// Message is the base message type
type Message struct {
	Type string `json:"type"`
}

// RegisterMessage is sent by agent to register with the listener
type RegisterMessage struct {
	Type          string `json:"type"`
	ApiKey        string `json:"apiKey"`
	Hostname      string `json:"hostname"`
	Os            string `json:"os"`
	ExecutionMode string `json:"executionMode"`
	IsRoot        bool   `json:"isRoot"`
}

// RegisterAckMessage is the response to registration
type RegisterAckMessage struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	AgentID string `json:"agentId,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// HeartbeatMessage is sent periodically to maintain connection
type HeartbeatMessage struct {
	Type string `json:"type"`
}

// HeartbeatAckMessage is the response to heartbeat
type HeartbeatAckMessage struct {
	Type string `json:"type"`
}

// ExecutionReportPayload contains the execution details
// SECURITY: All fields are logged verbatim for auditability - commands are NOT redacted.
type ExecutionReportPayload struct {
	JobID         string `json:"jobId"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exitCode"`
	ExecutingUID  int    `json:"executingUid"`      // UID of the user executing the job
	ExecutingUser string `json:"executingUser"`     // Username of the user executing the job
	Warning       string `json:"warning,omitempty"` // Security warnings (e.g., unexpected user)
	Stdout        string `json:"stdout"`
	Stderr        string `json:"stderr"`
	StartTime     string `json:"startTime"`
	DurationMs    int    `json:"durationMs"`
}

// ExecutionReportMessage wraps an execution report
type ExecutionReportMessage struct {
	Type    string                 `json:"type"`
	Payload ExecutionReportPayload `json:"payload"`
}

// ReportAckMessage acknowledges receipt of an execution report
type ReportAckMessage struct {
	Type        string `json:"type"`
	ExecutionID string `json:"executionId"`
}

// SyncJobsMessage contains job definitions to sync
type SyncJobsMessage struct {
	Type string          `json:"type"`
	Jobs []JobDefinition `json:"jobs"`
}

// JobDefinition represents a cron job to be synced
type JobDefinition struct {
	JobID          string `json:"jobId"`
	CronExpression string `json:"cronExpression"`
	Command        string `json:"command"`
}

// ErrorMessage indicates a protocol error
type ErrorMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

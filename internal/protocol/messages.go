package protocol

type RegisterRequest struct {
	Hostname      string `json:"hostname"`
	Os            string `json:"os"`
	ExecutionMode string `json:"executionMode"`
	IsRoot        bool   `json:"isRoot"`
}

type RegisterResponse struct {
	AgentID             string `json:"agentId"`
	AgentToken          string `json:"agentToken"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	PollJitterSeconds   int    `json:"pollJitterSeconds"`
}

type PollRequest struct {
	ManifestVersion string `json:"manifestVersion,omitempty"`
}

type PollResponse struct {
	ManifestVersion string          `json:"manifestVersion"`
	Changed         bool            `json:"changed"`
	Jobs            []JobDefinition `json:"jobs"`
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

type SpooledReport struct {
	EventID string                 `json:"eventId"`
	Payload ExecutionReportPayload `json:"payload"`
}

type LocalExecutionReport = SpooledReport

type ReportResponse struct {
	ExecutionID string `json:"executionId"`
	Duplicate   bool   `json:"duplicate"`
}

type LocalReportAck struct {
	Accepted bool   `json:"accepted"`
	Error    string `json:"error,omitempty"`
}

// JobDefinition represents a cron job to be synced
type JobDefinition struct {
	JobID          string `json:"jobId"`
	CronExpression string `json:"cronExpression"`
	Command        string `json:"command"`
}

package cmd

import (
	"strings"
	"testing"

	"github.com/croncommander/cc-agent/internal/protocol"
)

func TestGenerateCronContent_Sanitization(t *testing.T) {
	jobs := []protocol.JobDefinition{
		{
			JobID:          "safe-job",
			CronExpression: "*/5 * * * *",
			Command:        "echo safe",
		},
		{
			JobID:          "malicious-job-newline",
			CronExpression: "* * * * *",
			Command:        "echo hello\n* * * * * root echo 'pwned'",
		},
		{
			JobID:          "malicious-job-cron-newline",
			CronExpression: "* * * * *\n* * * * * root echo 'pwned'",
			Command:        "echo hello",
		},
		{
			JobID:          "malicious-job-id-newline",
			CronExpression: "* * * * *",
			Command:        "echo hello",
		},
	}
	jobs[3].JobID = "job\nid"

	content := generateCronContent(jobs)
	output := string(content)

	// The malicious jobs should be skipped.
	if strings.Contains(output, "pwned") {
		t.Errorf("Vulnerability found: Output contains injected content 'pwned'")
	}

	// Check that we have the safe job
	if !strings.Contains(output, "safe-job") {
		t.Errorf("Expected safe job to be present")
	}

	// Check for the split line from malicious-job-newline
	if strings.Contains(output, "root echo 'pwned'") {
		t.Errorf("Vulnerability found: Injected root command found")
	}

	// Check for newline in Job ID
	if strings.Contains(output, "job\nid") {
		t.Errorf("Vulnerability found: Job ID newline preserved")
	}
}

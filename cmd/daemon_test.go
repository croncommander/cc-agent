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

	content := generateCronContent(jobs, false, "/tmp/mock.sock")
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

func TestGenerateCronContent_CommandInjection(t *testing.T) {
	jobs := []protocol.JobDefinition{
		{
			JobID:          "injection-test",
			CronExpression: "* * * * *",
			Command:        "echo hello; echo pwned",
		},
		{
			JobID:          "id-injection; rm -rf /",
			CronExpression: "* * * * *",
			Command:        "echo safe",
		},
	}

	content := generateCronContent(jobs, false, "/tmp/mock.sock")
	output := string(content)

	// Expected output for command injection:
	// ... --job-id 'injection-test' -- /bin/sh -c 'echo hello; echo pwned'
	expectedCmd := " /bin/sh -c 'echo hello; echo pwned'"
	if !strings.Contains(output, expectedCmd) {
		t.Errorf("Security fix missing: Command should be wrapped in /bin/sh -c and quoted. Got: %s", output)
	}

	// Expected output for ID injection:
	// ... --job-id 'id-injection; rm -rf /' -- ...
	// Just check that it is quoted
	expectedID := "'id-injection; rm -rf /'"
	if !strings.Contains(output, expectedID) {
		t.Errorf("Security fix missing: Job ID should be quoted. Got: %s", output)
	}

	// Ensure no raw injection
	if strings.Contains(output, " --job-id id-injection; rm -rf /") {
		t.Errorf("Vulnerability found: Job ID injection possible")
	}
}

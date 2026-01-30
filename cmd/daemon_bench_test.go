package cmd

import (
	"fmt"
	"testing"

	"github.com/croncommander/cc-agent/internal/protocol"
)

func BenchmarkGenerateCronContent(b *testing.B) {
	// Create a reasonable number of jobs
	jobs := make([]protocol.JobDefinition, 100)
	for i := 0; i < 100; i++ {
		jobs[i] = protocol.JobDefinition{
			JobID:          fmt.Sprintf("job-%d", i),
			CronExpression: "*/5 * * * *",
			Command:        "echo hello world",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateCronContent(jobs, false)
	}
}

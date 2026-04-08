package cmd

import (
	"fmt"
	"testing"

	"github.com/croncommander/cc-agent/internal/protocol"
)

func BenchmarkGenerateCronContent(b *testing.B) {
	// Setup a large list of jobs
	jobs := make([]protocol.JobDefinition, 1000)
	for i := 0; i < 1000; i++ {
		jobs[i] = protocol.JobDefinition{
			JobID:          fmt.Sprintf("job-%d", i),
			CronExpression: "*/5 * * * *",
			Command:        fmt.Sprintf("echo job %d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateCronContent(jobs, false)
	}
}

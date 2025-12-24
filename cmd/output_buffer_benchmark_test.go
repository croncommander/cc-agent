package cmd

import (
	"strings"
	"testing"
)

func BenchmarkLimitedBuffer_WriteString(b *testing.B) {
	lb := newLimitedBuffer()
	s := strings.Repeat("a", 1024) // 1KB string

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.WriteString(s)
	}
}

func BenchmarkLimitedBuffer_String_Wrapped(b *testing.B) {
	lb := newLimitedBuffer()
	// Fill buffer to force wrapping
	fill := strings.Repeat("a", maxBufferSize+100)
	lb.WriteString(fill)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lb.String()
	}
}

package cmd

import (
	"bytes"
	"testing"
)

func TestWriteShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "simple string",
			input:    "simple",
			expected: "'simple'",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "string with single quote",
			input:    "don't",
			expected: "'don'\\''t'",
		},
		{
			name:     "string with multiple single quotes",
			input:    "'quoted'",
			expected: "''\\''quoted'\\'''",
		},
		{
			name:     "string with special chars",
			input:    "!@#$%^&*()",
			expected: "'!@#$%^&*()'",
		},
		{
			name:     "complex string",
			input:    "string-with-'single'-quotes-and-spaces",
			expected: "'string-with-'\\''single'\\''-quotes-and-spaces'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeShellQuote(&buf, tt.input)
			if got := buf.String(); got != tt.expected {
				t.Errorf("writeShellQuote() = %v, want %v", got, tt.expected)
			}
		})
	}
}

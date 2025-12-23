package cmd

import (
	"testing"
)

func TestLimitedBuffer_Write(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		writes    []string
		want      string
		wantTrunc bool
	}{
		{
			name:      "No truncation",
			limit:     10,
			writes:    []string{"hello", "world"},
			want:      "helloworld",
			wantTrunc: false,
		},
		{
			name:      "Exact limit",
			limit:     11,
			writes:    []string{"hello", "world!"},
			want:      "helloworld!",
			wantTrunc: false,
		},
		{
			name:      "Truncation keep tail single write",
			limit:     5,
			writes:    []string{"helloworld"},
			want:      "... output truncated ...\nworld",
			wantTrunc: true,
		},
		{
			name:      "Truncation keep tail multiple writes",
			limit:     5,
			writes:    []string{"12345", "67890"},
			want:      "... output truncated ...\n67890",
			wantTrunc: true,
		},
		{
			name:      "Truncation partial overwrite",
			limit:     5,
			writes:    []string{"12345", "67"},
			// Buffer: [1 2 3 4 5] -> wrapped -> [6 7 3 4 5] (head at 2)
			// Output: 3 4 5 6 7
			want:      "... output truncated ...\n34567",
			wantTrunc: true,
		},
		{
			name:      "Truncation with very large write",
			limit:     5,
			writes:    []string{"abcdefghijklmn"},
			// Should keep last 5: "jklmn"
			want:      "... output truncated ...\njklmn",
			wantTrunc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := newLimitedBuffer(tt.limit)
			for _, w := range tt.writes {
				n, err := lb.Write([]byte(w))
				if err != nil {
					t.Errorf("Write() error = %v", err)
				}
				if n != len(w) {
					t.Errorf("Write() returned %v, want %v", n, len(w))
				}
			}

			isTruncated := lb.totalWritten > int64(tt.limit)
			if isTruncated != tt.wantTrunc {
				t.Errorf("truncated = %v, want %v", isTruncated, tt.wantTrunc)
			}

			if got := lb.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLimitedBuffer_ExactBehavior(t *testing.T) {
	lb := newLimitedBuffer(5)

	// Write "abcde" (5 chars)
	n, err := lb.Write([]byte("abcde"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
	if lb.totalWritten > 5 {
		t.Errorf("should not be truncated yet")
	}
	if lb.String() != "abcde" {
		t.Errorf("got %q, want 'abcde'", lb.String())
	}

	// Write "f" -> should wrap to "bcdef"
	n, err = lb.Write([]byte("f"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
	if lb.totalWritten <= 5 {
		t.Errorf("should be truncated")
	}

	expected := "... output truncated ...\nbcdef"
	if lb.String() != expected {
		t.Errorf("got %q, want %q", lb.String(), expected)
	}
}

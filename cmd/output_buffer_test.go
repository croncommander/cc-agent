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
			name:      "Truncation in single write",
			limit:     5,
			writes:    []string{"helloworld"},
			want:      "hello\n... output truncated ...",
			wantTrunc: true,
		},
		{
			name:      "Truncation in second write",
			limit:     10,
			writes:    []string{"hello", " world! this is long"},
			want:      "hello worl\n... output truncated ...",
			wantTrunc: true,
		},
		{
			name:      "Writes after truncation are ignored",
			limit:     5,
			writes:    []string{"hello", "world", "ignored"},
			want:      "hello\n... output truncated ...",
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

			if lb.truncated != tt.wantTrunc {
				t.Errorf("truncated = %v, want %v", lb.truncated, tt.wantTrunc)
			}

			if got := lb.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLimitedBuffer_ExactBehavior(t *testing.T) {
	lb := newLimitedBuffer(10)
	n, err := lb.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
	if lb.String() != "hello" {
		t.Errorf("expected 'hello', got '%s'", lb.String())
	}

	// Write exceeding limit
	n, err = lb.Write([]byte("world!"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Errorf("expected 6 (fake success), got %d", n)
	}

	expected := "helloworld\n... output truncated ..."
	if lb.String() != expected {
		t.Errorf("expected '%s', got '%s'", expected, lb.String())
	}

	// Subsequent write
	n, err = lb.Write([]byte("foo"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
	if lb.String() != expected {
		t.Errorf("buffer changed after truncation")
	}
}

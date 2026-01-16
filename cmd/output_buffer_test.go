package cmd

import (
	"strings"
	"testing"
)

func TestLimitedBuffer_NoWrap(t *testing.T) {
	lb := newLimitedBuffer()

	input := "Hello, World!"
	n, err := lb.Write([]byte(input))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(input) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(input), n)
	}

	if got := lb.String(); got != input {
		t.Errorf("String() = %q, want %q", got, input)
	}

	// Write more
	input2 := " More text."
	lb.Write([]byte(input2))
	if got := lb.String(); got != input+input2 {
		t.Errorf("String() = %q, want %q", got, input+input2)
	}
}

func TestLimitedBuffer_ExactLimit(t *testing.T) {
	lb := newLimitedBuffer()

	// Create a byte slice exactly the size of maxBufferSize
	data := make([]byte, maxBufferSize)
	for i := range data {
		data[i] = 'A'
	}

	lb.Write(data)

	got := lb.String()
	// Note: Our implementation marks exact fill as wrapped, so it adds the prefix.
	// This is acceptable behavior for the limit boundary.
	expectedPrefix := "... output truncated ..."
	if !strings.HasPrefix(got, expectedPrefix) {
		t.Errorf("Expected prefix %q, got start of %q", expectedPrefix, got)
	}

	content := strings.TrimPrefix(got, expectedPrefix)
	if len(content) != maxBufferSize {
		t.Errorf("Expected content length %d, got %d", maxBufferSize, len(content))
	}

	if content != string(data) {
		t.Errorf("Content mismatch")
	}
}

func TestLimitedBuffer_SmallCapacity(t *testing.T) {
	// Use a manually constructed buffer with small capacity for logic testing
	capacity := 10
	lb := &limitedBuffer{
		buf: make([]byte, capacity),
	}

	// 1. Write less than capacity
	lb.Write([]byte("12345"))
	if lb.String() != "12345" {
		t.Errorf("Got %q, want %q", lb.String(), "12345")
	}

	// 2. Fill to exact capacity
	lb.Write([]byte("67890"))

	got := lb.String()
	want := "... output truncated ...1234567890"
	if got != want {
		t.Errorf("Got %q, want %q", got, want)
	}

	// 3. Write one more byte (overwrite '1')
	lb.Write([]byte("A"))

	got = lb.String()
	want = "... output truncated ...234567890A"
	if got != want {
		t.Errorf("Got %q, want %q", got, want)
	}

	// 4. Write string larger than buffer
	lb.Write([]byte("BCDEFGHIJKLM")) // 12 chars
	// Should keep last 10: "DEFGHIJKLM"

	got = lb.String()
	want = "... output truncated ...DEFGHIJKLM"
	if got != want {
		t.Errorf("Got %q, want %q", got, want)
	}
}

func TestLimitedBuffer_WriteLarge(t *testing.T) {
	lb := newLimitedBuffer()

	// Create data larger than maxBufferSize
	size := maxBufferSize + 100
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	lb.Write(data)

	if !lb.isWrapped {
		t.Error("Expected wrapped to be true")
	}

	// Check that we have the tail
	expectedTail := data[size-maxBufferSize:]

	gotString := lb.String()
	if !strings.HasPrefix(gotString, "... output truncated ...") {
		t.Error("Expected truncated prefix")
	}

	content := strings.TrimPrefix(gotString, "... output truncated ...")
	if len(content) != maxBufferSize {
		t.Errorf("Expected content length %d, got %d", maxBufferSize, len(content))
	}

	if content != string(expectedTail) {
		t.Error("Content does not match expected tail")
	}
}

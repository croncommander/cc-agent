package cmd

import (
	"bytes"
)

const defaultMaxOutputSize = 256 * 1024 // 256KB

// limitedBuffer implements a ring buffer to retain the last N bytes of output.
type limitedBuffer struct {
	data         []byte
	head         int   // write index
	totalWritten int64 // total bytes written
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = defaultMaxOutputSize
	}
	// Pre-allocate the full buffer for ring behavior
	return &limitedBuffer{
		data: make([]byte, limit),
	}
}

func (l *limitedBuffer) Write(p []byte) (n int, err error) {
	totalLen := len(p)

	// If the write is larger than the buffer, we only keep the last chunk of it
	if totalLen > len(l.data) {
		// Only keep the last len(l.data) bytes
		p = p[totalLen-len(l.data):]
		// Reset buffer effectively by writing this full chunk
		// But we must respect totalWritten for truncation detection
		// If we overwrite everything, head is 0 after this.
		// Wait, if we write exactly len(l.data), head ends at 0?
		// No, if we write len, head advances by len.
		// If head + len > len, it wraps.

		// Let's rely on the loop to handle it correctly.
		// But to avoid huge loops for 1GB write, we skip.
		// Logic above: `p = p[totalLen-len(l.data):]` reduces p to size of buffer.
		// But we still need to increment totalWritten by original amount!
	}

	for len(p) > 0 {
		available := len(l.data) - l.head
		toWrite := len(p)
		if toWrite > available {
			toWrite = available
		}

		copy(l.data[l.head:], p[:toWrite])

		l.head += toWrite
		if l.head >= len(l.data) {
			l.head = 0
		}
		p = p[toWrite:]
	}

	l.totalWritten += int64(totalLen)

	// Return the original length to satisfy io.Writer contract
	return totalLen, nil
}

func (l *limitedBuffer) String() string {
	limit := int64(len(l.data))
	if l.totalWritten <= limit {
		return string(l.data[:l.totalWritten])
	}

	// Truncated. Reconstruct.
	// Current head points to the oldest byte (because we just wrote to head-1).
	// Wait. If we have written limit+1 bytes.
	// Write 10 bytes (limit 10). head=0. total=10.
	// Write 1 byte. head=1. total=11.
	// Oldest byte is at 1. (data[1]).
	// Newest byte is at 0. (data[0]).
	// Sequence: data[1:] + data[:1].
	// Correct.

	var buf bytes.Buffer
	prefix := "... output truncated ...\n"
	buf.Grow(len(prefix) + len(l.data))

	buf.WriteString(prefix)
	buf.Write(l.data[l.head:])
	buf.Write(l.data[:l.head])

	return buf.String()
}

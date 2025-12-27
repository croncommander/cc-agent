package cmd

import (
	"strings"
)

const maxBufferSize = 256 * 1024 // 256KB

type limitedBuffer struct {
	buf       []byte
	writePos  int
	isWrapped bool
}

func newLimitedBuffer() *limitedBuffer {
	return &limitedBuffer{
		buf: make([]byte, maxBufferSize),
	}
}

func (lb *limitedBuffer) Write(p []byte) (n int, err error) {
	n = len(p)
	if n == 0 {
		return 0, nil
	}

	cap := len(lb.buf)

	// If the write is larger than the buffer, we only keep the last cap bytes
	if n >= cap {
		lb.isWrapped = true
		copy(lb.buf, p[n-cap:])
		lb.writePos = 0 // Next write starts at the beginning
		return n, nil
	}

	// Calculate how much we can write before reaching the end of the buffer
	remaining := cap - lb.writePos
	if n <= remaining {
		// No wrap-around needed during this write
		copy(lb.buf[lb.writePos:], p)
		lb.writePos += n
		if lb.writePos == cap {
			lb.writePos = 0
			lb.isWrapped = true
		}
	} else {
		// Wrap-around needed
		lb.isWrapped = true
		copy(lb.buf[lb.writePos:], p[:remaining])
		copy(lb.buf[0:], p[remaining:])
		lb.writePos = n - remaining
	}

	return n, nil
}

func (lb *limitedBuffer) WriteString(s string) (n int, err error) {
	// Optimization: Avoid allocation by implementing write logic specifically for string
	// instead of converting to []byte (which allocates).
	n = len(s)
	if n == 0 {
		return 0, nil
	}

	cap := len(lb.buf)

	// If the write is larger than the buffer, we only keep the last cap bytes
	if n >= cap {
		lb.isWrapped = true
		// Use copy built-in which supports copy(dst []byte, src string)
		start := n - cap
		copy(lb.buf, s[start:])
		lb.writePos = 0
		return n, nil
	}

	remaining := cap - lb.writePos
	if n <= remaining {
		copy(lb.buf[lb.writePos:], s)
		lb.writePos += n
		if lb.writePos == cap {
			lb.writePos = 0
			lb.isWrapped = true
		}
	} else {
		lb.isWrapped = true
		copy(lb.buf[lb.writePos:], s[:remaining])
		copy(lb.buf[0:], s[remaining:])
		lb.writePos = n - remaining
	}

	return n, nil
}

func (lb *limitedBuffer) String() string {
	if !lb.isWrapped {
		return string(lb.buf[:lb.writePos])
	}

	var builder strings.Builder
	// Optimization: Pre-allocate buffer to avoid resizing
	// Size = truncated msg len + buffer size
	truncatedMsg := "... output truncated ..."
	builder.Grow(len(truncatedMsg) + len(lb.buf))

	builder.WriteString(truncatedMsg)

	// Write the oldest data first (from writePos to end)
	builder.Write(lb.buf[lb.writePos:])
	// Write the newest data (from start to writePos)
	builder.Write(lb.buf[:lb.writePos])

	return builder.String()
}

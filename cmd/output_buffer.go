package cmd

import (
	"bytes"
)

const defaultMaxOutputSize = 256 * 1024 // 256KB

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = defaultMaxOutputSize
	}
	return &limitedBuffer{
		limit: limit,
	}
}

func (l *limitedBuffer) Write(p []byte) (n int, err error) {
	if l.truncated {
		return len(p), nil
	}
	if l.buf.Len()+len(p) > l.limit {
		remaining := l.limit - l.buf.Len()
		if remaining > 0 {
			l.buf.Write(p[:remaining])
		}
		l.buf.WriteString("\n... output truncated ...")
		l.truncated = true
		return len(p), nil
	}
	return l.buf.Write(p)
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

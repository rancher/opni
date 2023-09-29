package logger

import "sync"

// copied from https://github.com/golang/go/blob/master/src/log/slog/internal/buffer/buffer.go
type buffer []byte

// Having an initial size gives a dramatic speedup.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 1024)
		return (*buffer)(&b)
	},
}

func newBuffer() *buffer {
	return bufPool.Get().(*buffer)
}

func (b *buffer) Free() {
	// To reduce peak allocation, return only smaller buffers to the pool.
	const maxbufferSize = 16 << 10
	if cap(*b) <= maxbufferSize {
		*b = (*b)[:0]
		bufPool.Put(b)
	}
}

func (b *buffer) Write(p []byte) (int, error) {
	*b = append(*b, p...)
	return len(p), nil
}

func (b *buffer) WriteString(s string) (int, error) {
	*b = append(*b, s...)
	return len(s), nil
}

func (b *buffer) WriteStringIf(ok bool, str string) (int, error) {
	if !ok {
		return 0, nil
	}
	return b.WriteString(str)
}

func (b *buffer) WriteByte(c byte) error {
	*b = append(*b, c)
	return nil
}

package coverage

import (
	"io"
	"sync"
)

// SyncWriter returns a writer that serializes Write calls with a mutex.
// Concurrent Generate calls on the same Runner need synchronized writers so
// interleaved `go test` output does not race on the underlying stream.
func SyncWriter(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	if _, ok := w.(*syncWriter); ok {
		return w
	}
	return &syncWriter{w: w}
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

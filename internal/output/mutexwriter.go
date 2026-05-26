package output

import (
	"io"
	"sync"
)

// MutexWriter wraps an io.Writer with a mutex, making it safe for concurrent
// use from multiple goroutines. This is used for --log-file targets that are
// written to by every device goroutine simultaneously (guideline #4: mutex-
// protected log writer from day one).
type MutexWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewMutexWriter wraps w in a MutexWriter.
func NewMutexWriter(w io.Writer) *MutexWriter {
	return &MutexWriter{w: w}
}

// Write implements io.Writer. It serialises concurrent writes with m.mu so
// that lines from different device goroutines are not interleaved.
func (m *MutexWriter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.w.Write(p)
}

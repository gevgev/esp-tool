package output

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestMutexWriter_Write_ForwardsBytes(t *testing.T) {
	var buf bytes.Buffer
	w := NewMutexWriter(&buf)

	msg := []byte("hello mutex\n")
	n, err := w.Write(msg)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write: want n=%d, got %d", len(msg), n)
	}
	if buf.String() != string(msg) {
		t.Errorf("Write: want %q, got %q", msg, buf.String())
	}
}

func TestMutexWriter_Write_PropagatesError(t *testing.T) {
	// errWriter always returns an error.
	ew := &errWriter{err: fmt.Errorf("disk full")}
	w := NewMutexWriter(ew)

	_, err := w.Write([]byte("data"))
	if err == nil {
		t.Error("expected error from underlying writer, got nil")
	}
}

func TestMutexWriter_ConcurrentWrites_NoRace(t *testing.T) {
	var buf bytes.Buffer
	w := NewMutexWriter(&buf)

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fmt.Fprintf(w, "line %d\n", n)
		}(i)
	}
	wg.Wait()

	// Every goroutine wrote exactly one newline-terminated line.
	lines := strings.Count(buf.String(), "\n")
	if lines != goroutines {
		t.Errorf("want %d lines, got %d", goroutines, lines)
	}
}

// errWriter is an io.Writer that always returns an error.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

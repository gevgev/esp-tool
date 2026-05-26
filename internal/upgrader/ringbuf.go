package upgrader

// displayCapacity is the number of output lines held in the per-device ring
// buffer. It matches the architecture spec (~500 lines is enough for the TUI
// output-tail panel without consuming excessive memory per device).
const displayCapacity = 500

// RingBuffer is a bounded circular buffer of strings. When capacity is
// exceeded the oldest entry is silently overwritten. It is NOT safe for
// concurrent use — callers must synchronise if needed.
//
// Used in deviceBuffers.display to hold the most recent output lines for the
// TUI output-tail panel (Phase 1D). The ring buffer is created unconditionally
// so Phase 1D can read from it without a nil check.
type RingBuffer struct {
	buf []string
	pos int // next write position (wraps around)
	n   int // number of valid entries (≤ len(buf))
}

// NewRingBuffer creates a RingBuffer with the given capacity.
// Panics if capacity < 1.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 1 {
		panic("RingBuffer: capacity must be ≥ 1")
	}
	return &RingBuffer{buf: make([]string, capacity)}
}

// Push appends line. If the buffer is full, the oldest entry is overwritten.
func (r *RingBuffer) Push(line string) {
	r.buf[r.pos] = line
	r.pos = (r.pos + 1) % len(r.buf)
	if r.n < len(r.buf) {
		r.n++
	}
}

// Lines returns all buffered entries in oldest-to-newest order as a new slice.
// Returns nil when empty.
func (r *RingBuffer) Lines() []string {
	if r.n == 0 {
		return nil
	}
	out := make([]string, r.n)
	// When n < cap: entries are at buf[0..n-1] in insertion order.
	// When n == cap: oldest is at buf[pos], wrapping around.
	start := (r.pos - r.n + len(r.buf)) % len(r.buf)
	for i := 0; i < r.n; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}

// Len returns the number of entries currently stored.
func (r *RingBuffer) Len() int { return r.n }

// Cap returns the maximum number of entries the buffer can hold.
func (r *RingBuffer) Cap() int { return len(r.buf) }

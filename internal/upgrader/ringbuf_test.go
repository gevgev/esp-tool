package upgrader

import (
	"testing"
)

// ---------------------------------------------------------------------------
// RingBuffer
// ---------------------------------------------------------------------------

func TestRingBuffer_EmptyBuffer_LenZero(t *testing.T) {
	rb := NewRingBuffer(10)
	if rb.Len() != 0 {
		t.Errorf("want 0 initially, got %d", rb.Len())
	}
	if rb.Cap() != 10 {
		t.Errorf("want capacity 10, got %d", rb.Cap())
	}
}

func TestRingBuffer_LinesEmpty_ReturnsNil(t *testing.T) {
	rb := NewRingBuffer(5)
	if got := rb.Lines(); got != nil {
		t.Errorf("empty buffer Lines() should return nil, got %v", got)
	}
}

func TestRingBuffer_Push_SingleLine(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Push("hello")
	if rb.Len() != 1 {
		t.Errorf("want Len=1 after one Push, got %d", rb.Len())
	}
	lines := rb.Lines()
	if len(lines) != 1 || lines[0] != "hello" {
		t.Errorf("want [hello], got %v", lines)
	}
}

func TestRingBuffer_Push_UnderCapacity_PreservesOrder(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Push("a")
	rb.Push("b")
	rb.Push("c")

	got := rb.Lines()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("want %d lines, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: want %q, got %q", i, w, got[i])
		}
	}
}

func TestRingBuffer_Push_ExactlyCapacity(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Push("x")
	rb.Push("y")
	rb.Push("z")

	if rb.Len() != 3 {
		t.Errorf("want Len=3, got %d", rb.Len())
	}
	got := rb.Lines()
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d", len(got))
	}
	if got[0] != "x" || got[1] != "y" || got[2] != "z" {
		t.Errorf("want [x y z], got %v", got)
	}
}

func TestRingBuffer_Push_Overflow_OverwritesOldest(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Push("a") // slot 0 — oldest after overflow
	rb.Push("b")
	rb.Push("c")
	rb.Push("d") // overwrites "a"

	if rb.Len() != 3 {
		t.Errorf("want Len=3 after overflow, got %d", rb.Len())
	}
	got := rb.Lines()
	want := []string{"b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("want 3 lines, got %d: %v", len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: want %q, got %q", i, w, got[i])
		}
	}
}

func TestRingBuffer_Push_MultipleOverflows(t *testing.T) {
	rb := NewRingBuffer(3)
	for i := 0; i < 10; i++ {
		rb.Push(string(rune('a' + i)))
	}
	// After 10 pushes into cap=3: last 3 are h, i, j
	got := rb.Lines()
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d: %v", len(got), got)
	}
	// 'a'+7='h', 'a'+8='i', 'a'+9='j'
	want := []string{"h", "i", "j"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: want %q, got %q", i, w, got[i])
		}
	}
}

func TestRingBuffer_Cap_Returned(t *testing.T) {
	rb := NewRingBuffer(42)
	if rb.Cap() != 42 {
		t.Errorf("want Cap=42, got %d", rb.Cap())
	}
}

func TestRingBuffer_Len_NeverExceedsCap(t *testing.T) {
	cap := 4
	rb := NewRingBuffer(cap)
	for i := 0; i < 100; i++ {
		rb.Push("x")
		if rb.Len() > cap {
			t.Fatalf("Len() %d exceeded Cap() %d after %d pushes", rb.Len(), cap, i+1)
		}
	}
}

func TestRingBuffer_Lines_ReturnsCopy_NotSliceAlias(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Push("original")
	lines := rb.Lines()
	lines[0] = "mutated"

	// Re-fetch — should still see "original"
	got := rb.Lines()
	if got[0] != "original" {
		t.Errorf("Lines() should return a copy; got %q after external mutation", got[0])
	}
}

func TestRingBuffer_DisplayCapacity_Is500(t *testing.T) {
	// Ensure the package constant matches the architecture spec (~500 lines).
	if displayCapacity != 500 {
		t.Errorf("displayCapacity should be 500 per architecture spec, got %d", displayCapacity)
	}
}

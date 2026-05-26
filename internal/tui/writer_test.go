package tui

import (
	"fmt"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// mockSender — test double for *tea.Program
// ---------------------------------------------------------------------------

type mockSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (m *mockSender) Send(msg tea.Msg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
}

func (m *mockSender) received() []tea.Msg {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]tea.Msg, len(m.msgs))
	copy(out, m.msgs)
	return out
}

// ---------------------------------------------------------------------------
// TUIWriter — one method per test
// ---------------------------------------------------------------------------

func TestTUIWriter_WriteLine_SendsDeviceOutputMsg(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.WriteLine("cam", "INFO upload")

	msgs := ms.received()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	msg, ok := msgs[0].(DeviceOutputMsg)
	if !ok {
		t.Fatalf("want DeviceOutputMsg, got %T", msgs[0])
	}
	if msg.Device != "cam" {
		t.Errorf("Device: want %q, got %q", "cam", msg.Device)
	}
	if msg.Line != "INFO upload" {
		t.Errorf("Line: want %q, got %q", "INFO upload", msg.Line)
	}
}

func TestTUIWriter_DeviceStarted_SendsStatusRunning(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.DeviceStarted("cam")

	msgs := ms.received()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	msg, ok := msgs[0].(DeviceStatusMsg)
	if !ok {
		t.Fatalf("want DeviceStatusMsg, got %T", msgs[0])
	}
	if msg.Device != "cam" {
		t.Errorf("Device: want %q, got %q", "cam", msg.Device)
	}
	if msg.Status != StatusRunning {
		t.Errorf("Status: want StatusRunning, got %v", msg.Status)
	}
}

func TestTUIWriter_DeviceCompleted_Success_SendsStatusSuccess(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.DeviceCompleted("cam", true, 1, 30*time.Second, nil)

	msgs := ms.received()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	msg, ok := msgs[0].(DeviceStatusMsg)
	if !ok {
		t.Fatalf("want DeviceStatusMsg, got %T", msgs[0])
	}
	if msg.Status != StatusSuccess {
		t.Errorf("want StatusSuccess, got %v", msg.Status)
	}
	if msg.Attempts != 1 {
		t.Errorf("Attempts: want 1, got %d", msg.Attempts)
	}
	if msg.Duration != 30*time.Second {
		t.Errorf("Duration: want 30s, got %v", msg.Duration)
	}
}

func TestTUIWriter_DeviceCompleted_Failure_SendsStatusFailed(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	errLines := []string{"ERROR Connection refused"}
	w.DeviceCompleted("cam", false, 3, 5*time.Minute, errLines)

	msgs := ms.received()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	msg, ok := msgs[0].(DeviceStatusMsg)
	if !ok {
		t.Fatalf("want DeviceStatusMsg, got %T", msgs[0])
	}
	if msg.Status != StatusFailed {
		t.Errorf("want StatusFailed, got %v", msg.Status)
	}
	if len(msg.ErrLines) != 1 {
		t.Errorf("want 1 ErrLine, got %d", len(msg.ErrLines))
	}
}

func TestTUIWriter_DeviceRetrying_SendsDeviceRetryMsg(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.DeviceRetrying("cam", 2, 3, 5*time.Second)

	msgs := ms.received()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	msg, ok := msgs[0].(DeviceRetryMsg)
	if !ok {
		t.Fatalf("want DeviceRetryMsg, got %T", msgs[0])
	}
	if msg.Device != "cam" {
		t.Errorf("Device: want %q, got %q", "cam", msg.Device)
	}
	if msg.Attempt != 2 {
		t.Errorf("Attempt: want 2, got %d", msg.Attempt)
	}
	if msg.Max != 3 {
		t.Errorf("Max: want 3, got %d", msg.Max)
	}
	if msg.Delay != 5*time.Second {
		t.Errorf("Delay: want 5s, got %v", msg.Delay)
	}
}

// ---------------------------------------------------------------------------
// MarkDone — guideline #6: guard against late prog.Send() after quit
// ---------------------------------------------------------------------------

func TestTUIWriter_AfterMarkDone_DropsMessages(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.MarkDone()
	w.WriteLine("dev", "late message") // should be dropped
	w.DeviceStarted("dev")
	w.DeviceCompleted("dev", true, 1, time.Second, nil)

	if len(ms.received()) != 0 {
		t.Errorf("all messages after MarkDone should be dropped, got %d", len(ms.received()))
	}
}

func TestTUIWriter_MarkDone_Idempotent(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	// Calling MarkDone multiple times must not panic.
	w.MarkDone()
	w.MarkDone()
	w.MarkDone()
}

func TestTUIWriter_BeforeMarkDone_DeliversMessages(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.WriteLine("dev", "line 1")
	w.WriteLine("dev", "line 2")
	// Not yet marked done — messages should arrive.
	if len(ms.received()) != 2 {
		t.Errorf("want 2 messages before MarkDone, got %d", len(ms.received()))
	}
	w.MarkDone()
}

// ---------------------------------------------------------------------------
// AllDoneMsg helper
// ---------------------------------------------------------------------------

func TestTUIWriter_SendAllDone_DeliversMsg(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.SendAllDone()

	msgs := ms.received()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if _, ok := msgs[0].(AllDoneMsg); !ok {
		t.Errorf("want AllDoneMsg, got %T", msgs[0])
	}
}

func TestTUIWriter_SendAllDone_AfterMarkDone_Dropped(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)
	w.MarkDone()
	w.SendAllDone()
	if len(ms.received()) != 0 {
		t.Errorf("SendAllDone after MarkDone should be dropped, got %d messages", len(ms.received()))
	}
}

// ---------------------------------------------------------------------------
// Concurrent safety (run with -race)
// ---------------------------------------------------------------------------

func TestTUIWriter_ConcurrentSends_NoRace(t *testing.T) {
	ms := &mockSender{}
	w := NewTUIWriter(ms)

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			w.WriteLine(fmt.Sprintf("dev-%d", n), "concurrent line")
			w.DeviceStarted(fmt.Sprintf("dev-%d", n))
			w.DeviceRetrying(fmt.Sprintf("dev-%d", n), 2, 3, time.Millisecond)
		}(i)
	}
	wg.Wait()
}

func TestTUIWriter_MarkDone_ConcurrentWithSends_NoRace(t *testing.T) {
	// Ensure MarkDone is safe to call while goroutines are sending.
	ms := &mockSender{}
	w := NewTUIWriter(ms)

	var wg sync.WaitGroup
	const goroutines = 20
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			w.WriteLine(fmt.Sprintf("dev-%d", n), "racing")
		}(i)
	}
	w.MarkDone() // concurrent with the goroutines above
	wg.Wait()
}

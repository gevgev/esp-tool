package tui

import "time"

// DeviceOutputMsg is sent for every line of output from a device's esphome
// process. Delivered via TUIWriter.WriteLine → prog.Send.
type DeviceOutputMsg struct {
	Device string
	Line   string
}

// DeviceStatusMsg is sent when a device's status changes:
//   - StatusRunning:  TUIWriter.DeviceStarted (goroutine acquired semaphore)
//   - StatusSuccess:  TUIWriter.DeviceCompleted(success=true)
//   - StatusFailed:   TUIWriter.DeviceCompleted(success=false)
type DeviceStatusMsg struct {
	Device   string
	Status   Status
	Attempts int
	Duration time.Duration
	ErrLines []string // non-nil on failure; best-effort (guideline #8)
}

// DeviceRetryMsg is sent before each retry sleep.
// The model transitions the device to StatusRetrying; the next DeviceOutputMsg
// transitions it back to StatusRunning (first line of the new attempt).
type DeviceRetryMsg struct {
	Device  string
	Attempt int
	Max     int
	Delay   time.Duration
}

// AllDoneMsg signals that Upgrade() has returned and all device goroutines
// are finished. Sent by the goroutine in main.go via TUIWriter.SendAllDone().
//
// On all-success: Update returns tea.Quit immediately.
// On any failure: model sets Done+HasFailures and waits for the user to press q.
type AllDoneMsg struct{}

// TickMsg is sent every 100ms to drive the braille spinner animation and
// update the elapsed-time display. Initiated by Model.Init() via tickCmd().
type TickMsg struct{ Time time.Time }

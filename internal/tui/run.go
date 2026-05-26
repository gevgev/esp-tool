package tui

import tea "github.com/charmbracelet/bubbletea"

// Start initialises a bubbletea program for the upgrade TUI and returns:
//   - w: a TUIWriter that routes device events into the program's event loop
//   - runFn: a blocking function that runs the TUI; call it in the main goroutine
//
// Typical usage in main.go:
//
//	w, runTUI := tui.Start(model)
//	go func() {
//	    results = upgrader.Upgrade(devices, opts, w)
//	    w.SendAllDone()
//	}()
//	runTUI()      // blocks until user presses q or all devices succeed
//	w.MarkDone()  // drop any late prog.Send() calls (guideline #6)
//	cancel()      // cancel context to kill in-flight esphome processes
func Start(model Model) (*TUIWriter, func() error) {
	prog := tea.NewProgram(model, tea.WithAltScreen())
	w := NewTUIWriter(prog)
	return w, func() error {
		_, err := prog.Run()
		return err
	}
}

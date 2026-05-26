// Package main implements a fake esphome binary used only in test suites.
//
// Build this binary into a temporary directory, prepend that directory to PATH,
// and exec.Command("esphome", ...) calls will hit this stub instead of the real
// esphome CLI.
//
// Behavior is selected entirely via environment variables — command-line
// arguments are accepted but ignored, matching how the real esphome CLI would
// be invoked by runner.go.
//
// Modes (FAKE_MODE):
//
//	"succeed"       (default) print two INFO lines to stdout, exit 0
//	"fail"          print an ERROR line to stderr, exit 1
//	"fail-once"     exit 1 on the first invocation, exit 0 on subsequent ones
//	                FAKE_COUNTER_FILE must be set to a writable file path used to
//	                track the invocation count across separate process executions
//	"stall"         sleep indefinitely — used to exercise timeout / kill paths
//	"print-version" emit a realistic ESPHome version log line, then exit 0
//	"compile-error" print ESPHome config-validation error output, exit 1
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	switch os.Getenv("FAKE_MODE") {

	case "fail":
		fmt.Fprintln(os.Stderr, "ERROR Connection refused")
		os.Exit(1)

	case "fail-once":
		counterFile := os.Getenv("FAKE_COUNTER_FILE")
		if counterFile == "" {
			fmt.Fprintln(os.Stderr, "fakeesphome: FAKE_COUNTER_FILE not set")
			os.Exit(2)
		}
		data, _ := os.ReadFile(counterFile)
		count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		count++
		_ = os.WriteFile(counterFile, []byte(strconv.Itoa(count)), 0644)
		if count == 1 {
			fmt.Fprintln(os.Stderr, "ERROR Connection refused (attempt 1)")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "INFO Upload successful")
		os.Exit(0)

	case "compile-error":
		// Mimic ESPHome config-validation failure output.
		fmt.Fprintln(os.Stdout, "Traceback (most recent call last):")
		fmt.Fprintln(os.Stdout, "  File \"esphome/__main__.py\", line 42, in run_config")
		fmt.Fprintln(os.Stderr, "Failed config")
		fmt.Fprintln(os.Stderr, "  device.yaml:")
		fmt.Fprintln(os.Stderr, "    Invalid YAML syntax: unexpected character at line 17, column 3")
		os.Exit(1)

	case "stall":
		// Sleep until killed — exercises the timeout + killGroup path in fetchVersion.
		time.Sleep(10 * time.Minute)
		os.Exit(0)

	case "print-version":
		// Emit a realistic device log line containing the ESPHome version string.
		fmt.Fprintln(os.Stdout, "[09:57:03.106][I][app:154]: ESPHome version 2026.4.3 compiled on May 2026")
		// Brief pause so the scanner goroutine has time to read the line before
		// we close the pipe on exit.
		time.Sleep(50 * time.Millisecond)
		os.Exit(0)

	default: // "succeed" or unrecognised
		fmt.Fprintln(os.Stdout, "INFO Starting")
		fmt.Fprintln(os.Stdout, "INFO Upload successful")
		os.Exit(0)
	}
}

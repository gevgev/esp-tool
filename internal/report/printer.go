package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/diagnostics"
	"github.com/ggevorgyan/esp-tool/internal/upgrader"
)

const (
	green  = "\033[1;32m"
	red    = "\033[1;31m"
	yellow = "\033[1;33m"
	cyan   = "\033[1;36m"
	dim    = "\033[2m"
	reset  = "\033[0m"
)

// PrintUpgradeSummary prints a colored summary table of upgrade results,
// matching the style of the original upgrade-esp-devices.sh output.
func PrintUpgradeSummary(results []upgrader.Result, elapsed time.Duration) {
	fmt.Println()
	fmt.Println("Upgrade ESPHome devices to the latest firmware version.")
	fmt.Println("Summary:")
	fmt.Println()

	// Find the longest device name for alignment
	maxLen := 0
	for _, r := range results {
		if len(r.Device.Name) > maxLen {
			maxLen = len(r.Device.Name)
		}
	}

	succeeded := 0
	failed := 0
	for _, r := range results {
		padding := strings.Repeat(" ", maxLen-len(r.Device.Name)+2)
		attempts := ""
		if r.Attempts > 1 {
			attempts = fmt.Sprintf(" (%d attempts)", r.Attempts)
		}
		if r.Success {
			succeeded++
			fmt.Printf("  - %s:%s%sUpgrade successful%s%s  [%s]\n",
				r.Device.Name, padding, green, reset, attempts, r.Duration.Round(time.Second))
		} else {
			failed++
			fmt.Printf("  - %s:%s%sUpgrade failed%s%s  [%s]\n",
				r.Device.Name, padding, red, reset, attempts, r.Duration.Round(time.Second))
		}
	}

	fmt.Println()
	if failed == 0 {
		fmt.Printf("%sAll %d devices upgraded successfully.%s\n", green, succeeded, reset)
	} else {
		fmt.Printf("%s%d succeeded%s, %s%d failed%s\n",
			green, succeeded, reset, red, failed, reset)
	}
	fmt.Printf("Elapsed time: %s\n", elapsed.Round(time.Second))
}

// PrintDiagnosticsSummary prints a per-device health table derived from the
// initial log dump collected by diagnostics.Check.
//
// Example output:
//
//	Device Health Diagnostics
//	══════════════════════════════════════════════════════
//	  Device                   Version     Health
//	  ────────────────────────────────────────────────────
//	  air-quality-external      v2026.4.1   ✗ Crash on previous boot — Reason: Hardware WDT
//	  bluetooth-proxy-2         v2026.4.3   ✓ Healthy
//	  espvibration1             v2026.4.3   ⚠ Chip rev ≥3.0 — set minimum_chip_revision: "3.0"
//	                                        ⚠ Bootloader too old — flash via USB once
//	  ────────────────────────────────────────────────────
//	  5 devices · 1 crash · 2 warnings · 1 healthy  [15s]
func PrintDiagnosticsSummary(results []diagnostics.Result, elapsed time.Duration) {
	const (
		indent   = "  "
		verWidth = 11 // "v2026.4.3" is 9 chars; pad to 11
	)

	// Column widths
	maxName := len("Device") // minimum = header width
	for _, r := range results {
		if len(r.Device.Name) > maxName {
			maxName = len(r.Device.Name)
		}
	}
	nameColW := maxName + 2 // name + 2 trailing spaces
	verColW := verWidth + 2  // version + 2 trailing spaces

	// issuePad is the blank prefix used to align continuation issue lines.
	issuePad := indent + strings.Repeat(" ", nameColW+verColW)

	sepWidth := nameColW + verColW + 50
	sep := strings.Repeat("─", sepWidth)
	dbl := strings.Repeat("═", sepWidth)

	fmt.Println()
	fmt.Printf("%sDevice Health Diagnostics%s\n", cyan, reset)
	fmt.Println(indent + dbl)

	// Header row
	namePad := strings.Repeat(" ", nameColW-len("Device"))
	verPad := strings.Repeat(" ", verColW-len("Version"))
	fmt.Printf("%s%sDevice%s%sVersion%s%sHealth%s\n",
		indent, dim, reset,
		namePad+dim, reset,
		verPad+dim, reset)
	fmt.Println(indent + sep)

	// Tally counters
	nCrash, nWarn, nHealthy, nUnreachable := 0, 0, 0, 0

	for _, r := range results {
		namePadding := strings.Repeat(" ", nameColW-len(r.Device.Name))

		// Unreachable device
		if r.Err != "" {
			nUnreachable++
			verSpacer := strings.Repeat(" ", verColW)
			fmt.Printf("%s%s%s%s%s✗ Unreachable%s\n",
				indent, r.Device.Name, namePadding, verSpacer, red, reset)
			continue
		}

		verStr := r.Version
		if verStr == "" {
			verStr = dim + "unknown" + reset
		}
		// verPadding accounts for ANSI escapes not taking display width
		rawVerLen := len(r.Version)
		if rawVerLen == 0 {
			rawVerLen = len("unknown")
		}
		verPadding := strings.Repeat(" ", verColW-rawVerLen)

		switch deviceCategory(r) {
		case "crash":
			nCrash++
		case "warning":
			nWarn++
		default:
			nHealthy++
		}

		if len(r.Issues) == 0 {
			fmt.Printf("%s%s%s%s%s%s✓ Healthy%s\n",
				indent, r.Device.Name, namePadding, verStr, verPadding, green, reset)
			continue
		}

		// First issue on the same line as the device name + version
		first := r.Issues[0]
		icon, color := issueDisplay(first.Level)
		fmt.Printf("%s%s%s%s%s%s%s %s%s\n",
			indent, r.Device.Name, namePadding, verStr, verPadding,
			color, icon, first.Message, reset)

		// Remaining issues indented under the first
		for _, issue := range r.Issues[1:] {
			icon, color := issueDisplay(issue.Level)
			fmt.Printf("%s%s%s %s%s\n", issuePad, color, icon, issue.Message, reset)
		}
	}

	fmt.Println(indent + sep)

	// Summary line
	parts := []string{fmt.Sprintf("%d devices", len(results))}
	if nCrash > 0 {
		parts = append(parts, fmt.Sprintf("%s%d crash%s", red, nCrash, reset))
	}
	if nWarn > 0 {
		parts = append(parts, fmt.Sprintf("%s%d warning%s", yellow, nWarn, reset))
	}
	if nHealthy > 0 {
		parts = append(parts, fmt.Sprintf("%s%d healthy%s", green, nHealthy, reset))
	}
	if nUnreachable > 0 {
		parts = append(parts, fmt.Sprintf("%s%d unreachable%s", dim, nUnreachable, reset))
	}
	fmt.Printf("%s%s  [%s]\n\n", indent, strings.Join(parts, " · "), elapsed.Round(time.Second))
}

// deviceCategory returns the worst health category for a result.
func deviceCategory(r diagnostics.Result) string {
	if r.Err != "" {
		return "unreachable"
	}
	for _, issue := range r.Issues {
		if issue.Level == diagnostics.LevelCrash {
			return "crash"
		}
	}
	if len(r.Issues) > 0 {
		return "warning"
	}
	return "healthy"
}

// issueDisplay returns the icon and ANSI color for an issue level.
func issueDisplay(level diagnostics.IssueLevel) (icon, color string) {
	if level == diagnostics.LevelCrash {
		return "✗", red
	}
	return "⚠", yellow
}

// PrintVersionSummary prints a colored summary of device firmware versions,
// matching the style of check-esp-versions.sh.
func PrintVersionSummary(results []upgrader.VersionResult, elapsed time.Duration) {
	fmt.Println()
	fmt.Println("ESPHome device firmware versions.")
	fmt.Println("Summary:")
	fmt.Println()

	maxLen := 0
	for _, r := range results {
		if len(r.Device.Name) > maxLen {
			maxLen = len(r.Device.Name)
		}
	}

	reachable := 0
	unreachable := 0
	for _, r := range results {
		padding := strings.Repeat(" ", maxLen-len(r.Device.Name)+2)
		if r.Version != "" {
			reachable++
			fmt.Printf("  - %s:%s%s%s%s\n", r.Device.Name, padding, green, r.Version, reset)
		} else {
			unreachable++
			fmt.Printf("  - %s:%s%sUnreachable%s\n", r.Device.Name, padding, red, reset)
		}
	}

	fmt.Println()
	if unreachable == 0 {
		fmt.Printf("%sAll %d devices reachable.%s\n", green, reachable, reset)
	} else {
		fmt.Printf("%s%d reachable%s, %s%d unreachable%s\n",
			green, reachable, reset, yellow, unreachable, reset)
	}
	fmt.Printf("Elapsed time: %s\n", elapsed.Round(time.Second))
}

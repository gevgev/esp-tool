package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/upgrader"
)

const (
	green  = "\033[1;32m"
	red    = "\033[1;31m"
	yellow = "\033[1;33m"
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

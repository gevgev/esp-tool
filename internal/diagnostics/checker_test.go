package diagnostics

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// extractDeviceVersion
// ---------------------------------------------------------------------------

func TestExtractDeviceVersion_TypicalLine(t *testing.T) {
	line := "[09:57:03.106][I][app:154]: ESPHome version 2026.4.3 compiled on Apr 13 2026"
	got := extractDeviceVersion(line)
	want := "v2026.4.3"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestExtractDeviceVersion_CLIBannerNotMatched(t *testing.T) {
	// The CLI banner says "INFO ESPHome 2026.5.1" — no "version " token.
	line := "INFO ESPHome 2026.5.1"
	got := extractDeviceVersion(line)
	if got != "" {
		t.Errorf("CLI banner should not be matched as device version, got %q", got)
	}
}

func TestExtractDeviceVersion_EmptyLine(t *testing.T) {
	if got := extractDeviceVersion(""); got != "" {
		t.Errorf("empty line should return empty, got %q", got)
	}
}

func TestExtractDeviceVersion_NoMarker(t *testing.T) {
	line := "[I][sensor:123]: Sensor update"
	if got := extractDeviceVersion(line); got != "" {
		t.Errorf("line without marker should return empty, got %q", got)
	}
}

func TestExtractDeviceVersion_TrailingNewline(t *testing.T) {
	line := "[I][app:154]: ESPHome version 2025.12.1\n"
	got := extractDeviceVersion(line)
	if got != "v2025.12.1" {
		t.Errorf("want %q, got %q", "v2025.12.1", got)
	}
}

// ---------------------------------------------------------------------------
// parseLogLines
// ---------------------------------------------------------------------------

func TestParseLogLines_NoIssues(t *testing.T) {
	lines := []string{
		"[09:57:03.106][I][app:154]: ESPHome version 2026.4.3 compiled on ...",
		"[I][wifi:123]: Connected to AP",
		"[I][sensor:456]: Sensor reading: 21.5",
	}
	version, issues := parseLogLines(lines)
	if version != "v2026.4.3" {
		t.Errorf("version: want %q, got %q", "v2026.4.3", version)
	}
	if len(issues) != 0 {
		t.Errorf("want 0 issues, got %d: %+v", len(issues), issues)
	}
}

func TestParseLogLines_CrashDetected(t *testing.T) {
	lines := []string{
		"[E][app:000]: CRASH DETECTED ON PREVIOUS BOOT",
		"[E][app:001]: Reason: Hardware WDT",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Code != "CRASH" {
		t.Errorf("code: want %q, got %q", "CRASH", issues[0].Code)
	}
	if !strings.Contains(issues[0].Message, "Hardware WDT") {
		t.Errorf("expected crash reason in message, got %q", issues[0].Message)
	}
	if issues[0].Level != LevelCrash {
		t.Errorf("level: want LevelCrash, got %v", issues[0].Level)
	}
}

func TestParseLogLines_CrashWithoutReason(t *testing.T) {
	lines := []string{
		"[E][app:000]: CRASH DETECTED ON PREVIOUS BOOT",
		"[I][wifi:123]: Connected",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 || issues[0].Code != "CRASH" {
		t.Fatalf("want 1 CRASH issue, got %+v", issues)
	}
	// Message should still be set even without a Reason: line
	if issues[0].Message == "" {
		t.Error("crash message should not be empty")
	}
}

func TestParseLogLines_BootloaderOld(t *testing.T) {
	lines := []string{
		"[W][esp32:123]: Bootloader too old for OTA rollback",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 || issues[0].Code != "BOOTLOADER_OLD" {
		t.Fatalf("want 1 BOOTLOADER_OLD issue, got %+v", issues)
	}
	if issues[0].Level != LevelWarning {
		t.Error("BOOTLOADER_OLD should be LevelWarning")
	}
}

func TestParseLogLines_Sram1Available(t *testing.T) {
	lines := []string{
		"[I][esp32:456]: Bootloader supports SRAM1 as IRAM",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 || issues[0].Code != "SRAM1_AVAILABLE" {
		t.Fatalf("want 1 SRAM1_AVAILABLE issue, got %+v", issues)
	}
}

func TestParseLogLines_ChipRevWarning(t *testing.T) {
	lines := []string{
		"[I][esp32:789]: Chip rev >= 3.0 detected",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 || issues[0].Code != "CHIP_REV" {
		t.Fatalf("want 1 CHIP_REV issue, got %+v", issues)
	}
}

func TestParseLogLines_StrappingPin(t *testing.T) {
	lines := []string{
		"WARNING: GPIO4 is a strapping PIN",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 || issues[0].Code != "STRAPPING_PIN" {
		t.Fatalf("want 1 STRAPPING_PIN issue, got %+v", issues)
	}
}

func TestParseLogLines_OtaMultiConfig(t *testing.T) {
	lines := []string{
		"WARNING Found and merged multiple configurations for ota platform",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 || issues[0].Code != "OTA_MULTI_CONFIG" {
		t.Fatalf("want 1 OTA_MULTI_CONFIG issue, got %+v", issues)
	}
}

func TestParseLogLines_NoDuplicateIssues(t *testing.T) {
	lines := []string{
		"[W][esp32:1]: Bootloader too old for OTA rollback",
		"[W][esp32:2]: Bootloader too old for OTA rollback",
		"[W][esp32:3]: Bootloader too old for OTA rollback",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 1 {
		t.Errorf("duplicate patterns should be deduplicated, got %d issues", len(issues))
	}
}

func TestParseLogLines_MultipleDistinctIssues(t *testing.T) {
	lines := []string{
		"[E][app:000]: CRASH DETECTED ON PREVIOUS BOOT",
		"[E][app:001]: Reason: Software WDT",
		"[W][esp32:100]: Bootloader too old for OTA rollback",
		"[I][esp32:200]: Chip rev >= 3.0 detected",
	}
	_, issues := parseLogLines(lines)
	if len(issues) != 3 {
		t.Errorf("want 3 issues, got %d: %+v", len(issues), issues)
	}
}

func TestParseLogLines_EmptyLines(t *testing.T) {
	version, issues := parseLogLines([]string{})
	if version != "" {
		t.Errorf("empty input: want empty version, got %q", version)
	}
	if len(issues) != 0 {
		t.Errorf("empty input: want 0 issues, got %d", len(issues))
	}
}

func TestParseLogLines_VersionExtractedOnce(t *testing.T) {
	lines := []string{
		"[I][app:154]: ESPHome version 2026.4.3 compiled on ...",
		"[I][app:154]: ESPHome version 2026.4.3 compiled on ...", // duplicate — still one version
	}
	version, _ := parseLogLines(lines)
	if version != "v2026.4.3" {
		t.Errorf("want %q, got %q", "v2026.4.3", version)
	}
}

package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// resolveSubstitutions
// ---------------------------------------------------------------------------

func TestResolveSubstitutions_NoSubs(t *testing.T) {
	got := resolveSubstitutions("my-device", nil)
	if got != "my-device" {
		t.Errorf("want %q, got %q", "my-device", got)
	}
}

func TestResolveSubstitutions_BraceStyle(t *testing.T) {
	subs := map[string]string{"device_name": "air-quality"}
	got := resolveSubstitutions("${device_name}-sensor", subs)
	want := "air-quality-sensor"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestResolveSubstitutions_DollarStyle(t *testing.T) {
	subs := map[string]string{"name": "proxy"}
	got := resolveSubstitutions("bt-$name", subs)
	want := "bt-proxy"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestResolveSubstitutions_MultipleVars(t *testing.T) {
	subs := map[string]string{"prefix": "esp", "num": "1"}
	got := resolveSubstitutions("${prefix}-device-${num}", subs)
	want := "esp-device-1"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestResolveSubstitutions_UnknownVarLeftAsIs(t *testing.T) {
	subs := map[string]string{"other": "x"}
	got := resolveSubstitutions("${unknown}", subs)
	if got != "${unknown}" {
		t.Errorf("unknown var should be left untouched, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// parseAPIKey
// ---------------------------------------------------------------------------

func TestParseAPIKey_InlineKey(t *testing.T) {
	yaml := []byte(`
esphome:
  name: my-device
api:
  encryption:
    key: "abc123=="
`)
	got := parseAPIKey(yaml)
	if got != "abc123==" {
		t.Errorf("want %q, got %q", "abc123==", got)
	}
}

func TestParseAPIKey_SecretTag_ReturnsEmpty(t *testing.T) {
	yaml := []byte(`
esphome:
  name: my-device
api:
  encryption:
    key: !secret api_encryption_key
`)
	got := parseAPIKey(yaml)
	if got != "" {
		t.Errorf("!secret key should return empty string, got %q", got)
	}
}

func TestParseAPIKey_NoAPISection_ReturnsEmpty(t *testing.T) {
	yaml := []byte(`
esphome:
  name: my-device
wifi:
  ssid: MyNetwork
`)
	got := parseAPIKey(yaml)
	if got != "" {
		t.Errorf("missing api section should return empty string, got %q", got)
	}
}

func TestParseAPIKey_NoEncryptionSection_ReturnsEmpty(t *testing.T) {
	yaml := []byte(`
esphome:
  name: my-device
api:
  port: 6053
`)
	got := parseAPIKey(yaml)
	if got != "" {
		t.Errorf("missing encryption section should return empty string, got %q", got)
	}
}

func TestParseAPIKey_InvalidYAML_ReturnsEmpty(t *testing.T) {
	got := parseAPIKey([]byte("not: valid: yaml: :::"))
	// Should not panic; invalid YAML returns empty
	_ = got
}

// ---------------------------------------------------------------------------
// parseDevice (via temp files)
// ---------------------------------------------------------------------------

func TestParseDevice_BasicDevice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-sensor.yaml")
	err := os.WriteFile(path, []byte(`
esphome:
  name: my-sensor
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := parseDevice(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.Name != "my-sensor" {
		t.Errorf("name: want %q, got %q", "my-sensor", dev.Name)
	}
	if dev.Host != "my-sensor.local" {
		t.Errorf("host: want %q, got %q", "my-sensor.local", dev.Host)
	}
	if dev.File != "my-sensor.yaml" {
		t.Errorf("file: want %q, got %q", "my-sensor.yaml", dev.File)
	}
}

func TestParseDevice_WithSubstitution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy.yaml")
	err := os.WriteFile(path, []byte(`
substitutions:
  device_name: bluetooth-proxy-2
esphome:
  name: ${device_name}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := parseDevice(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.Name != "bluetooth-proxy-2" {
		t.Errorf("name: want %q, got %q", "bluetooth-proxy-2", dev.Name)
	}
}

func TestParseDevice_MissingName_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-name.yaml")
	err := os.WriteFile(path, []byte(`
wifi:
  ssid: MyNetwork
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseDevice(path)
	if err == nil {
		t.Error("expected error for missing esphome.name, got nil")
	}
}

func TestParseDevice_UnresolvedSubstitution_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unresolved.yaml")
	err := os.WriteFile(path, []byte(`
esphome:
  name: ${undefined_var}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseDevice(path)
	if err == nil {
		t.Error("expected error for unresolved substitution, got nil")
	}
}

func TestParseDevice_InlineAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encrypted.yaml")
	err := os.WriteFile(path, []byte(`
esphome:
  name: encrypted-device
api:
  encryption:
    key: "abc123=="
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := parseDevice(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.APIKey != "abc123==" {
		t.Errorf("APIKey: want %q, got %q", "abc123==", dev.APIKey)
	}
}

// ---------------------------------------------------------------------------
// Scan
// ---------------------------------------------------------------------------

func TestScan_BasicDirectory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"device-a.yaml": "esphome:\n  name: device-a\n",
		"device-b.yaml": "esphome:\n  name: device-b\n",
		"secrets.yaml":  "api_key: secret123\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("want 2 devices (secrets.yaml excluded), got %d", len(devices))
	}
}

func TestScan_SkipsSecrets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secrets.yaml"), []byte("key: val\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.yaml"), []byte("esphome:\n  name: real\n"), 0644); err != nil {
		t.Fatal(err)
	}

	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, d := range devices {
		if d.Name == "secrets" || d.File == "secrets.yaml" {
			t.Error("secrets.yaml should not appear in scan results")
		}
	}
}

func TestScan_SkipsUnparseable(t *testing.T) {
	dir := t.TempDir()
	// A valid device
	if err := os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("esphome:\n  name: good\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// A YAML without esphome.name — should be silently skipped
	if err := os.WriteFile(filepath.Join(dir, "helper.yaml"), []byte("packages:\n  common: !include common.yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}

	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "good" {
		t.Errorf("want exactly 1 device named 'good', got %+v", devices)
	}
}

func TestScan_EmptyDirectory_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := Scan(dir)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestScan_ParseFailure_ErrorListsSkippedFiles(t *testing.T) {
	// When YAML files exist but none parse as devices, the error should name
	// each skipped file and its reason — not just say "no files found".
	dir := t.TempDir()
	for _, name := range []string{"no-name.yaml", "also-bad.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("wifi:\n  ssid: x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Scan(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no-name.yaml") || !strings.Contains(msg, "also-bad.yaml") {
		t.Errorf("error should name skipped files, got: %s", msg)
	}
	if !strings.Contains(msg, "esphome.name") {
		t.Errorf("error should mention esphome.name field, got: %s", msg)
	}
}

func TestScan_OnlySecretsFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secrets.yaml"), []byte("key: val\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Scan(dir)
	if err == nil {
		t.Error("expected error when only secrets.yaml is present, got nil")
	}
}

// ---------------------------------------------------------------------------
// Case-insensitive extension tests (Windows compat — Notepad saves as .YAML)
// ---------------------------------------------------------------------------

func TestScan_LowercaseExtension(t *testing.T) {
	// Baseline: .yaml (lowercase) must always work on every platform.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "device-a.yaml"), []byte("esphome:\n  name: device-a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan should find .yaml files: %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "device-a" {
		t.Errorf("want 1 device 'device-a', got %+v", devices)
	}
}

func TestScan_UppercaseExtension(t *testing.T) {
	// .YAML (uppercase) — what Notepad on Windows produces by default.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "device-a.YAML"), []byte("esphome:\n  name: device-a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan should find .YAML files: %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "device-a" {
		t.Errorf("want 1 device 'device-a', got %+v", devices)
	}
}

func TestScan_MixedCaseExtension(t *testing.T) {
	// .Yaml — uncommon but valid; should be treated the same.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "device-a.Yaml"), []byte("esphome:\n  name: device-a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan should find .Yaml files: %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "device-a" {
		t.Errorf("want 1 device 'device-a', got %+v", devices)
	}
}

func TestScan_MixedExtensionsInSameDir(t *testing.T) {
	// A directory containing both .yaml and .YAML files — all should be found.
	dir := t.TempDir()
	files := map[string]string{
		"alpha.yaml": "esphome:\n  name: alpha\n",
		"beta.YAML":  "esphome:\n  name: beta\n",
		"gamma.Yaml": "esphome:\n  name: gamma\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 3 {
		t.Errorf("want 3 devices (all extensions), got %d: %+v", len(devices), devices)
	}
}

func TestScan_SkipsSecrets_UppercaseFilename(t *testing.T) {
	// SECRETS.YAML should be skipped just like secrets.yaml.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SECRETS.YAML"), []byte("api_key: secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real-device.YAML"), []byte("esphome:\n  name: real-device\n"), 0644); err != nil {
		t.Fatal(err)
	}
	devices, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, d := range devices {
		if d.File == "SECRETS.YAML" {
			t.Error("SECRETS.YAML should be skipped, but was returned as a device")
		}
	}
	if len(devices) != 1 || devices[0].Name != "real-device" {
		t.Errorf("want exactly 1 device 'real-device', got %+v", devices)
	}
}

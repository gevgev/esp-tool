package syncer

import (
	"encoding/json"
	"testing"
)

const sampleState = `{
  "air-quality-internal.yaml": {
    "deployed_config_hash": "3f5e9760",
    "expected_config_hash": "e720f69a",
    "deployed_version": "2026.6.0",
    "ip": "192.168.1.50"
  },
  "lux-living-christmas.yaml": {
    "deployed_config_hash": "abc123",
    "expected_config_hash": "abc123",
    "deployed_version": "2026.6.0",
    "ip": "192.168.1.51"
  }
}`

func TestPatch(t *testing.T) {
	out, res, err := Patch([]byte(sampleState), []string{
		"air-quality-internal.yaml", // needs patching
		"lux-living-christmas.yaml", // already in sync
		"missing.yaml",              // not in state file
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}

	if len(res.Patched) != 1 || res.Patched[0] != "air-quality-internal.yaml" {
		t.Fatalf("Patched = %v, want [air-quality-internal.yaml]", res.Patched)
	}
	if reason := res.Skipped["lux-living-christmas.yaml"]; reason != "already in sync" {
		t.Errorf("lux-living-christmas.yaml skip reason = %q, want %q", reason, "already in sync")
	}
	if reason := res.Skipped["missing.yaml"]; reason != "not present in device-builder state file" {
		t.Errorf("missing.yaml skip reason = %q, want %q", reason, "not present in device-builder state file")
	}

	var top map[string]map[string]any
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatalf("re-parse patched output: %v", err)
	}

	got := top["air-quality-internal.yaml"]["expected_config_hash"]
	if got != "3f5e9760" {
		t.Errorf("expected_config_hash = %v, want 3f5e9760", got)
	}
	// Unrelated fields must survive untouched.
	if got := top["air-quality-internal.yaml"]["ip"]; got != "192.168.1.50" {
		t.Errorf("ip field altered: got %v", got)
	}
	if got := top["lux-living-christmas.yaml"]["expected_config_hash"]; got != "abc123" {
		t.Errorf("already-in-sync entry changed: got %v", got)
	}
}

func TestPatchMissingDeployedHash(t *testing.T) {
	state := `{"foo.yaml": {"expected_config_hash": "x"}}`
	_, res, err := Patch([]byte(state), []string{"foo.yaml"})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if reason := res.Skipped["foo.yaml"]; reason != "missing deployed_config_hash" {
		t.Errorf("skip reason = %q, want %q", reason, "missing deployed_config_hash")
	}
}

func TestPatchInvalidJSON(t *testing.T) {
	if _, _, err := Patch([]byte("not json"), []string{"foo.yaml"}); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

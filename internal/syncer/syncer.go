// Package syncer patches the ESPHome Device Builder add-on's
// .device-builder-devices.json state file so that devices flashed externally
// via esp-tool no longer show an "out of sync" indicator in the Builder UI.
//
// The Builder tracks two hashes per device: deployed_config_hash (read from
// the running firmware's mDNS announce — already updated automatically after
// an esp-tool flash) and expected_config_hash (read from a Builder-initiated
// compile's build_info.json — never updated by esp-tool, since esp-tool's
// build artifacts never touch the add-on container). Patch sets the latter
// to match the former for each device esp-tool just flashed successfully.
package syncer

import (
	"encoding/json"
	"fmt"
)

// PatchResult reports what Patch did to each requested device file.
type PatchResult struct {
	// Patched lists device YAML filenames whose expected_config_hash was updated.
	Patched []string
	// Skipped maps device YAML filenames to the reason they were not patched
	// (not present in the state file, missing hash, or already in sync).
	Skipped map[string]string
}

// Patch reads the Builder's JSON state (keyed by device YAML filename) and,
// for each name in files, sets expected_config_hash = deployed_config_hash.
// Unknown fields and entries for devices not in files are preserved as-is.
// It returns the rewritten JSON document plus a report of what changed.
func Patch(data []byte, files []string) ([]byte, PatchResult, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, PatchResult{}, fmt.Errorf("parse device-builder state: %w", err)
	}

	res := PatchResult{Skipped: make(map[string]string)}
	for _, f := range files {
		raw, ok := top[f]
		if !ok {
			res.Skipped[f] = "not present in device-builder state file"
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			res.Skipped[f] = fmt.Sprintf("malformed entry: %v", err)
			continue
		}

		deployed, ok := entry["deployed_config_hash"].(string)
		if !ok || deployed == "" {
			res.Skipped[f] = "missing deployed_config_hash"
			continue
		}
		if expected, ok := entry["expected_config_hash"].(string); ok && expected == deployed {
			res.Skipped[f] = "already in sync"
			continue
		}

		entry["expected_config_hash"] = deployed
		newRaw, err := json.Marshal(entry)
		if err != nil {
			res.Skipped[f] = fmt.Sprintf("re-marshal failed: %v", err)
			continue
		}
		top[f] = newRaw
		res.Patched = append(res.Patched, f)
	}

	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, PatchResult{}, fmt.Errorf("marshal patched state: %w", err)
	}
	return out, res, nil
}

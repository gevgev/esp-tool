package upgrader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lastRunFile = ".esp-tool-last-run.json"

// LastRun records the outcome of the most recent upgrade invocation so that
// --retry-failed can filter to just the devices that failed.
type LastRun struct {
	// Time is when the run completed (UTC).
	Time time.Time `json:"time"`
	// Dir is the ESPHome YAML directory that was targeted.
	Dir string `json:"dir"`
	// Failed holds the names of devices that did not succeed.
	Failed []string `json:"failed"`
	// Total is the total number of devices that were attempted.
	Total int `json:"total"`
}

// LastRunPath returns the absolute path to the last-run state file stored in dir.
func LastRunPath(dir string) string {
	return filepath.Join(dir, lastRunFile)
}

// SaveLastRun writes a summary of results to <dir>/.esp-tool-last-run.json.
// It is called after every upgrade run (plain and TUI modes alike).
func SaveLastRun(dir string, results []Result) error {
	var failed []string
	for _, r := range results {
		if !r.Success {
			failed = append(failed, r.Device.Name)
		}
	}

	lr := LastRun{
		Time:   time.Now().UTC(),
		Dir:    dir,
		Failed: failed,
		Total:  len(results),
	}

	data, err := json.MarshalIndent(lr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal last-run: %w", err)
	}

	path := LastRunPath(dir)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write last-run file %s: %w", path, err)
	}
	return nil
}

// LoadLastRun reads <dir>/.esp-tool-last-run.json and returns the decoded
// LastRun.  Returns an error if the file does not exist or cannot be parsed.
func LoadLastRun(dir string) (*LastRun, error) {
	path := LastRunPath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no previous run found in %s (run 'esp-tool upgrade' first)", dir)
		}
		return nil, fmt.Errorf("read last-run file: %w", err)
	}

	var lr LastRun
	if err := json.Unmarshal(data, &lr); err != nil {
		return nil, fmt.Errorf("parse last-run file %s: %w", path, err)
	}
	return &lr, nil
}

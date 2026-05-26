package upgrader_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/upgrader"
)

func dev(name string) discovery.Device { return discovery.Device{Name: name} }

func TestSaveAndLoadLastRun_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	results := []upgrader.Result{
		{Device: dev("alpha"), Success: true},
		{Device: dev("beta"), Success: false},
		{Device: dev("gamma"), Success: true},
		{Device: dev("delta"), Success: false},
	}

	if err := upgrader.SaveLastRun(dir, results); err != nil {
		t.Fatalf("SaveLastRun: %v", err)
	}

	lr, err := upgrader.LoadLastRun(dir)
	if err != nil {
		t.Fatalf("LoadLastRun: %v", err)
	}

	if lr.Total != 4 {
		t.Errorf("Total: got %d, want 4", lr.Total)
	}
	if len(lr.Failed) != 2 {
		t.Errorf("Failed count: got %d, want 2", len(lr.Failed))
	}
	failed := map[string]bool{}
	for _, f := range lr.Failed {
		failed[f] = true
	}
	if !failed["beta"] {
		t.Error("expected 'beta' in failed list")
	}
	if !failed["delta"] {
		t.Error("expected 'delta' in failed list")
	}
	if failed["alpha"] {
		t.Error("'alpha' should NOT be in failed list")
	}
	if lr.Dir != dir {
		t.Errorf("Dir: got %q, want %q", lr.Dir, dir)
	}
	if lr.Time.IsZero() {
		t.Error("Time should not be zero")
	}
}

func TestSaveLastRun_AllSuccess_EmptyFailedList(t *testing.T) {
	dir := t.TempDir()
	results := []upgrader.Result{
		{Device: dev("alpha"), Success: true},
		{Device: dev("beta"), Success: true},
	}
	if err := upgrader.SaveLastRun(dir, results); err != nil {
		t.Fatalf("SaveLastRun: %v", err)
	}
	lr, err := upgrader.LoadLastRun(dir)
	if err != nil {
		t.Fatalf("LoadLastRun: %v", err)
	}
	if len(lr.Failed) != 0 {
		t.Errorf("Failed: got %v, want empty", lr.Failed)
	}
}

func TestLoadLastRun_NoFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := upgrader.LoadLastRun(dir)
	if err == nil {
		t.Fatal("expected error when no last-run file exists")
	}
}

func TestLoadLastRun_CorruptFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".esp-tool-last-run.json")
	if err := os.WriteFile(path, []byte("{not valid json}"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := upgrader.LoadLastRun(dir)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestSaveLastRun_OverwritesPreviousFile(t *testing.T) {
	dir := t.TempDir()

	// First run: one failure
	first := []upgrader.Result{
		{Device: dev("alpha"), Success: false},
		{Device: dev("beta"), Success: true},
	}
	if err := upgrader.SaveLastRun(dir, first); err != nil {
		t.Fatalf("first SaveLastRun: %v", err)
	}

	// Give it a moment so times differ
	time.Sleep(time.Millisecond)

	// Second run: different failure
	second := []upgrader.Result{
		{Device: dev("alpha"), Success: true},
		{Device: dev("beta"), Success: false},
	}
	if err := upgrader.SaveLastRun(dir, second); err != nil {
		t.Fatalf("second SaveLastRun: %v", err)
	}

	lr, err := upgrader.LoadLastRun(dir)
	if err != nil {
		t.Fatalf("LoadLastRun: %v", err)
	}
	if len(lr.Failed) != 1 || lr.Failed[0] != "beta" {
		t.Errorf("expected [beta] failed after second run, got %v", lr.Failed)
	}
}

func TestLastRunPath(t *testing.T) {
	got := upgrader.LastRunPath("/some/dir")
	want := "/some/dir/.esp-tool-last-run.json"
	if got != want {
		t.Errorf("LastRunPath: got %q, want %q", got, want)
	}
}

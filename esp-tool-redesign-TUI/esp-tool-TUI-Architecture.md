# esp-tool TUI Mode — Architecture & Redesign Strategy

**Version:** 1.1  
**Date:** 2026-05-25  
**Status:** Decisions resolved — ready for implementation  
**Scope:** Architecture/Design + Testing phases

---

## Revision History

| Version | Change |
|---------|--------|
| 1.0 | Initial draft with open architecture questions |
| 1.1 | Phase 1 scoped to `upgrade` only; per-device full buffer design for error parsing + log file; exit-code-driven error detection; stay-in-TUI-on-error behavior; updated phase plan and file structure |

---

## Resolved Architecture Decisions

| # | Question | Decision |
|---|----------|----------|
| 1 | OutputWriter interface | Rich semantic interface with `DeviceCompleted`, `DeviceRetry`, etc. |
| 2 | Phase 1 scope | `upgrade` command TUI only; `versions` and `diagnostics` deferred to Phase 2 |
| 3 | Error detection | Exit-code driven: full per-device buffer held during run, parsed only on non-zero exit, discarded on success |
| 4 | Full buffer / log file relationship | Same data, independent mechanisms: log file streams to disk as lines arrive; full buffer is in-memory only, GC'd after device completes |
| 5 | Summary reprint strategy | TUI exits (or user presses `q` on failure), then `report.PrintUpgradeSummary()` runs — identical to today |

---

## 1. Current Architecture Analysis

### What exists today

The current output pipeline is minimal: goroutines call `runStreaming()` which calls `fmt.Printf` directly to stdout. No interception layer, no buffering, no per-device output ownership. The structural change required to add TUI is localized to this output path.

**Files and their TUI impact:**

| File | Role | Phase 1 Change |
|------|------|----------------|
| `cmd/esp-tool/main.go` | CLI entry point | Add `--plain`, `--log-file` flags; add TUI/plain routing for `upgrade` |
| `internal/upgrader/runner.go` | Goroutine pool, retry logic, `runStreaming()` | Replace `fmt.Printf` with `OutputWriter` parameter |
| `internal/diagnostics/checker.go` | Log collection goroutines | **Unchanged in Phase 1** |
| `internal/report/printer.go` | Final summary, ANSI colors | **Unchanged** |
| `internal/discovery/scanner.go` | YAML parsing, device list | **Unchanged** |

### The core problem

`runStreaming()` writes to stdout like this today:

```go
for scanner.Scan() {
    line := scanner.Text()
    if prefix {
        fmt.Printf("[%s] %s\n", name, line)
    } else {
        fmt.Println(line)
    }
}
```

In TUI mode this must instead route into per-device buffers that the TUI model reads and renders. In log file mode it must additionally stream to disk. In both cases the goroutines must not touch stdout directly. The fix is replacing the `fmt.Printf` call with a writer abstraction.

---

## 2. TUI Library: bubbletea (Charm)

**Chosen:** `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/lipgloss`

**Rationale:** The Elm-architecture model (Model/Update/View) matches the pattern exactly: shared immutable state, concurrent goroutines pushing events via `prog.Send()`, a single-threaded event loop handling updates and rendering. The `tea.Cmd` / `tea.Msg` mechanism eliminates the need for locks in the view layer. `lipgloss` handles panel borders, alignment, and color without manual ANSI arithmetic.

**Key primitive:** `tea.Program.Send(msg)` is goroutine-safe. Device goroutines call `prog.Send(DeviceOutputMsg{...})` to push lines into the TUI event loop, which processes them one at a time. No mutexes needed in the Model itself.

**New Go module dependencies:**

```
github.com/charmbracelet/bubbletea   v0.27+
github.com/charmbracelet/lipgloss    v1.0+
github.com/charmbracelet/bubbles     v0.19+   (viewport for scrollable output pane)
golang.org/x/term                             (TTY detection, terminal size)
```

Note: `github.com/muesli/termenv` is pulled in transitively by lipgloss. `github.com/mattn/go-isatty` is pulled in by termenv. No direct dependency additions needed for TTY detection.

---

## 3. Key Architectural Changes

### 3.1 New package: `internal/output`

Contains the `OutputWriter` interface and its two implementations. This is the only place the rest of the codebase needs to care about.

```go
// internal/output/writer.go

// OutputWriter is the abstraction between execution engine and display layer.
// PlainWriter writes to stdout + optionally a log file.
// TUIWriter sends tea.Msgs to the bubbletea program + optionally a log file.
type OutputWriter interface {
    // WriteLine is called for every line of output from a device process.
    WriteLine(device, line string)
    // DeviceStarted is called when a device's goroutine acquires the semaphore.
    DeviceStarted(device string)
    // DeviceCompleted is called when a device finishes (success or failure).
    DeviceCompleted(device string, success bool, attempts int, duration time.Duration, errLines []string)
    // DeviceRetrying is called before each retry sleep.
    DeviceRetrying(device string, attempt, maxAttempts int, delay time.Duration)
}
```

**Why `errLines []string` in `DeviceCompleted`?** The runner holds the full buffer during execution and passes the parsed error lines on completion. The OutputWriter does not need to do its own parsing — it only receives the already-extracted error snippets. This keeps the error parsing logic in one place inside the runner.

### 3.2 Per-Device Full Buffer (critical design)

`runner.go` maintains two data structures per device, managed entirely inside `runWithRetry`:

```go
type deviceBuffers struct {
    display *RingBuffer   // bounded ~500 lines; sent to TUI output panel
    full    []string      // unbounded; held for error parsing; GC'd after device completes
    logFile io.Writer     // non-nil if --log-file is set; lines written as they arrive
}
```

**Line arrival path:**
```go
for scanner.Scan() {
    line := scanner.Text()
    bufs.display.Push(line)              // always: update display ring buffer
    bufs.full = append(bufs.full, line) // always: accumulate full buffer
    if bufs.logFile != nil {
        fmt.Fprintln(bufs.logFile, "["+name+"] "+line) // always if log file set: stream to disk
    }
    writer.WriteLine(name, line)         // always: route to TUI or plain output
}
```

**On device success (exit code 0):**
```go
bufs.full = nil  // discard immediately — GC eligible
writer.DeviceCompleted(name, true, attempts, elapsed, nil)
```

**On device failure (non-zero exit code):**
```go
errLines := parseErrors(bufs.full)  // extract error snippets
bufs.full = nil                     // discard full buffer regardless
writer.DeviceCompleted(name, false, attempts, elapsed, errLines)
```

This means the full buffer's maximum lifetime is one device's execution duration. On a 4-minute upgrade with 4 concurrent devices, at most 4 full buffers exist simultaneously — a bounded and predictable memory cost.

### 3.3 Error Parsing

```go
// internal/upgrader/errors.go

var errorKeywords = []string{
    "ERROR", "FAILED", "Exception", "Traceback",
    "refused", "timeout", "unreachable", "CRIT",
}

// parseErrors scans lines for known error patterns and returns
// up to 4 snippets (each up to 2 lines: the matching line + the next).
func parseErrors(lines []string) []string {
    var snippets []string
    for i, line := range lines {
        for _, kw := range errorKeywords {
            if strings.Contains(line, kw) {
                snippet := line
                if i+1 < len(lines) && lines[i+1] != "" {
                    snippet += "\n" + lines[i+1]
                }
                snippets = append(snippets, snippet)
                break
            }
        }
        if len(snippets) >= 4 {
            break
        }
    }
    if len(snippets) == 0 {
        return []string{"Process exited with error (no parseable error line found)"}
    }
    return snippets
}
```

**Why exit-code driven, not live keyword scanning:** ESPHome's verbose output contains many INFO lines that include words like "timeout" in context that is not an error (e.g., `"waiting for result timeout 300s"`). Scanning live would flood the Errors panel with false positives. Waiting for the exit code and then scanning the full buffer gives a clean, accurate signal. The tradeoff is that errors appear in the panel only after a device attempt finishes, not during — which is acceptable given that the Output Tail panel shows all live output continuously.

### 3.4 Log File Implementation

The log file is opened in `main.go` before the upgrade starts, passed into `RunOptions`, and the runner's scanner goroutine writes to it.

```go
// internal/upgrader/runner.go

type RunOptions struct {
    // ... existing fields ...
    LogFile io.Writer  // non-nil if --log-file is set; writes happen in scanner goroutines
}
```

The log file writer is thread-safe because:
- Each goroutine calls `fmt.Fprintln(opts.LogFile, ...)` which goes to an `os.File`
- `os.File.Write()` on POSIX is atomic for small writes (< PIPE_BUF) — interleaved lines from different goroutines are possible but each individual line write is atomic
- For strict line ordering, wrap `os.File` in a `sync.Mutex`-protected writer — a one-line change

**Flag wiring in `main.go`:**
```go
var logFilePath string
cmd.Flags().StringVar(&logFilePath, "log-file", "", "Write full device output to file")

// In RunE:
var logWriter io.Writer
if logFilePath != "" {
    f, err := os.Create(logFilePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "warning: cannot open log file %s: %v\n", logFilePath, err)
    } else {
        defer f.Close()
        logWriter = f
    }
}
opts := upgrader.RunOptions{..., LogFile: logWriter}
```

### 3.5 TUI Writer

```go
// internal/tui/writer.go

type TUIWriter struct {
    prog *tea.Program
}

func (w *TUIWriter) WriteLine(device, line string) {
    w.prog.Send(DeviceOutputMsg{Device: device, Line: line})
}

func (w *TUIWriter) DeviceStarted(device string) {
    w.prog.Send(DeviceStatusMsg{Device: device, Status: StatusRunning})
}

func (w *TUIWriter) DeviceCompleted(device string, success bool, attempts int, d time.Duration, errLines []string) {
    status := StatusSuccess
    if !success { status = StatusFailed }
    w.prog.Send(DeviceStatusMsg{
        Device: device, Status: status,
        Attempts: attempts, Duration: d, ErrLines: errLines,
    })
}

func (w *TUIWriter) DeviceRetrying(device string, attempt, max int, delay time.Duration) {
    w.prog.Send(DeviceRetryMsg{Device: device, Attempt: attempt, Max: max, Delay: delay})
}
```

### 3.6 Plain Writer

```go
// internal/output/plain.go

type PlainWriter struct {
    prefix bool
}

func (w *PlainWriter) WriteLine(device, line string) {
    if w.prefix {
        fmt.Printf("[%s] %s\n", device, line)
    } else {
        fmt.Println(line)
    }
}

func (w *PlainWriter) DeviceStarted(device string) {} // no-op in plain mode

func (w *PlainWriter) DeviceCompleted(device string, success bool, attempts int, d time.Duration, errLines []string) {
    // Print retry/completion messages (current behavior from runWithRetry):
    // e.g. "[device] retrying (attempt 2/3) after 5s..."
    // These were previously inline fmt.Printf calls in runWithRetry — moved here
}

func (w *PlainWriter) DeviceRetrying(device string, attempt, max int, delay time.Duration) {
    fmt.Printf("[%s] retrying (attempt %d/%d) after %s...\n", device, attempt, max, delay)
}
```

### 3.7 TUI Model

```go
// internal/tui/model.go

type Model struct {
    // Static config (set at init, never changes)
    Devices   []string   // ordered device names from discovery
    Jobs      int        // --jobs
    Retries   int        // --retries
    DryRun    bool
    LogFile   string     // path, for header badge display

    // Dynamic state (updated via tea.Msg in Update())
    States      map[string]DeviceState     // per-device status
    DisplayBufs map[string]*RingBuffer     // per-device display ring buffers
    GlobalTail  []TailLine                 // last N lines across all devices (for output panel)
    Errors      []ErrorEntry               // last 8 error entries for Errors panel
    Elapsed     time.Duration
    Done        bool
    HasFailures bool

    // Layout (updated by TermSizeMsg)
    TermWidth  int
    TermHeight int

    // UI state
    Tick          int            // spinner animation frame
    DeviceScroll  int            // device list scroll offset
    AutoScroll    bool           // re-centers list on active devices
    OutputFocused bool           // Tab toggles output pane focus
    OutputScroll  int            // output pane scroll offset
}

type DeviceState struct {
    Status    Status
    Attempts  int
    StartedAt time.Time
    Duration  time.Duration    // final duration when completed
    LastLine  string           // last output line (shown in Active Jobs panel)
    ErrLines  []string         // parsed error snippets (populated on failure)
}
```

### 3.8 Execution Flow in TUI Mode (upgrade)

```
upgradeCmd.RunE()
  │
  ├── detect TUI: ShouldUseTUI(plain, termWidth, termHeight)
  │
  ├── open log file if --log-file set → logWriter io.Writer
  │
  ├── [TUI mode]:
  │     model = tui.NewModel(devices, opts)
  │     prog = tea.NewProgram(model, tea.WithAltScreen())
  │     tuiWriter = &tui.TUIWriter{prog: prog}
  │     // inject logWriter into tuiWriter if set
  │     go func() {
  │         results = upgrader.Upgrade(devices, opts, tuiWriter)
  │         prog.Send(tui.AllDoneMsg{Results: results, Elapsed: elapsed})
  │     }()
  │     prog.Run()  // blocks until model calls tea.Quit
  │     // AllDoneMsg in Update():
  │     //   success → tea.Quit immediately
  │     //   failure → set Done+HasFailures, wait for 'q' keypress → tea.Quit
  │     report.PrintUpgradeSummary(results, elapsed)  // printed after TUI exits
  │
  └── [Plain mode]:
        plainWriter = &output.PlainWriter{prefix: logPrefix, logFile: logWriter}
        results = upgrader.Upgrade(devices, opts, plainWriter)
        report.PrintUpgradeSummary(results, elapsed)
```

### 3.9 TTY Detection

```go
// internal/output/detect.go

func ShouldUseTUI(plain bool) (bool, int, int) {
    if plain {
        return false, 0, 0
    }
    fd := int(os.Stdout.Fd())
    if !term.IsTerminal(fd) {
        return false, 0, 0
    }
    w, h, err := term.GetSize(fd)
    if err != nil || w < 80 || h < 24 {
        return false, w, h
    }
    return true, w, h
}
```

Returns the terminal dimensions so `main.go` can pass them into the initial model without a second syscall.

---

## 4. New File Structure (Phase 1 only)

```
esp-tool/
├── cmd/esp-tool/main.go                  MODIFIED: --plain, --log-file flags; TUI routing for upgrade
├── internal/
│   ├── output/
│   │   ├── writer.go                     NEW: OutputWriter interface
│   │   ├── plain.go                      NEW: PlainWriter (wraps current fmt.Printf behavior exactly)
│   │   ├── plain_test.go                 NEW
│   │   ├── detect.go                     NEW: TTY + terminal size detection
│   │   └── detect_test.go                NEW
│   ├── tui/
│   │   ├── model.go                      NEW: bubbletea Model (Init, Update, View)
│   │   ├── model_test.go                 NEW
│   │   ├── messages.go                   NEW: all tea.Msg types
│   │   ├── writer.go                     NEW: TUIWriter (sends tea.Msgs)
│   │   ├── writer_test.go                NEW
│   │   ├── ringbuffer.go                 NEW: thread-safe bounded ring buffer
│   │   ├── ringbuffer_test.go            NEW
│   │   ├── layout.go                     NEW: panel dimension math
│   │   └── panels/
│   │       ├── header.go                 NEW
│   │       ├── header_test.go            NEW
│   │       ├── devices.go                NEW
│   │       ├── devices_test.go           NEW
│   │       ├── active.go                 NEW
│   │       ├── active_test.go            NEW
│   │       ├── errors.go                 NEW
│   │       ├── errors_test.go            NEW
│   │       └── output.go                 NEW
│   ├── upgrader/
│   │   ├── runner.go                     MODIFIED: inject OutputWriter; add deviceBuffers; move error parsing here
│   │   ├── errors.go                     NEW: parseErrors() function
│   │   ├── errors_test.go                NEW: table-driven tests for parseErrors
│   │   └── runner_test.go                MODIFIED: add capture-writer tests; existing tests unchanged
│   ├── diagnostics/
│   │   └── checker.go                    UNCHANGED (Phase 1)
│   ├── discovery/
│   │   └── scanner.go                    UNCHANGED
│   └── report/
│       └── printer.go                    UNCHANGED
├── go.mod                                +bubbletea, +lipgloss, +bubbles, +term
└── README.md                             +--plain, +--log-file documentation
```

**Phase 2 additions** (not in scope now):
- `internal/tui/panels/issues.go` (diagnostics Issues panel)
- `internal/diagnostics/checker.go` — OutputWriter injection
- TUI routing in `versionsCmd.RunE` and `diagnosticsCmd.RunE`

---

## 5. Test-Driven Development Strategy

### 5.1 Mandate

No production code changes until:
1. A coverage baseline is established and documented
2. Tests for new functionality are written and confirmed failing
3. Production code is written to make them pass

`go test -race ./...` must pass at every commit. This is the primary guard against the most dangerous failure mode: goroutine races in the output pipeline.

### 5.2 Phase 1 — Coverage Baseline (no production changes)

```bash
go test ./... -coverprofile=baseline.out -covermode=atomic
go tool cover -func=baseline.out | grep -v "100.0%"
```

**Known gaps to fill before any TUI code:**

| Package | Function | Gap | Test Strategy |
|---------|----------|-----|---------------|
| `upgrader` | `runWithRetry` retry path | Not tested (requires real `esphome`) | Fake `esphome` binary via `os.Setenv("PATH", ...)` with a script that exits 1 on first call, 0 on second |
| `upgrader` | `fetchVersion` timeout path | Not tested | Stall subprocess: `sleep 30` script |
| `upgrader` | `runStreaming` with `prefix=false` | Not exercised | DryRun path skips `runStreaming` entirely; add integration test |
| `diagnostics` | `parseLogLines` | Minimal coverage | Fixture-based table tests with realistic ESPHome log excerpts |
| `diagnostics` | `rebootDevice` | Untested | Mock `python3` in PATH |
| `discovery` | `parseAPIKey` with `!secret` tag | Partially tested | Add fixture YAML files in `testdata/` |
| `main` | `loadDevices` with empty result after filter | Untested | Temp dir with a single YAML, filter for nonexistent device |

**Golden file regression test (critical — write this first):**

```bash
# test/golden/upgrade_dry_run.txt
esp-tool upgrade --plain --dry-run --dir testdata/devices
```

Check this file into the repo. Any CI run that changes the output of `--plain` will break this test immediately.

### 5.3 Phase 2 — `output` Package (test-first)

Write all tests in `internal/output/` before writing the implementation:

```go
// internal/output/plain_test.go
func TestPlainWriter_WithPrefix_FormatsCorrectly(t *testing.T)
func TestPlainWriter_WithoutPrefix_PassesThrough(t *testing.T)
func TestPlainWriter_DeviceRetrying_PrintsMessage(t *testing.T)
func TestPlainWriter_DeviceCompleted_Success_Silent(t *testing.T)   // no output on success
func TestPlainWriter_DeviceCompleted_Failure_PrintsErrLines(t *testing.T)

// internal/output/detect_test.go
func TestShouldUseTUI_PlainFlag_ReturnsFalse(t *testing.T)
func TestShouldUseTUI_NotATerminal_ReturnsFalse(t *testing.T)
// Positive TTY case requires a real PTY; skip in CI with t.Skip("requires TTY")
```

### 5.4 Phase 3 — `upgrader/errors.go` (test-first)

```go
// internal/upgrader/errors_test.go
func TestParseErrors_FindsErrorKeyword(t *testing.T)
func TestParseErrors_IncludesFollowingLine(t *testing.T)
func TestParseErrors_CapsAtFourSnippets(t *testing.T)
func TestParseErrors_NoMatch_ReturnsFallback(t *testing.T)
func TestParseErrors_EmptyInput_ReturnsFallback(t *testing.T)
func TestParseErrors_MultipleKeywordsInOneLine_DeduplicatesSnippets(t *testing.T)
func TestParseErrors_DoesNotFalsePositiveOnInfoLines(t *testing.T)  // "waiting 300s timeout"

// Table-driven with realistic ESPHome log excerpts:
var parseErrorCases = []struct{
    name    string
    lines   []string
    wantLen int
    wantContains string
}{
    {"connection refused", []string{"[E][conn:1]: Connection refused"}, 1, "refused"},
    {"ota timeout", []string{"[E][ota:12]: OTA timeout"}, 1, "timeout"},
    {"no error in success log", successLogLines, 0, ""},
}
```

### 5.5 Phase 4 — `runner.go` Modification (test-first)

A `captureWriter` type enables testing the runner's OutputWriter calls without a real terminal:

```go
// internal/upgrader/runner_test.go

type captureWriter struct {
    mu        sync.Mutex
    lines     []struct{ device, line string }
    started   []string
    completed []struct{ device string; success bool; errLines []string }
    retries   []struct{ device string; attempt int }
}
// implements output.OutputWriter

func TestUpgrade_WriterReceivesAllLines_DryRun(t *testing.T)
func TestUpgrade_WriterReceivesDeviceStarted(t *testing.T)
func TestUpgrade_WriterReceivesDeviceCompleted_Success(t *testing.T)
func TestUpgrade_WriterReceivesDeviceCompleted_Failure_WithErrLines(t *testing.T)
func TestUpgrade_WriterReceivesRetry(t *testing.T)
func TestUpgrade_FullBufferDiscardedOnSuccess(t *testing.T)   // measure heap after run

// Existing tests MUST still pass with no changes:
// TestExtractVersion_*, TestUpgrade_DryRunReturnsSuccess, TestUpgrade_PreservesOrder, etc.
```

### 5.6 Phase 5 — TUI Model (test-first, headless)

The bubbletea Model is pure functions — no terminal needed:

```go
// internal/tui/model_test.go

func TestModel_Init_AllDevicesQueued(t *testing.T)
func TestModel_DeviceStatusMsg_Running(t *testing.T)
func TestModel_DeviceStatusMsg_Success(t *testing.T)
func TestModel_DeviceStatusMsg_Failure_PopulatesErrors(t *testing.T)
func TestModel_DeviceOutputMsg_UpdatesDisplayBuf(t *testing.T)
func TestModel_DeviceOutputMsg_UpdatesGlobalTail(t *testing.T)
func TestModel_AllDoneMsg_NoFailures_Quits(t *testing.T)
func TestModel_AllDoneMsg_WithFailures_StaysOpen(t *testing.T)
func TestModel_KeyQ_WhenDone_Quits(t *testing.T)
func TestModel_KeyQ_WhileRunning_KillsAndQuits(t *testing.T)
func TestModel_TickMsg_AdvancesSpinnerFrame(t *testing.T)
func TestModel_TermSizeMsg_UpdatesLayout(t *testing.T)
func TestModel_AutoScroll_DisabledByUpArrow(t *testing.T)
func TestModel_AutoScroll_ReenabledByHome(t *testing.T)
func TestModel_TabKey_TogglesFocus(t *testing.T)

// internal/tui/ringbuffer_test.go
func TestRingBuffer_BoundedAtCapacity(t *testing.T)
func TestRingBuffer_PreservesInsertionOrder(t *testing.T)
func TestRingBuffer_AllReturnsCorrectSlice(t *testing.T)
func TestRingBuffer_ThreadSafe(t *testing.T)  // 100 goroutines writing concurrently

// internal/tui/panels/*_test.go — render output string tests
func TestHeaderPanel_ShowsBadges(t *testing.T)
func TestHeaderPanel_ShowsProgressBar(t *testing.T)
func TestHeaderPanel_ShowsDryRunBadge(t *testing.T)
func TestHeaderPanel_ShowsLogFileBadge(t *testing.T)
func TestDevicesPanel_ShowsSpinnerForActive(t *testing.T)
func TestDevicesPanel_ShowsRetryBadge(t *testing.T)
func TestErrorsPanel_CapacityEight(t *testing.T)
func TestErrorsPanel_EmptyStateMessage(t *testing.T)
func TestActivePanel_ShowsEmptySlot(t *testing.T)
func TestActivePanel_ShowsPhaseHint(t *testing.T)
```

---

## 6. Implementation Phases

### Phase 1A — Baseline (1–2 days, zero production changes)
- Run coverage report; document and commit gap analysis
- Write golden file test (`test/golden/upgrade_dry_run.txt`)
- Fill missing tests for existing functionality (runner retry path, diagnostics parseLogLines, discovery parseAPIKey)
- Establish `go test -race ./...` passes as CI gate
- All new tests fail at this point (expected — nothing is implemented yet)

### Phase 1B — OutputWriter Abstraction (1 day)
- Write `internal/output/writer.go` (interface definition only)
- Write `internal/output/plain.go` tests → implement to pass
- Write `internal/output/detect.go` tests → implement to pass
- Modify `runner.go` to accept `OutputWriter` parameter (minimal change — thread the parameter through, replace `fmt.Printf` call)
- Run `go test ./...` — all existing tests must pass; golden file test must pass
- Verify: `esp-tool upgrade --dry-run | diff - test/golden/upgrade_dry_run.txt` succeeds

### Phase 1C — Error Parsing + Buffers (1 day)
- Write `internal/upgrader/errors.go` tests → implement `parseErrors`
- Add `deviceBuffers` struct to `runner.go`; thread it through `runWithRetry` and `runStreaming`
- Write capture-writer tests for runner → implement by injecting `captureWriter` in tests
- Run `go test -race ./...` — must pass

### Phase 1D — TUI Foundation (2 days)
- Write `internal/tui/ringbuffer.go` tests → implement
- Write `internal/tui/messages.go` (no tests needed — data types only)
- Write `internal/tui/model.go` tests (skeleton) → implement Init + Update
- Write `internal/tui/writer.go` tests → implement TUIWriter
- Run `go test ./...` — must pass

### Phase 1E — Panel Renderers (2–3 days)
- Implement panels one at a time, test-first:
  1. `header.go` — simplest, no state transitions
  2. `devices.go` — spinner, color states
  3. `active.go` — sub-entries per device
  4. `errors.go` — ring behavior, empty state
  5. `output.go` — tail + scrolling via `bubbles/viewport`
- Iterative visual review in a real terminal throughout

### Phase 1F — Wiring + `--log-file` (1 day)
- Add `--plain` and `--log-file` flags to `upgradeCmd` in `main.go`
- Add TUI/plain routing in `upgradeCmd.RunE`
- Add log file writer threading through `RunOptions`
- End-to-end manual test: real ESPHome environment, 4+ devices

### Phase 1G — Polish + CI (1 day)
- Terminal resize handling (bubbletea emits `tea.WindowSizeMsg` automatically)
- `?` help overlay implementation
- Minimum terminal size degradation path (fallback to plain if < 80×24)
- Add `go test -race ./...` to CI pipeline
- README update: `--plain`, `--log-file` documented

**Total Phase 1 estimate: 9–12 developer days**

---

## 7. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Plain mode output regression | Low | High | Golden file test + CI enforcement from Phase 1A |
| Race condition in ring buffer | Medium | High | `sync.RWMutex`; mandatory `-race` in CI |
| `prog.Send()` called after `prog.Quit()` | Medium | Low | Bubbletea drops messages after quit; goroutines check `Done` flag |
| Full buffer memory growth on very long runs | Low | Low | Buffer is bounded to one device's lifetime; GC'd immediately on completion |
| Log file write contention from multiple goroutines | Medium | Low | Wrap `os.File` in a `sync.Mutex` writer — one-line change |
| Terminal resize causing layout panic | Medium | Low | Guard all dimension math against zero values |
| Bubbletea API change in minor version | Low | Low | Pin exact minor version in `go.mod` |
| `--log-file` path not writable | Low | Low | Non-fatal warning to stderr; continue without logging |

---

## 8. What Does NOT Change

**Guaranteed unchanged by Phase 1:**

- `discovery/scanner.go` — device scanning, YAML parsing, substitution resolution
- `diagnostics/checker.go` — diagnostic log collection and health analysis
- `report/printer.go` — all three summary printers
- `versions` and `diagnostics` commands — entirely unchanged user experience
- The `esphome` subprocess invocation (command, args, working directory, process group setup)
- All exit codes (non-zero if any device failed)
- Any behavior when piped, redirected, or run in CI

The TUI is purely additive. Removing it means deleting the `internal/tui` package and the `internal/output` package, reverting the `runner.go` parameter change, and removing two flags — the core tool is unaffected.

---

## 9. Handoff Notes for Claude Code

When implementing this, the correct sequence is strictly:

1. **Read and understand all existing tests before touching any code.** The existing test suite defines the contract; do not break it.
2. **Implement Phase 1A first.** The golden file test must be committed and passing before any production code changes.
3. **Follow the phase order.** Each phase builds on the previous. Do not skip ahead to TUI rendering before the OutputWriter abstraction is stable.
4. **Run `go test -race ./...` after every commit.** Not just `go test ./...`. The `-race` flag is the primary guard for the concurrent output pipeline.
5. **The plain mode path through `PlainWriter` must be functionally identical to the current `fmt.Printf` path.** Any behavioral difference is a regression.
6. **Do not modify `report/printer.go`, `discovery/scanner.go`, or `diagnostics/checker.go` in Phase 1.** These are out of scope.

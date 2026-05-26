# esp-tool Pseudo-GUI TUI Mode — Product Requirements Document

**Version:** 1.1  
**Date:** 2026-05-25  
**Status:** Decisions resolved — ready for implementation  
**Scope:** Discovery/Research + GUI Design phases

---

## Revision History

| Version | Change |
|---------|--------|
| 1.0 | Initial draft with open design questions |
| 1.1 | Resolved all four open questions; scoped TUI to `upgrade` command only in Phase 1; promoted S1 log file to Phase 1 co-deliverable; updated error detection approach |

---

## Resolved Design Decisions

These questions were open in v1.0 and are now closed:

| # | Question | Decision |
|---|----------|----------|
| 1 | Output pane layout | Last N lines per active device (as specced) — confirmed |
| 2 | End-of-run behavior | **Success:** TUI exits, plain summary printed. **Errors (upgrade only):** TUI stays up showing error detail; user presses `q` to exit, then plain summary printed |
| 3 | Error classification | Buffer full per-device output during run; on process exit with non-zero code, parse buffer for error patterns; discard buffer on success. This full buffer is also the source for S1 log file writes |
| 4 | OutputWriter interface | Rich semantic interface (`DeviceCompleted`, `DeviceRetry`, etc.) — confirmed |

---

## 1. Problem Statement

`esp-tool` runs ESPHome firmware operations across up to 20+ ESP32 devices in parallel. Today the output is a real-time, unordered stream of lines from all running processes mixed together on stdout. This creates two distinct pain points:

1. **Progress is opaque while running.** During a 4-minute upgrade of 14 devices, there is no at-a-glance answer to "how many are done, which are stuck, is anything failing?"
2. **Errors are buried in noise.** When a systematic failure (network outage, broken library, OTA server down) affects multiple devices simultaneously, the error messages are scattered across hundreds of interleaved lines and are easily missed until the final summary.

The end-of-run summary is well-designed and sufficient for the happy path. The gap is entirely in the *during-execution* experience.

---

## 2. Goals

- **G1.** Provide a structured, continuously-updating terminal screen for the `upgrade` command that shows device status, live output, and errors in clearly delineated panels — without scrolling past the screen.
- **G2.** Make it trivially obvious which devices are done, in-progress, queued, failed, or retrying at any moment during execution.
- **G3.** Surface captured error snippets from failed devices immediately after failure, without manual log-digging.
- **G4.** When an upgrade run completes with no failures, exit the TUI cleanly and print the familiar plain-text summary. When failures occur, hold the TUI so the operator can read the error detail before exiting.
- **G5.** Provide a `--log-file` option that writes the complete raw output of every device to a file, regardless of success or failure — enabling post-mortem analysis without re-running.
- **G6.** Preserve the existing plain-text output mode fully. The TUI is an enhancement layer, not a replacement.
- **G7.** Respect terminal size. Degrade gracefully on small terminals (< 80 cols / < 24 rows) with automatic fallback to plain mode.
- **G8.** Produce zero behavioral changes to the underlying device operations. The TUI is a display concern only.

## 3. Non-Goals — Phase 1

- TUI for `versions` and `diagnostics` commands (deferred to Phase 2 — learnings from `upgrade` TUI will inform those designs)
- Interactive job control (kill, pause, reorder)
- Mouse support
- No Windows support (existing tool is macOS/Linux only)
- Changes to the final summary output — it must remain byte-identical to today's plain output

---

## 4. Phase Scoping

### Phase 1 (this document)
- `upgrade` command TUI
- `--plain` flag on `upgrade` only
- `--log-file` flag on `upgrade` only
- Full test coverage baseline across all commands

### Phase 2 (future — informed by Phase 1 learnings)
- TUI for `versions` command (simpler layout, fast-running)
- TUI for `diagnostics` command (Issues panel replaces Errors panel)
- `--plain` and `--log-file` flags extended to `versions` and `diagnostics`

---

## 5. Flag Specification

### `--plain` (alias: `--no-tui`)

| Flag | Type | Default | Scope |
|------|------|---------|-------|
| `--plain` | bool | `false` | `upgrade` (Phase 1) |

**Activation logic (TTY auto-detection):**

The TUI activates automatically when ALL of the following are true:
1. `--plain` is NOT set
2. `stdout` is a real TTY (`isatty(1) == true`)
3. Terminal reports ≥ 80 columns and ≥ 24 rows

If any condition fails, the tool silently falls back to plain-text mode. CI pipelines, shell pipes, and file redirections always get plain text automatically.

### `--log-file <path>` (new flag, Phase 1)

| Flag | Type | Default | Scope |
|------|------|---------|-------|
| `--log-file` | string | `""` (disabled) | `upgrade` (Phase 1) |

When set, writes the complete, untruncated, prefixed output from every device to the specified file — regardless of TUI mode, success, or failure. Output is streamed to disk as lines arrive, so the file is useful even if the process is killed mid-run.

**Format:** Same as plain mode with `--prefix=true` — `[device-name] <line>`. One device's output may interleave with another's (same order it arrives). Intended for post-mortem grep and review, not for structured parsing.

**Example:**
```bash
esp-tool upgrade --log-file /tmp/esp-upgrade-$(date +%Y%m%d).log
```

The flag works in both TUI and plain modes. In TUI mode it provides the full raw output that the TUI compresses into panel views.

---

## 6. Screen Layout (`upgrade` command)

The screen is divided into four panels arranged in a fixed grid that scales to terminal dimensions.

```
┌─ HEADER ──────────────────────────────────────────────────────────────────┐
│ esp-tool upgrade  │  13 devices  │  ⚡ 4 jobs  │  ↺ 2 retries  │  2m34s  │
│ ████████████░░░░░░░░░░  8 / 13 completed                                  │
└───────────────────────────────────────────────────────────────────────────┘
┌─ Device Queue (60%) ──────────────────────────┐ ┌─ Active Jobs (40%) ─────┐
│  ✓  air-quality-external         [18s]        │ │  ▶ espvibration1        │
│  ✓  air-quality-internal         [21s]        │ │    Uploading firmware…  │
│  ✓  aram-display                 [44s]        │ │    3m02s                │
│  ✓  bluetooth-proxy-2  ↺2      [2m39s]       │ │                         │
│  ✓  esp32-bluetooth-proxy         [40s]       │ │  ▶ lux-living-christmas │
│  ▶  espvibration1                3m02s…      │ │    Compiling app…       │
│  ▶  lux-living-christmas         1m41s…      │ │    1m41s                │
│  ▶  ocamera                        28s…      │ │                         │
│  ◷  step-motor-1                  queued      │ │  ▶ ocamera              │
│  ◷  step-motor-2                  queued      │ │    Connecting…          │
│  ◷  step-motor-3                  queued      │ │    28s                  │
│  ◷  widecamera-2                  queued      │ └─────────────────────────┘
│  ◷  widecamera                    queued      │ ┌─ Errors (1) ────────────┐
└───────────────────────────────────────────────┘ │  bluetooth-proxy-2      │
                                                   │  Connection refused     │
                                                   │  attempt 1 — retried ✓ │
                                                   │  ✓ No other errors      │
                                                   └─────────────────────────┘
┌─ Output: espvibration1 ────────────────────────────────────────────────────┐
│  [espvibration1] INFO  Uploading /path/firmware.bin (870752 bytes)        │
│  [espvibration1] INFO  Upload took 165.07s, waiting for result...         │
│  [espvibration1] INFO  OTA successful                                     │
│  [espvibration1] INFO  Successfully uploaded program.                     │
└───────────────────────────────────────────────────────────────────────────┘
```

### Panel allocation

| Panel | Width | Height | Notes |
|-------|-------|--------|-------|
| Header | 100% | 3 rows fixed | Always at top |
| Device List | 60% | remaining − output height | Left column |
| Active Jobs | 40% | 50% of remaining | Right column, top half |
| Errors | 40% | 50% of remaining | Right column, bottom half |
| Output Tail | 100% | adaptive (min 4, max ~8 rows) | Always at bottom |

**Minimum terminal size for full layout:** 80 × 28.  
**Below threshold:** TUI does not activate; falls back to plain mode automatically.

---

## 7. Panel Specifications

### 7.1 Header Panel

**Line 1 — Badges:**
- `esp-tool upgrade` — bold command label
- `N devices` — total discovered
- `⚡ N jobs` — concurrency (`--jobs`)
- `↺ N retries` — retry limit (`--retries`)
- `⏱ Xm Ys` — elapsed wall-clock time, updated every second
- `dir: <path>` — working directory, dim, truncated if necessary
- `filter: <names>` — shown only when `--filter` is active (dim)
- `dry-run` badge in yellow when `--dry-run` is set
- `log: <path>` badge in dim when `--log-file` is active

**Line 2 — Progress bar:**
- Green fill for succeeded devices, red fill for failed devices, grey for remaining
- Right label: `N / M completed`

### 7.2 Device List Panel

Scrollable list of all discovered devices, auto-scrolling to keep the earliest in-progress device in view.

**Columns:** Status icon · Device name · Retry badge (when attempts > 1) · Duration

**Status icons:**

| State | Icon | Color |
|-------|------|-------|
| Queued | `◷` | dim grey |
| In progress | animated braille spinner | cyan |
| Retrying | `↺` | yellow |
| Success | `✓` | green |
| Failed | `✗` | red |

**Braille spinner frames:** `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` cycling at 100ms. In-progress devices show spinner instead of static icon.

**Duration display:**
- Queued: `queued` (dim)
- In progress: `Xm Ys…` with ellipsis, live-updated every second
- Completed: `[Xs]` or `[Xm Ys]` (dim brackets), final value

**Keyboard:** `↑` / `↓` scroll; `Home` re-enables auto-scroll.

### 7.3 Active Jobs Panel

Shows only devices currently holding a semaphore slot (running). Up to `--jobs` entries maximum.

Each active entry shows:
- Device name (bold cyan) and live elapsed time
- Last meaningful output line from that device (truncated to panel width), updated as lines arrive
- Attempt indicator if retrying: `attempt 2/3` in yellow

Empty slots shown as `—` in dim when fewer than `--jobs` devices are active.

**Upgrade phase hints** parsed from output to show brief labels:
- `Compiling app…` when output contains `Compiling app`
- `Uploading firmware…` when output contains `Uploading`
- `OTA in progress…` when output contains `waiting for result`
- `Connecting…` when output contains `Connecting to`

### 7.4 Errors Panel

Shows error snippets captured from devices that have failed at least one attempt. Entries are populated **only from the exit-code-driven error buffer** — not from keyword scanning of live output (see Section 8.1).

**Error entry format:**
```
  [device-name]  (attempt N — retried ✓ / failed)
  <first meaningful error line from exit buffer, max 2 display lines>
```

**Capacity:** Last 8 error entries; oldest pushed off top as new ones arrive. Title badge shows total count: `ERRORS (7)`.

**Color:** Device name in red; error text in dim; resolution status (`retried ✓` / `failed`) in green or red.

**Empty state:** `✓ No errors` in dim green.

### 7.5 Output Tail Panel

Shows the last N lines of raw combined output across all active devices, each line prefixed `[device-name]` in cyan.

**Height:** Adaptive — `floor((termHeight - fixedRows) * 0.25)`, minimum 4 lines, maximum 8 lines.

**Source:** The per-device display ring buffer (bounded, ~500 lines). The tail panel shows the globally most-recent N lines interleaved.

**Panel title:** `OUTPUT: <most-recently-active-device>` — updates as new lines arrive from different devices.

**Keyboard:** `Tab` focuses output pane. When focused, `↑` / `↓` scrolls the ring buffer for the most-recently-active device. `Esc` releases focus.

---

## 8. Error Detection Design

### 8.1 Per-Device Full Output Buffer

Each device's goroutine maintains two independent structures:

**Ring buffer (display):** Bounded to ~500 lines. Used by the Output Tail panel. Always active.

**Full buffer (analysis):** Unbounded slice appended to as lines arrive during execution. Used only for error parsing when a device exits with a non-zero exit code. **Discarded immediately on successful exit** — no memory held after success.

This design avoids the noise of live keyword scanning (which produces false positives from ESPHome's verbose INFO logs) while still capturing errors precisely when they occur. The exit code is the ground truth for "did something go wrong."

### 8.2 Error Parsing on Failure

When a device process exits with a non-zero code:

1. The full buffer is passed to `parseErrors(lines []string) []string`
2. `parseErrors` scans for lines containing: `ERROR`, `FAILED`, `Exception`, `Traceback`, `refused`, `timeout`, `unreachable`, `CRIT`
3. The first matched line (and the line immediately following it, if non-empty) forms the error snippet
4. The snippet is sent as a `DeviceErrorMsg` to the TUI model → populates the Errors panel
5. The full buffer is then nil'd (GC'd)

If no error keywords are found despite a non-zero exit code, the snippet defaults to `"Process exited with error (no parseable error line found)"`.

### 8.3 Relationship to `--log-file`

The full buffer and `--log-file` share the same data source but are independent mechanisms:

- **Log file** is written **as lines arrive** (streaming to disk), completely independent of success/failure. It is never discarded. It exists whether or not TUI mode is active.
- **Full buffer** is held in memory only during execution of a single device, and only for error parsing purposes. It is discarded after the device completes.

A device that succeeds: its output streams to `--log-file` (if set), and its full buffer is GC'd.  
A device that fails: its output streams to `--log-file` (if set), its full buffer is parsed for errors, then GC'd.

This means `--log-file` provides the complete record even for failed devices, making it the right tool for post-mortem analysis.

---

## 9. End-of-Run Behavior

### Successful run (all devices succeeded)

1. `AllDoneMsg` arrives with zero failures
2. TUI calls `tea.Quit` immediately
3. Terminal is restored to normal
4. `report.PrintUpgradeSummary()` is called — identical output to today's plain summary
5. Process exits with code 0

### Failed run (one or more devices failed)

1. `AllDoneMsg` arrives with ≥ 1 failure
2. TUI **does not quit** — it transitions to a "Run complete with errors" state:
   - Header updates: elapsed time freezes, badge shows `✗ N FAILED` in red
   - Device list shows final states for all devices (no more animation)
   - Errors panel shows all captured error snippets
   - Output tail stops auto-updating
   - A prompt appears at the bottom: `Press q to exit and see summary`
3. User reads the error detail at their own pace
4. User presses `q` (or `Ctrl-C`)
5. Terminal is restored
6. `report.PrintUpgradeSummary()` is called — identical output to today's plain summary
7. Process exits with non-zero code (current behavior preserved)

This gives the operator the error context they need without it scrolling away, while still ending with the same machine-readable summary they may be scripting against.

---

## 10. Keyboard Controls

| Key | Action |
|-----|--------|
| `q` / `Ctrl-C` | Exit TUI (graceful on success; allowed after run completes on failure) |
| `↑` / `↓` | Scroll device list (disables auto-scroll) |
| `Home` | Re-enable device list auto-scroll |
| `Tab` | Cycle focus: device list → output pane → device list |
| `Esc` | Release output pane focus |
| `p` | Pause output pane auto-scroll |
| `r` | Resume output pane auto-scroll |
| `?` | Toggle keyboard shortcut help overlay |

**During run:** `q` / `Ctrl-C` terminates all running processes gracefully (same behavior as current `Ctrl-C` handling).

**After successful run:** TUI exits automatically; `q` is not needed.

**After failed run:** `q` is required to exit, giving the operator time to read the error panel.

---

## 11. Visual Design Language

Follows btop/htop aesthetic: dark terminal background, thin panel borders, muted secondary text, vivid accent colors for status.

**Color palette (ANSI):**

| Element | Color |
|---------|-------|
| Panel borders / dividers | dim (`\033[2m`) |
| Panel title labels | muted white |
| Success | bright green (`\033[1;32m`) |
| Failure / errors | bright red (`\033[1;31m`) |
| In-progress / active | bright cyan (`\033[1;36m`) |
| Warnings / retries | bright yellow (`\033[1;33m`) |
| Queued / dim info | dim grey (`\033[2m`) |
| Device name in output tail | cyan (`\033[36m`) |
| Progress bar fill (success) | `█` green |
| Progress bar fill (failure) | `█` red |
| Progress bar unfilled | `░` dim grey |

---

## 12. `--log-file` Flag Specification (S1)

This feature ships in Phase 1 alongside the `upgrade` TUI.

**Flag:** `--log-file <path>`  
**Default:** disabled (empty string)  
**Works in:** both TUI mode and plain mode  
**Format:** plain text, one line per output line, prefixed `[device-name] ` (same as `--prefix=true`)

**Behavior:**
- File is created (or truncated) at startup, before any devices begin
- Lines are written as they arrive from each device's scanner goroutine — streaming, not buffered
- File is closed when all devices finish (or on process termination)
- If the specified path is not writable, the tool prints a warning to stderr and continues without logging (non-fatal)
- Log file writes happen in the `PlainWriter.WriteLine()` and `TUIWriter.WriteLine()` paths — one implementation used by both modes

**Example log file content:**
```
[air-quality-external] INFO  Connecting to air-quality-external.local...
[air-quality-internal] INFO  Connecting to air-quality-internal.local...
[air-quality-external] INFO  Connected to 192.168.68.101
[bluetooth-proxy-2] ERROR  Connection refused
...
```

---

## 13. Success Criteria

The Phase 1 implementation is complete when:

1. All existing tests pass unchanged in plain mode
2. `--plain` flag on `upgrade` produces output byte-for-byte identical to today's output (verified by golden file test)
3. TUI activates automatically on `upgrade` when running interactively
4. On a 13-device upgrade, a user can identify all in-progress devices and any errors within 2 seconds of glancing at the screen
5. On a failed run, the TUI holds open and shows parseable error snippets from all failed devices
6. `--log-file` produces a complete, readable log file for both successful and failed runs
7. No goroutine races introduced (`go test -race ./...` passes)
8. TTY auto-detection correctly falls back to plain in CI (verified with `| cat`)
9. Terminal resize does not crash or corrupt layout
10. `versions` and `diagnostics` commands are completely unchanged from today's behavior

---

## 14. Out of Scope / Future Phases

**Phase 2:**
- TUI for `versions` command
- TUI for `diagnostics` command
- `--log-file` on `versions` and `diagnostics`

**Phase 3 and beyond:**
- Interactive job control (pause, kill individual device)
- Mouse support
- Color theme selection
- Per-device expandable output in device list (`Enter` to expand)
- `--output-lines N` flag for configurable output tail height
- `--tick N` flag for configurable animation rate

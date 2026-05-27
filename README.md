# esp-tool

[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go 1.21+](https://img.shields.io/badge/go-1.21%2B-00ADD8.svg?logo=go&logoColor=white)](https://go.dev/)
[![ESPHome](https://img.shields.io/badge/ESPHome-compatible-blue.svg)](https://esphome.io/)

A Go CLI for managing [ESPHome](https://esphome.io) devices. It auto-discovers devices from your ESPHome YAML configuration directory, runs OTA firmware upgrades in parallel with retry logic, and checks running firmware versions — all without maintaining a manual device list.

**Replaces** the handwritten `upgrade-esp-devices.sh` and `check-esp-versions.sh` shell scripts. Adding a new device YAML to your ESPHome repo is all that's needed for it to be picked up automatically.

---

## Screenshots

### `upgrade`

**TUI — compilation phase** (14 devices, 4 parallel jobs, dependency resolution in progress):

![TUI startup — compilation phase](docs/screenshots/tui-startup.png)

**TUI — upload phase** (progress bar advancing, completed devices marked ✓, active uploads in right panel):

![TUI mid-run — firmware upload phase](docs/screenshots/tui-mid-run.png)

**Post-upgrade summary** (all 14 devices upgraded, retry badges where applicable, per-device timing):

![Upgrade summary](docs/screenshots/upgrade-summary.png)

**TUI — device unreachable, retry pending** (step-motor-2 offline; `↺ in 4s` countdown in device list, last error line in Active Jobs panel, Output panel showing the DNS timeout):

![TUI device retrying](docs/screenshots/tui-device-retrying.png)

**TUI — upgrade complete with one failure** (step-motor-2 exhausted all retries; Errors panel populated with snippets, failed device marked ✗, progress bar full):

![TUI device failed](docs/screenshots/tui-device-failed.png)

### `versions`

**TUI — mid-run** (9/14 devices resolved with ✓ and firmware version, remaining still spinning):

![Versions TUI mid-run](docs/screenshots/versions-tui-mid-run.png)

### `diagnostics`

**TUI — startup** (all 14 devices spinning, log collection in progress):

![Diagnostics TUI startup](docs/screenshots/diagnostics-tui-startup.png)

**Post-diagnostics summary** (all 14 devices reachable and healthy, firmware versions listed):

![Diagnostics summary](docs/screenshots/diagnostics-summary.png)

---

## How it works

1. **Discovers devices** by scanning `*.yaml` files in the target directory using a case-insensitive extension match (`.yaml`, `.YAML`, `.Yaml` are all accepted — handles files saved by Windows tools like Notepad). Skips `secrets.yaml` and subdirectories like `archive/`.
2. **Parses `esphome.name`** from each YAML, resolving ESPHome substitution variables (`${name}`, `$hostname`, etc.) via the `substitutions:` block.
3. **Derives the OTA hostname** as `<name>.local`.
4. **Runs `esphome` commands** in parallel, bounded by a configurable concurrency limit.

---

## Prerequisites

> **`esphome` CLI is required.** esp-tool is a wrapper around the ESPHome command-line tool — it calls `esphome run`, `esphome logs`, and `esphome config` under the hood. If `esphome` is not installed and on your `PATH`, every command that talks to devices will fail.
>
> Install it with: `pip3 install esphome` (requires Python 3.9+).  
> Full instructions: [ESPHome — Getting Started with the CLI](https://esphome.io/guides/getting_started_command_line).

| Platform | ESPHome availability |
|---|---|
| **macOS / Linux** | Install via `pip3 install esphome` — works natively |
| **Windows (WSL2)** | Run esp-tool's **Linux** binary inside WSL2 where ESPHome is installed — recommended |
| **Windows (native)** | Install ESPHome via `pip3 install esphome` in a Windows Python environment, then use esp-tool's `.exe` — ESPHome's Windows support is limited; most users prefer WSL2 |

- [Go 1.21+](https://go.dev/dl/) — only needed if building from source (Option B)

---

## Installation

### Option A — download a pre-built binary (recommended)

Pre-built binaries for macOS, Linux, and Windows are published with every tagged release on
[GitHub Releases](https://github.com/gevgev/esp-tool/releases).

**macOS / Linux** — one-liner install (auto-detects the latest version):

```bash
# macOS (Apple Silicon)
VERSION=$(curl -s https://api.github.com/repos/gevgev/esp-tool/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
curl -sL "https://github.com/gevgev/esp-tool/releases/download/v${VERSION}/esp-tool_${VERSION}_darwin_arm64.tar.gz" | tar -xz
sudo mv esp-tool /usr/local/bin/

# macOS (Intel)
VERSION=$(curl -s https://api.github.com/repos/gevgev/esp-tool/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
curl -sL "https://github.com/gevgev/esp-tool/releases/download/v${VERSION}/esp-tool_${VERSION}_darwin_amd64.tar.gz" | tar -xz
sudo mv esp-tool /usr/local/bin/

# Linux (amd64)
VERSION=$(curl -s https://api.github.com/repos/gevgev/esp-tool/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
curl -sL "https://github.com/gevgev/esp-tool/releases/download/v${VERSION}/esp-tool_${VERSION}_linux_amd64.tar.gz" | tar -xz
sudo mv esp-tool /usr/local/bin/

# Linux (arm64 — Raspberry Pi 64-bit, etc.)
VERSION=$(curl -s https://api.github.com/repos/gevgev/esp-tool/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
curl -sL "https://github.com/gevgev/esp-tool/releases/download/v${VERSION}/esp-tool_${VERSION}_linux_arm64.tar.gz" | tar -xz
sudo mv esp-tool /usr/local/bin/
```

**Windows (PowerShell)** — auto-detects the latest version:

```powershell
$VERSION = (Invoke-RestMethod https://api.github.com/repos/gevgev/esp-tool/releases/latest).tag_name.TrimStart('v')
Invoke-WebRequest `
  -Uri "https://github.com/gevgev/esp-tool/releases/download/v$VERSION/esp-tool_${VERSION}_windows_amd64.zip" `
  -OutFile esp-tool.zip
Expand-Archive esp-tool.zip -DestinationPath .
# Move the binary to any folder on your PATH, for example:
Move-Item .\esp-tool.exe "$env:USERPROFILE\AppData\Local\Microsoft\WindowsApps\"
```

> **Windows + ESPHome:** `esphome` has limited native Windows support. Most Windows users run esp-tool's **Linux binary inside WSL2** (where ESPHome is pip-installed) rather than the native `.exe`. The Windows binary is most useful for scripting or environments where ESPHome is explicitly installed in the Windows Python environment.

Each release archive also contains the `completions/_esp-tool` zsh script and `README.md`.

### Option B — build from source

Requires [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/gevgev/esp-tool.git
cd esp-tool
make build              # produces bin/esp-tool  (macOS / Linux)
make build-windows      # produces bin/esp-tool.exe  (cross-compile for Windows)
```

Install the binary into your ESPHome YAML directory (so you can run it from there):

```bash
make install                                        # installs to ../esphome/esphome/ by default
make install ESPHOME_DIR=/path/to/your/esphome/dir  # or specify a custom path
```

The installed binary is a build artifact — add it to `.gitignore` in your ESPHome repo:

```
/esp-tool
```

### Shell completion (zsh)

**Option 1 — system-wide (requires sudo):**

```bash
sudo make install-completions
```

**Option 2 — user-local, no sudo.** Add to `~/.zshrc`:

```zsh
fpath=(/path/to/esp-tool/completions $fpath)
autoload -Uz compinit && compinit
```

Then `source ~/.zshrc` (or open a new shell).

Completion covers all subcommands, flags with descriptions, directory path completion for `--dir`, and device name completion for `--filter` (scanned from YAML files in the active `--dir`).

---

## Configuration file

esp-tool looks for `.esp-tool.yaml` in the current directory first, then in `~/.esp-tool.yaml`. This lets you persist your usual settings so you don't need to repeat them on every invocation. Command-line flags always override config file values.

**Supported keys:**

| Key | Type | Description |
|---|---|---|
| `dir` | string | Directory containing ESPHome YAML files |
| `jobs` | int | Maximum simultaneous `esphome` processes |
| `retries` | int | Retry attempts after the first upgrade failure |
| `retry-delay` | duration | Wait between retry attempts (e.g. `5s`, `15s`) |
| `timeout` | duration | Per-attempt timeout; `0` means no limit |
| `filter` | string | Comma-separated device names (all if omitted) |

**Example — place this in your ESPHome directory as `.esp-tool.yaml`:**

```yaml
# .esp-tool.yaml
dir: ~/git/esp32/esphome/esphome   # default --dir for all commands
jobs: 4
retries: 3
retry-delay: 10s
```

A fully annotated example is in [`docs/esp-tool.yaml.example`](docs/esp-tool.yaml.example).

> Run with `--verbose` to see which config file was loaded.

---

## Commands

### `upgrade`

Rebuilds firmware and OTA-flashes all discovered devices. Runs:

```
esphome run <file> --no-logs --device <name>.local
```

Devices are processed in parallel (default: 4 at a time, since compilation is CPU/RAM intensive). On failure, each device is retried up to `--retries` additional times before being marked as failed. A colored summary table is printed when all devices finish.

#### TUI vs plain-text output

When stdout is a TTY at least 80 × 24 characters, the `upgrade` command shows an interactive terminal UI (TUI):

- **Header** — jobs, retries, progress bar (`█░`), elapsed time
- **Left panel** — all devices with live status icons (⠋ running, ✓ success, ✗ failed, ↺ retrying, ◷ queued), retry badge, and final duration
- **Right panels** — active jobs with last output line (top) and error snippets (bottom)
- **Bottom panel** — scrollable tail of the most recent output lines from all devices (or press `?` for help)

When the TUI exits, the same colored summary table as plain mode is printed to stdout.

The TUI is **not** shown when:
- `--plain` or `--no-tui` is passed
- stdout is not a TTY (piped, redirected, CI environment)
- the terminal is smaller than 80 × 24
- (use `--verbose` to see which condition triggered the fallback)

#### TUI keyboard shortcuts

| Key | Action |
|---|---|
| `q` / `ctrl+c` | Quit (auto-exits on all-success; waits for `q` on failure) |
| `?` | Toggle keyboard-shortcut help overlay |
| `↑` / `k` | Scroll device list up |
| `↓` / `j` | Scroll device list down |
| `Home` / `g` | Jump to top of device list |
| `End` / `G` | Jump to bottom of device list |

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--dir` | `-d` | `.` (cwd) | Directory containing ESPHome YAML files |
| `--jobs` | `-j` | `4` | Maximum simultaneous `esphome` processes |
| `--retries` | `-r` | `2` | Retry attempts after the first failure |
| `--retry-delay` | | `5s` | Wait time between retry attempts |
| `--timeout` | | `0` (none) | Per-attempt timeout; kills the process if exceeded (e.g. `10m`) |
| `--filter` | | | Comma-separated device names to upgrade (all if omitted) |
| `--retry-failed` | | `false` | Re-run only devices that failed in the previous upgrade run |
| `--dry-run` | | `false` | Print commands without executing them |
| `--prefix` | | `true` | Prefix live output lines with `[device-name]` (plain mode) |
| `--plain` | | `false` | Disable TUI; use plain-text output |
| `--no-tui` | | `false` | Alias for `--plain` |
| `--log-file` | | | Append all device output to a file (streamed line-by-line) |
| `--verbose` | `-v` | `false` | Print diagnostic logs to stderr (retries, timing, TUI fallback reason) |

After every run (whether all devices succeeded or not) a small JSON state file is saved to `.esp-tool-last-run.json` in the `--dir` directory. `--retry-failed` reads this file and filters the device list to only the ones that failed last time. It is an error to combine `--retry-failed` with `--filter`.

**Examples:**

```bash
# Upgrade all devices from the current directory (esphome repo root)
./esp-tool upgrade

# Upgrade from a specific directory
esp-tool upgrade --dir ~/git/esp32/esphome/esphome

# Increase parallelism and retries
esp-tool upgrade --jobs 6 --retries 3

# Use a longer pause between retries (e.g. device is slow to reboot)
esp-tool upgrade --retry-delay 15s

# Upgrade only specific devices (comma-separated)
esp-tool upgrade --filter lux-living-christmas
esp-tool upgrade --filter step-motor-1,step-motor-2

# Re-run only the devices that failed last time (reads .esp-tool-last-run.json)
esp-tool upgrade --retry-failed

# Dry-run: verify device discovery and see exact commands without flashing
esp-tool upgrade --dry-run

# Force plain-text output (no TUI) — useful in scripts or over SSH
esp-tool upgrade --plain

# Stream all output to a file while TUI is active
esp-tool upgrade --log-file /tmp/upgrade-$(date +%Y%m%d).log

# Show why TUI was not activated (runs in plain mode in this example)
esp-tool upgrade --verbose 2>&1 | head -3

# Combine: dry-run a filtered set from a specific directory
esp-tool upgrade --dir ~/git/esp32/esphome/esphome --filter ocamera --dry-run

# Kill any attempt that takes longer than 10 minutes (prevents stuck OTA from blocking a slot)
esp-tool upgrade --timeout 10m
```

**Sample output:** see the [Screenshots](#screenshots) section at the top of this README.

---

### `validate`

Runs `esphome config <file>` for every discovered device in parallel. Reports which configs are valid and which have errors, **without compiling or flashing anything**. Use this as a pre-flight check after editing YAML before a batch upgrade.

Compile errors detected here also cause `upgrade` to skip retries automatically — if a device's config is broken, `upgrade` reports `Upgrade failed (compile error)` and moves on immediately rather than retrying.

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--dir` | `-d` | `.` (cwd) | Directory containing ESPHome YAML files |
| `--jobs` | `-j` | `4` | Maximum simultaneous `esphome` processes |
| `--timeout` | | `30s` | Per-device timeout for config validation |
| `--filter` | | | Comma-separated device names to check (all if omitted) |
| `--dry-run` | | `false` | Print commands without executing them |
| `--verbose` | `-v` | `false` | Print diagnostic logs to stderr |

**Examples:**

```bash
# Validate all devices in the current directory
esp-tool validate

# Validate from a specific directory
esp-tool validate --dir ~/git/esp32/esphome/esphome

# Validate a single device
esp-tool validate --filter lux-living-christmas

# Dry-run: see what would be checked
esp-tool validate --dry-run
```

---

### `versions`

Connects to each device's live log stream in parallel, grabs the first `ESPHome version` line, and exits. Prints a colored summary. Times out per device after `--timeout` (default 12 s).

When stdout is a TTY ≥ 80×24, shows a **live TUI** with a spinner per device that updates in real time as each result arrives. The TUI auto-quits after all devices report and the summary is printed to stdout. Use `--plain` to suppress the TUI.

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--dir` | `-d` | `.` (cwd) | Directory containing ESPHome YAML files |
| `--timeout` | | `12s` | Per-device timeout before marking unreachable |
| `--filter` | | | Comma-separated device names to check (all if omitted) |
| `--plain` | | `false` | Disable TUI; use plain-text output |
| `--verbose` | `-v` | `false` | Print diagnostic logs to stderr |

**Examples:**

```bash
# Check all devices from the current directory
./esp-tool versions

# Check from a specific directory
esp-tool versions --dir ~/git/esp32/esphome/esphome

# Allow more time for slow devices
esp-tool versions --timeout 20s

# Check only a subset of devices
esp-tool versions --filter ocamera,widecamera,widecamera-2

# Force plain-text output (no TUI)
esp-tool versions --plain
```

**Sample output:**

```
Checking firmware versions for 14 devices...

ESPHome device firmware versions.
Summary:

  - air-quality-external:          v2024.11.0
  - air-quality-internal:          v2024.11.0
  - aram-display:                  v2024.11.0
  - bluetooth-proxy-2:             v2024.11.0
  - bluetooth-proxy-9c866c:        Unreachable
  ...

13 reachable, 1 unreachable
Elapsed time: 12s
```

---

### `diagnostics`

Connects to each device's live log stream in parallel, collects the initial boot dump, and prints a per-device health table. Times out per device after `--timeout` (default 15 s).

When stdout is a TTY ≥ 80×24, shows a **live TUI** with a spinner per device that resolves as each result arrives: ✓ Healthy, ⚠ N warnings (with detail lines), or ✗ crash. Use `--plain` to suppress the TUI.

Detects:
- Crash on previous boot (hardware WDT, exception, etc.)
- Bootloader too old for OTA rollback (needs one-time USB flash)
- Bootloader supports SRAM1 (+40 KB IRAM, opt-in flag available)
- Chip rev ≥ 3.0 (binary size can be reduced with `minimum_chip_revision`)
- GPIO strapping pin in use
- Multiple OTA platform configs merged

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--dir` | `-d` | `.` (cwd) | Directory containing ESPHome YAML files |
| `--timeout` | | `15s` | Per-device timeout for log collection |
| `--filter` | | | Comma-separated device names to check (all if omitted) |
| `--plain` | | `false` | Disable TUI; use plain-text output |
| `--reboot` | `-r` | `false` | Soft-reboot each device before capturing logs |
| `--reboot-wait` | | `12s` | Time to wait after rebooting before collecting logs |
| `--verbose` | `-v` | `false` | Print diagnostic logs to stderr |

**Examples:**

```bash
# Check all devices from the current directory
./esp-tool diagnostics

# Check from a specific directory
esp-tool diagnostics --dir ~/git/esp32/esphome/esphome

# Check a subset with verbose output
esp-tool diagnostics --filter espvibration1,lux-living-christmas --verbose

# Reboot each device first to capture a fresh boot log
esp-tool diagnostics --reboot

# Force plain-text output (no TUI)
esp-tool diagnostics --plain
```

---

## Typical workflow

After a new ESPHome version is released:

```bash
# 1. Upgrade the esphome tool itself
pip3 install esphome --upgrade

# 2. (Optional) Validate all device configs before touching any hardware
./esp-tool validate

# 3. (Optional) Verify all devices are currently reachable
./esp-tool versions

# 4. Upgrade all devices
./esp-tool upgrade

# 5. If any failed, retry just those (reads .esp-tool-last-run.json automatically)
./esp-tool upgrade --retry-failed

# 6. Or target a specific device by name
./esp-tool upgrade --filter bluetooth-proxy-9c866c
```

---

## Project structure

```
esp-tool/
├── cmd/esp-tool/main.go          # CLI entry point (cobra commands, flag wiring)
├── internal/
│   ├── diagnostics/              # Boot log collection and health analysis
│   ├── discovery/scanner.go      # YAML glob, esphome.name parsing, substitution resolution
│   ├── output/                   # OutputWriter abstraction (PlainWriter, MutexWriter, ShouldUseTUI)
│   ├── tui/                      # Bubbletea TUI — model, panel renderers, TUIWriter
│   ├── upgrader/runner.go        # Parallel esphome execution, semaphore, retry logic
│   └── report/printer.go        # Colored ANSI summary table
├── completions/
│   └── _esp-tool                 # Zsh completion script
├── go.mod
├── Makefile
└── README.md
```

# Changelog

## [v0.3.0] — 2026-06-18

### Added

- **`sync`** — patches the ESPHome Device Builder add-on's `.device-builder-devices.json` state file so devices flashed externally via `esp-tool upgrade` no longer show a stale "out of sync" indicator in the Builder UI. Reads the last upgrade run's results to determine which devices succeeded, then sets `expected_config_hash = deployed_config_hash` for each. Supports patching the file directly (`--db-file`, for running on the HA host) or remotely over SSH (`--ssh-host` + `--remote-file`, authenticating via `ssh-agent` or `--ssh-key`, with host keys verified against `~/.ssh/known_hosts`). `--dry-run` previews changes without writing.

[v0.3.0]: https://github.com/gevgev/esp-tool/releases/tag/v0.3.0

## [v0.2.0] — 2026-05-27

### Added

- **Windows support** — `windows/amd64` is now a first-class release target. Pre-built `esp-tool.exe` is published with every release. Cross-compilation requires no additional toolchain (`CGO_ENABLED=0`).
- **Case-insensitive YAML discovery** — `.yaml`, `.YAML`, and `.Yaml` extensions are all accepted. Fixes silent failures on Windows where tools like Notepad save files with an uppercase `.YAML` extension.
- **Diagnostic scan errors** — when no device YAMLs are found, the error now lists every skipped file and the reason it was rejected (e.g. missing `esphome: name:` field), making misconfigured directories easy to debug.
- **`make build-windows`** — cross-compiles `bin/esp-tool.exe` from any platform.
- **`make test-windows-compile`** — fast `GOOS=windows go build` gate for local development.
- **CI test matrix** — tests now run on `ubuntu-latest`, `macos-latest`, and `windows-latest` (with `-race`). A dedicated job uploads `esp-tool.exe` as a downloadable artifact on every branch push.

### Fixed

- Process-group management (`syscall.Kill`, `SysProcAttr.Setpgid`) extracted into build-tagged files so the codebase compiles cleanly for Windows.
- `TestLastRunPath` used a hardcoded Unix path separator; now uses `filepath.Join` so it passes on Windows.
- Golden file comparison normalises `\r\n` → `\n` before comparing, fixing failures when git checks out the file with Windows line endings.
- `.gitignore` entry `esp-tool` was unanchored and matched the `cmd/esp-tool/` source directory — changed to `/esp-tool`.

### Changed

- `.gitattributes` added: enforces LF line endings for all text files, preventing CRLF issues for contributors on Windows.

[v0.2.0]: https://github.com/gevgev/esp-tool/releases/tag/v0.2.0

## [v0.1.0] — 2026-05-08

Initial release.

### Added

- **`upgrade`** — rebuilds firmware and OTA-flashes all discovered devices in parallel. Configurable concurrency (`--jobs`), retry count (`--retries`), retry delay (`--retry-delay`), and `--filter` for targeting specific devices. Prints a coloured summary table on completion.
- **`versions`** — connects to each device's log stream in parallel, extracts the running ESPHome version, and prints a summary. Times out per device after `--timeout`.
- **`diagnostics`** — collects boot logs in parallel and detects: crash on previous boot, bootloader too old for OTA rollback, SRAM1 support, chip revision ≥ 3.0, GPIO strapping pin conflicts, and multiple OTA platform configs.
- **Auto-discovery** — globs `*.yaml` files in the target directory, parses `esphome.name` (resolving `substitutions:` variables), and derives OTA hostnames automatically. No device list to maintain.
- **`--dry-run`** mode for `upgrade` — prints commands without executing them.
- **`--filter`** flag — comma-separated device names for all commands.
- **Zsh shell completion** — covers all subcommands, flags, and per-device name completion for `--filter`.

[v0.1.0]: https://github.com/gevgev/esp-tool/releases/tag/v0.1.0

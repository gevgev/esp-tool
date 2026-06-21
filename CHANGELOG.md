# Changelog

## [v0.3.0] — 2026-06-21

### Added

- **`--jobs`/`-j`** on `versions` and `diagnostics` — both commands already accepted a `Concurrency` option internally but never exposed a flag for it, so they ran with unlimited parallel `esphome` log connections. Now default to `4`, same as `upgrade`/`validate`, overridable via `--jobs`.
- **`--no-tui`** alias on `versions` and `diagnostics` — `upgrade` already accepted `--no-tui` as an alias for `--plain`; the other two TUI-capable commands now do too.
- **Config-file support** for `verbose`, `plain` (covers both `--plain` and `--no-tui`), `log-file` (`upgrade`), and `reboot-wait` (`diagnostics`) — in addition to the existing `dir`, `jobs`, `retries`, `retry-delay`, `timeout`, `filter` keys. `--dry-run`, `--retry-failed`, `--reboot`, and `--prefix` remain CLI-only by design (one-off safety/state flags or a real device side-effect).
- **CLI flag-consistency test** — guards against a shorthand letter meaning different things in different subcommands, and against a flag name shared by multiple commands drifting in type or shorthand.

### Fixed

- `-r` meant `--retries` in `upgrade` but `--reboot` in `diagnostics` — confusing when switching between subcommands. `-r` is now reserved for `--retries` only; `--reboot` remains usable as a long flag.

### Changed

- Rewrote the README's "Configuration file" section and `docs/esp-tool.yaml.example`: explicit precedence rules (CLI flag > config file > built-in default), a full key table with an "applies to" column per command, a table of flags intentionally excluded from config and why, and a troubleshooting section (`--verbose` to confirm which file loaded, `~` expansion, a real gotcha where a bare YAML number like `retry-delay: 10` parses as 10 nanoseconds rather than 10 seconds).
- Documented why `versions`/`diagnostics` have no `--dry-run` (they only read an existing log stream — there's nothing to preview).

[v0.3.0]: https://github.com/gevgev/esp-tool/releases/tag/v0.3.0

## [v0.2.1] — 2026-06-18

### Added

- **`--version`** — prints the esp-tool version, commit, and build date (e.g. `esp-tool version 0.2.1 (commit 284cdb3, built 2026-06-19T00:32:52Z)`). Released binaries get real values via goreleaser's `-ldflags`, which was already configured but had no `main.version`/`main.commit`/`main.date` variables to target until now. `make build`/`make build-windows` embed the same info for local builds via `git describe`. No `-v` shorthand, to avoid any association with the unrelated `-v, --verbose` flag on subcommands.

### Fixed

- Config file (`.esp-tool.yaml`) string values starting with `~` (e.g. `dir: ~/esphome`) were passed through literally instead of expanding to the user's home directory, since config values — unlike CLI flags — are never shell-expanded. Affects any string config key, not just `dir`.

[v0.2.1]: https://github.com/gevgev/esp-tool/releases/tag/v0.2.1

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

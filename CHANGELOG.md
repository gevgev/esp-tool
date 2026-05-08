# Changelog

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

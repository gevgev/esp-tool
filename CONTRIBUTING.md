# Contributing to esp-tool

Contributions are welcome — bug reports, feature requests, and pull requests.

## Before you open a PR

- Check [open issues](https://github.com/gevgev/esp-tool/issues) to avoid duplicate work
- For anything beyond a small fix, open an issue first to discuss the approach

## Development setup

```bash
git clone https://github.com/gevgev/esp-tool.git
cd esp-tool
make build          # produces bin/esp-tool
./bin/esp-tool --help
```

## Testing

esp-tool operates against real ESPHome devices. If you're changing:

- `internal/discovery/` — test with a real ESPHome YAML directory containing at least 2–3 device files
- `internal/upgrader/` — use `--dry-run` first, then test OTA against a non-critical device
- `internal/diagnostics/` — test against a reachable device on your network

Please note the ESPHome version you tested against in your PR description.

## Code style

- Go 1.21+, standard `go fmt` formatting
- Device operations run in goroutines — any shared state must be goroutine-safe
- Keep the `--dry-run` path working for all commands that perform network operations

## Reporting bugs

Please use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md) and include your ESPHome version and the output of `--dry-run` if applicable.

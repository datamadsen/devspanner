# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-24

### Added

- Config-driven TUI for the local dev loop, reading `.devspanner/config.yaml` from
  the repo root.
- Live status and health for each service, with behaviour derived from config fields:
  `container:` → docker-style, `port:` → process-style.
- Start / stop / restart that frees a busy port before restarting, with per-service
  captured logs.
- Command palette (`c`) for one-shot tasks (build, deploy), with optional `confirm:`
  prompts and streamed output.
- Open-in-browser (`o`) and configurable global `shortcuts`.
- Ownership and stale-build detection via `/proc` that survive a devspanner restart
  (Linux); graceful degradation on macOS.
- Carriage-return collapsing so progress output stays readable in the log view.
- `devspanner -v` / `--version` and `-h` / `--help`.

[Unreleased]: https://github.com/datamadsen/devspanner/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/datamadsen/devspanner/releases/tag/v0.1.0

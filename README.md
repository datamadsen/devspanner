# devspanner

[![ci](https://github.com/datamadsen/devspanner/actions/workflows/ci.yml/badge.svg)](https://github.com/datamadsen/devspanner/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/datamadsen/devspanner?sort=semver)](https://github.com/datamadsen/devspanner/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/datamadsen/devspanner.svg)](https://pkg.go.dev/github.com/datamadsen/devspanner)
[![Go Report Card](https://goreportcard.com/badge/github.com/datamadsen/devspanner)](https://goreportcard.com/report/github.com/datamadsen/devspanner)
[![License: MIT](https://img.shields.io/github/license/datamadsen/devspanner)](LICENSE)

A small, config-driven terminal UI for the local dev loop. One screen to see and
steer everything you juggle while developing locally — a docker stack, app
backends, frontends — plus a command palette for the one-shot recipes (build,
deploy) that would otherwise clutter the view.

```
 my-project · devspanner

 shared                             actions
   ● platform  up                   [a] all start
                                     [x] stop apps
 api                                [c] commands
 ➤ ● backend   running   :8080      [g] dashboard
                                     [q] quit
 web
   ● frontend  running   :5173

 ↑/↓ select   [r] (re)start   [s] stop   [l]ogs   [o]pen in browser
```

## Why

`docker compose` + a couple of `npm run dev`s + a backend + the odd build/deploy
script is a lot of terminals and stale-process footguns. devspanner gives you:

- **Live status & health** for every service (up / starting / running / stale).
- **Start / stop / restart** that frees a busy port first (no more "address already
  in use") and tails each service's captured log.
- **Open in browser** — jump to a frontend or an API docs page.
- **A command palette** (`c`) for one-shot tasks (build, deploy), run in the
  background with streamed output, so they never crowd the dashboard.
- **Ownership that survives restarts** — close devspanner, reopen it, and it still
  knows which processes it started.

It's a single static binary; everything it manages lives in a YAML file in your repo.

## Install

devspanner is Linux-first (macOS works with graceful degradation; see
[Notes](#notes)). Pick whichever fits:

**Go** (any platform with a Go toolchain):

```bash
go install github.com/datamadsen/devspanner@latest
```

**Arch Linux** (AUR — e.g. with `yay`):

```bash
yay -S devspanner        # builds from source
# or: yay -S devspanner-bin   # prebuilt binary
```

**Homebrew** (macOS / Linux):

```bash
brew install datamadsen/tap/devspanner
```

**Prebuilt binaries & packages**: grab a `.tar.gz`, `.deb`, or `.rpm` from the
[Releases](https://github.com/datamadsen/devspanner/releases) page.

**From source**:

```bash
git clone https://github.com/datamadsen/devspanner
cd devspanner && make build      # or: go build -o devspanner .
```

## Quick start

1. Drop a config at your repo root — see [`examples/config.yaml`](examples/config.yaml):

   ```bash
   mkdir -p .devspanner && cp examples/config.yaml .devspanner/config.yaml
   ```

2. Edit it for your project, then run `devspanner` from anywhere in the repo.

devspanner finds the repo root (via `git`) and reads `.devspanner/config.yaml`. A
missing or invalid config prints a specific error and exits before taking over the
screen.

## Configuration

```yaml
name: my-project            # optional; shown in the header

groups:                     # display groupings (shared infra, then one per app)
  - name: api
    services:               # long-running things — shown on the dashboard
      - name: backend
        start: npm run dev  # required
        port: 8080          # process-style: liveness + free-before-restart
        dir: services/api   # working dir (relative to repo root); optional
        health: http://localhost:8080/health   # <500 = healthy; optional
        open: http://localhost:8080/docs        # [o]pen in browser; optional
        watch: services/api/src                 # flag build "stale" when newer; optional
    tasks:                  # one-shot recipes — behind the `c` palette, not the dashboard
      - name: deploy
        run: ./scripts/deploy.sh
        confirm: true       # ask y/n before running (use for anything outward-facing)

shortcuts:                  # global keys: open a URL or run a command
  - key: g
    label: dashboard
    open: http://localhost:3000
```

**Service behaviour is derived from the fields, not a `type`:**

- `container:` → **docker-style**. Liveness = that container running; `start`/`stop`
  are run as commands; logs come from the `logs:` command.
- `port:` → **process-style**. devspanner owns the child it spawns, captures its
  output to `.devspanner/logs/<group>-<name>.log`, and frees the port before a
  restart. `logs:` is optional here — the captured file is tailed.

Full per-field notes are in [`examples/config.yaml`](examples/config.yaml).

## Keys

| Key | Action |
|-----|--------|
| `↑`/`↓`, `j`/`k` | move selection |
| `r` | (re)start selected service |
| `s` | stop selected service |
| `l` / `enter` | view logs |
| `o` | open selected service's URL (only shown when it has one) |
| `a` | start everything |
| `x` | stop all process-style services (docker/infra stays up) |
| `c` | command palette (build/deploy tasks) |
| `q` | quit |

In the log/output view: `↑`/`↓` scroll, `esc` back, `x` cancels a running task.
Configured `shortcuts` add their own global keys (anything not reserved above).

## Notes

- **Linux**: ownership detection and stale-build timing read `/proc` (env marker +
  process start time), so a service started by a previous devspanner session is still
  recognised as "running" after you reopen. On other platforms these degrade
  gracefully (a service is "owned" only within the current session).
- Carriage-return progress output (build timers, docker/npm progress bars) is
  collapsed the way a terminal would render it, so the log view stays clean.
- devspanner manages the dev loop; it does not replace your build/deploy scripts — it
  runs them.

## License

[MIT](LICENSE)

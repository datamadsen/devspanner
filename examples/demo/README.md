# devspanner demo project

A throwaway sample project that the repo-root [`.devspanner/config.yaml`](../../.devspanner/config.yaml)
drives, so you can see devspanner work immediately:

```bash
go build -o devspanner .   # or: go install github.com/datamadsen/devspanner@latest
./devspanner               # run from anywhere inside this repo
```

devspanner finds the repo root (via `git`) and reads `.devspanner/config.yaml` there.
That config manages the two tiny services in this folder.

## What's here

| Path | What it is |
|------|------------|
| `api/` | A process-style service: a Go HTTP server on `:8080` with a `/health` endpoint and a heartbeat log. |
| `web/` | A process-style service: a static file server on `:5173` serving `web/public/`. |
| `scripts/build.sh` | A one-shot **task** (behind `c`) that compiles the api. |
| `scripts/deploy.sh` | A `confirm: true` **task** — a no-op "deploy" that shows the y/n prompt. |

Everything is standard-library Go (its own nested module, so it stays out of the main
project's build) and has no external dependencies.

## Things to try

- `↑/↓` to select a service, `r` to (re)start, `s` to stop, `l` to tail its log.
- Watch `backend` go `starting…` → `running` once `/health` answers.
- Edit `api/main.go` while it's running — the row flags `(code changed — press r)`.
- Press `o` on a service to open it in the browser; `g` opens the api health URL.
- Press `c` for the command palette, run **build**, then **deploy (demo)** to see the
  confirm prompt and streamed task output.

#!/usr/bin/env bash
# Demo "build" task — compiles the api service so you can see task output stream
# into devspanner's log view.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> building demo api"
go build -o bin/api ./api
echo "==> done: bin/api ($(du -h bin/api | cut -f1))"

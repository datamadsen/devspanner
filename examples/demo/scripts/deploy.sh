#!/usr/bin/env bash
# Demo "deploy" task — a harmless no-op that just prints steps. It's marked
# `confirm: true` in the config to show the y/n prompt before outward-facing tasks.
set -euo pipefail

echo "==> deploying devspanner-demo (no-op demo)"
for step in "packaging artifact" "uploading" "switching traffic"; do
  echo "    $step…"
  sleep 1
done
echo "==> deployed ✓"

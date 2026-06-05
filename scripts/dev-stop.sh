#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="${ROOT_DIR}/.tmp/dev/dev-watch.pid"

if [[ ! -f "${PID_FILE}" ]]; then
  echo "dev watcher is not running"
  exit 0
fi

PID="$(cat "${PID_FILE}")"
if kill -0 "${PID}" 2>/dev/null; then
  kill "${PID}"
  for _ in $(seq 1 20); do
    if ! kill -0 "${PID}" 2>/dev/null; then
      break
    fi
    sleep 0.2
  done
fi

rm -f "${PID_FILE}"
echo "stopped dev watcher"

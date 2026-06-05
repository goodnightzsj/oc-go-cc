#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="${ROOT_DIR}/.tmp/dev/dev-watch.pid"
LOG_FILE="${ROOT_DIR}/.tmp/dev/dev-watch.log"

if [[ ! -f "${PID_FILE}" ]]; then
  echo "dev watcher is not running"
  exit 0
fi

PID="$(cat "${PID_FILE}")"
if ! kill -0 "${PID}" 2>/dev/null; then
  echo "dev watcher PID file exists, but process is not running (PID ${PID})"
  exit 1
fi

echo "dev watcher is running (PID ${PID})"
echo "log: ${LOG_FILE}"

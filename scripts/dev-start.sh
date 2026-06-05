#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_HOME="${OC_GO_CC_DEV_HOME:-${HOME}/.oc-go-cc-devhome}"
RUN_DIR="${ROOT_DIR}/.tmp/dev"
PID_FILE="${RUN_DIR}/dev-watch.pid"
LOG_FILE="${RUN_DIR}/dev-watch.log"

mkdir -p "${RUN_DIR}" "${DEV_HOME}/.config/oc-go-cc"

if [[ -f "${PID_FILE}" ]]; then
  EXISTING_PID="$(cat "${PID_FILE}")"
  if kill -0 "${EXISTING_PID}" 2>/dev/null; then
    echo "dev watcher already running (PID ${EXISTING_PID})"
    echo "log: ${LOG_FILE}"
    exit 1
  fi
  rm -f "${PID_FILE}"
fi

: "${OC_GO_CC_API_KEY:?OC_GO_CC_API_KEY must be set}"

(
  cd "${ROOT_DIR}"
  export HOME="${DEV_HOME}"
  export OC_GO_CC_API_KEY
  setsid ./scripts/dev-watch.sh "$@" >"${LOG_FILE}" 2>&1 < /dev/null &
  echo $! >"${PID_FILE}"
)

echo "started dev watcher"
echo "  pid: $(cat "${PID_FILE}")"
echo "  home: ${DEV_HOME}"
echo "  log: ${LOG_FILE}"

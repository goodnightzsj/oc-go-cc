#!/usr/bin/env bash
set -euo pipefail
# Give each owned background job a process group, including its compiler children.
set -m

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/.tmp/dev"
DEV_BIN="${BUILD_DIR}/routatic-proxy"
WATCH_INTERVAL="${WATCH_INTERVAL:-1}"
source "${ROOT_DIR}/scripts/go-common.sh"

# Stay in the parent shell so signal traps do not wait for another source edit.
wait_for_change() {
  local baseline="$1"
  local hasher="$2"
  while true; do
    sleep "${WATCH_INTERVAL}" || return
    local next
    next="$(source_fingerprint "${ROOT_DIR}" "${hasher}" inputs)" || return
    if [[ "${next}" != "${baseline}" ]]; then
      LAST_FINGERPRINT="${next}"
      return 0
    fi
  done
}

GO_BIN="$(resolve_go_bin)"
HASHER="$(hash_cmd)"
VERSION_LDFLAGS="-X main.version=dev"
SERVER_PID=""
BUILD_PID=""

run_build() {
  (cd "${ROOT_DIR}" && exec "$@") &
  BUILD_PID=$!
  local status=0
  wait "${BUILD_PID}" || status=$?
  BUILD_PID=""
  return "${status}"
}

cleanup() {
  trap '' INT TERM
  if [[ -n "${BUILD_PID}" ]]; then
    kill -TERM -- "-${BUILD_PID}" 2>/dev/null || true
    local attempt
    for ((attempt=0; attempt<50; attempt++)); do
      kill -0 -- "-${BUILD_PID}" 2>/dev/null || break
      sleep 0.02 || break
    done
    kill -KILL -- "-${BUILD_PID}" 2>/dev/null || true
    wait "${BUILD_PID}" 2>/dev/null || true
  fi
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "${BUILD_DIR}"

echo "Using Go: ${GO_BIN}"
echo "Building to: ${DEV_BIN}"
echo "Watching for changes under: cmd/, internal/, pkg/, Go modules and CSS build inputs"

LAST_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}" inputs)"

while true; do
  echo
  echo "[dev] building CSS"
  if ! run_build bash "${ROOT_DIR}/scripts/build-css.sh"; then
    echo "[dev] CSS build failed; waiting for file changes" >&2
    wait_for_change "${LAST_FINGERPRINT}" "${HASHER}"
    continue
  fi
  # Keep the pre-CSS input baseline so edits anywhere in the build stay pending.
  echo "[dev] building routatic-proxy"
  if ! run_build env GOTOOLCHAIN=local "${GO_BIN}" build -ldflags "${VERSION_LDFLAGS}" -o "${DEV_BIN}" ./cmd/routatic-proxy; then
    echo "[dev] build failed; waiting for file changes"
    wait_for_change "${LAST_FINGERPRINT}" "${HASHER}"
    continue
  fi

  echo "[dev] starting routatic-proxy serve $*"
  (
    cd "${ROOT_DIR}"
    exec "${DEV_BIN}" serve "$@"
  ) &
  SERVER_PID=$!

  while true; do
    sleep "${WATCH_INTERVAL}"

    CURRENT_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}" inputs)"
    if [[ "${CURRENT_FINGERPRINT}" != "${LAST_FINGERPRINT}" ]]; then
      echo
      echo "[dev] source change detected; restarting"
      LAST_FINGERPRINT="${CURRENT_FINGERPRINT}"
      kill "${SERVER_PID}" 2>/dev/null || true
      wait "${SERVER_PID}" 2>/dev/null || true
      SERVER_PID=""
      break
    fi

    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
      wait "${SERVER_PID}" || true
      SERVER_PID=""
      echo
      echo "[dev] process exited; waiting for file changes before retry"
      wait_for_change "${LAST_FINGERPRINT}" "${HASHER}"
      break
    fi
  done
done

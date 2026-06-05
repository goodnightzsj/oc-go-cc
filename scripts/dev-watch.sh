#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/.tmp/dev"
DEV_BIN="${BUILD_DIR}/oc-go-cc"
WATCH_INTERVAL="${WATCH_INTERVAL:-1}"
source "${ROOT_DIR}/scripts/go-common.sh"

wait_for_change() {
  local baseline="$1"
  local hasher="$2"
  while true; do
    sleep "${WATCH_INTERVAL}"
    local next
    next="$(source_fingerprint "${ROOT_DIR}" "${hasher}")"
    if [[ "${next}" != "${baseline}" ]]; then
      printf '%s\n' "${next}"
      return 0
    fi
  done
}

GO_BIN="$(resolve_go_bin)"
HASHER="$(hash_cmd)"
VERSION_LDFLAGS="-X main.version=dev"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
}

trap cleanup EXIT INT TERM

mkdir -p "${BUILD_DIR}"

echo "Using Go: ${GO_BIN}"
echo "Building to: ${DEV_BIN}"
echo "Watching for changes under: cmd/, internal/, pkg/, go.mod, go.sum"

LAST_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"

while true; do
  echo
  echo "[dev] building oc-go-cc"
  if ! (
    cd "${ROOT_DIR}"
    GOTOOLCHAIN=local "${GO_BIN}" build -ldflags "${VERSION_LDFLAGS}" -o "${DEV_BIN}" ./cmd/oc-go-cc
  ); then
    echo "[dev] build failed; waiting for file changes"
    LAST_FINGERPRINT="$(wait_for_change "${LAST_FINGERPRINT}" "${HASHER}")"
    continue
  fi

  echo "[dev] starting oc-go-cc serve $*"
  (
    cd "${ROOT_DIR}"
    exec "${DEV_BIN}" serve "$@"
  ) &
  SERVER_PID=$!

  while true; do
    sleep "${WATCH_INTERVAL}"

    CURRENT_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"
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
      LAST_FINGERPRINT="$(wait_for_change "${LAST_FINGERPRINT}" "${HASHER}")"
      break
    fi
  done
done

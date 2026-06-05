#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WATCH_INTERVAL="${WATCH_INTERVAL:-1}"

source "${ROOT_DIR}/scripts/go-common.sh"

HASHER="$(hash_cmd)"
LAST_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"

echo "[prod-watch] watching cmd/, internal/, pkg/, go.mod, go.sum"

while true; do
  sleep "${WATCH_INTERVAL}"
  CURRENT_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"
  if [[ "${CURRENT_FINGERPRINT}" == "${LAST_FINGERPRINT}" ]]; then
    continue
  fi

  LAST_FINGERPRINT="${CURRENT_FINGERPRINT}"
  echo "[prod-watch] source change detected"
  if ! "${ROOT_DIR}/scripts/prod-deploy.sh"; then
    echo "[prod-watch] deploy failed; keeping previous healthy binary"
  fi
done

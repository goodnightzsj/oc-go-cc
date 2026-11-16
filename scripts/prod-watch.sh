#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WATCH_INTERVAL="${WATCH_INTERVAL:-1}"

source "${ROOT_DIR}/scripts/go-common.sh"

HASHER="$(hash_cmd)"
LAST_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}" inputs)"

echo "[prod-watch] watching cmd/, internal/, pkg/, Go modules and CSS build inputs"

while true; do
  sleep "${WATCH_INTERVAL}"
  CURRENT_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}" inputs)"
  if [[ "${CURRENT_FINGERPRINT}" == "${LAST_FINGERPRINT}" ]]; then
    continue
  fi

  LAST_FINGERPRINT="${CURRENT_FINGERPRINT}"
  echo "[prod-watch] source change detected"
  if ! bash "${ROOT_DIR}/scripts/build-css.sh"; then
    echo "[prod-watch] CSS build failed; waiting for source changes" >&2
    continue
  fi
  # Keep the pre-CSS input baseline; generated output must not consume edits.
  if "${ROOT_DIR}/scripts/prod-deploy.sh"; then
    continue
  else
    deploy_status=$?
    if [[ "${deploy_status}" == "75" ]]; then
      # The owner will release the lock; do not consume the pending source version.
      LAST_FINGERPRINT=""
      echo "[prod-watch] deployment busy; retrying on the next poll" >&2
    else
      echo "[prod-watch] deploy failed; inspect deploy output for rollback status" >&2
    fi
  fi
done

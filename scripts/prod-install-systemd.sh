#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UNIT_DIR="/etc/systemd/system"
ENABLE_WATCH_SERVICE="${OC_GO_CC_ENABLE_WATCH_SERVICE:-0}"

install -D -m 0644 "${ROOT_DIR}/ops/systemd/oc-go-cc.service" "${UNIT_DIR}/oc-go-cc.service"
install -D -m 0644 "${ROOT_DIR}/ops/systemd/oc-go-cc-watch.service" "${UNIT_DIR}/oc-go-cc-watch.service"

systemctl daemon-reload

if [[ "${ENABLE_WATCH_SERVICE}" == "1" ]]; then
  touch "${ROOT_DIR}/.enable-prod-watch"
else
  rm -f "${ROOT_DIR}/.enable-prod-watch"
  systemctl disable --now oc-go-cc-watch.service >/dev/null 2>&1 || true
fi

"${ROOT_DIR}/scripts/prod-deploy.sh"
systemctl enable --now oc-go-cc.service

if [[ "${ENABLE_WATCH_SERVICE}" == "1" ]]; then
  systemctl enable --now oc-go-cc-watch.service
fi

echo "installed systemd units and deployed current source build"

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UNIT_DIR="/etc/systemd/system"

install -D -m 0644 "${ROOT_DIR}/ops/systemd/oc-go-cc.service" "${UNIT_DIR}/oc-go-cc.service"
install -D -m 0644 "${ROOT_DIR}/ops/systemd/oc-go-cc-watch.service" "${UNIT_DIR}/oc-go-cc-watch.service"

systemctl daemon-reload
"${ROOT_DIR}/scripts/prod-deploy.sh"
systemctl enable --now oc-go-cc.service
systemctl enable --now oc-go-cc-watch.service

echo "installed systemd units and deployed current source build"

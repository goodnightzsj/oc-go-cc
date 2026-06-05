#!/usr/bin/env bash
set -euo pipefail

echo "== oc-go-cc.service =="
systemctl status oc-go-cc.service --no-pager
echo
echo "== oc-go-cc-watch.service =="
systemctl status oc-go-cc-watch.service --no-pager
echo
echo "== local health =="
curl -fsS http://127.0.0.1:3456/health

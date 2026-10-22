#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROD_DIR="${ROOT_DIR}/.tmp/prod"
RELEASES_DIR="${PROD_DIR}/releases"
CURRENT_LINK="${PROD_DIR}/current"
SERVICE_NAME="${OC_GO_CC_SERVICE_NAME:-oc-go-cc.service}"
HEALTH_URL="${OC_GO_CC_HEALTH_URL:-http://127.0.0.1:3456/health}"
KEEP_RELEASES="${OC_GO_CC_KEEP_RELEASES:-5}"
START_TIMEOUT="${OC_GO_CC_START_TIMEOUT:-30}"

source "${ROOT_DIR}/scripts/go-common.sh"

wait_for_health() {
  local deadline=$((SECONDS + START_TIMEOUT))
  while (( SECONDS < deadline )); do
    if systemctl is-active --quiet "${SERVICE_NAME}" && curl -fsS --max-time 2 "${HEALTH_URL}" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

prune_releases() {
  mapfile -t release_dirs < <(find "${RELEASES_DIR}" -mindepth 1 -maxdepth 1 -type d | sort)
  local count="${#release_dirs[@]}"
  if (( count <= KEEP_RELEASES )); then
    return 0
  fi

  local current_target=""
  if [[ -L "${CURRENT_LINK}" ]]; then
    current_target="$(readlink -f "${CURRENT_LINK}")"
  fi

  local to_remove=$((count - KEEP_RELEASES))
  for dir in "${release_dirs[@]}"; do
    if [[ -n "${current_target}" && "${dir}" == "${current_target}" ]]; then
      continue
    fi
    rm -rf "${dir}"
    to_remove=$((to_remove - 1))
    if (( to_remove == 0 )); then
      break
    fi
  done
}

GO_BIN="$(resolve_go_bin)"
HASHER="$(hash_cmd)"

# This script builds whatever is checked out here; it never fetches. That is a
# silent failure mode: a deploy started right after a push would rebuild the
# previous commit, report success, pass the health check, and leave the fix
# undeployed - which is exactly how one shipped fix was reported as live while
# the old binary was still serving. Staying quiet about a stale checkout is the
# problem, not a missing pull: whether to fetch is the operator's call, so this
# refuses instead of deciding for them.
if [[ "${OC_GO_CC_ALLOW_STALE:-0}" != "1" ]] && git -C "${ROOT_DIR}" rev-parse --git-dir >/dev/null 2>&1; then
	if git -C "${ROOT_DIR}" fetch --quiet origin 2>/dev/null; then
		head_sha="$(git -C "${ROOT_DIR}" rev-parse HEAD)"
		upstream_sha="$(git -C "${ROOT_DIR}" rev-parse '@{upstream}' 2>/dev/null || echo "${head_sha}")"
		if [[ "${head_sha}" != "${upstream_sha}" ]]; then
			echo "[prod] refusing to deploy: HEAD ${head_sha:0:12} is not its upstream ${upstream_sha:0:12}" >&2
			echo "[prod] run 'git -C ${ROOT_DIR} pull --ff-only' first, or set OC_GO_CC_ALLOW_STALE=1" >&2
			exit 1
		fi
	fi
fi

VERSION="$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"
STAMP="$(date +%Y%m%d%H%M%S)"
RELEASE_DIR="${RELEASES_DIR}/${STAMP}-${FINGERPRINT:0:12}"
BUILD_PATH="${RELEASE_DIR}/routatic-proxy"

mkdir -p "${RELEASES_DIR}"
rm -rf "${RELEASE_DIR}"
mkdir -p "${RELEASE_DIR}"

OLD_TARGET=""
if [[ -L "${CURRENT_LINK}" ]]; then
  OLD_TARGET="$(readlink -f "${CURRENT_LINK}")"
fi

echo "[prod] building ${BUILD_PATH}"
(
  cd "${ROOT_DIR}"
	GOTOOLCHAIN=local "${GO_BIN}" build \
		-ldflags "-X main.version=${VERSION} -X github.com/routatic/proxy/internal/buildinfo.Version=${VERSION} -X github.com/routatic/proxy/internal/buildinfo.Commit=${COMMIT} -X github.com/routatic/proxy/internal/buildinfo.Date=${BUILD_TIME}" \
		-o "${BUILD_PATH}" ./cmd/routatic-proxy
)

echo "[prod] syncing model catalog"
"${BUILD_PATH}" catalog sync

ln -sfn "${RELEASE_DIR}" "${CURRENT_LINK}"

echo "[prod] restarting ${SERVICE_NAME}"
if systemctl restart "${SERVICE_NAME}" && wait_for_health; then
  echo "[prod] deploy succeeded"
  prune_releases
  exit 0
fi

echo "[prod] deploy failed; rolling back" >&2
if [[ -n "${OLD_TARGET}" ]]; then
  ln -sfn "${OLD_TARGET}" "${CURRENT_LINK}"
  systemctl restart "${SERVICE_NAME}" || true
  wait_for_health || true
else
  rm -f "${CURRENT_LINK}"
  systemctl stop "${SERVICE_NAME}" || true
fi

exit 1

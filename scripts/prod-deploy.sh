#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
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
  local release_dirs=() dir
  for dir in "${RELEASES_DIR}"/*; do
    if [[ -d "${dir}" && ! -L "${dir}" ]]; then
      release_dirs+=("${dir}")
    fi
  done
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

rollback() {
  local failed=0
  echo "[prod] deploy failed; rolling back" >&2
  if [[ -n "${OLD_TARGET}" ]]; then
    if ! ln -sfn "${OLD_TARGET}" "${CURRENT_LINK}"; then
      echo "[prod] CRITICAL: rollback link restore failed" >&2
      failed=1
    fi
    if ! systemctl restart "${SERVICE_NAME}"; then
      echo "[prod] CRITICAL: rollback service restart failed" >&2
      failed=1
    fi
    if ! wait_for_health; then
      echo "[prod] CRITICAL: rollback health check failed" >&2
      failed=1
    fi
  else
    if ! rm -f "${CURRENT_LINK}"; then
      echo "[prod] CRITICAL: rollback candidate link removal failed" >&2
      failed=1
    fi
    if ! systemctl stop "${SERVICE_NAME}"; then
      echo "[prod] CRITICAL: rollback service stop failed" >&2
      failed=1
    fi
  fi
  if (( failed == 0 )); then
    echo "[prod] rollback succeeded" >&2
  fi
  return "${failed}"
}

cleanup() {
  local status=$?
  set +e # Diagnostic failures must not interrupt recovery or replace its exit status.
  trap - EXIT
  trap '' INT TERM
  if [[ "${NEEDS_ROLLBACK}" == 1 ]]; then
    if ! rollback; then
      echo "[prod] CRITICAL: rollback failed; manual recovery required" >&2
    fi
    if (( status == 0 )); then status=1; fi
  fi
  if ! rmdir -- "${DEPLOY_LOCK}"; then
    echo "[prod] CRITICAL: deployment lock cleanup failed" >&2
    if (( status == 0 )); then status=1; fi
  fi
  exit "${status}"
}

GO_BIN="$(resolve_go_bin)"
HASHER="$(hash_cmd)"

mkdir -p "${PROD_DIR}"
DEPLOY_LOCK="${PROD_DIR}/deploy.lock"
if ! mkdir "${DEPLOY_LOCK}" 2>/dev/null; then
  if [[ -d "${DEPLOY_LOCK}" ]]; then
    echo "[prod] refusing to deploy: deployment lock exists at ${DEPLOY_LOCK}; check the active deploy or stale lock" >&2
    exit 75 # Temporary contention; watchers must retry this source version.
  fi
  echo "[prod] refusing to deploy: cannot create deployment lock at ${DEPLOY_LOCK}" >&2
  exit 1
fi
# Fail closed after SIGKILL; remove a stale lock only after confirming no deploy owns it.
NEEDS_ROLLBACK=0
OLD_TARGET=""
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# This script builds this checkout; it never pulls or updates its files. That is a
# silent failure mode: a deploy started right after a push would rebuild the
# previous commit, report success, pass the health check, and leave the fix
# undeployed - which is exactly how one shipped fix was reported as live while
# the old binary was still serving. Staying quiet about a stale checkout is the
# problem, not a missing pull: whether to fetch is the operator's call, so this
# refuses instead of deciding for them.
if [[ "${OC_GO_CC_ALLOW_STALE:-0}" != "1" ]] && git -C "${ROOT_DIR}" rev-parse --git-dir >/dev/null 2>&1; then
	if fetch_err="$(git -C "${ROOT_DIR}" fetch --quiet origin 2>&1)"; then
		head_sha="$(git -C "${ROOT_DIR}" rev-parse HEAD)"
		upstream_sha="$(git -C "${ROOT_DIR}" rev-parse '@{upstream}' 2>/dev/null || echo "${head_sha}")"
		if [[ "${head_sha}" != "${upstream_sha}" ]]; then
			echo "[prod] refusing to deploy: HEAD ${head_sha:0:12} is not its upstream ${upstream_sha:0:12}" >&2
			echo "[prod] run 'git -C ${ROOT_DIR} pull --ff-only' first, or set OC_GO_CC_ALLOW_STALE=1" >&2
			exit 1
		fi
	else
		# A fetch that could not reach the remote proves nothing about this
		# checkout. Continuing here would rebuild whatever is checked out and
		# report success - the same silent outcome the check above exists to
		# prevent, reached by a different route (DNS, a proxy, an expired
		# credential). If the remote cannot be consulted, the deploy stops.
		echo "[prod] refusing to deploy: cannot reach origin to verify HEAD is current" >&2
		echo "[prod] ${fetch_err}" >&2
		echo "[prod] set OC_GO_CC_ALLOW_STALE=1 to deploy this checkout anyway" >&2
		exit 1
	fi
fi

# Guard inputs across both compilers, then include generated assets in the release hash.
INPUT_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}" inputs)"
echo "[prod] building CSS"
bash "${ROOT_DIR}/scripts/build-css.sh"

VERSION="$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"
STAMP="$(date +%Y%m%d%H%M%S)"
mkdir -p "${RELEASES_DIR}"
RELEASE_DIR="$(mktemp -d "${RELEASES_DIR}/${STAMP}-${FINGERPRINT:0:12}.XXXXXX")"
chmod 755 "${RELEASE_DIR}"
BUILD_PATH="${RELEASE_DIR}/routatic-proxy"

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

if ! POST_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}")"; then
  echo "[prod] refusing to deploy: cannot verify source fingerprint after build" >&2
  exit 1
fi
POST_INPUT_FINGERPRINT="$(source_fingerprint "${ROOT_DIR}" "${HASHER}" inputs)"
if [[ "${POST_FINGERPRINT}" != "${FINGERPRINT}" ]] ||
   [[ "${POST_INPUT_FINGERPRINT}" != "${INPUT_FINGERPRINT}" ]] ||
   [[ "$(git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || echo unknown)" != "${COMMIT}" ]]; then
  echo "[prod] refusing to deploy: source changed during build; retry from a stable checkout" >&2
  exit 1
fi

echo "[prod] syncing model catalog"
"${BUILD_PATH}" catalog sync

# The lock owner restores activation on failures/signals until health is confirmed.
NEEDS_ROLLBACK=1
ln -sfn "${RELEASE_DIR}" "${CURRENT_LINK}"

echo "[prod] restarting ${SERVICE_NAME}"
if systemctl restart "${SERVICE_NAME}" && wait_for_health; then
  NEEDS_ROLLBACK=0
  echo "[prod] deploy succeeded"
  prune_releases
  exit 0
fi

exit 1

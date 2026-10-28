#!/usr/bin/env bash
# Fixed-patch, isolated-source updates for an existing systemd installation.
# Required: WB2A_REPO. See README.md for paths, tools and rollback semantics.
set -Eeuo pipefail
umask 077

REPO=${WB2A_REPO:?set WB2A_REPO to the existing WorkBuddy checkout}
PATCH_FILE=${WB2A_PATCH_FILE:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/local.patch}
STATE_DIR=${WB2A_STATE_DIR:-/var/lib/workbuddy-updater}
GO_BIN=${WB2A_GO_BIN:-go}
SERVICE=${WB2A_SERVICE:-workbuddy2api.service}
HEALTH_URL=${WB2A_HEALTH_URL:-http://127.0.0.1:7863/healthz}
REF=${WB2A_REF:-main}
BUILD_TIMEOUT=${WB2A_BUILD_TIMEOUT:-900}
HEALTH_ATTEMPTS=${WB2A_HEALTH_ATTEMPTS:-30}
HEALTH_INTERVAL=${WB2A_HEALTH_INTERVAL:-1}
RUN=""
NEEDS_ROLLBACK=0
HAD_MANIFEST=0

log() {
    printf '[wb-update] %s\n' "$*" >&2
    if [[ -n "$RUN" ]]; then
        printf '[wb-update] %s\n' "$*" >> "$RUN/update.log"
    fi
}

fail() { log "ERROR: $*"; exit 1; }
run() { "$@" >> "$RUN/update.log" 2>&1; }
digest() { sha256sum "$1" | cut -d ' ' -f 1; }

wait_for_health() {
    local attempt code
    for ((attempt=1; attempt<=HEALTH_ATTEMPTS; attempt++)); do
        if systemctl is-active --quiet "$SERVICE" &&
            code=$(curl --silent --show-error --output "$RUN/health.json" --write-out '%{http_code}' --max-time 3 "$HEALTH_URL") &&
            [[ "$code" == 200 ]] && jq -e '.service == "workbuddy2api"' "$RUN/health.json" >/dev/null; then
            return 0
        fi
        if (( attempt < HEALTH_ATTEMPTS )); then
            sleep "$HEALTH_INTERVAL"
        fi
    done
    return 1
}

rollback() {
    local restored
    restored=$(mktemp "$REPO/bin/wb2api.rollback.XXXXXX") || return 1
    cp -p "$RUN/wb2api.previous" "$restored" || return 1
    mv -f "$restored" "$REPO/bin/wb2api" || return 1
    if (( HAD_MANIFEST )); then
        cp "$RUN/previous.manifest" "$STATE_DIR/restore.manifest" || return 1
        mv -f "$STATE_DIR/restore.manifest" "$STATE_DIR/current.manifest" || return 1
    elif [[ -f "$STATE_DIR/current.manifest" ]]; then
        mv "$STATE_DIR/current.manifest" "$RUN/failed.manifest" || return 1
    fi
    systemctl restart "$SERVICE" || return 1
    wait_for_health || return 1
    [[ $(digest "$REPO/bin/wb2api") == "$PREVIOUS_HASH" ]]
}

on_exit() {
    local status=$?
    trap - EXIT ERR
    trap '' INT TERM
    if (( NEEDS_ROLLBACK )); then
        if (( status == 0 )); then status=1; fi
        log "activation failed; restoring the previous binary and manifest"
        if rollback; then
            log "rollback verified: previous binary is active and healthy"
        else
            log "CRITICAL: rollback could not be verified; inspect $RUN before any retry"
        fi
    fi
    if (( status != 0 )); then
        log "update stopped (exit $status); candidate and diagnostics retained at ${RUN:-not-created}"
    fi
    exit "$status"
}

trap on_exit EXIT
trap 'log "command failed at line $LINENO"' ERR
trap 'exit 130' INT
trap 'exit 143' TERM

[[ "$REPO" == /* && "$REPO" != / ]] || fail "WB2A_REPO must be an absolute checkout path"
[[ "$STATE_DIR" == /* && "$STATE_DIR" != / ]] || fail "WB2A_STATE_DIR must be an absolute dedicated directory"
[[ "$BUILD_TIMEOUT" =~ ^[1-9][0-9]*$ && "$HEALTH_ATTEMPTS" =~ ^[1-9][0-9]*$ && "$HEALTH_INTERVAL" =~ ^[0-9]+$ ]] || fail "timeouts/attempts must be positive integers (interval may be zero)"
for command in git flock sha256sum cut systemctl curl timeout jq "$GO_BIN"; do
    command -v "$command" >/dev/null || fail "required executable not found: $command"
done
git -C "$REPO" rev-parse --git-dir >/dev/null
git check-ref-format --branch "$REF" >/dev/null
[[ -s "$PATCH_FILE" ]] || fail "fixed patch is missing or empty; it will never be captured from the working tree"
[[ -f "$REPO/bin/wb2api" && -x "$REPO/bin/wb2api" && ! -L "$REPO/bin/wb2api" ]] || fail "existing bin/wb2api must be a regular executable for rollback"
mkdir -p "$STATE_DIR/runs"
exec 9> "$STATE_DIR/update.lock"
flock -n 9 || fail "another updater holds the lock"
export GOCACHE=${GOCACHE:-$STATE_DIR/cache/build}
export GOMODCACHE=${GOMODCACHE:-$STATE_DIR/cache/mod}
RUN=$(mktemp -d "$STATE_DIR/runs/update.XXXXXX")
cp "$PATCH_FILE" "$RUN/local.patch"
PATCH_HASH=$(digest "$RUN/local.patch")
PREVIOUS_HASH=$(digest "$REPO/bin/wb2api")
log "fetching origin/$REF; production checkout and fixed patch remain untouched"
run git -C "$REPO" fetch --quiet origin "$REF"
SHA=$(git -C "$REPO" rev-parse 'FETCH_HEAD^{commit}')
printf 'source_sha=%s\npatch_sha256=%s\nbinary_sha256=%s\n' "$SHA" "$PATCH_HASH" "$PREVIOUS_HASH" > "$RUN/current.manifest"
if [[ -f "$STATE_DIR/current.manifest" ]] && cmp -s "$RUN/current.manifest" "$STATE_DIR/current.manifest"; then
    wait_for_health || fail "manifest matches, but the running service is not healthy"
    log "already deployed: source=$SHA patch=$PATCH_HASH binary=$PREVIOUS_HASH"
    exit 0
fi

run git clone --quiet --no-hardlinks --no-checkout "$REPO" "$RUN/source"
# Exact-SHA fetches need not be reachable from any cloned branch or tag.
run git -C "$RUN/source" fetch --quiet "$REPO" "$SHA"
run git -C "$RUN/source" checkout --quiet --detach "$SHA"
run git -C "$RUN/source" apply --check "$RUN/local.patch"
run git -C "$RUN/source" apply "$RUN/local.patch"
for guard in \
    'internal/upstream:TestResinProxy' \
    'cmd/server:TestLocalResinConfiguration' \
    'internal/upstream:TestLocalDeepSeekMaxFallback' \
    'internal/updater:TestLocalManagedUpdateCannotInstall'; do
    directory=${guard%:*}
    test_name=${guard#*:}
    grep -Eq "^func ${test_name}\\(" "$RUN/source/$directory/"*_test.go || fail "required patch guard missing: $guard"
done
log "candidate source=$SHA patch=$PATCH_HASH; checking vet, race, then build"
(
    cd "$RUN/source"
    run env GOTOOLCHAIN=local timeout "$BUILD_TIMEOUT" "$GO_BIN" vet ./...
    run env GOTOOLCHAIN=local timeout "$BUILD_TIMEOUT" "$GO_BIN" test -race ./... -count=1 -timeout=180s
    run env GOTOOLCHAIN=local timeout "$BUILD_TIMEOUT" "$GO_BIN" build -trimpath -ldflags '-s -w' -o "$RUN/wb2api" ./cmd/server
)
[[ -x "$RUN/wb2api" ]] || fail "build did not produce an executable"
BINARY_HASH=$(digest "$RUN/wb2api")
printf 'source_sha=%s\npatch_sha256=%s\nbinary_sha256=%s\n' "$SHA" "$PATCH_HASH" "$BINARY_HASH" > "$RUN/manifest"
[[ $(digest "$REPO/bin/wb2api") == "$PREVIOUS_HASH" ]] || fail "running binary changed during build; refusing to overwrite another deployment"
wait_for_health || fail "existing service is unhealthy; activation was not attempted"
cp -p "$REPO/bin/wb2api" "$RUN/wb2api.previous"
if [[ -f "$STATE_DIR/current.manifest" ]]; then
    cp "$STATE_DIR/current.manifest" "$RUN/previous.manifest"
    HAD_MANIFEST=1
fi
NEXT_BINARY=$(mktemp "$REPO/bin/wb2api.next.XXXXXX")
cp "$RUN/wb2api" "$NEXT_BINARY"
chmod 755 "$NEXT_BINARY"
NEEDS_ROLLBACK=1
mv -f "$NEXT_BINARY" "$REPO/bin/wb2api"
log "restarting $SERVICE with the validated candidate"
systemctl restart "$SERVICE"
wait_for_health || fail "candidate did not become active with HTTP 200"
cp "$RUN/manifest" "$STATE_DIR/next.manifest"
mv -f "$STATE_DIR/next.manifest" "$STATE_DIR/current.manifest"
NEEDS_ROLLBACK=0
log "deployed source=$SHA patch=$PATCH_HASH binary=$BINARY_HASH; previous version retained at $RUN/wb2api.previous"

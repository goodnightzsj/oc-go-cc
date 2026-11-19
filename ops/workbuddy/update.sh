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
KEEP_RUNS=${WB2A_KEEP_RUNS:-3}
RUN=""
ACTIVATION_STATE=candidate
HAD_MANIFEST=0
ROLLBACK_FAILED=0
CHECK_STAGE=""
CHECKED_AT=""

log() {
    printf '[wb-update] %s\n' "$*" >&2
    if [[ -n "$RUN" ]]; then
        if ! printf '[wb-update] %s\n' "$*" >> "$RUN/update.log"; then
            printf '[wb-update] ERROR: cannot append diagnostics to %s/update.log\n' "$RUN" >&2
            return 1
        fi
    fi
}

fail() { log "ERROR: $*"; exit 1; }
run() { "$@" >> "$RUN/update.log" 2>&1; }
digest() { sha256sum < "$1" | cut -d ' ' -f 1; }

write_check() {
    local result=$1 code=${2:-0} temporary
    temporary=$(mktemp "$STATE_DIR/.check.XXXXXX") || return 1
    if ! jq -n --arg checked_at "$CHECKED_AT" --arg updated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --arg stage "$CHECK_STAGE" --arg result "$result" --arg source_sha "${SHA:-}" \
        --arg patch_sha256 "${PATCH_HASH:-}" --argjson exit_code "$code" --argjson rollback_failed "$ROLLBACK_FAILED" \
        '{checked_at:$checked_at,updated_at:$updated_at,stage:$stage,result:$result,target_source_sha:$source_sha,target_patch_sha256:$patch_sha256,exit_code:$exit_code,rollback_failed:($rollback_failed == 1)}' > "$temporary"; then
        rm -f -- "$temporary"
        return 1
    fi
    if ! mv -f "$temporary" "$STATE_DIR/last-check.json"; then
        rm -f -- "$temporary"
        return 1
    fi
}

stage() { CHECK_STAGE=$1; write_check running; }

# Keep the newest KEEP_RUNS successful deployments and drop everything else.
# A run directory holds a full source clone plus the Go build output (~15 MB),
# so an hourly timer that keeps every attempt fills the disk: 494 runs reached
# 7.2 GB in 22 days. Only deployments are worth keeping - the source is
# reproducible and the new binary already lives in the repo. The run in progress
# is skipped so a success never deletes its own evidence.
prune_runs() {
    local keep=$KEEP_RUNS n=0 dir i
    local dirs=()
    for dir in "$STATE_DIR"/runs/update.*; do
        [[ -d "$dir" && ! -L "$dir" ]] || continue
        i=${#dirs[@]}
        while (( i > 0 )) && [[ "$dir" -nt "${dirs[i-1]}" ]]; do
            dirs[i]=${dirs[i-1]}
            i=$((i - 1))
        done
        dirs[i]=$dir
    done
    for ((i=0; i<${#dirs[@]}; i++)); do
        dir=${dirs[i]}
        if [[ -f "$dir/wb2api" ]]; then
            n=$((n + 1))
            if (( n <= keep )); then continue; fi
        fi
        # The run in progress counts toward the budget above but is never removed.
        if [[ "$dir" == "$RUN" ]]; then continue; fi
        rm -rf -- "$dir"
    done
}

# A failed candidate keeps nothing but its log: the clone and build output are
# reproducible, and the log is what an operator reads. Never called when the
# rollback itself failed - that case keeps every artifact for inspection.
prune_failed_run() {
    [[ -n "$RUN" && -d "$RUN" ]] || return 0
    if ! cp "$RUN/update.log" "$STATE_DIR/last-failure.log" 2>/dev/null; then
        printf '[wb-update] ERROR: cannot preserve the failure log; keeping %s\n' "$RUN" >&2
        return 0
    fi
    rm -rf -- "$RUN"
    RUN=""
}

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
    # Cleanup must not depend on diagnostic I/O; rollback checks every failure.
    set +e
    if [[ "$ACTIVATION_STATE" == activating ]]; then
        if (( status == 0 )); then status=1; fi
        log "activation failed; restoring the previous binary and manifest"
        if rollback; then
            log "rollback verified: previous binary is active and healthy"
        else
            ROLLBACK_FAILED=1
            log "CRITICAL: rollback could not be verified; inspect $RUN before any retry"
        fi
    fi
    if (( status != 0 )); then
        if [[ "$ACTIVATION_STATE" == deployed ]]; then
            log "post-deployment cleanup failed (exit $status); active version and rollback artifacts retained at $RUN"
        elif (( ROLLBACK_FAILED )) || [[ -z "$RUN" ]]; then
            log "update stopped (exit $status); candidate and diagnostics retained at ${RUN:-not-created}"
        else
            # Failed candidates are not kept: only the log is worth reading, and
            # the clone plus build output is what filled the disk. Retention here
            # is a single overwritten file, so a repeated failure cannot grow.
            prune_failed_run
            log "update stopped (exit $status); candidate removed, last log at $STATE_DIR/last-failure.log"
        fi
    fi
    if [[ -n "$CHECKED_AT" ]]; then
        local result=succeeded
        if (( status != 0 )); then result=failed; fi
        if ! write_check "$result" "$status"; then
            printf '[wb-update] ERROR: cannot publish managed check status\n' >&2
            if (( status == 0 )); then status=1; fi
        fi
    fi
    exit "$status"
}

trap on_exit EXIT
trap 'log "command failed at line $LINENO" || :' ERR
trap 'exit 130' INT
trap 'exit 143' TERM

[[ "$REPO" == /* && "$REPO" != / ]] || fail "WB2A_REPO must be an absolute checkout path"
[[ "$STATE_DIR" == /* && "$STATE_DIR" != / ]] || fail "WB2A_STATE_DIR must be an absolute dedicated directory"
[[ "$BUILD_TIMEOUT" =~ ^[1-9][0-9]*$ && "$HEALTH_ATTEMPTS" =~ ^[1-9][0-9]*$ && "$HEALTH_INTERVAL" =~ ^[0-9]+$ && "$KEEP_RUNS" =~ ^[1-9][0-9]*$ ]] || fail "timeouts/attempts/keep-count must be positive integers (interval may be zero)"
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
CHECKED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
stage prepare
export GOCACHE=${GOCACHE:-$STATE_DIR/cache/build}
export GOMODCACHE=${GOMODCACHE:-$STATE_DIR/cache/mod}
RUN=$(mktemp -d "$STATE_DIR/runs/update.XXXXXX")
cp "$PATCH_FILE" "$RUN/local.patch"
PATCH_HASH=$(digest "$RUN/local.patch")
PREVIOUS_HASH=$(digest "$REPO/bin/wb2api")
log "fetching origin/$REF; production checkout and fixed patch remain untouched"
stage fetch
run git -C "$REPO" fetch --quiet origin "$REF"
SHA=$(git -C "$REPO" rev-parse 'FETCH_HEAD^{commit}')
printf 'source_sha=%s\npatch_sha256=%s\nbinary_sha256=%s\n' "$SHA" "$PATCH_HASH" "$PREVIOUS_HASH" > "$RUN/current.manifest"
if [[ -f "$STATE_DIR/current.manifest" ]] && cmp -s "$RUN/current.manifest" "$STATE_DIR/current.manifest"; then
    stage health
    wait_for_health || fail "manifest matches, but the running service is not healthy"
    log "already deployed: source=$SHA patch=$PATCH_HASH binary=$PREVIOUS_HASH"
    # Hourly no-op checks would otherwise leave one directory each, forever.
    stage cleanup
    prune_runs
    exit 0
fi

stage checkout
run git clone --quiet --no-hardlinks --no-checkout "$REPO" "$RUN/source"
# Exact-SHA fetches need not be reachable from any cloned branch or tag.
run git -C "$RUN/source" fetch --quiet "$REPO" "$SHA"
run git -C "$RUN/source" checkout --quiet --detach "$SHA"
stage patch
run git -C "$RUN/source" apply --check "$RUN/local.patch"
run git -C "$RUN/source" apply "$RUN/local.patch"
for guard in \
    'internal/upstream:TestResinProxy' \
    'cmd/server:TestLocalResinConfiguration' \
    'internal/upstream:TestLocalDeepSeekMaxFallback' \
    'internal/upstream:TestLocalGlobalDeepSeekSGCatalog' \
    'internal/updater:TestLocalManagedStatus' \
    'internal/updater:TestLocalManagedUpdateCannotInstall'; do
    directory=${guard%:*}
    test_name=${guard#*:}
    grep -Eq "^func ${test_name}\\(" "$RUN/source/$directory/"*_test.go || fail "required patch guard missing: $guard"
done
log "candidate source=$SHA patch=$PATCH_HASH; checking vet, race, then build"
stage vet
(cd "$RUN/source"; run env GOTOOLCHAIN=local timeout "$BUILD_TIMEOUT" "$GO_BIN" vet ./...)
stage test
(cd "$RUN/source"; run env GOTOOLCHAIN=local timeout "$BUILD_TIMEOUT" "$GO_BIN" test -race ./... -count=1 -timeout=180s)
stage build
(cd "$RUN/source"; run env GOTOOLCHAIN=local timeout "$BUILD_TIMEOUT" "$GO_BIN" build -trimpath -ldflags '-s -w' -o "$RUN/wb2api" ./cmd/server)
[[ -x "$RUN/wb2api" ]] || fail "build did not produce an executable"
BINARY_HASH=$(digest "$RUN/wb2api")
printf 'source_sha=%s\npatch_sha256=%s\nbinary_sha256=%s\n' "$SHA" "$PATCH_HASH" "$BINARY_HASH" > "$RUN/manifest"
[[ $(digest "$REPO/bin/wb2api") == "$PREVIOUS_HASH" ]] || fail "running binary changed during build; refusing to overwrite another deployment"
stage health
wait_for_health || fail "existing service is unhealthy; activation was not attempted"
cp -p "$REPO/bin/wb2api" "$RUN/wb2api.previous"
if [[ -f "$STATE_DIR/current.manifest" ]]; then
    cp "$STATE_DIR/current.manifest" "$RUN/previous.manifest"
    HAD_MANIFEST=1
fi
NEXT_BINARY=$(mktemp "$REPO/bin/wb2api.next.XXXXXX")
cp "$RUN/wb2api" "$NEXT_BINARY"
chmod 755 "$NEXT_BINARY"
stage activate
ACTIVATION_STATE=activating
mv -f "$NEXT_BINARY" "$REPO/bin/wb2api"
log "restarting $SERVICE with the validated candidate"
systemctl restart "$SERVICE"
stage health
wait_for_health || fail "candidate did not become active with HTTP 200"
stage commit
cp "$RUN/manifest" "$STATE_DIR/next.manifest"
mv -f "$STATE_DIR/next.manifest" "$STATE_DIR/current.manifest"
ACTIVATION_STATE=deployed
log "deployed source=$SHA patch=$PATCH_HASH binary=$BINARY_HASH; previous version retained at $RUN/wb2api.previous"
stage cleanup
prune_runs

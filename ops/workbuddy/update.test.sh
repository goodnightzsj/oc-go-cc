#!/usr/bin/env bash
# Synthetic repositories and process substitutes only; never calls real systemd.
set -euo pipefail

case "${0##*/}" in
    git)
        if [[ "$1" == clone ]]; then
            shift
            # Copying all local loose objects can hide missing FETCH_HEAD objects.
            exec "$WB_TEST_REAL_GIT" clone --no-local "$@"
        fi
        exec "$WB_TEST_REAL_GIT" "$@"
        ;;
    go)
        if [[ "${GOCACHE:-}" != /* || "${GOMODCACHE:-}" != /* ]]; then
            printf 'Go requires absolute GOCACHE and GOMODCACHE without HOME\n' >&2
            exit 23
        fi
        printf 'GOCACHE=%s\nGOMODCACHE=%s\n' "$GOCACHE" "$GOMODCACHE" >> "$WB_TEST_CASE/go.cache"
        printf '%s\n' "$1" >> "$WB_TEST_CASE/go.calls"
        if [[ "$1" == "${WB_TEST_FAIL_GO:-}" ]]; then exit 1; fi
        if [[ "$1" == vet && -n "${WB_TEST_WAIT:-}" ]]; then
            touch "$WB_TEST_CASE/build.waiting"
            while [[ ! -f "$WB_TEST_CASE/build.continue" ]]; do sleep 0.1; done
        fi
        if [[ "$1" == build ]]; then
            output=""
            while (( $# )); do
                if [[ "$1" == -o ]]; then output=$2; break; fi
                shift
            done
            [[ -n "$output" ]]
            printf '#!/bin/sh\n# %s\nexit 0\n' "$(git hash-object feature.txt)" > "$output"
            chmod 755 "$output"
        fi
        exit 0
        ;;
    systemctl)
        printf '%s\n' "$*" >> "$WB_TEST_CASE/service.calls"
        [[ "${*: -1}" == workbuddy2api.service ]] || exit 9
        if [[ "$1" == restart ]]; then
            touch "$WB_TEST_CASE/restarted"
            if [[ -n "${WB_TEST_FAIL_SERVICE:-}" ]] && ! cmp -s "$WB2A_REPO/bin/wb2api" "$WB_TEST_CASE/original.binary"; then exit 1; fi
            if [[ -n "${WB_TEST_SIGNAL:-}" ]] && ! cmp -s "$WB2A_REPO/bin/wb2api" "$WB_TEST_CASE/original.binary"; then
                kill -"$WB_TEST_SIGNAL" "$PPID"
            fi
        fi
        exit 0
        ;;
    curl)
        output=""
        while (( $# )); do
            if [[ "$1" == --output ]]; then output=$2; break; fi
            shift
        done
        [[ -n "$output" ]]
        identity=workbuddy2api
        code=200
        if [[ -f "$WB_TEST_CASE/restarted" ]] && ! cmp -s "$WB2A_REPO/bin/wb2api" "$WB_TEST_CASE/original.binary"; then
            if [[ -n "${WB_TEST_FAIL_HEALTH:-}" ]]; then code=503; fi
            if [[ -n "${WB_TEST_BAD_IDENTITY:-}" ]]; then identity=other-service; fi
        fi
        printf '{"service":"%s"}\n' "$identity" > "$output"
        printf '%s' "$code"
        exit 0
        ;;
    flock) exit 0 ;;
    sha256sum) exec shasum -a 256 "$@" ;;
    timeout) shift; exec "$@" ;;
esac

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/wb-updater-test.XXXXXX")
printf 'Test artifacts: %s\n' "$TEST_ROOT"
REAL_PATH=$PATH
WB_TEST_REAL_GIT=$(command -v git)
export WB_TEST_REAL_GIT
PORTABLE=${WB2A_TEST_PORTABLE:-0}
for command in git jq; do command -v "$command" >/dev/null; done
if [[ "$PORTABLE" != 1 ]]; then
    for command in flock sha256sum timeout; do
        if ! command -v "$command" >/dev/null; then
            printf 'SKIP: %s unavailable; run on Linux, or WB2A_TEST_PORTABLE=1 for functional checks without OS locking/timeouts\n' "$command" >&2
            exit 77
        fi
    done
else
    printf 'SKIP: portable mode does not validate native flock or timeout\n' >&2
fi
mkdir "$TEST_ROOT/commands"
for command in git go systemctl curl; do ln -s "$SCRIPT_DIR/update.test.sh" "$TEST_ROOT/commands/$command"; done
if [[ "$PORTABLE" == 1 ]]; then
    for command in flock sha256sum timeout; do ln -s "$SCRIPT_DIR/update.test.sh" "$TEST_ROOT/commands/$command"; done
fi
export PATH="$TEST_ROOT/commands:$REAL_PATH"
export WB2A_GO_BIN="$TEST_ROOT/commands/go"
export WB2A_SERVICE=workbuddy2api.service
export WB2A_HEALTH_URL=http://synthetic.invalid/healthz
export WB2A_HEALTH_ATTEMPTS=2 WB2A_HEALTH_INTERVAL=0

assert() { "$@" || { printf 'FAIL: %s\n' "$*" >&2; exit 1; }; }

new_case() {
    export WB_TEST_CASE="$TEST_ROOT/$1"
    mkdir "$WB_TEST_CASE"
    unset WB_TEST_FAIL_GO WB_TEST_FAIL_SERVICE WB_TEST_FAIL_HEALTH WB_TEST_BAD_IDENTITY WB_TEST_WAIT WB_TEST_SIGNAL WB2A_REF GOCACHE GOMODCACHE
    export WB2A_REPO="$WB_TEST_CASE/production" WB2A_STATE_DIR="$WB_TEST_CASE/state" WB2A_PATCH_FILE="$WB_TEST_CASE/local.patch"
    git init -q -b main "$WB_TEST_CASE/upstream"
    git -C "$WB_TEST_CASE/upstream" config user.name synthetic
    git -C "$WB_TEST_CASE/upstream" config user.email synthetic@example.invalid
    for guard in \
        'internal/upstream:TestResinProxy' \
        'cmd/server:TestLocalResinConfiguration' \
        'internal/upstream:TestLocalDeepSeekMaxFallback' \
        'internal/updater:TestLocalManagedUpdateCannotInstall'; do
        mkdir -p "$WB_TEST_CASE/upstream/${guard%:*}"
        printf 'package fixture\nfunc %s(t *testing.T) {}\n' "${guard#*:}" > "$WB_TEST_CASE/upstream/${guard%:*}/${guard#*:}_test.go"
    done
    printf 'upstream\n' > "$WB_TEST_CASE/upstream/feature.txt"
    git -C "$WB_TEST_CASE/upstream" add .
    git -C "$WB_TEST_CASE/upstream" commit -qm initial
    git clone -q "$WB_TEST_CASE/upstream" "$WB2A_REPO"
    printf 'local patch\n' > "$WB_TEST_CASE/upstream/feature.txt"
    git -C "$WB_TEST_CASE/upstream" diff > "$WB2A_PATCH_FILE"
    printf 'upstream\n' > "$WB_TEST_CASE/upstream/feature.txt"
    mkdir "$WB2A_REPO/bin"
    printf '#!/bin/sh\n# original\nexit 0\n' > "$WB2A_REPO/bin/wb2api"
    chmod 755 "$WB2A_REPO/bin/wb2api"
    cp "$WB2A_REPO/bin/wb2api" "$WB_TEST_CASE/original.binary"
    cp "$WB2A_PATCH_FILE" "$WB_TEST_CASE/original.patch"
    printf 'keep-config\n' > "$WB2A_REPO/config.json"
    printf 'keep-data\n' > "$WB2A_REPO/state.json"
    git -C "$WB2A_REPO" rev-parse HEAD > "$WB_TEST_CASE/original.sha"
}

update() { bash "$SCRIPT_DIR/update.sh" > "$WB_TEST_CASE/run.log" 2>&1; }

unchanged() {
    assert cmp -s "$WB2A_REPO/bin/wb2api" "$WB_TEST_CASE/original.binary"
    assert cmp -s "$WB2A_PATCH_FILE" "$WB_TEST_CASE/original.patch"
    assert grep -qx keep-config "$WB2A_REPO/config.json"
    assert grep -qx keep-data "$WB2A_REPO/state.json"
    assert test "$(git -C "$WB2A_REPO" rev-parse HEAD)" = "$(< "$WB_TEST_CASE/original.sha")"
    assert git -C "$WB2A_REPO" diff --quiet
}

failed_update() {
    if update; then printf 'FAIL: update unexpectedly succeeded\n' >&2; exit 1; fi
    unchanged
    assert test ! -f "$WB2A_STATE_DIR/current.manifest"
    assert test -d "$WB2A_STATE_DIR/runs"
}

new_case no_home
env -u HOME -u GOPATH -u GOCACHE -u GOMODCACHE bash "$SCRIPT_DIR/update.sh" > "$WB_TEST_CASE/run.log" 2>&1
assert grep -qx "GOCACHE=$WB2A_STATE_DIR/cache/build" "$WB_TEST_CASE/go.cache"
assert grep -qx "GOMODCACHE=$WB2A_STATE_DIR/cache/mod" "$WB_TEST_CASE/go.cache"
printf 'PASS: Go cache paths are provided without HOME or GOPATH\n'

new_case explicit_cache
export GOCACHE="$WB_TEST_CASE/existing-build-cache" GOMODCACHE="$WB_TEST_CASE/existing-module-cache"
update
assert grep -qx "GOCACHE=$GOCACHE" "$WB_TEST_CASE/go.cache"
assert grep -qx "GOMODCACHE=$GOMODCACHE" "$WB_TEST_CASE/go.cache"
printf 'PASS: operator-provided Go caches are preserved\n'

new_case exact_sha
git -C "$WB2A_REPO" show-ref > "$WB_TEST_CASE/original.refs"
printf 'new upstream commit\n' > "$WB_TEST_CASE/upstream/new.txt"
git -C "$WB_TEST_CASE/upstream" add new.txt
git -C "$WB_TEST_CASE/upstream" commit -qm exact-sha-update
WB2A_REF=$(git -C "$WB_TEST_CASE/upstream" rev-parse HEAD)
export WB2A_REF
update
assert grep -qx "source_sha=$WB2A_REF" "$WB2A_STATE_DIR/current.manifest"
git -C "$WB2A_REPO" show-ref > "$WB_TEST_CASE/current.refs"
assert cmp -s "$WB_TEST_CASE/original.refs" "$WB_TEST_CASE/current.refs"
candidate_sources=("$WB2A_STATE_DIR"/runs/update.*/source)
assert test "$(git -C "${candidate_sources[0]}" rev-parse HEAD)" = "$WB2A_REF"
assert test -f "${candidate_sources[0]}/new.txt"
printf 'PASS: exact SHA fetch deploys without advancing production advertised refs\n'

new_case success
update
assert grep -q '^source_sha=' "$WB2A_STATE_DIR/current.manifest"
assert grep -q '^patch_sha256=' "$WB2A_STATE_DIR/current.manifest"
assert grep -q '^binary_sha256=' "$WB2A_STATE_DIR/current.manifest"
assert test "$(tr '\n' ' ' < "$WB_TEST_CASE/go.calls")" = 'vet test build '
assert test "$(git -C "$WB2A_REPO" rev-parse HEAD)" = "$(< "$WB_TEST_CASE/original.sha")"
assert cmp -s "$WB2A_PATCH_FILE" "$WB_TEST_CASE/original.patch"
update
assert test "$(wc -l < "$WB_TEST_CASE/go.calls" | tr -d ' ')" = 3
assert grep -q 'already deployed' "$WB_TEST_CASE/run.log"
cp "$WB2A_STATE_DIR/current.manifest" "$WB_TEST_CASE/first.manifest"
printf 'changed patch\n' > "$WB_TEST_CASE/upstream/feature.txt"
git -C "$WB_TEST_CASE/upstream" diff > "$WB2A_PATCH_FILE"
printf 'upstream\n' > "$WB_TEST_CASE/upstream/feature.txt"
update
assert test "$(wc -l < "$WB_TEST_CASE/go.calls" | tr -d ' ')" = 6
if cmp -s "$WB2A_STATE_DIR/current.manifest" "$WB_TEST_CASE/first.manifest"; then exit 1; fi
printf 'new upstream file\n' > "$WB_TEST_CASE/upstream/new.txt"
git -C "$WB_TEST_CASE/upstream" add new.txt
git -C "$WB_TEST_CASE/upstream" commit -qm upstream-update
update
assert grep -qx "source_sha=$(git -C "$WB_TEST_CASE/upstream" rev-parse HEAD)" "$WB2A_STATE_DIR/current.manifest"
assert test "$(wc -l < "$WB_TEST_CASE/go.calls" | tr -d ' ')" = 9
printf 'binary changed outside updater\n' >> "$WB2A_REPO/bin/wb2api"
update
assert test "$(wc -l < "$WB_TEST_CASE/go.calls" | tr -d ' ')" = 12
printf 'PASS: success, repeated version, patch-only/upstream updates and binary-hash verification\n'

new_case conflict
printf 'upstream changed\n' > "$WB_TEST_CASE/upstream/feature.txt"
git -C "$WB_TEST_CASE/upstream" commit -qam changed
failed_update
assert test ! -f "$WB_TEST_CASE/go.calls"
assert test ! -f "$WB_TEST_CASE/service.calls"
printf 'PASS: conflict preserves the fixed patch, checkout and running binary\n'

new_case missing_guard
rm "$WB_TEST_CASE/upstream/internal/upstream/TestResinProxy_test.go"
git -C "$WB_TEST_CASE/upstream" add -u
git -C "$WB_TEST_CASE/upstream" commit -qm removed-guard
failed_update
assert test ! -f "$WB_TEST_CASE/go.calls"
printf 'PASS: missing guard stops before build\n'

for stage in vet test build; do
    new_case "$stage-failure"
    export WB_TEST_FAIL_GO=$stage
    failed_update
    assert test ! -f "$WB_TEST_CASE/service.calls"
done
printf 'PASS: vet, race and build failures do not activate a candidate\n'

for failure in service health identity; do
    new_case "$failure-failure"
    case "$failure" in
        service) export WB_TEST_FAIL_SERVICE=1 ;;
        health) export WB_TEST_FAIL_HEALTH=1 ;;
        identity) export WB_TEST_BAD_IDENTITY=1 ;;
    esac
    failed_update
    assert grep -q 'rollback verified' "$WB_TEST_CASE/run.log"
    assert test "$(grep -c '^restart ' "$WB_TEST_CASE/service.calls")" = 2
done
printf 'PASS: restart failure, HTTP 503 and wrong service identity restore and verify the old binary\n'

for signal in INT TERM; do
    new_case "interrupted-$signal"
    export WB_TEST_SIGNAL=$signal
    failed_update
    assert grep -q 'rollback verified' "$WB_TEST_CASE/run.log"
    if [[ "$signal" == INT ]]; then code=130; else code=143; fi
    assert grep -q "exit $code" "$WB_TEST_CASE/run.log"
done
printf 'PASS: INT/TERM after switching roll back\n'

new_case manifest_rollback
update
cp "$WB2A_STATE_DIR/current.manifest" "$WB_TEST_CASE/previous.manifest"
cp "$WB2A_REPO/bin/wb2api" "$WB_TEST_CASE/original.binary"
printf 'next patch\n' > "$WB_TEST_CASE/upstream/feature.txt"
git -C "$WB_TEST_CASE/upstream" diff > "$WB2A_PATCH_FILE"
printf 'upstream\n' > "$WB_TEST_CASE/upstream/feature.txt"
cp "$WB2A_PATCH_FILE" "$WB_TEST_CASE/original.patch"
export WB_TEST_FAIL_HEALTH=1
if update; then exit 1; fi
unchanged
assert cmp -s "$WB2A_STATE_DIR/current.manifest" "$WB_TEST_CASE/previous.manifest"
assert grep -q 'rollback verified' "$WB_TEST_CASE/run.log"
printf 'PASS: failure restores the previous manifest as well as the binary\n'

if [[ "$PORTABLE" != 1 ]]; then
    new_case concurrent
    export WB_TEST_WAIT=1
    update &
    first=$!
    for ((attempt=0; attempt<100; attempt++)); do
        [[ -f "$WB_TEST_CASE/build.waiting" ]] && break
        sleep 0.1
    done
    assert test -f "$WB_TEST_CASE/build.waiting"
    if bash "$SCRIPT_DIR/update.sh" > "$WB_TEST_CASE/concurrent.log" 2>&1; then exit 1; fi
    assert grep -q 'another updater holds the lock' "$WB_TEST_CASE/concurrent.log"
    touch "$WB_TEST_CASE/build.continue"
    wait "$first"
    printf 'PASS: native flock excludes a concurrent updater\n'
fi
printf 'Updater checks passed; artifacts retained at %s\n' "$TEST_ROOT"

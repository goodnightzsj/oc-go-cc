# Installation

## Homebrew (macOS & Linux)

```bash
brew tap routatic/tap
brew install routatic-proxy
```

## Scoop (Windows)

```powershell
scoop bucket add routatic https://github.com/routatic/scoop-bucket
scoop install routatic-proxy
```

## Build from Source

Install Go as required by `go.mod` and Node.js 22 with npm. Run `npm ci` before `make build`, `make build-ui` or `make dist`. These targets stop on CSS compilation failure; they never fall back to stale styles. After editing UI sources, regenerate and commit `internal/gui/assets/compiled-tailwind.css`; `make check-css` checks it against Git, as CI and release validation do.

Direct `go build` and the Dockerfile embed the committed CSS without rebuilding it. They remain usable without Node, but do not validate UI source/asset consistency.

CSS staging uses `internal/gui/.compiled-tailwind.*` on the same filesystem, outside the embedded `assets/*` tree. Compilation failure or changed source inputs preserves the old stylesheet; output is renamed into place only after successful compilation and matching before/after input fingerprints.

`scripts/dev-watch.sh` rebuilds CSS before each Go build, so it also requires the local compiler installed by `npm ci`. CSS failure preserves the generated asset and waits for a source edit without building or starting a new process. Its input fingerprint excludes generated CSS and staging files and retains the pre-CSS baseline, so edits anywhere during CSS/Go compilation remain pending without restarting for its own output. Waiting propagates polling and fingerprint errors instead of spinning on failure.

INT/TERM stop the development watcher with status 130/143; EXIT owns child cleanup, so a later source edit cannot restart it. Waiting runs in the parent shell, without a command substitution delaying signal traps until another edit. Shared source fingerprinting also uses Node.js 22 via `scripts/source-fingerprint.mjs`: one streamed reader preserves filename bytes and the sorted path/content SHA-256 encoding, and propagates read errors instead of launching `cat` for every file. Keep this helper alongside `go-common.sh` when copying watcher/deployment scripts.

Builds run as owned background process groups: stopping the watcher cancels compiler descendants, waits about one second, then kills remaining workers and reaps the build job. Fingerprinting runs from the root selected by Bash, evaluates the helper without a filesystem entry path, and reads relative Buffer paths without passing the root through Node argv. Missing or empty helpers fail without returning an empty-stream hash; production deployment stops before building or creating a release. The existing LF-separated filename list is unchanged; newline-containing filenames are not newly supported. Linux byte-root tests are included; on macOS only controlled decoding-boundary tests can run.

`make lint` checks Go formatting and runs `go vet`. Formatting uses `scripts/check-go-format.sh`, shared with CI, release validation and the optional pre-push hook: it checks tracked and non-ignored untracked Go files, including names with spaces, but not ignored research copies. Enumeration or formatter errors fail the check; it never rewrites source files. The hook's existing protected-branch skip policy is unchanged.

Production hosts using `scripts/prod-deploy.sh` also need Node.js 22 and dependencies installed with `npm ci`. The script records an input fingerprint, rebuilds CSS, then fingerprints the full release inputs; a missing compiler, failed/empty output or changed inputs stops deployment without changing the running release. It does not install dependencies. The optional `scripts/prod-watch.sh` retains its pre-CSS input baseline across deployment, so generated assets do not trigger a duplicate restart and source edits remain pending.

Deployment is serialized by `.tmp/prod/deploy.lock` from before CSS compilation through health verification and pruning. Each release gets a unique directory and never overwrites an existing release. Normal exits release the lock; after SIGKILL or power loss, confirm no deployment is running before removing the empty stale lock with `rmdir`. Fingerprints include Tailwind config, npm manifests, the CSS build script and shared fingerprint helpers. After Go compilation, both the pre-CSS input fingerprint and the post-CSS full fingerprint must still match, along with HEAD, before catalog sync or service cutover. This checks a stable worktree, not an isolated source snapshot; publish from a fixed checkout. Successful pruning retains the current release and never follows release symlinks.

The deployment root uses its physical path so pruning also protects the current release when the checkout is accessed through a symlink.

After cutover begins and until health is confirmed, EXIT cleanup handles both ordinary failures and INT/TERM: it restores the old link and verifies the restarted service, or removes the candidate link and stops a failed first deployment. Recovery failures are reported as `CRITICAL`; the original failure/signal exit status is preserved and the lock is released last. Once health is confirmed, pruning failure or a signal does not roll back the healthy version. Watcher output no longer assumes that rollback succeeded.

Lock contention returns exit status `75`; `prod-watch.sh` retries that pending source version on its next poll. Other build/deployment failures still wait for a source edit. Failed source enumeration or reads return a nonzero fingerprint result, and the post-build check refuses deployment before catalog sync or cutover if it cannot read the fingerprint.

Linux CI and shared release validation run `WB2A_TEST_PORTABLE=0 bash ops/workbuddy/update.test.sh` after delivery checks. Missing native tools exit `77` and fail the gate; portable checks do not replace native locking/timeout validation.

```bash
git clone https://github.com/routatic/proxy.git
cd proxy
npm ci
make build

# Binary is at bin/routatic-proxy
# bin/oc-go-cc is created as a compatibility alias
# Optionally install to $GOPATH/bin
make install
```

## Download a Release Binary

Download the latest release for your platform from the [Releases page](https://github.com/routatic/proxy/releases):

| Platform              | File                         |
| --------------------- | ---------------------------- |
| macOS (Apple Silicon) | `routatic-proxy_darwin-arm64`      |
| macOS (Intel)         | `routatic-proxy_darwin-amd64`      |
| Linux (x86_64)        | `routatic-proxy_linux-amd64`       |
| Linux (ARM64)         | `routatic-proxy_linux-arm64`       |
| Windows (x86_64)      | `routatic-proxy_windows-amd64.exe` |
| Windows (ARM64)       | `routatic-proxy_windows-arm64.exe` |

```bash
# macOS Apple Silicon
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_darwin-arm64
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_windows-amd64.exe" -OutFile "routatic-proxy.exe"
Move-Item -Path "routatic-proxy.exe" -Destination "$env:LOCALAPPDATA\Microsoft\WindowsApps\routatic-proxy.exe"
```

Homebrew and Scoop installs also provide `oc-go-cc` as an alias for `routatic-proxy`.

## Docker

### Pull the prebuilt image

Prebuilt multi-arch images (linux/amd64, linux/arm64) are published to GitHub Container Registry:

```bash
# Latest stable release
docker pull ghcr.io/routatic/proxy:latest

# Latest beta (newest prerelease build)
docker pull ghcr.io/routatic/proxy:beta

# A specific stable release
docker pull ghcr.io/routatic/proxy:v1.0.0

docker run -d --restart unless-stopped --name routatic-proxy \
  --env-file .env -p 3456:3456 ghcr.io/routatic/proxy:latest
```

### Quick start with Makefile

```bash
cp .env.example .env
# Edit .env and put your API key
make docker-up
```

Stop the container:

```bash
make docker-stop
```

### Build and run manually

```bash
docker build -t routatic-proxy .
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 routatic-proxy
```

### Use a custom config

The Docker image uses `configs/config.json` by default (or `configs/config.example.json` as fallback). Override with a volume:

```bash
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 \
  -v /path/to/your/config.json:/etc/routatic-proxy/config.json:ro \
  routatic-proxy
```

## Requirements

- An [OpenCode Go](https://opencode.ai/auth) subscription and API key
- Go 1.21+ (only needed if building from source)
- Docker (only needed for Docker setup)

## Updating

If you installed via `go install` or downloaded a release binary directly, you can self-update with the built-in command:

```bash
# See whether a newer release is available without changing anything
routatic-proxy update --check

# Download, verify checksum, and replace the running binary in place
routatic-proxy update

# Skip the confirmation prompt (useful in scripts)
routatic-proxy update --yes
```

The updater queries the [routatic/proxy releases on GitHub](https://github.com/routatic/proxy/releases), picks the asset that matches your OS/arch, verifies its SHA256 against `checksums.txt` when available, and writes a `.old` backup of the previous binary next to the running executable before replacing it. On Windows the `.old` backup is scheduled for deletion after the process exits because the running executable is locked until then.

A `dev` build (e.g. when compiled from source without a version tag) refuses to update unless you pass `--force`.

If you installed via **Homebrew** (`brew upgrade routatic-proxy`) or **Scoop** (`scoop update routatic-proxy`), prefer your package manager — it tracks the same releases and handles uninstall/reinstall cleanly.

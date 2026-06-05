#!/usr/bin/env bash

resolve_go_bin() {
  if [[ -n "${OC_GO_CC_GO_BIN:-}" && -x "${OC_GO_CC_GO_BIN}" ]]; then
    printf '%s\n' "${OC_GO_CC_GO_BIN}"
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    local version
    version="$(GOTOOLCHAIN=local go version 2>/dev/null || true)"
    if [[ "${version}" =~ go1\.(25|26) ]]; then
      command -v go
      return 0
    fi
  fi

  if [[ -x "${HOME}/.local/go-current/bin/go" ]]; then
    printf '%s\n' "${HOME}/.local/go-current/bin/go"
    return 0
  fi

  if compgen -G "${HOME}/.local/go1.25*/bin/go" >/dev/null 2>&1; then
    ls -1d "${HOME}"/.local/go1.25*/bin/go | sort | tail -n1
    return 0
  fi

  echo "No Go 1.25+ toolchain found. Set OC_GO_CC_GO_BIN to a Go 1.25+ binary." >&2
  exit 1
}

hash_cmd() {
  if command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum"
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    echo "shasum -a 256"
    return 0
  fi
  echo "No SHA-256 command found (need sha256sum or shasum)." >&2
  exit 1
}

source_fingerprint() {
  local root_dir="$1"
  local hasher="$2"

  (
    cd "${root_dir}"
    find cmd internal pkg -type f -name '*.go' -print
    printf '%s\n' go.mod go.sum
  ) | sort | while IFS= read -r path; do
    [[ -f "${root_dir}/${path}" ]] || continue
    printf '%s\0' "${path}"
    cat "${root_dir}/${path}"
    printf '\0'
  done | eval "${hasher}" | awk '{print $1}'
}

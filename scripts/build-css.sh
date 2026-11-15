#!/usr/bin/env bash
# Rebuild the compiled Tailwind stylesheet for the GUI dashboard.
#
# The runtime only embeds internal/gui/assets/compiled-tailwind.css (via
# go:embed); tailwind-input.css holds the source @tailwind directives. This
# script regenerates the compiled output from source so style changes are
# reproducible instead of hand-committed one-offs.
#
# Requires Node and dependencies installed with npm ci. Never download a compiler
# implicitly. Output is minified and replaced only after successful compilation.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INPUT="${ROOT_DIR}/internal/gui/assets/tailwind-input.css"
CONFIG="${ROOT_DIR}/tailwind.config.js"
OUTPUT="${ROOT_DIR}/internal/gui/assets/compiled-tailwind.css"
TAILWIND="${ROOT_DIR}/node_modules/.bin/tailwindcss"
cd "${ROOT_DIR}"

if [[ ! -x "${TAILWIND}" ]]; then
  echo "[build-css] local tailwindcss not found; install Node and run 'npm ci' in the repo root." >&2
  exit 1
fi

CSS_TEMP=$(mktemp "${OUTPUT}.tmp.XXXXXX")
trap 'rm -f -- "${CSS_TEMP}"' EXIT
"${TAILWIND}" -c "${CONFIG}" -i "${INPUT}" -o "${CSS_TEMP}" -m
test -s "${CSS_TEMP}"
chmod 644 "${CSS_TEMP}"
mv -- "${CSS_TEMP}" "${OUTPUT}"

echo "[build-css] wrote $(wc -c < "${OUTPUT}") bytes to ${OUTPUT}"

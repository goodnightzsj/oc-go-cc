#!/usr/bin/env bash
# Rebuild the compiled Tailwind stylesheet for the GUI dashboard.
#
# The runtime only embeds internal/gui/assets/compiled-tailwind.css (via
# go:embed); tailwind-input.css holds the source @tailwind directives. This
# script regenerates the compiled output from source so style changes are
# reproducible instead of hand-committed one-offs.
#
# Requires a Go toolchain (optional) and node/npx with the tailwindcss dev
# dependency already installed (see package.json). Output is minified to keep
# the embedded asset small.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INPUT="${ROOT_DIR}/internal/gui/assets/tailwind-input.css"
CONFIG="${ROOT_DIR}/tailwind.config.js"
OUTPUT="${ROOT_DIR}/internal/gui/assets/compiled-tailwind.css"

# 1. Locate a Tailwind binary.
#    - Prefer npx (project devDependency, pin 3.4.x via package.json).
#    - Fall back to a global npx tailwindcss.
if ! command -v npx >/dev/null 2>&1; then
  echo "[build-css] npx not found; install node/npm first (see package.json)." >&2
  exit 1
fi

# 2. Generate minified CSS from the input directives.
#    -c: config scan pattern
#    -m: minify
if ! npx tailwindcss -c "${CONFIG}" -i "${INPUT}" -o "${OUTPUT}" -m 2>/dev/null; then
  echo "[build-css] tailwindcss failed. Run 'npm install' in the repo root first." >&2
  exit 1
fi

echo "[build-css] wrote $(wc -c < "${OUTPUT}") bytes to ${OUTPUT}"

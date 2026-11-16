#!/usr/bin/env bash
set -euo pipefail

source_root=$(git rev-parse --show-toplevel)
cd "$source_root"
source_list=$(mktemp)
trap 'rm -f -- "$source_list"' EXIT
git ls-files --cached --others --exclude-standard -z -- '*.go' > "$source_list"

files=()
while IFS= read -r -d '' file; do
    # A tracked deletion is not a formatter input; broken links must still fail.
    if [[ -e "$file" || -L "$file" ]]; then files+=("./$file"); fi
done < "$source_list"
if (( ${#files[@]} == 0 )); then
    printf 'No project Go source files found\n' >&2
    exit 1
fi
unformatted=$(gofmt -l "${files[@]}")
if [[ -n "$unformatted" ]]; then
    printf 'The following project files are not formatted:\n%s\n' "$unformatted" >&2
    exit 1
fi

#!/usr/bin/env bash
# Record the demo GIFs with VHS in Docker and copy them to docs/public.
# Only demo*.gif go to docs/public; the others (showcase) stay in docs/tapes.
#
# Usage: demo.sh [-e KEY=VAL ...] [NAME ...]
#   NAME  a tape to record, without .tape (demo, demo-apply, ...); all when none
#   -e    an environment variable for the recording containers
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tapes_dir="$root/docs/tapes"

names=()
args=("$@")
while [[ $# -gt 0 ]]; do
  case "$1" in
    -e) shift 2 ;;
    *) names+=("$1"); shift ;;
  esac
done

if ! docker info > /dev/null 2>&1; then
  echo "Docker is not running — please start Docker and try again." >&2
  exit 1
fi

echo "Building gh-infra for Linux..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$("$root/scripts/ldflags.sh")" -trimpath -o "$tapes_dir/.gh-infra" "$root/cmd/gh-infra/"
trap 'rm -f "$tapes_dir/.gh-infra"' EXIT

"$tapes_dir/vhs.sh" ${args[@]+"${args[@]}"}

if [[ ${#names[@]} -eq 0 ]]; then
  for tape in "$tapes_dir"/*.tape; do
    names+=("$(basename "$tape" .tape)")
  done
fi

echo "Copying assets to docs/public/..."
for name in "${names[@]}"; do
  case "$name" in demo*) cp "$tapes_dir/$name.gif" "$root/docs/public/" ;; esac
done

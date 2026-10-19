#!/usr/bin/env bash
# Print the -ldflags that stamp the version and revision into gh-infra.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
echo "-X main.version=$(cat "$root/VERSION") -X main.revision=$(git -C "$root" rev-parse --verify --short HEAD)"

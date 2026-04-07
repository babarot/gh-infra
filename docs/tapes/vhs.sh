#!/usr/bin/env bash
# Record all VHS tapes in parallel, then convert MP4→GIF sequentially.
#
# VHS's GIF generation is unreliable under parallel Docker on macOS.
# Workaround: let VHS produce MP4 (which works fine in parallel),
# then re-generate GIFs from MP4 using ffmpeg one at a time.
#
# Usage: vhs.sh [-e KEY=VAL ...]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VHS_IMAGE="ghcr.io/charmbracelet/vhs"
FFMPEG_IMAGE="jrottenberg/ffmpeg:7-alpine"

docker_flags=()
if [[ $# -gt 0 ]]; then
  docker_flags=("$@")
fi

tapes=("$SCRIPT_DIR"/*.tape)
if [[ ${#tapes[@]} -eq 0 ]]; then
  echo "No .tape files found"
  exit 0
fi

docker pull "$VHS_IMAGE"

echo "Recording ${#tapes[@]} tapes in parallel ..."

pids=()
names=()
for tape in "${tapes[@]}"; do
  name="$(basename "$tape" .tape)"
  names+=("$name")
  echo "  ▶ $name"
  docker run --rm \
    -v "$SCRIPT_DIR":/data \
    -w /data \
    ${docker_flags[@]+"${docker_flags[@]}"} \
    "$VHS_IMAGE" "$name.tape" > /dev/null 2>&1 &
  pids+=($!)
done

failures=0
for i in "${!pids[@]}"; do
  name="${names[$i]}"
  if wait "${pids[$i]}"; then
    echo "  ✓ $name"
  else
    echo "  ✗ $name"
    failures=$((failures + 1))
  fi
done

if [[ $failures -gt 0 ]]; then
  echo "ERROR: $failures tape(s) failed" >&2
  exit 1
fi

# Re-generate GIFs from MP4 sequentially (VHS GIF output is unreliable in parallel)
echo "Converting MP4 → GIF ..."
for name in "${names[@]}"; do
  mp4="$SCRIPT_DIR/$name.mp4"
  gif="$SCRIPT_DIR/$name.gif"
  if [[ -f "$mp4" && "$(stat -f%z "$mp4" 2>/dev/null || stat -c%s "$mp4")" -gt 0 ]]; then
    echo "  ▶ $name.gif"
    docker run --rm -v "$SCRIPT_DIR":/data -w /data "$FFMPEG_IMAGE" \
      -y -i "$name.mp4" \
      -vf "fps=10,scale=1200:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256[p];[s1][p]paletteuse=dither=sierra2_4a" \
      "$name.gif" > /dev/null 2>&1
    echo "  ✓ $name.gif"
  fi
done

echo "All ${#tapes[@]} tapes recorded."

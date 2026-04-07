---
name: vhs-demo
description: >
  Use when running demo recordings, diagnosing recording failures,
  or regenerating GIFs from existing MP4s. Covers the Docker + VHS + ffmpeg pipeline.
---

# VHS Demo Recording

Use this skill to run, debug, or regenerate gh-infra demo GIF recordings.

## When To Use

- Running `make demo` or `docs/tapes/vhs.sh` directly
- Diagnosing why a recording failed or produced a 0-byte GIF
- Regenerating GIFs from existing MP4s without re-recording
- Understanding the recording pipeline

## Prerequisites

- Docker must be running
- Go toolchain (for cross-compiling the Linux binary)

## Pipeline

```text
make demo
  1. go build -o docs/tapes/.gh-infra  (GOOS=linux GOARCH=amd64)
  2. docs/tapes/vhs.sh
     a. docker build → gh-infra-vhs image (VHS + vim)
     b. For each *.tape in parallel:
        docker run --memory=1g --cpus=2 → produces .mp4
     c. For each .mp4 sequentially:
        docker run jrottenberg/ffmpeg:7-alpine → produces .gif
  3. Copy GIFs to docs/public/
  4. Clean up .gh-infra binary
```

## Why MP4 → GIF Instead of Direct GIF

VHS's built-in GIF output is unreliable when multiple containers run in parallel on macOS. The workaround is to output MP4 only from VHS, then convert to GIF via ffmpeg with high-quality settings (lanczos scaling, sierra2_4a dithering, 256 colors).

## Key Files

| File | Role |
|------|------|
| `docs/tapes/vhs.sh` | Orchestrator: parallel recording + sequential GIF conversion |
| `docs/tapes/Dockerfile` | `ghcr.io/charmbracelet/vhs` + vim |
| `docs/tapes/*.tape` | VHS scenario files |
| `docs/tapes/setup*.sh` | Per-demo setup scripts (mock data, gh wrapper) |
| `docs/tapes/mock-gh` | Generic mock for `gh` CLI |
| `docs/tapes/.gh-infra` | Cross-compiled Linux binary (ephemeral) |

## Output Locations

- Raw recordings: `docs/tapes/*.mp4` and `docs/tapes/*.gif`
- Published assets: `docs/public/demo*.gif` (copied by Makefile)

## Environment Variables

`make demo` forwards `DEMO_ENV` variables into Docker via `-e` flags. Use this to pass environment overrides (e.g. `GH_INFRA_OUTPUT`) into the recording containers.

## Resource Planning

All tapes run in parallel. Each container requests `--memory` and `--cpus` (see `vhs.sh`). The total resource demand is:

```
total memory = number_of_tapes × per-container memory
total CPUs   = number_of_tapes × per-container CPUs
```

For example, 6 tapes × `--memory=2g --cpus=2` = 12 GB / 12 CPUs.

**This is constrained by Docker Desktop's resource allocation, not host RAM.** Docker Desktop defaults are often low (e.g. 7.6 GB on a 24 GB machine). If total demand exceeds Docker Desktop's allocation, containers will OOM or produce 0-byte outputs.

### When Adding New Tapes

Adding a tape increases parallel resource demand. Before adding, check:

1. Count existing tapes: `ls docs/tapes/*.tape | wc -l`
2. Calculate total: `count × per-container memory`
3. Compare against Docker Desktop memory allocation

If the total exceeds Docker Desktop's limit, you have two options:

- **Increase Docker Desktop memory** — Open Docker Desktop → Settings → Resources → Memory. On an M-series Mac with 24+ GB RAM, allocating 16 GB is safe and is the recommended approach since it's a one-time setting.
- **Reduce per-container resources** — Lower `--memory` in `vhs.sh`. This is a last resort since it may cause recording failures for complex tapes.

### Recommended Docker Desktop Settings

For comfortable parallel recording of 6+ tapes:

| Setting | Recommended |
|---------|-------------|
| Memory | 16 GB (minimum: number_of_tapes × 2 GB) |
| CPUs | 8+ |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| 0-byte GIF | MP4 was also 0-byte or missing | Check the tape's setup script and mock data |
| 0-byte MP4 | VHS crashed or setup script failed | Run the single tape manually: `docker run --rm -v docs/tapes:/data -w /data gh-infra-vhs <name>.tape` |
| `gh-infra: not found` in recording | Binary not copied or wrong arch | Verify `GOOS=linux GOARCH=amd64 go build` succeeded |
| Docker OOM | Container hit memory limit | Check Docker Desktop memory allocation (see Resource Planning above) |
| Multiple tapes fail simultaneously | Docker Desktop memory too low for parallel count | Increase Docker Desktop memory or reduce tape count |
| "Docker is not running" | Docker daemon not started | Start Docker Desktop or `dockerd` |

## Regenerating GIFs Only

To re-convert existing MP4s without re-recording, run the ffmpeg step manually:

```bash
docker run --rm -v docs/tapes:/data -w /data jrottenberg/ffmpeg:7-alpine \
  -y -i <name>.mp4 \
  -vf "fps=10,scale=1200:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256[p];[s1][p]paletteuse=dither=sierra2_4a" \
  <name>.gif
```

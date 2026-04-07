---
name: vhs-tape-authoring
description: >
  Use when creating new VHS demo tapes or editing existing ones.
  Covers tape syntax, setup scripts, mock-gh data layout, and the checklist
  for adding a new demo end-to-end.
---

# VHS Tape Authoring

Use this skill to create or edit VHS tape files and their supporting setup scripts for gh-infra demos.

## When To Use

- Adding a new demo for a gh-infra feature
- Editing an existing tape's scenario or timing
- Setting up mock data for a new demo
- Understanding how mock-gh works

## Tape File Structure

Every `.tape` follows this pattern:

```tape
# 1. Output targets
Output demo-<name>.gif
Output demo-<name>.mp4

# 2. Theme and layout
Set Theme { ... }
Set Shell "bash"
Set FontSize 20
Set Width 1400
Set Height 900
Set Padding 20
Set WindowBar ""
Set TypingSpeed 50ms

# 3. Hidden setup
Hide
Type "source /data/setup-<name>.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# 4. Visible demo sequence
Sleep 700ms
Type "gh infra ..."
Enter
Sleep 3s
# ... more actions ...
Sleep 2s
```

## Themes

Two themes are available. Copy from an existing tape:

| Theme | Based on | Use in |
|-------|----------|--------|
| `gh-infra-dark` | Tokyo Night | All tapes except light variant |
| `gh-infra-light` | Tokyo Night Day | `demo-light.tape` only |

## VHS Commands Reference

| Command | Usage | Notes |
|---------|-------|-------|
| `Type "text"` | Types text into terminal | Respects `TypingSpeed` |
| `Enter` | Press Enter | |
| `Escape` | Press Escape | |
| `Tab` | Press Tab | |
| `Sleep 1s` | Wait | Use `ms` or `s` units |
| `Hide` / `Show` | Toggle recording visibility | Setup runs in `Hide` |
| `Output <file>` | Set output file | Always set both `.gif` and `.mp4` |

## Timing Guidelines

| Action | Recommended Sleep |
|--------|-------------------|
| After `source setup-*.sh` | `2s` |
| After `clear` | `500ms` |
| Before first command | `700ms`-`1s` |
| After `Type` before `Enter` | `350ms`-`500ms` |
| After running `gh infra plan/apply` | `5s`-`8s` (depends on output size) |
| After confirmation (`y` + Enter) | `5s`-`6s` |
| End of demo | `2s`-`3s` |

## Setup Script Pattern

Each tape has a corresponding `setup-<name>.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. Install gh-infra binary
cp /data/.gh-infra /usr/local/bin/gh-infra
chmod +x /usr/local/bin/gh-infra

# 2. Create gh wrapper (routes "gh infra" to real binary, rest to mock)
cat > /usr/local/bin/gh << 'WRAPPER'
#!/usr/bin/env bash
if [[ "$1" == "infra" ]]; then
  shift
  exec /usr/local/bin/gh-infra "$@"
fi
exec /data/mock-gh "$@"
WRAPPER
chmod +x /usr/local/bin/gh

# 3. Populate mock data
export MOCK_DIR=/tmp/mock-data
mkdir -p "$MOCK_DIR/<owner>/<repo>"
cat > "$MOCK_DIR/<owner>/<repo>/view.json" << 'JSON'
{ ... }
JSON

# 4. Create demo working directory and manifests
mkdir -p /tmp/demo
cat > /tmp/demo/<manifest>.yaml << 'YAML'
...
YAML

# 5. Set prompt
export PS1='$ '
```

The `gh` wrapper is the key mechanism:
- `gh infra ...` → real `gh-infra` binary (spinners, diffs, colors are genuine)
- `gh repo view ...`, `gh api ...` etc. → `mock-gh` returns prepared data

## mock-gh Data Layout

`mock-gh` is data-driven and scenario-agnostic. It reads from `$MOCK_DIR`:

```
$MOCK_DIR/{owner}/{repo}/
  view.json                       # gh repo view --json response
  commit-settings.json            # squash/merge commit title/message (optional)
  contents/{path}                 # raw file content (base64-encoded on the fly)
```

### Supported mock responses

| gh command pattern | Data source | Default if missing |
|----|----|----|
| `repo view --json description,...` | `view.json` | Empty/default settings |
| `repo view --json defaultBranchRef` | hardcoded | `"main"` |
| REST `/actions/permissions` | hardcoded | `enabled:true, allowed_actions:all` |
| `/contents/{path}` (GET) | `contents/{path}` file | 404 (new file) |
| `secret list` | hardcoded | empty |
| `variable list` | hardcoded | `[]` |
| `repo edit`, write APIs | no-op | success with delay |

All mock responses include a random 200-500ms delay to simulate network latency.

## Checklist: Adding a New Demo

1. **Check resource headroom**: Adding a tape increases parallel resource demand. Count existing tapes (`ls docs/tapes/*.tape | wc -l`) and multiply by per-container memory (`--memory` in `vhs.sh`). If the total approaches or exceeds Docker Desktop's memory allocation, warn the user to either increase Docker Desktop memory (recommended: 16 GB) or consider reducing per-container resources. See the `vhs-demo` skill for details.

2. **Create the tape**: `docs/tapes/demo-<name>.tape`
   - Copy theme/layout from an existing tape
   - Set `Output demo-<name>.gif` and `Output demo-<name>.mp4`
   - Script the demo sequence

3. **Create the setup script**: `docs/tapes/setup-<name>.sh`
   - Make it executable: `chmod +x`
   - Populate `$MOCK_DIR` with the GitHub state the demo needs
   - Create manifest YAML in `/tmp/demo/`

4. **Update Makefile**: Add the new GIF to the `cp` line in the `demo` target:
   ```makefile
   @cp docs/tapes/demo-<name>.gif docs/public/ 2>/dev/null || true
   ```

5. **Test locally**: Run `make demo` or test the single tape:
   ```bash
   docker run --rm -v docs/tapes:/data -w /data gh-infra-vhs demo-<name>.tape
   ```

6. **Verify output**: Check that both `.mp4` and `.gif` are non-zero size

## Tips

- The `Hide`/`Show` block is critical. Viewers should never see the setup phase.
- Keep demos focused on one feature. Aim for under 30 seconds of visible recording.
- If mock-gh doesn't support an API your feature needs, extend `mock-gh` with a new pattern-match block.
- `Type` speed is global (`TypingSpeed 50ms`). For dramatic pauses, use `Sleep` between `Type` and `Enter`.

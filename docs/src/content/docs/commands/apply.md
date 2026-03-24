---
title: apply
sidebar:
  order: 3
---

Apply changes to GitHub. By default, requires interactive confirmation before proceeding.

```bash
gh infra apply [path]
```

## Path

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra apply` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra apply repos/gomi.yaml` | That file only |
| Directory | `gh infra apply repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |

YAML files that are not gh-infra manifests are silently skipped. Use `--fail-on-unknown` to treat them as errors.

## Flags

| Flag | Description |
|------|-------------|
| `-r, --repo <owner/repo>` | Target a specific repository |
| `--auto-approve` | Skip confirmation prompt |
| `--force-secrets` | Re-set all secrets (even existing ones) |
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |

## Interactive Diff Viewer

After the plan is displayed, the confirmation prompt offers three options:

```
> Do you want to apply these changes? (yes / no / diff)
```

Press `d` to open a full-screen diff viewer before deciding:

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Select file |
| `Tab` | Cycle `on_drift` (warn → overwrite → skip) |
| `Shift+Tab` | Cycle `on_drift` backwards |
| `d`/`u` | Scroll diff pane |
| `q`/`Esc` | Return to confirmation |

The diff viewer shows different content depending on the `on_drift` setting:

| `on_drift` | Right pane shows |
|------------|-----------------|
| `warn` | Unified diff (current → desired) |
| `overwrite` | Desired content in green |
| `skip` | Current content (will be kept as-is) |

Changing `on_drift` with `Tab` in the viewer takes effect for that apply run — the YAML file is not modified. This lets you decide per-file whether to apply, skip, or just warn without editing configuration.

When you return to the confirmation prompt, any overrides are shown as a summary:

```
  on_drift overrides (this run only):
    .gitignore: warn → overwrite
    go.mod: warn → skip

> Do you want to apply these changes? (yes / no / diff)
```

## Examples

```bash
# Apply all changes
gh infra apply ./repos/

# Apply without confirmation (for CI)
gh infra apply ./repos/ --auto-approve

# Force re-set secrets
gh infra apply ./repos/ --force-secrets

# Apply to a specific repository
gh infra apply ./repos/ --repo babarot/gomi
```

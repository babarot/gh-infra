---
title: apply
sidebar:
  order: 3
---

Apply changes to GitHub. By default, requires interactive confirmation before proceeding.

```bash
gh infra apply [path...]
```

## Path

One or more paths can be given. When multiple paths are provided, manifests from all paths are collected and applied together.

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra apply` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra apply repos/my-cli.yaml` | That file only |
| Directory | `gh infra apply repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |
| Multiple | `gh infra apply repos/ files/` | Manifests from all listed paths combined |

Overlapping paths (e.g., `.` and `./repos/`) are rejected to prevent duplicate processing.

YAML files that are not gh-infra manifests are silently skipped. Use `--fail-on-unknown` to treat them as errors.

## Flags

| Flag | Description |
|------|-------------|
| `-r, --repo <owner/repo>` | Target a specific repository |
| `--auto-approve` | Skip confirmation prompt |
| `--force-secrets` | Re-set all secrets (even existing ones) |
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |

## Exit status

`apply` exits with `1` if any change fails, or if the current state of a repository could not be fetched. Repositories that could not be fetched are left out of the plan and are not applied; the other repositories are still applied.

## Interactive Diff Viewer

After the plan is displayed, the confirmation prompt offers three options:

```
> Do you want to apply these changes? (y)es / (n)o / (d)iff
```

Press `d` to open a full-screen diff viewer before deciding:

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Select file |
| `Tab` | Toggle apply/skip for the selected file |
| `d`/`u` | Scroll diff pane |
| `q`/`Esc` | Return to confirmation |

Each file defaults to **apply**. Use `Tab` to toggle a file to **skip** if you want to exclude it from the current run. Skipped files are shown dimmed in the viewer. The YAML manifest is not modified — this is a runtime-only decision.

When you return to the confirmation prompt, any skipped files are shown as a summary:

```
  Skipped files (this run only):
    go.mod

> Do you want to apply these changes? (y)es / (n)o / (d)iff
```

## Examples

```bash
# Apply all changes
gh infra apply ./repos/

# Apply multiple directories at once
gh infra apply ./repos/ ./files/

# Apply without confirmation (for CI)
gh infra apply ./repos/ --auto-approve

# Force re-set secrets
gh infra apply ./repos/ --force-secrets

# Apply to a specific repository
gh infra apply ./repos/ --repo babarot/my-cli
```

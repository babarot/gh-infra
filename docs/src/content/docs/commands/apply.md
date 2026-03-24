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

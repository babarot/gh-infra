---
title: plan
sidebar:
  order: 2
---

Show diff between YAML and current GitHub state. No mutations are made.

```bash
gh infra plan [path]
```

## Path

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra plan` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra plan repos/gomi.yaml` | That file only |
| Directory | `gh infra plan repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |

YAML files that are not gh-infra manifests are silently skipped. Use `--fail-on-unknown` to treat them as errors.

## Flags

| Flag | Description |
|------|-------------|
| `-r, --repo <owner/repo>` | Target a specific repository |
| `--ci` | Exit with code 1 if changes detected (useful for CI drift detection) |
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |

## Examples

```bash
# Plan all YAML files in a directory
gh infra plan ./repos/

# Plan a single file
gh infra plan ./repos/gomi.yaml

# Plan a specific repository
gh infra plan ./repos/ --repo babarot/gomi

# CI drift detection
gh infra plan ./repos/ --ci
```

---
title: validate
sidebar:
  order: 1
---

Check YAML syntax and schema without contacting GitHub.

```bash
gh infra validate [path]
```

## Path

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra validate` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra validate repos/gomi.yaml` | That file only |
| Directory | `gh infra validate repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |

YAML files that are not gh-infra manifests are silently skipped. Use `--fail-on-unknown` to treat them as errors.

## Flags

| Flag | Description |
|------|-------------|
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |

## Examples

```bash
# Validate all files in a directory
gh infra validate ./repos/

# Validate a single file
gh infra validate ./repos/gomi.yaml
```

Exits with code 0 if all files are valid, non-zero otherwise.

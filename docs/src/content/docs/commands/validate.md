---
title: validate
sidebar:
  order: 1
---

Check YAML syntax and schema without contacting GitHub.

```bash
gh infra validate [path...]
```

## Path

One or more paths can be given. When multiple paths are provided, manifests from all paths are validated together.

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra validate` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra validate repos/my-cli.yaml` | That file only |
| Directory | `gh infra validate repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |
| Multiple | `gh infra validate repos/ files/` | Manifests from all listed paths combined |

Overlapping paths (e.g., `.` and `./repos/`) are rejected to prevent duplicate processing.

Checks that span manifests run on the combined set, e.g. two `pull_request` FileSets that would use the same branch on one repository are rejected (see [Pull request branch](../../resources/file/delivery/#pull-request-branch)).

YAML files that are not gh-infra manifests are silently skipped. Use `--fail-on-unknown` to treat them as errors.

## Flags

| Flag | Description |
|------|-------------|
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |

## Examples

```bash
# Validate all files in a directory
gh infra validate ./repos/

# Validate multiple directories at once
gh infra validate ./repos/ ./files/

# Validate a single file
gh infra validate ./repos/my-cli.yaml
```

Exits with code 0 if all files are valid, non-zero otherwise.

---
title: plan
sidebar:
  order: 2
---

Show diff between YAML and current GitHub state. No mutations are made.

```bash
gh infra plan [path...]
```

## Path

One or more paths can be given. When multiple paths are provided, manifests from all paths are collected and planned together.

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra plan` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra plan repos/my-cli.yaml` | That file only |
| Directory | `gh infra plan repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |
| Multiple | `gh infra plan repos/ files/` | Manifests from all listed paths combined |

Overlapping paths (e.g., `.` and `./repos/`) are rejected to prevent duplicate processing.

YAML files that are not gh-infra manifests are silently skipped. Use `--fail-on-unknown` to treat them as errors.

## Flags

| Flag | Description |
|------|-------------|
| `-r, --repo <owner/repo>` | Target a specific repository |
| `--detailed-exitcode` | Exit with `2` if changes are detected, `1` on error, `0` otherwise (see [Exit status](#exit-status)) |
| `--ci` | Deprecated: use `--detailed-exitcode`. Exits with `1` if changes are detected, and disables colors |
| `--changed-since <ref>` | Only the manifests changed since a git ref, and those that read a changed local file. See [Planning only what changed](../../patterns/ci-cd/#planning-only-what-changed) |
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |
| `--diff` | Show a unified diff for each FileSet file change, capped at 500 lines per file. Deleted files show no diff |
| `-o, --output <format>` | `text` (default), `json`, or `markdown`. See [JSON output](../json-output/) and [Markdown output](../markdown-output/) |

## Exit status

| Code | Meaning |
|------|---------|
| `0` | Plan computed for every repository (with `--detailed-exitcode`: and no changes) |
| `1` | An error, or the current state of one or more repositories could not be fetched. With `--detailed-exitcode`, also an interrupted plan and paths with no manifests |
| `2` | Changes detected (`--detailed-exitcode` only) |

An error wins over changes: a plan with changes that also skipped a repository exits with `1`. Without `--detailed-exitcode`, changes alone exit with `0`.

`--ci` is deprecated and will be removed in a future release. It exits with `1` for changes as well as errors, so a job cannot tell them apart; switch to `--detailed-exitcode`. `--ci` also disabled colors; set `NO_COLOR=1` for that.

When fetching fails for a repository (for example, a token without the required permissions), its error is shown and the repository is left out of the plan. A plan with skipped repositories is incomplete, so `plan` exits with `1` even if the other repositories have no changes. A file that cannot be fetched is never treated as missing, so no create is planned for it.

## Examples

```bash
# Plan all YAML files in a directory
gh infra plan ./repos/

# Plan multiple directories at once
gh infra plan ./repos/ ./files/

# Plan a single file
gh infra plan ./repos/my-cli.yaml

# Plan a specific repository
gh infra plan ./repos/ --repo babarot/my-cli

# CI drift detection (exit 2 when changes are detected)
gh infra plan ./repos/ --detailed-exitcode

# Plan only what changed on this branch
gh infra plan ./repos/ --changed-since origin/main

# Show the content changes of FileSet files
gh infra plan ./files/ --diff
```

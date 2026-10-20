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
| `--changed-since <ref>` | Only the manifests changed since a git ref, and those that read a changed local file. See [Only what changed](../../patterns/ci-cd/#only-what-changed) |
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |
| `--concurrency <n>` | Maximum number of `gh` processes (GitHub API calls) at once, across the whole command. Default `8`, or `GH_INFRA_CONCURRENCY`. Lower it if you hit GitHub's secondary rate limit or network timeouts |
| `--diff` | Show a unified diff for each FileSet file change, capped at 500 lines per file. Deleted files show no diff |
| `-o, --output <format>` | `text` (default), `json`, or `markdown`. See [JSON output](../json-output/) and [Markdown output](../markdown-output/) |

## Repositories with the same changes

Repositories whose changes read the same are printed once, with every name listed under the header and the counts of one repository:

```
  ~ 12 repositories                                         2 to update each
    acme/svc-auth  acme/svc-billing  acme/svc-catalog  acme/svc-checkout  acme/svc-gateway
    acme/svc-inventory  acme/svc-mailer  acme/svc-orders  acme/svc-payments  acme/svc-search
    acme/svc-shipping  acme/svc-users
    ! differs per repository: file contents
      ~ FileSet: 2 files, via pull_request
          ~ .github/workflows/ci.yaml  +2 -1
          ~ README.md                  +1 -1
```

The block is exactly what each repository would show on its own. Some of what `apply` does is not in the plan, though: file contents (without `--diff`), the bypass actors and rules of a ruleset (shown as `1 actors` and `enabled`), secret values, and the pull request branch and commit message of a FileSet, which templates can render per repository. When any of these differ among the repositories, the `!` line says which. With `--diff`, files whose contents differ show different diffs, so their repositories are not merged. A repository that is created or destroyed is always printed on its own. JSON and Markdown output list every repository separately.

## Exit status

| Code | Meaning |
|------|---------|
| `0` | Plan computed for every repository (with `--detailed-exitcode`: and no changes) |
| `1` | An error, or the current state of one or more repositories could not be fetched. With `--detailed-exitcode`, also an interrupted plan and paths with no manifests |
| `2` | Changes detected (`--detailed-exitcode` only) |

An error wins over changes: a plan with changes that also skipped a repository exits with `1`. Without `--detailed-exitcode`, changes alone exit with `0`.

`--ci` is deprecated and will be removed in a future release. It exits with `1` for changes as well as errors, so a job cannot tell them apart; switch to `--detailed-exitcode`. `--ci` also disabled colors; set `NO_COLOR=1` for that.

`plan` reads only the settings each manifest sets, so the token needs only their permissions (see [Authentication](../../patterns/ci-cd/#authentication)). When fetching fails for a repository (for example, the token cannot read a setting the manifest manages), its error is shown and the repository is left out of the plan; an unreadable setting is never compared as if it were absent or at its default. A plan with skipped repositories is incomplete, so `plan` exits with `1` even if the other repositories have no changes. A file that cannot be fetched is never treated as missing, so no create is planned for it. A repository that is already archived is reported and left out the same way when the manifest changes it without `archived: false`, since gh-infra does not change an archived repository except to unarchive it.

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

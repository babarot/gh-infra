---
title: Delivery Method
sidebar:
  order: 5
---

`via` controls how file changes are delivered to the target repository.

## `push` (default)

Commits directly to the default branch. All file changes are bundled into a single atomic commit.

```yaml
spec:
  via: push
  commit_message: "ci: sync shared files"  # optional, auto-generated if omitted
```

Use this for low-risk files (LICENSE, CODEOWNERS, security policies) or routine syncs of already-reviewed templates.

## `pull_request`

Creates a branch, commits all files, and opens a pull request for review before merging.

```yaml
spec:
  via: pull_request
  commit_message: "ci: sync shared files"  # optional, auto-generated if omitted
  branch: gh-infra/sync-shared             # optional, auto-generated if omitted
  pr_title: "Sync shared files"            # optional, defaults to commit_message
  pr_body: |                               # optional, supports Markdown
    Automated file sync by gh-infra.
    Updates CI workflows and shared config.
```

Use this when changes need review — for example, CI workflows or Dockerfiles that could break builds.

If a pull request already exists for the branch, gh-infra updates it instead of creating a new one.

## Related fields

| Field | Used by | Default | Description |
|---|---|---|---|
| `commit_message` | both | auto | Commit message for the sync commit |
| `branch` | `pull_request` | auto | Branch name for the pull request |
| `pr_title` | `pull_request` | value of `commit_message` | Pull request title |
| `pr_body` | `pull_request` | auto | Pull request body (supports Markdown) |

## Empty repositories

For repositories with no commits yet, gh-infra falls back to the Contents API regardless of the `via` setting. Each file becomes a separate commit. See [Git Data API vs Contents API](/internals/git-api/) for details.

---
title: Commit Strategy
sidebar:
  order: 4
---

The commit strategy controls **how** file changes are committed to the target repository.

## `push` (default)

Commits directly to the default branch using the Git Data API. All files are included in a single atomic commit.

```yaml
spec:
  commit_strategy: push
```

Use this when you trust the changes and want them applied immediately — for example, syncing a LICENSE or CODEOWNERS that doesn't need review.

## `pull_request`

Creates a branch, commits all files, and opens a pull request. Reviewers can inspect the diff before merging.

```yaml
spec:
  commit_strategy: pull_request
  commit_message: "ci: sync shared files"
  # branch: gh-infra/custom-branch   # optional, auto-generated if omitted
  pr_title: "Sync shared files from gh-infra"
  pr_body: |
    ## Summary

    Automated file sync by gh-infra.
    Updates shared configuration files from the gh-infra central config.

    ## Changed Files

    - CI workflows
    - Label definitions
    - PR templates

    ## Notes

    This PR was created automatically. Please review the diff before merging.
```

Use this when changes need review — for example, updating CI workflows that could break builds if something is wrong.

### Customizing the Pull Request

By default, the PR title is the `commit_message` and the body is an auto-generated description. You can override both:

| Field | Default | Description |
|---|---|---|
| `pr_title` | value of `commit_message` | Custom title for the pull request |
| `pr_body` | auto | Custom body/description for the pull request |

The `pr_body` field supports multi-line YAML (`|`) and Markdown formatting including headings, lists, and links. This is useful when you want a structured PR description that's different from the commit message.

## How to Choose

| Scenario | Recommended strategy |
|----------|---------------------|
| Updating LICENSE, CODEOWNERS, SECURITY.md | `push` — low risk, no review needed |
| Updating CI workflows, Dockerfiles | `pull_request` — changes could break things |
| Initial rollout to a repo | `pull_request` — lets the team review |
| Routine sync of already-reviewed templates | `push` — the template was already reviewed |

## Empty Repositories

For repositories with no commits yet, gh-infra automatically falls back to the Contents API regardless of the commit strategy setting. This creates one commit per file as the initial commit.

---
title: Git Data API vs Contents API
sidebar:
  order: 4
---

gh-infra uses two different GitHub APIs to write files, depending on the repository state.

## Git Data API (default)

For repositories with at least one commit, gh-infra uses the **Git Data API**. This API operates on Git's low-level objects (blobs, trees, commits, refs), which means **all file changes are bundled into a single atomic commit** regardless of how many files are modified.

The process:

1. Create a **blob** for each file's content
2. Create a **tree** containing all blobs (deletions use SHA=null)
3. Create a **commit** pointing to that tree
4. Update the default branch **ref** to the new commit (`via: push`), or create a new branch and open a PR (`via: pull_request`)

This applies to both `push` and `pull_request` — changes are always a single commit.

## Contents API (empty repository fallback)

Repositories with **no commits** (e.g. freshly created) cannot use the Git Data API because there is no existing HEAD to use as the `base_tree`. In this case, gh-infra automatically falls back to the **Contents API**.

The Contents API can only operate on one file per request, so **each file becomes a separate commit**. The `via` setting is ignored — all files are pushed directly to the default branch.

```
# Normal repository (Git Data API)
commit abc123: "chore: sync files via gh-infra"
  - .github/CODEOWNERS       (created)
  - .github/workflows/ci.yml (created)
  - LICENSE                   (created)

# Empty repository (Contents API fallback)
commit abc123: "chore: sync files: .github/CODEOWNERS"
commit def456: "chore: sync files: .github/workflows/ci.yml"
commit 789ghi: "chore: sync files: LICENSE"
```

This fallback is automatic — no user configuration is needed. After the first files are committed, all subsequent `apply` runs use the Git Data API as normal.

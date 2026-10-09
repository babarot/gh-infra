---
title: GraphQL Commit API vs Contents API
sidebar:
  order: 4
---

gh-infra uses two different GitHub APIs to write files, depending on the repository state.

## GraphQL `createCommitOnBranch` (default)

For repositories with at least one commit, gh-infra uses the **GraphQL `createCommitOnBranch` mutation**. This mutation commits all file additions and deletions atomically in a single API call, and GitHub automatically marks the resulting commit as **Verified** — no local GPG or SSH signing is required.

The process:

1. Get the HEAD SHA of the default branch
2. Send a `createCommitOnBranch` mutation with all file changes (additions as base64-encoded content, deletions by path)
3. For `via: push` — the mutation targets the default branch directly
4. For `via: pull_request` — gh-infra creates a branch first, then the mutation targets that branch, and a PR is opened

All file changes are bundled into a single atomic, verified commit regardless of how many files are modified. This applies to both `push` and `pull_request` delivery methods.

## Git Data API (file mode changes)

`createCommitOnBranch` cannot set file modes: it creates new files as `100644` and keeps the mode of existing files. When a commit has to change a mode (see [File modes](/resources/file/delivery/#file-modes)), gh-infra builds the commit with the Git Data API instead:

1. Create a tree on top of the HEAD tree, with the desired mode for each changed file. Files without `executable` keep their current mode
2. Create a commit with that tree and HEAD as its parent
3. Move the branch to the new commit (a non-fast-forward update is retried like a HEAD conflict)

Commits created this way are not signed, so they are not marked Verified, and branches that require signed commits reject them. Only commits that change a mode take this path.

## Contents API (empty repository fallback)

Repositories with **no commits** (e.g. freshly created) cannot use the GraphQL mutation because there is no existing HEAD. In this case, gh-infra automatically falls back to the **Contents API**.

The Contents API can only operate on one file per request, so **each file becomes a separate commit**. The `via` setting is ignored — all files are pushed directly to the default branch.

```
# Normal repository (GraphQL createCommitOnBranch)
commit abc123 ✓: "chore: sync files via gh-infra"
  - .github/CODEOWNERS       (created)
  - .github/workflows/ci.yml (created)
  - LICENSE                   (created)

# Empty repository (Contents API fallback)
commit abc123: "chore: sync files: .github/CODEOWNERS"
commit def456: "chore: sync files: .github/workflows/ci.yml"
commit 789ghi: "chore: sync files: LICENSE"
```

This fallback is automatic — no user configuration is needed. After the first files are committed, all subsequent `apply` runs use the GraphQL mutation as normal.

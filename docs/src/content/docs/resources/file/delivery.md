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
  branch: gh-infra/sync-shared             # optional, see below
  pr_title: "Sync shared files"            # optional, defaults to commit_message
  pr_body: |                               # optional, supports Markdown
    Automated file sync by gh-infra.
    Updates CI workflows and shared config.
```

Use this when changes need review — for example, CI workflows or Dockerfiles that could break builds.

If a pull request already exists for the branch, gh-infra updates it instead of creating a new one.

### Pull request branch

Without `branch`, the branch is `gh-infra/sync-<owner>-<name>`: `<name>` is the repository for `File` and `metadata.name` for `FileSet`. An unnamed `FileSet` uses its sorted repository names joined by `+`, e.g. `gh-infra/sync-babarot-gomi+gh-infra`.

Each apply resets the branch to the default branch before committing, so two `pull_request` resources that would use the same branch on the same repository are rejected by `validate`, `plan`, and `apply`. This happens when both set the same `branch`, or neither sets one and they share a name: two `File` manifests for one repository, or unnamed `FileSet`s that list the same repositories. Set a distinct `branch`, or a distinct `metadata.name` on a `FileSet`.

## Related fields

| Field | Used by | Default | Description |
|---|---|---|---|
| `commit_message` | both | auto | Commit message for the sync commit |
| `branch` | `pull_request` | `gh-infra/sync-<owner>-<name>` | Branch name for the pull request (see [Pull request branch](#pull-request-branch)) |
| `pr_title` | `pull_request` | value of `commit_message` | Pull request title |
| `pr_body` | `pull_request` | auto | Pull request body (supports Markdown) |

## File modes

Set `executable` on a file entry to control its executable bit, for example for git hooks and scripts:

```yaml
spec:
  files:
    - path: .githooks/pre-commit
      source: ./templates/pre-commit
      executable: true
```

| Value | Result |
|---|---|
| `true` | The file is committed with mode `100755` |
| `false` | The file is committed with mode `100644`, clearing an existing executable bit |
| *(omitted)* | An existing file keeps its current mode; a new file is created as `100644` |

`plan` compares the mode as well as the content, so adding `executable: true` to a file whose content is already in sync shows a mode change (`mode 100644 → 100755`). With `reconcile: create_only`, the mode is applied only when the file is created.

:::caution
GitHub's `createCommitOnBranch` mutation cannot change file modes. When a commit has to change a mode, gh-infra creates it with the Git Data API instead, and that commit is not signed (not Verified). `plan` notes this for each affected repository, and pushes to branches that require signed commits are rejected. Commits that do not change a mode still use `createCommitOnBranch` and stay Verified. See [GraphQL Commit API vs Contents API](/internals/git-api/).
:::

Only regular files can be marked: a symlink or submodule at the path is reported as an error. Executable files cannot be created in an empty repository, because the Contents API used there cannot set a mode; create the first commit without them.

## Empty repositories

For repositories with no commits yet, gh-infra falls back to the Contents API regardless of the `via` setting. Each file becomes a separate commit. See [GraphQL Commit API vs Contents API](/internals/git-api/) for details.

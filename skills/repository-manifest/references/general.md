# General

## Basic Fields

```yaml
spec:
  description: "My awesome project"
  homepage: "https://example.com"
  visibility: public
  archived: false
  topics: [go, cli]
```

- `visibility`: `public`, `private`, `internal`
- `archived`: reversible; set `false` to unarchive
- apply unarchives first and archives last (after the repo's other changes and its FileSets in the same run); if any of those fail, it does not archive and reports the archive as failed
- gh-infra never changes an already archived repo except to unarchive it (even where GitHub would accept the write, e.g. secrets, topics): such changes fail that repo at plan (`repository is archived (read-only) ...`), like a fetch failure: left out, others proceed, exit 1. Set `archived: false` to change it. FileSets to it are not caught at plan
- `topics`: full list, not additive merge. Omitted leaves current topics unchanged; `[]` removes all

## Creating a Repository

A manifest for a repository that does not exist plans `+ repository` and, under it, the rulesets, secrets, variables, labels and milestones; one apply creates it and applies all of them (settings, features, merge strategy, topics, security and Actions go with the create; `archived: true` last).

- GitHub's default labels (`bug`, `documentation`, ...; org-specific) exist in a new repository: a declared label with one of their names is updated, not created, and an omitted description stays as GitHub's
- with `reconcile.labels: authoritative`, undeclared default labels are deleted by the next plan/apply, not the creating one
- branch protection needs a branch, which a new repository lacks: the next plan after the branch exists adds it (rulesets don't need one)
- these notes appear under the repository in the text plan and as `warnings` with a `target` in `-o json` / markdown

## Features

```yaml
spec:
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
    pull_requests: true
```

`pull_requests` accepts bool or object:

```yaml
# Object form with creation restriction (implicitly enables PRs)
pull_requests:
  creation: collaborators_only  # "all" (default) or "collaborators_only"
```

- `pull_requests: true` enables PRs (default behavior)
- `pull_requests: false` disables the PR feature entirely
- Object form implicitly enables PRs; there is no `enabled` field
- In RepositorySet defaults, `pull_requests.creation` is independently overridable

## Merge Strategy

```yaml
spec:
  merge_strategy:
    allow_merge_commit: false
    allow_squash_merge: true
    allow_rebase_merge: false
    allow_auto_merge: true
    auto_delete_head_branches: true
    merge_commit_title: MERGE_MESSAGE
    merge_commit_message: PR_TITLE
    squash_merge_commit_title: PR_TITLE
    squash_merge_commit_message: COMMIT_MESSAGES
```

## Release Immutability

```yaml
spec:
  release_immutability: true
```

Use this to lock releases after publishing.

## Lifecycle

- Missing repository in GitHub: `plan` shows create, `apply` creates it
- Archiving is supported
- Deletion is not supported by gh-infra

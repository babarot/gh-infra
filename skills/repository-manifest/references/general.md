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
- `topics`: full list, not additive merge. Omitted leaves current topics unchanged; `[]` removes all

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

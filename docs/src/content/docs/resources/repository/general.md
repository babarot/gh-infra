---
title: General Settings
sidebar:
  order: 1
---

General settings cover the repository's basic properties, feature toggles, and pull request merge behavior.

## Basic Settings

```yaml
spec:
  description: "My awesome project"
  homepage: "https://example.com"
  visibility: public          # public | private | internal
  archived: false

  topics:
    - go
    - cli
    - github
```

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Repository description |
| `homepage` | string | URL displayed on the repo page |
| `visibility` | string | `public`, `private`, or `internal` (GitHub Enterprise) |
| `archived` | bool | `true` to archive (read-only). Reversible — set to `false` to unarchive |
| `topics` | list | GitHub topics for discoverability |

### Archiving

Set `archived: true` to mark a repository as read-only:

```yaml
spec:
  archived: true
```

This is reversible — set it back to `false` to unarchive.

## Features

Toggle repository features on or off:

```yaml
spec:
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `issues` | bool | `true` | Enable Issues tab |
| `projects` | bool | `true` | Enable Projects tab |
| `wiki` | bool | `true` | Enable Wiki tab |
| `discussions` | bool | `false` | Enable Discussions tab |

## Merge Strategy

Control how pull requests can be merged:

```yaml
spec:
  merge_strategy:
    allow_merge_commit: false
    allow_squash_merge: true
    allow_rebase_merge: false
    auto_delete_head_branches: true
    merge_commit_title: MERGE_MESSAGE            # MERGE_MESSAGE | PR_TITLE
    merge_commit_message: PR_TITLE               # PR_TITLE | PR_BODY | BLANK
    squash_merge_commit_title: PR_TITLE          # PR_TITLE | COMMIT_OR_PR_TITLE
    squash_merge_commit_message: COMMIT_MESSAGES # COMMIT_MESSAGES | PR_BODY | BLANK
```

| Field | Type | Description |
|-------|------|-------------|
| `allow_merge_commit` | bool | Allow merge commits |
| `allow_squash_merge` | bool | Allow squash merging |
| `allow_rebase_merge` | bool | Allow rebase merging |
| `auto_delete_head_branches` | bool | Automatically delete head branches after merge |
| `merge_commit_title` | string | Title format for merge commits |
| `merge_commit_message` | string | Message format for merge commits |
| `squash_merge_commit_title` | string | Title format for squash merges |
| `squash_merge_commit_message` | string | Message format for squash merges |

## Release Immutability

Prevent releases and their assets from being modified or deleted after publishing:

```yaml
spec:
  release_immutability: true
```

| Field | Type | Description |
|-------|------|-------------|
| `release_immutability` | bool | `true` to lock releases after publishing. Once enabled, release assets and metadata cannot be edited or deleted |

This setting uses a dedicated GitHub API endpoint (`/repos/{owner}/{repo}/immutable-releases`) rather than the standard repository settings endpoint.

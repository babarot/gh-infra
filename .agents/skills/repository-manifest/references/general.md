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
- `topics`: full list, not additive merge

## Features

```yaml
spec:
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
```

## Merge Strategy

```yaml
spec:
  merge_strategy:
    allow_merge_commit: false
    allow_squash_merge: true
    allow_rebase_merge: false
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

## Vulnerability Alerts

```yaml
spec:
  vulnerability_alerts: true
```

Enable Dependabot vulnerability alerts. Required when integrating with tools like Renovate's `osvVulnerabilityAlerts`. Uses PUT/DELETE on `/repos/{owner}/{repo}/vulnerability-alerts`; GitHub reports state via HTTP status (204 = enabled, 404 = disabled).

## Lifecycle

- Missing repository in GitHub: `plan` shows create, `apply` creates it
- Archiving is supported
- Deletion is not supported by gh-infra

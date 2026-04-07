# Reconcile And Delivery

## Reconcile

Modes:

- `patch`: create and update only
- `mirror`: create, update, and delete orphans under the managed directory
- `create_only`: create if missing, never update on apply

Examples:

```yaml
files:
  - path: .github/CODEOWNERS
    content: "* @platform-team"
```

```yaml
files:
  - path: .github/workflows
    source: ./templates/workflows/
    reconcile: mirror
```

```yaml
files:
  - path: VERSION
    content: "0.1.0"
    reconcile: create_only
```

## Delivery

`push`:

```yaml
spec:
  via: push
  commit_message: "ci: sync managed files"
```

`pull_request`:

```yaml
spec:
  via: pull_request
  branch: gh-infra/sync
  pr_title: "Sync shared files"
```

If the PR branch already exists, gh-infra updates that PR.

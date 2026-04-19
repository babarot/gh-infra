---
title: Labels
sidebar:
  order: 2
---

Manage repository labels declaratively. Labels declared in the manifest are created or updated on GitHub. Existing labels not listed in the manifest are left untouched by default.

```yaml
spec:
  labels:
    - name: kind/bug
      color: d73a4a
      description: "A bug; unintended behavior"
    - name: kind/feature
      color: "425df5"
      description: "A feature request"
    - name: priority/high
      color: ff0000
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Label name (must be unique within the list) |
| `color` | string | yes | Hex color code without `#` prefix (e.g., `d73a4a`) |
| `description` | string | no | Short description of the label's purpose |

## Reconcile Mode

By default, gh-infra only creates and updates labels. Labels on GitHub that are not in the manifest are left untouched. To delete unmanaged labels, set `reconcile.labels` to `authoritative`:

```yaml
reconcile:
  labels: authoritative  # additive (default) | authoritative

spec:
  labels:
    - name: kind/bug
      color: d73a4a
    - name: kind/feature
      color: "425df5"
```

| Value | Behavior |
|-------|----------|
| `additive` | Create and update only. Unmanaged labels are left in place. This is the default when `reconcile.labels` is omitted. |
| `authoritative` | Create, update, and **delete**. Labels on GitHub that are not in the manifest are removed. |

When `authoritative` mode marks labels for deletion, `plan` shows usage information (issue/PR count and when the label was last used) so you can verify before applying:

```
  ~ org/my-repo
      + labels.kind/bug               #d73a4a
      - labels.help wanted            #0e8a16 (42 issues/PRs, last used 3d ago)
      - labels.wontfix                #ffffff (0 issues/PRs)
```

:::caution
Authoritative mode deletes labels that are not in the manifest. Issues and PRs with deleted labels will lose those labels. Use `plan` to review before applying.
:::

To delete all labels, use `authoritative` with an empty label list:

```yaml
reconcile:
  labels: authoritative
spec:
  labels: []
```

:::note[Compatibility]
`spec.label_sync: mirror` is still accepted for existing manifests, but it is deprecated. Use top-level `reconcile.labels: authoritative` for new manifests.
:::

:::note[Planned]
**Aliases** (rename labels while preserving issue/PR associations) and **exclude patterns** (protect specific labels from authoritative deletion, e.g., `tagpr*`) are not yet available.
:::

## Replacing Label-Sync Actions

With `reconcile.labels: authoritative`, gh-infra can replace dedicated label synchronization GitHub Actions such as [EndBug/label-sync](https://github.com/EndBug/label-sync) and [crazy-max/ghaction-github-labeler](https://github.com/crazy-max/ghaction-github-labeler).

Instead of maintaining a separate workflow and label config file, define labels directly in your gh-infra manifest alongside other repository settings:

```yaml
# Before: separate workflow + label-definitions.yaml
# After: everything in one manifest
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-repo
  owner: my-org
reconcile:
  labels: authoritative
spec:
  labels:
    - name: kind/bug
      color: d73a4a
      description: "A bug; unintended behavior"
    - name: kind/feature
      color: "425df5"
      description: "A feature request"
  rulesets:
    # ... other settings managed in the same file
```

A workflow that applies on push looks like this:

```yaml
# .github/workflows/infra.yaml
on:
  push:
    branches: [main]
    paths: [repos/**]
  workflow_dispatch:

jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: gh infra apply ./repos/
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

`gh` CLI is [preinstalled on all GitHub-hosted runners](https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/using-github-cli-in-workflows), so no additional setup is needed.

Benefits over dedicated Actions:

- **Single source of truth**: labels, rulesets, secrets, and other settings live in one manifest.
- **Better preview**: `plan` shows a full diff with usage stats before applying.
- **One fewer third-party Action**: no additional SHA pin to maintain.

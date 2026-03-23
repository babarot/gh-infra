---
title: File
sidebar:
  label: Overview
  order: 0
---

`File` manages **files** in a **single** repository — CODEOWNERS, LICENSE, CI workflows, and any other files you want to keep in a declared state.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: babarot
  name: gomi

spec:
  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot

    - path: go.mod
      content: |
        module github.com/<% .Repo.FullName %>
        go 1.24.0

    - path: LICENSE
      source: ./templates/LICENSE

  on_drift: warn
  strategy: direct
  commit_message: "ci: sync managed files"
```
:::

## File vs FileSet

| Kind | Scope | metadata |
|------|-------|----------|
| **File** | 1 repo | `owner` + `name` (identifies the target repo) |
| **FileSet** | N repos | `owner` (repos listed in `spec.repositories`) |

Use `File` when you manage files for a specific repository. Use `FileSet` when you distribute the same files across multiple repositories.

## Metadata

`File` uses the same metadata pattern as `Repository`:

```yaml
metadata:
  owner: babarot    # GitHub owner or organization
  name: gomi        # Repository name
```

The combination of `owner` and `name` identifies the target repository (`babarot/gomi`).

## Spec

The spec is the same as `FileSet` but without `repositories` — the target repo is already identified by the metadata.

| Field | Default | Description |
|-------|---------|-------------|
| `files` | *(required)* | List of files to manage |
| `on_drift` | `warn` | How to handle drift: `warn`, `overwrite`, or `skip` |
| `strategy` | `direct` | Apply strategy: `direct` or `pull_request` |
| `commit_message` | auto | Custom commit message |
| `branch` | auto | Branch name for `pull_request` strategy |

For details on file sources, templating, drift handling, and apply strategies, see the corresponding [FileSet documentation](../fileset/) — these features work identically for both resource kinds.

## Internal Behavior

At parse time, `File` is expanded into a `FileSet` with a single repository entry. All downstream processing (plan, apply, templating, drift detection) uses the same code path. This means every feature available in `FileSet` works in `File` as well.

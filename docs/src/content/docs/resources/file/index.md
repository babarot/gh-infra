---
title: File
sidebar:
  label: Overview
  order: 0
---

`File` manages **files** in a **single** repository — CODEOWNERS, LICENSE, CI workflows, security policies, and any other files you want to keep in a declared state.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: babarot
  name: my-cli

spec:
  files:
    # Inline content
    - path: .github/CODEOWNERS
      content: |
        * @babarot

    # Inline content with templating (<% %> expanded per repo)
    - path: go.mod
      content: |
        module github.com/<% .Repo.FullName %>
        go 1.24.0

    # From local file
    - path: LICENSE
      source: ./templates/LICENSE

    # From GitHub repository
    - path: .github/workflows/ci.yml
      source: github://babarot/shared-config/workflows/ci.yml

  via: push                               # push | pull_request
  commit_message: "ci: sync managed files"
  # pr_title: "chore: sync files"         # pull_request only
  # pr_body: |                            # pull_request only
  #   ## Summary
  #   Automated file sync.
```
:::

## Metadata

```yaml
metadata:
  owner: babarot    # GitHub owner or organization
  name: my-cli        # Repository name
```

The combination of `owner` and `name` identifies the target repository (`babarot/my-cli`).

## Spec

| Field | Default | Description |
|---|---|---|
| `files` | *(required)* | List of files to manage — see [File Sources](./sources/) |
| `files[].patches` | *(none)* | List of unified diff patches (file paths or inline) applied after template expansion — see [Patches](./patches/). |
| `files[].reconcile` | `additive` | Per-entry reconcile mode: `additive` (add/update), `authoritative` (add/update/delete), or `create_only` (create if missing, never update) — see [Reconcile](./reconcile/). |
| `via` | `push` | Delivery method: `push` or `pull_request` — see [Delivery Method](./delivery/). |
| `commit_message` | auto | Custom commit message |
| `branch` | auto | Branch name for `pull_request` mode |
| `pr_title` | `commit_message` | Custom PR title (`pull_request` only) |
| `pr_body` | auto | Custom PR body (`pull_request` only) |

File content supports `<% %>` template syntax for per-repo customization — see [Templating](./templating/).

## When to Use

Use `File` when you want to manage files in one repo's YAML file. This is the simplest file management resource and the starting point for most users.

It works best when:

- **Each repo has its own distinct files** — different CODEOWNERS, different CI workflows, different licenses.
- **You want per-repo change tracking** — each file maps to one repo, so `git blame` tells you exactly who changed what and when.
- **You're managing files in a small number of repos** — for 1–5 repos, separate files are easy to maintain.

If you find yourself copying the same files across many repos, consider [FileSet](../fileset/) instead.

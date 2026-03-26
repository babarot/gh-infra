---
title: FileSet
sidebar:
  label: Overview
  order: 0
---

`FileSet` distributes **files** to **multiple** repositories with shared defaults. Use this when you have many repos that should contain identical files and want to avoid repeating the same configuration in every file. Each repository in the set receives the same files and can override specific ones.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot

spec:
  repositories:
    - gomi
    - enhancd
    - name: gh-infra
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @babarot @co-maintainer

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

  via: push
  commit_message: "ci: sync shared files"
```
:::

## Metadata

```yaml
metadata:
  owner: babarot    # GitHub owner or organization
```

All repositories in the set belong to this owner. Individual repo names are listed in `spec.repositories`.

## Shared Features

All settings available in [File](../file/) — file sources, templating, reconcile modes, and delivery methods — work identically in `FileSet`. See the File documentation for details:

- [File Sources](../file/sources/) — Inline content, local files, directories, and `github://` references
- [Templating](../file/templating/) — `<% %>` syntax, built-in variables, custom vars
- [Patches](../file/patches/) — Unified diff patches for per-repo customization of shared templates
- [Reconcile](../file/reconcile/) — `patch` (add/update) vs `mirror` (add/update/delete orphans)
- [Delivery Method](../file/delivery/) — `push` vs `pull_request`

## When to Use FileSet

### The Problem

Suppose you manage 20 repositories that all need the same CODEOWNERS, LICENSE, and CI workflows. With individual `File` resources, you'd repeat those file definitions in every manifest:

```
repos/
├── gomi-files.yaml          # CODEOWNERS, LICENSE, ci.yml...
├── enhancd-files.yaml       # same CODEOWNERS, LICENSE, ci.yml...
├── oksskolten-files.yaml    # same CODEOWNERS, LICENSE, ci.yml...
└── ... (17 more files with the same content)
```

When you need to change a shared file — say, update the CODEOWNERS to add a new team member — you have to edit all 20 files. Miss one, and your repos drift out of sync.

### The Solution

`FileSet` solves this by declaring files once and distributing them to all target repositories. A single manifest replaces 20:

```yaml
spec:
  repositories:
    - gomi
    - enhancd
    - oksskolten
    # ... 17 more repos

  files:
    # Change once, applies to all 20 repos
    - path: .github/CODEOWNERS
      content: |
        * @babarot @new-team-member
```

### When Not to Use It

`FileSet` isn't always the right choice. Use separate `File` resources when:

- **Each repo has mostly unique files** — the shared `files` block would be nearly empty, so there's no benefit.
- **You need clean per-repo git blame** — with `FileSet`, all repos share one manifest, so `git blame` shows who changed the file, not which repo was affected.
- **Different teams own different repos** — separate files let each team manage their own config independently.

### Comparison

| | FileSet | Separate File resources |
|---|---|---|
| Shared files | Write once in `files` | Repeated in every manifest |
| Adding a repo | Add 1 line to `repositories` | Create a new file with full spec |
| Changing a shared file | Edit one place | Edit every manifest |
| Per-repo git blame | All changes in one file | Clean, one file per repo |
| Team ownership | Single file, shared ownership | Each team owns their manifest |


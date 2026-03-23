---
title: Repository Lifecycle
sidebar:
  order: 5
---

## Creating repositories

If a YAML manifest references a repository that doesn't exist, `plan` shows it as a new resource:

```
$ gh infra plan ./repos/

Plan: 1 to create, 0 to update, 0 to destroy

  + babarot/new-project (new)
      + repository: babarot/new-project
```

`apply` creates it with `gh repo create`, then applies all settings (features, topics, branch protection, etc.) in a single pass.

## Archiving repositories

Set `archived: true` in the spec to mark a repository as read-only:

```yaml
spec:
  archived: true
```

This is a reversible operation — set it back to `false` to unarchive.

## Deleting repositories

gh-infra does **not** support repository deletion. Without a state file, there is no way to distinguish "removed from YAML" from "never managed" — and repository deletion is irreversible.

To delete a repository, use `gh repo delete` directly.
